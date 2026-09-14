package cli

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestLoadPutsTheSetValueIntoTheFile(t *testing.T) {
	dir := linkTestBundle(t, map[string]string{
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
	root := t.TempDir()
	set := map[string]string{"PUBLIC_HOST": "dmas.acme.local"}
	if err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture), dir, root, set, false); err != nil {
		t.Fatalf("load: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/acme/realm.json"))
	if err != nil {
		t.Fatalf("the file never reached the root: %v", err)
	}
	if got, want := string(body), `{"realm":"dmas.acme.local"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLoadWritesNothingWhenAValueIsMissing(t *testing.T) {
	dir := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nvariables:\n  - name: PUBLIC_HOST\n"})
	root := t.TempDir()
	stub := &podmanStub{}
	if err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, nil, false); err == nil {
		t.Fatal("load succeeded with no value for PUBLIC_HOST")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/containers/systemd")); err == nil {
		t.Error("load wrote units even though a value was missing")
	}
	for _, call := range stub.calls {
		if call == "podman load" {
			t.Error("load called podman load despite the missing value")
		}
	}
}

func TestLoadLoadsImagesAfterWritingFiles(t *testing.T) {
	dir := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
`,
		"realm.json": `{"realm":"fixed"}`,
	})
	root := t.TempDir()
	stub := &podmanStub{watchFile: filepath.Join(root, "etc/acme/realm.json")}
	if err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, nil, false); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !stub.sawFileAtLoad {
		t.Error("podman load ran before the file reached the root")
	}
}

func TestLoadCreatesASecretWithoutStoringIt(t *testing.T) {
	dir := linkTestDelivery(t, "Secret=DB_PASSWORD,type=env,target=POSTGRES_PASSWORD\n",
		map[string]string{
			"vessel.yaml": `
name: acme
version: 1.4.0
variables:
  - name: DB_PASSWORD
    secret: true
`,
		})
	root := t.TempDir()
	stub := &podmanStub{}
	set := map[string]string{"DB_PASSWORD": "hunter2"}
	if err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, set, false); err != nil {
		t.Fatalf("load: %v", err)
	}
	created, ok := stub.secrets["DB_PASSWORD"]
	if !ok {
		t.Fatal("podman secret create was never called for DB_PASSWORD")
	}
	if created != "hunter2" {
		t.Errorf("got %q, want %q", created, "hunter2")
	}
	body, err := os.ReadFile(filepath.Join(root, "var/lib/vessel/acme/values"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(body), "DB_PASSWORD") || strings.Contains(string(body), "hunter2") {
		t.Errorf("the store carries the secret: %s", body)
	}
}

func TestLoadStartsTheServicesItJustInstalled(t *testing.T) {
	dir := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	stub := &podmanStub{}
	if err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, t.TempDir(), nil, false); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl start web.service") {
		t.Errorf("load never started the service: %v", stub.calls)
	}
}

// noCapture is the shell of a test where no delivery action or "from" ever runs.
func noCapture(context.Context, string) ([]byte, error) {
	return nil, errors.New("the shell should not run in this test")
}

// podmanStub answers podman well enough to drive load without podman: a version new enough,
// an empty secret list, and a successful load. It remembers every command it was given, what
// it was asked to create, and whether watchFile already existed the moment "load" ran.
type podmanStub struct {
	calls         []string
	secrets       map[string]string
	watchFile     string
	sawFileAtLoad bool
}

func (p *podmanStub) run(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	p.calls = append(p.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	switch {
	case len(args) > 0 && args[0] == "--version":
		return []byte("podman version 6.0.0\n"), nil
	case len(args) >= 2 && args[0] == "image" && args[1] == "ls":
		return nil, nil
	case len(args) >= 2 && args[0] == "secret" && args[1] == "ls":
		return nil, nil
	case len(args) >= 3 && args[0] == "secret" && args[1] == "create":
		body, _ := io.ReadAll(stdin)
		if p.secrets == nil {
			p.secrets = map[string]string{}
		}
		p.secrets[args[2]] = string(body)
		return nil, nil
	case len(args) > 0 && args[0] == "load":
		if p.watchFile != "" {
			if _, err := os.Stat(p.watchFile); err == nil {
				p.sawFileAtLoad = true
			}
		}
		return []byte("Loaded image: registry.test/acme/web:1.0\n"), nil
	}
	return nil, nil
}

// registryHost starts a local registry seeded with one image and returns its host:port.
func registryHost(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	image, err := random.Image(128, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	tag, err := name.NewTag(address.Host + "/acme/web:1.0")
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	if err := remote.Write(tag, image); err != nil {
		t.Fatalf("remote.Write: %v", err)
	}
	return address.Host
}

// linkTestBundle writes a source directory carrying one quadlet unit that references a seeded
// image, plus the given extra files, links it through the real link path, and returns the
// bundle directory. It needs no network: the registry is a local, in-memory one.
func linkTestBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	return linkTestDelivery(t, "", files)
}

// linkTestDelivery does the same, with extra lines appended to the unit.
func linkTestDelivery(t *testing.T, unitExtra string, files map[string]string) string {
	t.Helper()
	host := registryHost(t)
	source := t.TempDir()
	unit := "[Container]\nImage=" + host + "/acme/web:1.0\n" + unitExtra
	if err := os.WriteFile(filepath.Join(source, "web.container"), []byte(unit), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle")
	if err := link(context.Background(), report.New(io.Discard), report.New(io.Discard), source, out, "linux/amd64", "", "", ""); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

func TestSetRefusesAnEmptyValue(t *testing.T) {
	if _, err := parseSet([]string{"PUBLIC_HOST="}); !errors.Is(err, ErrSetIsEmpty) {
		t.Errorf("got %v, want ErrSetIsEmpty", err)
	}
}

func TestLoadRefusesABundleNoMachineCanInstallBeforeWritingAnything(t *testing.T) {
	root := t.TempDir()
	err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture),
		unknownKindBundle(t), root, nil, false)
	if !errors.Is(err, machine.ErrNoMachine) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/realm.json")); err == nil {
		t.Error("load wrote a file for a bundle it cannot install")
	}
}

func TestLoadRefusesABundleThatCarriesNoName(t *testing.T) {
	err := load(context.Background(), io.Discard, report.New(io.Discard), strings.NewReader(""),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture), namelessBundle(t), t.TempDir(), nil, false)
	if !errors.Is(err, ErrBundleHasNoName) {
		t.Errorf("got %v, want ErrBundleHasNoName", err)
	}
}

// unknownKindBundle writes a bundle naming a machine kind this build cannot install, carrying
// one file so a test sees whether anything reached the root.
func unknownKindBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: emptyLayout(t),
		Files:  []descriptor.File{{Path: "etc/acme/realm.json", Data: []byte("{}")}},
		Config: bundle.Config{Name: "acme", Version: "1.0.0", Reader: "compose", Platform: "linux/amd64"},
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// namelessBundle writes a bundle whose config carries no name, the way a vessel older than this
// branch produced one.
func namelessBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: emptyLayout(t),
		Config: bundle.Config{Version: "1.0.0", Reader: "quadlet", Platform: "linux/amd64"},
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// emptyLayout writes an OCI layout carrying no image.
func emptyLayout(t *testing.T) string {
	t.Helper()
	layout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(layout, "blobs/sha256"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(layout, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	write("oci-layout", `{"imageLayoutVersion":"1.0.0"}`)
	write("index.json", `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	return layout
}
