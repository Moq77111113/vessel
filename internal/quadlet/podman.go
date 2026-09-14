package quadlet

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// loadedPrefix is what podman prints once per image it took in.
const loadedPrefix = "Loaded image: "

// podman is the container engine this machine drives.
type podman struct {
	run machine.Run
}

// Load puts every image of the layout into local storage, one archive at a time, announcing
// each before podman load runs so a load running for minutes shows it is moving.
//
// podman only takes one image per oci-archive, and only an oci-archive keeps a
// manifest byte for byte, which is what keeps the registry digest alive.
func (p *podman) Load(ctx context.Context, work report.Report, layout *machine.Layout) ([]string, error) {
	names, err := layout.Names()
	if err != nil {
		return nil, err
	}
	images := make([]string, 0, len(names))
	for i, name := range names {
		var archive bytes.Buffer
		if err := layout.Archive(i, &archive); err != nil {
			return nil, fmt.Errorf("cut %s out of the layout: %w", name, err)
		}
		work.Line("Loading", name)
		output, err := p.run(ctx, &archive, "podman", "load")
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

// Ready reports whether podman can actually run here, not just answer its version.
// podman 6 dropped cgroup v1, so a machine can carry a new enough podman that still refuses to run.
func (p *podman) Ready(ctx context.Context) error {
	output, err := p.run(ctx, nil, "podman", "image", "ls")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPodmanBroken, strings.TrimSpace(string(output)))
	}
	return nil
}

// Secrets names the podman secrets this machine already holds.
func (p *podman) Secrets(ctx context.Context) ([]string, error) {
	output, err := p.run(ctx, nil, "podman", "secret", "ls", "--format", "{{.Name}}")
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

// CreateSecret puts one secret into podman, reading its value from standard input so it never
// appears in a command line another process can list.
func (p *podman) CreateSecret(ctx context.Context, name, value string) error {
	output, err := p.run(ctx, strings.NewReader(value), "podman", "secret", "create", name, "-")
	if err != nil {
		return fmt.Errorf("podman secret create %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Version returns the podman version the machine carries.
func (p *podman) Version(ctx context.Context) (string, error) {
	output, err := p.run(ctx, nil, "podman", "--version")
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPodmanMissing, strings.TrimSpace(string(output)))
	}
	version := readVersion(string(output))
	if version == "" {
		return "", fmt.Errorf("%q: %w", string(output), ErrPodmanVersion)
	}
	return version, nil
}
