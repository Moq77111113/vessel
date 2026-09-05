package target

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	minPodmanMajor = 5
	minPodmanMinor = 0
)

// Errors the preflight returns before a bundle touches the disk.
var (
	ErrPodmanTooOld  = errors.New("podman is too old for quadlet")
	ErrPodmanVersion = errors.New("unreadable podman version")
	ErrPodmanMissing = errors.New("podman not found")
	ErrPodmanBroken  = errors.New("podman cannot run on this machine")
	ErrNoSystemd     = errors.New("systemd not found")
	ErrRootReadOnly  = errors.New("cannot write")
	ErrNotReady      = errors.New("this machine is not ready")
)

// Preflight reports everything wrong with the machine at once, so one run names every fix.
func Preflight(ctx context.Context, loader *Loader, root string) error {
	var problems []error
	version, err := loader.Version(ctx)
	if err != nil {
		problems = append(problems, err)
	} else if err := CheckPodman(version); err != nil {
		problems = append(problems, err)
	} else if err := loader.Ready(ctx); err != nil {
		problems = append(problems, err)
	}
	if err := checkSystemd(); err != nil {
		problems = append(problems, err)
	}
	if err := checkWritable(root); err != nil {
		problems = append(problems, err)
	}
	if len(problems) == 0 {
		return nil
	}
	lines := make([]string, len(problems))
	for i, problem := range problems {
		lines[i] = "  " + problem.Error()
	}
	return fmt.Errorf("%w:\n%s", ErrNotReady, strings.Join(lines, "\n"))
}

// CheckPodman reports whether a podman version carries quadlet and the Secret key.
func CheckPodman(version string) error {
	major, minor, err := majorMinor(version)
	if err != nil {
		return err
	}
	if major > minPodmanMajor || (major == minPodmanMajor && minor >= minPodmanMinor) {
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

// checkWritable probes the deepest directory that already exists, so a failed
// preflight leaves nothing behind.
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

func majorMinor(version string) (int, int, error) {
	fields := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	major, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	minor, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("%q: %w", version, ErrPodmanVersion)
	}
	return major, minor, nil
}

// readVersion takes the number out of the banner "podman version 5.4.2".
func readVersion(banner string) string {
	fields := strings.Fields(strings.TrimSpace(banner))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
