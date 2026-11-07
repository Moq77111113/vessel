package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/Moq77111113/vessel/internal/atomicfile"
	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var _ driver.Driver = (*Machine)(nil)

var (
	ErrCommand    = errors.New("command failed")
	ErrSecretsDir = errors.New("the project holds a secrets entry vessel did not make")
	ErrContainers = errors.New("unreadable container list")
)

const storeLink = "../.secrets"

type Machine struct {
	*Reader
	docker *docker
	root   string
}

func New(root string, run Run) *Machine {
	return &Machine{Reader: NewReader(), docker: &docker{run: run}, root: root}
}

func (m *Machine) store() string { return filepath.Join(m.root, storePath) }

func (m *Machine) Secrets(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(m.store())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", m.store(), err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (m *Machine) AddSecret(ctx context.Context, secret delivery.Secret) error {
	owner, declared, err := m.owner(secret.Name)
	if err != nil {
		return err
	}
	if err := atomicfile.MkdirAll(m.store(), 0o700); err != nil {
		return err
	}
	target := filepath.Join(m.store(), secret.Name)
	if err := atomicfile.Write(target, []byte(secret.Value), owner.mode); err != nil {
		return err
	}
	if !declared {
		return nil
	}
	if err := os.Chown(target, owner.uid, owner.gid); err != nil {
		return fmt.Errorf("give %s to %d:%d: %w", target, owner.uid, owner.gid, err)
	}
	return nil
}

func (m *Machine) owner(name string) (owner, bool, error) {
	projects, err := m.projects()
	if err != nil {
		return owner{}, false, err
	}
	found, declared := rootOnly, false
	for _, file := range projects {
		owners, err := file.owners()
		if err != nil {
			return owner{}, false, err
		}
		next, ok := owners[name]
		if !ok {
			continue
		}
		if declared && next != found {
			return owner{}, false, fmt.Errorf("%s, %s: %w", name, file.Name, ErrSecretOwner)
		}
		found, declared = next, true
	}
	return found, declared, nil
}

func (m *Machine) projects() ([]project, error) {
	files, err := filepath.Glob(filepath.Join(m.root, composePath, "*", composeFile))
	if err != nil {
		return nil, fmt.Errorf("list the compose files: %w", err)
	}
	projects := make([]project, 0, len(files))
	for _, name := range files {
		file, err := readProjectAt(name)
		if err != nil {
			return nil, err
		}
		projects = append(projects, file)
	}
	return projects, nil
}

func readProjectAt(name string) (project, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return project{}, fmt.Errorf("read %s: %w", name, err)
	}
	file, err := readProject(data)
	if err != nil {
		return project{}, fmt.Errorf("%s: %w", name, err)
	}
	return file, nil
}

func (m *Machine) RemoveHint(secret string) string {
	return "rm " + filepath.Join(m.store(), secret)
}

func (m *Machine) AddImages(ctx context.Context, work report.Report, images *oci.Images) ([]string, error) {
	return m.docker.Load(ctx, work, images)
}

func (m *Machine) Start(ctx context.Context, services []delivery.Service) error {
	return m.up(ctx, services, "up", "--detach", "--pull", "never", "--remove-orphans", "--wait")
}

func (m *Machine) Restart(ctx context.Context, services []delivery.Service) error {
	return m.up(ctx, services, "up", "--detach", "--pull", "never", "--remove-orphans", "--force-recreate", "--wait")
}

func (m *Machine) up(ctx context.Context, services []delivery.Service, words ...string) error {
	for _, service := range services {
		if err := m.prepare(service); err != nil {
			return err
		}
		if _, err := m.docker.compose(ctx, m.file(service), words...); err != nil {
			return err
		}
	}
	return nil
}

func (m *Machine) prepare(service delivery.Service) error {
	file, err := readProjectAt(m.file(service))
	if err != nil {
		return err
	}
	if err := m.checkOwners(file); err != nil {
		return err
	}
	return m.link(filepath.Join(m.root, file.dir(), secretsDir))
}

func (m *Machine) checkOwners(file project) error {
	owners, err := file.owners()
	if err != nil {
		return err
	}
	for name, want := range owners {
		info, err := os.Stat(filepath.Join(m.store(), name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read the secret %s: %w", name, err)
		}
		held := ownerOf(info)
		if held != want {
			return fmt.Errorf("%s, held as %d:%d %o, %s wants %d:%d %o: %w",
				name, held.uid, held.gid, held.mode, file.Name, want.uid, want.gid, want.mode, ErrSecretOwner)
		}
	}
	return nil
}

func ownerOf(info fs.FileInfo) owner {
	stat := info.Sys().(*syscall.Stat_t)
	return owner{uid: int(stat.Uid), gid: int(stat.Gid), mode: info.Mode().Perm()}
}

func (m *Machine) link(path string) error {
	target, err := os.Readlink(path)
	if errors.Is(err, fs.ErrNotExist) {
		return os.Symlink(storeLink, path)
	}
	if err != nil || target != storeLink {
		return fmt.Errorf("%s: %w", path, ErrSecretsDir)
	}
	return nil
}

func (m *Machine) Stop(ctx context.Context, services []delivery.Service) error {
	for _, service := range services {
		if _, err := m.docker.compose(ctx, m.file(service), "down"); err != nil {
			return err
		}
	}
	return nil
}

func (m *Machine) Running(ctx context.Context, services []delivery.Service) ([]delivery.Service, error) {
	var running []delivery.Service
	for _, service := range services {
		up, err := m.allUp(ctx, service)
		if err != nil {
			return nil, err
		}
		if up {
			running = append(running, service)
		}
	}
	return running, nil
}

type container struct {
	Service  string `json:"Service"`
	State    string `json:"State"`
	ExitCode int    `json:"ExitCode"`
}

func (c container) up() bool {
	if c.State == "running" {
		return true
	}
	return c.State == "exited" && c.ExitCode == 0
}

func (m *Machine) allUp(ctx context.Context, service delivery.Service) (bool, error) {
	declared, err := m.docker.compose(ctx, m.file(service), "config", "--services")
	if err != nil {
		return false, err
	}
	output, err := m.docker.compose(ctx, m.file(service), "ps", "--all", "--format", "json")
	if err != nil {
		return false, err
	}
	containers, err := readContainers(output)
	if err != nil {
		return false, err
	}
	for _, name := range strings.Fields(string(declared)) {
		if !serviceUp(containers, name) {
			return false, nil
		}
	}
	return true, nil
}

func serviceUp(containers []container, name string) bool {
	mine := slices.DeleteFunc(slices.Clone(containers), func(c container) bool { return c.Service != name })
	if len(mine) == 0 {
		return false
	}
	return !slices.ContainsFunc(mine, func(c container) bool { return !c.up() })
}

func readContainers(output []byte) ([]container, error) {
	var containers []container
	for line := range bytes.SplitSeq(bytes.TrimSpace(output), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var next container
		if err := json.Unmarshal(line, &next); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrContainers, err)
		}
		containers = append(containers, next)
	}
	return containers, nil
}

func (m *Machine) file(service delivery.Service) string {
	return filepath.Join(m.root, service.Path)
}
