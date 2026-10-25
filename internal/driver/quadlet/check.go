package quadlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Moq77111113/vessel/pkg/driver"
)

const (
	minPodmanMajor = 5
	minPodmanMinor = 0
)

var (
	ErrPodmanTooOld  = errors.New("podman is too old for quadlet")
	ErrPodmanVersion = errors.New("unreadable podman version")
	ErrPodmanMissing = errors.New("podman not found")
	ErrPodmanBroken  = errors.New("podman cannot run on this machine")
	ErrNoSystemd     = errors.New("systemd not found")
	ErrRootReadOnly  = errors.New("cannot write")
)

func (m *Machine) Check(ctx context.Context) error {
	var problems []error
	version, err := m.podman.Version(ctx)
	if err != nil {
		problems = append(problems, err)
	} else if err := CheckPodman(version); err != nil {
		problems = append(problems, err)
	} else if err := m.podman.Ready(ctx); err != nil {
		problems = append(problems, err)
	}
	if err := checkSystemd(); err != nil {
		problems = append(problems, err)
	}
	if err := checkWritable(m.root); err != nil {
		problems = append(problems, err)
	}
	if len(problems) == 0 {
		return nil
	}
	lines := make([]string, len(problems))
	for i, problem := range problems {
		lines[i] = "  " + problem.Error()
	}
	return fmt.Errorf("%w:\n%s", driver.ErrNotReady, strings.Join(lines, "\n"))
}

func CheckPodman(version string) error {
	podman, err := parseRelease(version)
	if err != nil {
		return err
	}
	if podman.atLeast(release{minPodmanMajor, minPodmanMinor}) {
		return nil
	}
	return fmt.Errorf("%w: found %s, want %d.%d or newer",
		ErrPodmanTooOld, version, minPodmanMajor, minPodmanMinor)
}

func checkSystemd() error {
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return ErrNoSystemd
	}
	return nil
}

func checkWritable(root string) error {
	target := filepath.Join(root, systemdPath)
	dir := target
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("%w to %s", ErrRootReadOnly, target)
		}
		dir = parent
	}
	probe, err := os.CreateTemp(dir, ".vessel-probe-*")
	if err != nil {
		return fmt.Errorf("%w to %s", ErrRootReadOnly, target)
	}
	probe.Close()
	return os.Remove(probe.Name())
}

type release struct {
	major int
	minor int
}

func (r release) atLeast(other release) bool {
	if r.major != other.major {
		return r.major > other.major
	}
	return r.minor >= other.minor
}

func parseRelease(version string) (release, error) {
	fields := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(fields) < 2 {
		return release{}, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	major, err := strconv.Atoi(fields[0])
	if err != nil {
		return release{}, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	minor, err := strconv.Atoi(fields[1])
	if err != nil {
		return release{}, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	return release{major, minor}, nil
}

func readVersion(banner string) string {
	fields := strings.Fields(strings.TrimSpace(banner))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
