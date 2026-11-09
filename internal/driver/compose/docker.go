package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
)

const loadedPrefix = "Loaded image: "

const containerdStore = "io.containerd.snapshotter.v1"

// compose 2.21 is the first to print ps --format json one container per line.
const (
	minComposeMajor = 2
	minComposeMinor = 21
)

var (
	ErrDockerMissing     = errors.New("docker not found")
	ErrDockerBroken      = errors.New("docker cannot reach its daemon")
	ErrComposeMissing    = errors.New("docker compose v2 not found")
	ErrComposeTooOld     = errors.New("docker compose is too old to list containers as JSON")
	ErrClassicImageStore = errors.New(`docker keeps images in its classic store, which forgets the digest vessel pins: set {"features": {"containerd-snapshotter": true}} in /etc/docker/daemon.json and restart docker`)
)

type docker struct {
	run Run
}

func (d *docker) Load(ctx context.Context, work report.Report, layout *oci.Images) ([]string, error) {
	names, err := layout.Names()
	if err != nil {
		return nil, err
	}
	images := make([]string, 0, len(names))
	for i, name := range names {
		work.Line("Loading", name)
		output, err := d.load(ctx, layout, i)
		if err != nil {
			return nil, fmt.Errorf("docker load %s: %w: %v: %s", name, ErrCommand, err, strings.TrimSpace(string(output)))
		}
		for line := range strings.SplitSeq(string(output), "\n") {
			if image, ok := strings.CutPrefix(strings.TrimSpace(line), loadedPrefix); ok {
				images = append(images, image)
			}
		}
	}
	return images, nil
}

func (d *docker) load(ctx context.Context, layout *oci.Images, i int) ([]byte, error) {
	archive, err := os.CreateTemp("", "vessel-image-*.tar")
	if err != nil {
		return nil, fmt.Errorf("create the image archive: %w", err)
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	if err := layout.Archive(i, archive); err != nil {
		return nil, fmt.Errorf("cut image %d out of the layout: %w", i, err)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind the image archive: %w", err)
	}
	return d.run(ctx, Command{Line: []string{"docker", "load"}, Stdin: archive})
}

func (d *docker) flush(ctx context.Context) error {
	output, err := d.run(ctx, line("docker", "info", "--format", "{{.DockerRootDir}}"))
	if err != nil {
		return fmt.Errorf("docker info: %w: %s", ErrCommand, strings.TrimSpace(string(output)))
	}
	return syncFileSystem(ctx, d.run, strings.TrimSpace(string(output)))
}

func (d *docker) ready(ctx context.Context) []error {
	output, err := d.run(ctx, line("docker", "version", "--format", "{{.Server.Version}}"))
	if errors.Is(err, exec.ErrNotFound) {
		return []error{ErrDockerMissing}
	}
	if err != nil {
		return []error{fmt.Errorf("%w: %s", ErrDockerBroken, strings.TrimSpace(string(output)))}
	}
	var problems []error
	if err := d.checkCompose(ctx); err != nil {
		problems = append(problems, err)
	}
	output, err = d.run(ctx, line("docker", "info", "--format", "{{json .DriverStatus}}"))
	if err != nil {
		return append(problems, fmt.Errorf("%w: %s", ErrDockerBroken, strings.TrimSpace(string(output))))
	}
	if !strings.Contains(string(output), containerdStore) {
		problems = append(problems, ErrClassicImageStore)
	}
	return problems
}

func (d *docker) checkCompose(ctx context.Context) error {
	output, err := d.run(ctx, line("docker", "compose", "version", "--short"))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrComposeMissing, strings.TrimSpace(string(output)))
	}
	version := strings.TrimPrefix(strings.TrimSpace(string(output)), "v")
	fields := strings.SplitN(version, ".", 3)
	if len(fields) < 2 {
		return fmt.Errorf("%w: found %q", ErrComposeTooOld, version)
	}
	major, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("%w: found %q", ErrComposeTooOld, version)
	}
	minor, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("%w: found %q", ErrComposeTooOld, version)
	}
	if major > minComposeMajor {
		return nil
	}
	if major == minComposeMajor && minor >= minComposeMinor {
		return nil
	}
	return fmt.Errorf("%w: found %s, want %d.%d or newer", ErrComposeTooOld, version, minComposeMajor, minComposeMinor)
}

func (d *docker) compose(ctx context.Context, file string, words ...string) ([]byte, error) {
	command := append([]string{"docker", "compose", "-f", file}, words...)
	output, err := d.run(ctx, line(command...))
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", strings.Join(command, " "), ErrCommand, strings.TrimSpace(string(output)))
	}
	return output, nil
}
