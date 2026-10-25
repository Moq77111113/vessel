package site

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/driver"
)

func credentialBundle(t *testing.T, version string) string {
	t.Helper()
	return linkTestDelivery(t, "[Service]\nLoadCredentialEncrypted=KC_DB_PASSWORD\n", map[string]string{
		"vessel.yaml": "name: acme\nversion: " + version + "\nvariables:\n  - name: KC_DB_PASSWORD\n    secret: true\n",
	})
}

func TestInstallSealsACredentialAUnitLoads(t *testing.T) {
	root := t.TempDir()
	stub := &podmanStub{}
	job := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).job(credentialBundle(t, "1.4.0"), nil)
	job.SetFile = map[string]string{"KC_DB_PASSWORD": "hunter2"}
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD"))
	if err != nil {
		t.Fatalf("read the credential: %v", err)
	}
	if string(body) != "KC_DB_PASSWORD=hunter2\n" {
		t.Errorf("got %q, want the environment line systemd-creds sealed", body)
	}
	if _, held := stub.secrets["KC_DB_PASSWORD"]; held {
		t.Error("the credential also went to podman")
	}
}

func TestUpgradeKeepsTheCredentialTheMachineHolds(t *testing.T) {
	root := t.TempDir()
	stub := &podmanStub{}
	kinds := []driver.Driver{quadlet.New(root, stub.run)}
	first := at(t, root, kinds).job(credentialBundle(t, "1.4.0"), nil)
	first.SetFile = map[string]string{"KC_DB_PASSWORD": "hunter2"}
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	stub.calls = nil
	if err := at(t, root, kinds).upgrade(credentialBundle(t, "1.5.0")); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD"))
	if err != nil {
		t.Fatalf("read the credential: %v", err)
	}
	if string(body) != "KC_DB_PASSWORD=hunter2\n" {
		t.Errorf("got %q, want the value the install sealed", body)
	}
	if slices.ContainsFunc(stub.calls, func(call string) bool { return strings.HasPrefix(call, "systemd-creds") }) {
		t.Errorf("the upgrade sealed the credential again: %v", stub.calls)
	}
}

func TestUninstallKeepsTheCredentialAndNamesItsRemoval(t *testing.T) {
	root := t.TempDir()
	stub := &podmanStub{}
	kinds := []driver.Driver{quadlet.New(root, stub.run)}
	job := at(t, root, kinds).job(credentialBundle(t, "1.4.0"), nil)
	job.SetFile = map[string]string{"KC_DB_PASSWORD": "hunter2"}
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	var out bytes.Buffer
	if err := at(t, root, kinds).uninstall(&out, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	path := filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("uninstall removed the credential: %v", err)
	}
	if !strings.Contains(out.String(), "rm "+path) {
		t.Errorf("got %q, want the command that removes the credential", out.String())
	}
}
