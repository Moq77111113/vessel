package e2e

import (
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func TestLinkPinsEveryUnitToADigest(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range opened.Files {
		body := string(file.Data)
		if !strings.Contains(body, "Image=") {
			continue
		}
		if !strings.Contains(body, "@sha256:") {
			t.Errorf("%s still carries a tag: %s", file.Path, body)
		}
	}
}

func TestLinkRecordsEveryImageInTheLock(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := len(opened.Config.Images), len(images); got != want {
		t.Errorf("lock entries: got %d, want %d", got, want)
	}
	if got, want := opened.Config.Platform, "linux/amd64"; got != want {
		t.Errorf("platform: got %q, want %q", got, want)
	}
}

func TestLinkCarriesTheUnitThatHoldsNoImage(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range opened.Files {
		if strings.HasSuffix(file.Path, "app.network") {
			return
		}
	}
	t.Error("the network unit never reached the bundle")
}

func TestLinkLeavesOutAFileThatIsNotAUnit(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range opened.Files {
		if strings.HasSuffix(file.Path, "README.txt") {
			t.Errorf("README.txt reached the bundle at %s", file.Path)
		}
	}
}
