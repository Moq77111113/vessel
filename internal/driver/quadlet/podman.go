package quadlet

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
)

const loadedPrefix = "Loaded image: "

type podman struct {
	run Run
}

func (p *podman) Load(ctx context.Context, work report.Report, layout *oci.Images) ([]string, error) {
	names, err := layout.Names()
	if err != nil {
		return nil, err
	}
	images := make([]string, 0, len(names))
	for i, name := range names {
		work.Line("Loading", name)
		output, err := p.load(ctx, layout, i)
		if err != nil {
			return nil, fmt.Errorf("podman load %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
		for line := range strings.SplitSeq(string(output), "\n") {
			if image, ok := strings.CutPrefix(strings.TrimSpace(line), loadedPrefix); ok {
				images = append(images, image)
			}
		}
	}
	return images, nil
}

func (p *podman) load(ctx context.Context, layout *oci.Images, i int) ([]byte, error) {
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
	return p.run(ctx, Command{Line: []string{"podman", "load"}, Stdin: archive})
}

func (p *podman) Ready(ctx context.Context) error {
	output, err := p.run(ctx, line("podman", "image", "ls"))
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPodmanBroken, strings.TrimSpace(string(output)))
	}
	return nil
}

func (p *podman) Secrets(ctx context.Context) ([]string, error) {
	output, err := p.run(ctx, line("podman", "secret", "ls", "--format", "{{.Name}}"))
	if err != nil {
		return nil, fmt.Errorf("podman secret ls: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var names []string
	for line := range strings.SplitSeq(string(output), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (p *podman) CreateSecret(ctx context.Context, name, value string) error {
	output, err := p.run(ctx, Command{Line: []string{"podman", "secret", "create", name, "-"}, Stdin: strings.NewReader(value)})
	if err != nil {
		return fmt.Errorf("podman secret create %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (p *podman) Version(ctx context.Context) (string, error) {
	output, err := p.run(ctx, line("podman", "--version"))
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPodmanMissing, strings.TrimSpace(string(output)))
	}
	version := readVersion(string(output))
	if version == "" {
		return "", fmt.Errorf("%q: %w", string(output), ErrPodmanVersion)
	}
	return version, nil
}
