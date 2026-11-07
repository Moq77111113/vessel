package site

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/driver"
)

func webBundle(t *testing.T, version, unit string) string {
	t.Helper()
	return linkTestDelivery(t, unit, map[string]string{"vessel.yaml": "name: acme\nversion: " + version + "\n"})
}

func restarts(calls []string) bool {
	return slices.ContainsFunc(calls, func(call string) bool { return strings.HasPrefix(call, "systemctl restart") })
}

func TestInstallStartsTheServicesWithoutRestartingThem(t *testing.T) {
	root := t.TempDir()
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(webBundle(t, "1.3.0", "ContainerName=web\n")); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl start web.service") || restarts(stub.calls) {
		t.Errorf("got %v, want web.service started and nothing restarted", stub.calls)
	}
}

func TestUpgradeRestartsTheServicesTheNewVersionCarries(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(webBundle(t, "1.3.0", "ContainerName=web\n")); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).upgrade(webBundle(t, "1.4.0", "ContainerName=web-1.4\n")); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl restart web.service") {
		t.Errorf("got %v, want web.service restarted", stub.calls)
	}
}

func TestInstallingTheSameBundleAgainRestartsNothing(t *testing.T) {
	root := t.TempDir()
	dir := webBundle(t, "1.3.0", "ContainerName=web\n")
	if err := at(t, root, nil).install(dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(dir); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if restarts(stub.calls) {
		t.Errorf("got %v, want nothing restarted", stub.calls)
	}
}

func TestResumeOfACutUpgradeRestartsTheServices(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(webBundle(t, "1.3.0", "ContainerName=web\n")); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	newer := webBundle(t, "1.4.0", "ContainerName=web-1.4\n")
	cut := at(t, root, []driver.Driver{brokenImages{quadlet.New(root, (&podmanStub{}).run)}})
	if err := cut.upgrade(newer); err == nil {
		t.Fatal("upgrade succeeded with a broken image load")
	}
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).resume(newer); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl restart web.service") {
		t.Errorf("got %v, want web.service restarted", stub.calls)
	}
}

func TestADryRunOfAnUpgradeNamesTheServiceItRestarts(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(webBundle(t, "1.3.0", "ContainerName=web\n")); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	var out bytes.Buffer
	job := at(t, root, nil).job(webBundle(t, "1.4.0", "ContainerName=web-1.4\n"), nil).Preview()
	if err := job.Upgrade(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if output := collapse(out.String()); !strings.Contains(output, "Restart web.service") {
		t.Errorf("got %q, want web.service named as restarted", output)
	}
}
