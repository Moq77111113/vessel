package e2e

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/cli"
	"github.com/Moq77111113/vessel/internal/delivery"
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

func TestLinkCarriesTheFilesAndVariablesTheDeliveryDeclares(t *testing.T) {
	dir := linkDelivery(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
variables:
  - name: PUBLIC_HOST
    ask: public address
`,
		"realm.json": `{"realm":"###PUBLIC_HOST###"}`,
	})
	opened, err := bundle.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened.Config.Name != "acme" || opened.Config.Version != "1.4.0" {
		t.Errorf("got %s %s, want the name and version vessel.yaml declares",
			opened.Config.Name, opened.Config.Version)
	}
	if len(opened.Config.Variables) != 1 || opened.Config.Variables[0].Name != "PUBLIC_HOST" {
		t.Errorf("got variables %+v", opened.Config.Variables)
	}
	for _, file := range opened.Files {
		if file.Path == "etc/acme/realm.json" {
			return
		}
	}
	t.Errorf("etc/acme/realm.json was not carried, got %v", paths(opened.Files))
}

func TestLinkRefusesAFileTargetThatCollidesWithAUnit(t *testing.T) {
	err := linkDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: realm.json
    target: /etc/containers/systemd/web.container
`,
		"realm.json": `{}`,
	})
	if !errors.Is(err, cli.ErrTargetCollides) {
		t.Errorf("got %v, want ErrTargetCollides", err)
	}
}

func TestLinkRefusesTwoFilesDeclaringTheSameTarget(t *testing.T) {
	err := linkDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: a.json
    target: /etc/acme/shared.json
  - source: b.json
    target: /etc/acme/shared.json
`,
		"a.json": `{}`,
		"b.json": `{}`,
	})
	if !errors.Is(err, cli.ErrTargetDuplicate) {
		t.Errorf("got %v, want ErrTargetDuplicate", err)
	}
}

func TestLinkRefusesAFileTargetInsideTheUnitDirectory(t *testing.T) {
	err := linkDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: extra.unit
    target: /etc/containers/systemd/extra.container
`,
		"extra.unit": "[Container]\nImage=registry.test/acme/web:1.0\n",
	})
	if !errors.Is(err, cli.ErrTargetInUnitDirectory) {
		t.Errorf("got %v, want ErrTargetInUnitDirectory", err)
	}
}

func TestLinkRefusesADeliveryWithNoName(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	err := runVessel("link", "-o", out, serveStack(t))
	if !errors.Is(err, delivery.ErrNoName) {
		t.Errorf("got %v, want ErrNoName", err)
	}
}

func TestLinkRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	err := runVessel("link", "-o", out, "--name", "../../etc", serveStack(t))
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestLinkRefusesASecretNoUnitReads(t *testing.T) {
	err := linkDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
variables:
  - name: DB_PASSWORD
    secret: true
    from: openssl rand -hex 32
`,
	})
	if !errors.Is(err, cli.ErrSecretNoUnitReads) {
		t.Errorf("got %v, want ErrSecretNoUnitReads", err)
	}
}

func TestLinkRefusesAMarkerNoVariableDeclares(t *testing.T) {
	err := linkDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
`,
		"realm.json": `{"host":"###PUBLIC_HOST###"}`,
	})
	if !errors.Is(err, delivery.ErrUnknownVariable) {
		t.Errorf("got %v, want ErrUnknownVariable", err)
	}
}
