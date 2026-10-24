package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Moq77111113/vessel/internal/build"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

func TestLinkPinsEveryUnitToADigest(t *testing.T) {
	artifact, err := bundle.Open(buildStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range artifact.Files {
		body := string(file.Data)
		if !strings.Contains(body, "Image=") {
			continue
		}
		if !strings.Contains(body, "@sha256:") {
			t.Errorf("%s still carries a tag: %s", file.Path, body)
		}
	}
}

func TestLinkPrintsEachImageAsItIsResolvedRatherThanAllAtTheEnd(t *testing.T) {
	out, exe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	output, err := runVesselCapture("build", "-o", exe, "--layout", out, "--name", "acme", "--version", "1.0", "--insecure-unsigned", serveStack(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	postgres := strings.Index(output, "library/postgres")
	web := strings.Index(output, "acme/web")
	if postgres == -1 || web == -1 {
		t.Fatalf("both images should be named in the output: %s", output)
	}
	if postgres > web {
		t.Errorf("acme/web printed before library/postgres, want resolution order: %s", output)
	}
}

func TestLinkTwiceFromTheSameSourceProducesTheSameBundle(t *testing.T) {
	source := serveStack(t)
	first, firstExe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	if err := runVessel("build", "-o", firstExe, "--layout", first, "--name", "acme", "--version", "1.0", "--insecure-unsigned", source); err != nil {
		t.Fatalf("first build: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	second, secondExe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	if err := runVessel("build", "-o", secondExe, "--layout", second, "--name", "acme", "--version", "1.0", "--insecure-unsigned", source); err != nil {
		t.Fatalf("second build: %v", err)
	}
	firstBundle, err := bundle.Open(first)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	secondBundle, err := bundle.Open(second)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	if firstBundle.Root != secondBundle.Root {
		t.Errorf("got %s then %s, want the same root", firstBundle.Root, secondBundle.Root)
	}
}

func TestLinkRecordsEveryImageInTheLock(t *testing.T) {
	artifact, err := bundle.Open(buildStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := len(artifact.Config.Images), len(images); got != want {
		t.Errorf("lock entries: got %d, want %d", got, want)
	}
	if got, want := artifact.Config.Platform, "linux/amd64"; got != want {
		t.Errorf("platform: got %q, want %q", got, want)
	}
}

func TestLinkCarriesTheUnitThatHoldsNoImage(t *testing.T) {
	artifact, err := bundle.Open(buildStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range artifact.Files {
		if strings.HasSuffix(file.Path, "app.network") {
			return
		}
	}
	t.Error("the network unit never reached the bundle")
}

func TestLinkLeavesOutAFileThatIsNotAUnit(t *testing.T) {
	artifact, err := bundle.Open(buildStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range artifact.Files {
		if strings.HasSuffix(file.Path, "README.txt") {
			t.Errorf("README.txt reached the bundle at %s", file.Path)
		}
	}
}

func TestLinkCarriesTheFilesAndVariablesTheDeliveryDeclares(t *testing.T) {
	dir := buildDelivery(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
variables:
  - name: PUBLIC_HOST
    description: public address
`,
		"realm.json": `{"realm":"###PUBLIC_HOST###"}`,
	})
	artifact, err := bundle.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if artifact.Config.Name != "acme" || artifact.Config.Version != "1.4.0" {
		t.Errorf("got %s %s, want the name and version vessel.yaml declares",
			artifact.Config.Name, artifact.Config.Version)
	}
	if len(artifact.Config.Variables) != 1 || artifact.Config.Variables[0].Name != "PUBLIC_HOST" {
		t.Errorf("got variables %+v", artifact.Config.Variables)
	}
	for _, file := range artifact.Files {
		if file.Path == "etc/acme/realm.json" {
			return
		}
	}
	t.Errorf("etc/acme/realm.json was not carried, got %v", paths(artifact.Files))
}

func TestLinkRefusesAFileTargetThatCollidesWithAUnit(t *testing.T) {
	err := buildDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: realm.json
    target: /etc/containers/systemd/web.container
`,
		"realm.json": `{}`,
	})
	if !errors.Is(err, delivery.ErrTargetCollides) {
		t.Errorf("got %v, want ErrTargetCollides", err)
	}
}

func TestLinkRefusesTwoFilesDeclaringTheSameTarget(t *testing.T) {
	err := buildDeliveryErr(t, map[string]string{
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
	if !errors.Is(err, delivery.ErrTargetDuplicate) {
		t.Errorf("got %v, want ErrTargetDuplicate", err)
	}
}

func TestLinkRefusesAFileTargetInsideTheUnitDirectory(t *testing.T) {
	err := buildDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: extra.unit
    target: /etc/containers/systemd/extra.container
`,
		"extra.unit": "[Container]\nImage=registry.test/acme/web:1.0\n",
	})
	if !errors.Is(err, build.ErrTargetInDriverDirectory) {
		t.Errorf("got %v, want ErrTargetInDriverDirectory", err)
	}
}

func TestLinkRefusesADeliveryWithNoName(t *testing.T) {
	out := filepath.Join(t.TempDir(), "vessel-stack")
	err := runVessel("build", "-o", out, "--insecure-unsigned", serveStack(t))
	if !errors.Is(err, delivery.ErrNoName) {
		t.Errorf("got %v, want ErrNoName", err)
	}
}

func TestLinkRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "vessel-stack")
	err := runVessel("build", "-o", out, "--name", "../../etc", "--insecure-unsigned", serveStack(t))
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestLinkRefusesASecretNoUnitReads(t *testing.T) {
	err := buildDeliveryErr(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
variables:
  - name: DB_PASSWORD
    secret: true
    from: openssl rand -hex 32
`,
	})
	if !errors.Is(err, build.ErrSecretNotRead) {
		t.Errorf("got %v, want ErrSecretNotRead", err)
	}
}

func TestLinkRefusesAMarkerNoVariableDeclares(t *testing.T) {
	err := buildDeliveryErr(t, map[string]string{
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

func TestLinkRefusesALinkThatLeadsOutsideTheDelivery(t *testing.T) {
	source := serveStack(t)
	if err := os.Symlink("/etc/hostname", filepath.Join(source, "hostname")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.CopyFS(source, os.DirFS("testdata/outside-link")); err != nil {
		t.Fatalf("copy the fixture: %v", err)
	}
	err := runVessel("build", "-o", filepath.Join(t.TempDir(), "myapp"), "--insecure-unsigned", source)
	if err == nil {
		t.Error("the build carried a file from outside the delivery")
	}
}
