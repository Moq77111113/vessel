package target

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// loadedPrefix is what podman prints once per image it took in.
const loadedPrefix = "Loaded image: "

// Run executes one command, feeding it stdin, and returns everything it printed.
type Run func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

// Loader puts images into the local container storage through podman.
type Loader struct {
	run Run
}

// NewLoader returns a loader driving the given command runner.
func NewLoader(run Run) *Loader { return &Loader{run: run} }

// Exec runs a command for real, and is what NewLoader takes outside tests.
func Exec(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	return command.CombinedOutput()
}

// Load puts every image of the layout into local storage, one archive at a time.
//
// podman only takes one image per oci-archive, and only an oci-archive keeps a
// manifest byte for byte, which is what keeps the registry digest alive.
func (l *Loader) Load(ctx context.Context, layout *Layout) ([]string, error) {
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
		output, err := l.run(ctx, &archive, "podman", "load")
		if err != nil {
			return nil, fmt.Errorf("podman load %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
		for line := range strings.SplitSeq(string(output), "\n") {
			if loaded, ok := strings.CutPrefix(strings.TrimSpace(line), loadedPrefix); ok {
				images = append(images, loaded)
			}
		}
	}
	return images, nil
}

// Ready reports whether podman can actually run here, not just answer its version.
// podman 6 dropped cgroup v1, so a machine can carry a new enough podman that still refuses to run.
func (l *Loader) Ready(ctx context.Context) error {
	output, err := l.run(ctx, nil, "podman", "image", "ls")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPodmanBroken, strings.TrimSpace(string(output)))
	}
	return nil
}

// Secrets names the podman secrets this machine already holds.
func (l *Loader) Secrets(ctx context.Context) ([]string, error) {
	output, err := l.run(ctx, nil, "podman", "secret", "ls", "--format", "{{.Name}}")
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

// Version returns the podman version the machine carries.
func (l *Loader) Version(ctx context.Context) (string, error) {
	output, err := l.run(ctx, nil, "podman", "--version")
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrPodmanMissing, strings.TrimSpace(string(output)))
	}
	version := readVersion(string(output))
	if version == "" {
		return "", fmt.Errorf("%q: %w", string(output), ErrPodmanVersion)
	}
	return version, nil
}
