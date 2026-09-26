package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAnInterruptedInstallLeavesNoUnpackedBundle(t *testing.T) {
	scratch := t.TempDir()
	interruptInstall(t, scratch, "TMPDIR="+scratch)
	if left := bundles(t, scratch); len(left) != 0 {
		t.Errorf("got %v left in %s, want nothing", left, scratch)
	}
}

func TestAnInstallUnpacksUnderVarTmpWhenNothingSaysWhere(t *testing.T) {
	if unpacked := interruptInstall(t, "/var/tmp", "TMPDIR="); len(unpacked) == 0 {
		t.Error("the bundle was not unpacked under /var/tmp")
	}
}

// interruptInstall runs a packed install until it asks for podman, lists the bundles unpacked in dir meanwhile, then interrupts it.
func interruptInstall(t *testing.T, dir, env string) []string {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "podman-called")
	podman, err := filepath.Abs("testdata/slow-podman")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	before := bundles(t, dir)
	install := exec.Command(packStack(t), "install")
	install.Env = append(os.Environ(), "PATH="+podman+":"+os.Getenv("PATH"), "VESSEL_TEST_MARKER="+marker, env)
	if err := install.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline, _ := t.Deadline()
	for _, err := os.Stat(marker); err != nil; _, err = os.Stat(marker) {
		if !deadline.IsZero() && time.Now().After(deadline) {
			t.Fatal("the install never asked for podman")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var unpacked []string
	for _, name := range bundles(t, dir) {
		if !slices.Contains(before, name) {
			unpacked = append(unpacked, name)
		}
	}
	if err := install.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	install.Wait()
	return unpacked
}

// bundles names the unpacked bundles sitting in dir.
func bundles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "vessel-bundle-") {
			names = append(names, entry.Name())
		}
	}
	return names
}
