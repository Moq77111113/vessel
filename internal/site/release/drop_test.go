package release

import (
	"testing"

	"github.com/Moq77111113/vessel/internal/site/disk"
)

func TestDropsRemovesAFileThePreviousVersionCarriedThatTheNewOneDoesNot(t *testing.T) {
	previous := []Entry{
		{Path: "etc/containers/systemd/web.container", Digest: "sha256:aaaa"},
		{Path: "etc/containers/systemd/cache.container", Digest: "sha256:bbbb"},
	}
	next := []Entry{
		{Path: "etc/containers/systemd/web.container", Digest: "sha256:cccc"},
	}
	got, _, err := Drops(disk.NewTree(t.TempDir()), previous, next)
	if err != nil {
		t.Fatalf("drops: %v", err)
	}
	want := []string{"etc/containers/systemd/cache.container"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDropsSkipsAFileBothVersionsCarry(t *testing.T) {
	entries := []Entry{{Path: "etc/acme/realm.json", Digest: "sha256:aaaa"}}
	if got, _, _ := Drops(disk.NewTree(t.TempDir()), entries, entries); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}
