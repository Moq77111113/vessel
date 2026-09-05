package quadlet

import (
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func units(names ...string) []descriptor.File {
	files := make([]descriptor.File, len(names))
	for i, name := range names {
		files[i] = descriptor.File{Path: "etc/containers/systemd/" + name}
	}
	return files
}

func TestStartTellsSystemdToReloadThenStart(t *testing.T) {
	got := NewReader().Start(units("web.container"))
	want := []string{"systemctl daemon-reload", "systemctl start web.service"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStartSaysNothingWhenNoUnitStartsAService(t *testing.T) {
	if got := NewReader().Start(units("app.network")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestServicesNamesOneServicePerContainerUnit(t *testing.T) {
	got := Services(units("web.container", "db.container"))
	if want := "db.service web.service"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestServicesLeavesOutNetworksAndVolumes(t *testing.T) {
	got := Services(units("web.container", "app.network", "db.volume"))
	if len(got) != 1 {
		t.Errorf("got %v, want only the container unit", got)
	}
}

func TestServicesSuffixesAPod(t *testing.T) {
	got := Services(units("app.pod"))
	if want := "app-pod.service"; strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestServicesReturnsNothingWhenNoUnitStartsAService(t *testing.T) {
	if got := Services(units("app.network")); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}
