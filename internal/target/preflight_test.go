package target

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPodmanAcceptsTheVersionFloor(t *testing.T) {
	if err := CheckPodman("5.0.0"); err != nil {
		t.Fatalf("CheckPodman(5.0.0): %v", err)
	}
}

func TestCheckPodmanAcceptsANewerVersion(t *testing.T) {
	if err := CheckPodman("5.4.2"); err != nil {
		t.Fatalf("CheckPodman(5.4.2): %v", err)
	}
}

func TestCheckPodmanAcceptsANewerMajor(t *testing.T) {
	if err := CheckPodman("6.0.0"); err != nil {
		t.Fatalf("CheckPodman(6.0.0): %v", err)
	}
}

func TestCheckPodmanRejectsTheVersionDebian12Ships(t *testing.T) {
	err := CheckPodman("4.3.1")
	if !errors.Is(err, ErrPodmanTooOld) {
		t.Fatalf("got %v, want ErrPodmanTooOld", err)
	}
	if !strings.Contains(err.Error(), "4.3.1") {
		t.Errorf("error does not name the version found: %v", err)
	}
	if !strings.Contains(err.Error(), "5.0") {
		t.Errorf("error does not name the floor: %v", err)
	}
}

func TestCheckPodmanRejectsTheLastVersionWithoutQuadlet(t *testing.T) {
	if err := CheckPodman("4.9.9"); !errors.Is(err, ErrPodmanTooOld) {
		t.Errorf("got %v, want ErrPodmanTooOld", err)
	}
}

func TestCheckPodmanRejectsAnUnreadableVersion(t *testing.T) {
	if err := CheckPodman("nightly"); !errors.Is(err, ErrPodmanVersion) {
		t.Errorf("got %v, want ErrPodmanVersion", err)
	}
}

func TestReadVersionTakesTheNumberOutOfThePodmanBanner(t *testing.T) {
	if got, want := readVersion("podman version 5.4.2\n"), "5.4.2"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPreflightNamesEveryProblemAtOnce(t *testing.T) {
	podman := &recorder{output: "podman version 4.3.1\n"}
	err := Preflight(context.Background(), NewLoader(podman.run), filepath.Join(t.TempDir(), "root"))
	if err == nil {
		t.Fatal("Preflight: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "4.3.1") {
		t.Errorf("the version found is missing from %v", err)
	}
	if !strings.Contains(err.Error(), "5.0") {
		t.Errorf("the version wanted is missing from %v", err)
	}
	if !errors.Is(err, ErrNotReady) {
		t.Errorf("got %v, want ErrNotReady", err)
	}
}

func TestPreflightPutsOneProblemPerLine(t *testing.T) {
	podman := &recorder{output: "podman version 4.3.1\n"}
	err := Preflight(context.Background(), NewLoader(podman.run), "/proc/nope")
	if err == nil {
		t.Fatal("Preflight: want an error, got nil")
	}
	if got := strings.Count(err.Error(), "\n"); got < 2 {
		t.Errorf("got %d line breaks in %q, want one problem per line", got, err)
	}
}

func TestPreflightPassesOnAMachineThatHasEverything(t *testing.T) {
	if !systemdIsHere() {
		t.Skip("no systemd on this machine")
	}
	podman := &recorder{output: "podman version 5.4.2\n"}
	if err := Preflight(context.Background(), NewLoader(podman.run), t.TempDir()); err != nil {
		t.Errorf("Preflight: %v", err)
	}
}

func TestPreflightRefusesARootItCannotWriteTo(t *testing.T) {
	podman := &recorder{output: "podman version 5.4.2\n"}
	root := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(root, 0o500); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	err := Preflight(context.Background(), NewLoader(podman.run), root)
	if err == nil || !strings.Contains(err.Error(), ErrRootReadOnly.Error()) {
		t.Errorf("got %v, want it to name the write failure", err)
	}
}

func systemdIsHere() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func TestPreflightRefusesAPodmanThatCannotRun(t *testing.T) {
	podman := &recorder{output: "podman version 6.1.1\n"}
	podman.failAfter = 1
	err := Preflight(context.Background(), NewLoader(podman.run), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), ErrPodmanBroken.Error()) {
		t.Errorf("got %v, want it to name the broken podman", err)
	}
}
