package cli

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/Moq77111113/vessel/internal/delivery"
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
    description: public address
`,
		"realm.json": `{"realm":"###PUBLIC_HOST###"}`,
	})
	root := t.TempDir()
	set := map[string]string{"PUBLIC_HOST": "dmas.acme.local"}
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture), dir, root, set, false, modeInstall); err != nil {
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
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, nil, false, modeInstall); err == nil {
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
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, nil, false, modeInstall); err != nil {
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
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, root, set, false, modeInstall); err != nil {
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
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New(stub.run)}, machine.NewShell(noCapture), dir, t.TempDir(), nil, false, modeInstall); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl start web.service") {
		t.Errorf("load never started the service: %v", stub.calls)
	}
}

func TestInstallFailsWhenAServiceDidNotStart(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	err := install(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}})
	if err == nil {
		t.Fatal("install succeeded with a service that is down")
	}
	if !errors.Is(err, ErrServiceIsDown) {
		t.Errorf("got %v, want ErrServiceIsDown", err)
	}
	if !strings.Contains(err.Error(), "web.service") {
		t.Errorf("got %q, want the service name in it", err.Error())
	}
}

func TestInstallNamesEveryServiceThatDidNotStart(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	down := []string{"db.service", "web.service"}
	err := install(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), down}})
	if err == nil {
		t.Fatal("install succeeded with two services down")
	}
	for _, name := range down {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("got %q, want %s in it", err.Error(), name)
		}
	}
}

func TestInstallLeavesTheRecordOpenWhenAServiceDidNotStart(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := install(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}}); err == nil {
		t.Fatal("install succeeded with a service that is down")
	}
	record, found, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !found {
		t.Fatal("no record after an install that failed")
	}
	if record.Done() {
		t.Error("the record is closed even though a service failed to start")
	}
}

// downServices wraps a machine.Machine and reports the named services as not running, so a test
// drives an install past Start into a machine that never came up.
type downServices struct {
	machine.Machine
	names []string
}

func (d downServices) Services(context.Context, []descriptor.File) ([]machine.Service, error) {
	services := make([]machine.Service, 0, len(d.names))
	for _, name := range d.names {
		services = append(services, machine.Service{Name: name, Running: false})
	}
	return services, nil
}

func TestInstallWritesARecordOfWhatItPut(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")

	if err := install(t, root, dir, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, found, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !found {
		t.Fatal("no record after an install that succeeded")
	}
	if record.Version != "1.4.0" {
		t.Errorf("got %q, want %q", record.Version, "1.4.0")
	}
	if !record.Done() {
		t.Error("the record is still open after an install that succeeded")
	}
	if len(record.Files) == 0 {
		t.Error("the record names no file")
	}
	if len(record.Images) == 0 {
		t.Error("the record names no image")
	}
	if !machine.Same(filepath.Join(root, record.Files[0].Path), record.Files[0].Digest) {
		t.Errorf("the record's digest for %s does not match the file on disk", record.Files[0].Path)
	}
}

func TestAFailedInstallLeavesAnOpenRecord(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")

	err := installWithBrokenLoad(t, root, dir)
	if err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	record, found, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !found {
		t.Fatal("no record after an install that failed midway")
	}
	if record.Done() {
		t.Error("the record is closed after an install that failed")
	}
	if len(record.Files) == 0 {
		t.Fatal("the record names no file")
	}
	if !machine.Same(filepath.Join(root, record.Files[0].Path), record.Files[0].Digest) {
		t.Errorf("%s never reached disk before the image load failed", record.Files[0].Path)
	}
}

// writeBundle links a minimal bundle named delivery at version and returns its directory.
func writeBundle(t *testing.T, delivery, version string) string {
	t.Helper()
	return linkTestBundle(t, map[string]string{
		"vessel.yaml": fmt.Sprintf("name: %s\nversion: %s\n", delivery, version),
	})
}

// install runs load against dir under root, through kinds (a single working podman stub when nil).
func install(t *testing.T, root, dir string, kinds []machine.Machine) error {
	t.Helper()
	if kinds == nil {
		kinds = []machine.Machine{quadlet.New((&podmanStub{}).run)}
	}
	return load(context.Background(), io.Discard, report.New(io.Discard),
		kinds, machine.NewShell(noCapture), dir, root, nil, false, modeInstall)
}

// upgrade runs load in upgrade mode against dir under root, through kinds (a single working
// podman stub when nil).
func upgrade(t *testing.T, root, dir string, kinds []machine.Machine) error {
	t.Helper()
	if kinds == nil {
		kinds = []machine.Machine{quadlet.New((&podmanStub{}).run)}
	}
	return load(context.Background(), io.Discard, report.New(io.Discard),
		kinds, machine.NewShell(noCapture), dir, root, nil, false, modeUpgrade)
}

// installWithBrokenLoad runs install through a machine whose AddImages fails after the files
// already reached disk.
func installWithBrokenLoad(t *testing.T, root, dir string) error {
	t.Helper()
	return install(t, root, dir, []machine.Machine{brokenImages{quadlet.New((&podmanStub{}).run)}})
}

// brokenImages wraps a machine.Machine and fails AddImages, so a test drives a load that dies
// after the files and before the images.
type brokenImages struct {
	machine.Machine
}

// errBrokenImageLoad is the failure brokenImages.AddImages reports.
var errBrokenImageLoad = errors.New("broken image load")

func (brokenImages) AddImages(context.Context, report.Report, *machine.Layout) ([]string, error) {
	return nil, errBrokenImageLoad
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
	case len(args) >= 1 && args[0] == "is-active":
		return []byte("active\n"), nil
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
	err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture),
		unknownKindBundle(t), root, nil, false, modeInstall)
	if !errors.Is(err, machine.ErrUnknownMachine) {
		t.Fatalf("got %v, want ErrUnknownMachine", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/realm.json")); err == nil {
		t.Error("load wrote a file for a bundle it cannot install")
	}
}

func TestLoadRefusesABundleThatCarriesNoName(t *testing.T) {
	err := load(context.Background(), io.Discard, report.New(io.Discard),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture), namelessBundle(t), t.TempDir(), nil, false, modeInstall)
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

func TestInstallRefusesABundleWhoseNameLeavesTheTargetRoot(t *testing.T) {
	root := t.TempDir()
	err := install(t, root, hostileBundle(t, "../../../etc/cron.daily"), nil)
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Fatalf("got %v, want ErrDeliveryName", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %v, want an untouched root", entries)
	}
}

// hostileBundle writes a bundle carrying name, a name link itself would never let through.
func hostileBundle(t *testing.T, name string) string {
	t.Helper()
	layout := t.TempDir()
	writeFile(t, filepath.Join(layout, "index.json"),
		`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	writeFile(t, filepath.Join(layout, "oci-layout"), `{"imageLayoutVersion":"1.0.0"}`)
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Config: bundle.Config{Name: name, Version: "1.0.0", Reader: "quadlet"},
		Layout: layout,
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("bundle.Write: %v", err)
	}
	return dir
}

func TestTheRecordNamesOnlyTheSecretsThisInstallCreated(t *testing.T) {
	dir := linkTestDelivery(t, "Secret=OLD_PASSWORD,type=env,target=OLD\nSecret=NEW_PASSWORD,type=env,target=NEW\n",
		map[string]string{
			"vessel.yaml": `
name: acme
version: 1.4.0
variables:
  - name: OLD_PASSWORD
    secret: true
  - name: NEW_PASSWORD
    secret: true
`,
		})
	root := t.TempDir()
	kinds := []machine.Machine{heldSecrets{quadlet.New((&podmanStub{}).run), []string{"OLD_PASSWORD"}}}
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		kinds, machine.NewShell(noCapture), dir, root,
		map[string]string{"NEW_PASSWORD": "hunter2"}, false, modeInstall); err != nil {
		t.Fatalf("load: %v", err)
	}
	record, _, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !slices.Equal(record.Secrets, []string{"NEW_PASSWORD"}) {
		t.Errorf("got %v, want only the secret this install created", record.Secrets)
	}
}

func TestASecondInstallStillNamesTheSecretTheFirstCreated(t *testing.T) {
	dir := linkTestDelivery(t, "Secret=NEW_PASSWORD,type=env,target=NEW\n",
		map[string]string{
			"vessel.yaml": `
name: acme
version: 1.4.0
variables:
  - name: NEW_PASSWORD
    secret: true
`,
		})
	root := t.TempDir()
	set := map[string]string{"NEW_PASSWORD": "hunter2"}
	first := []machine.Machine{quadlet.New((&podmanStub{}).run)}
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		first, machine.NewShell(noCapture), dir, root, set, false, modeInstall); err != nil {
		t.Fatalf("first load: %v", err)
	}
	second := []machine.Machine{heldSecrets{quadlet.New((&podmanStub{}).run), []string{"NEW_PASSWORD"}}}
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		second, machine.NewShell(noCapture), dir, root, nil, false, modeUpgrade); err != nil {
		t.Fatalf("second load: %v", err)
	}
	record, _, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !slices.Equal(record.Secrets, []string{"NEW_PASSWORD"}) {
		t.Errorf("got %v, want the secret the first install created still named", record.Secrets)
	}
}

func TestTheRecordNamesThePlatformTheBundleWasLinkedFor(t *testing.T) {
	root := t.TempDir()
	if err := install(t, root, writeBundle(t, "acme", "1.4.0"), nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, _, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if record.Platform != "linux/amd64" {
		t.Errorf("got %q, want linux/amd64", record.Platform)
	}
}

func TestAnInstallThatDiesWritingAFileStoresTheSiteValues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/acme"), "an ordinary file where the delivery wants a directory")
	set := map[string]string{"PUBLIC_HOST": "dmas.acme.local"}
	if err := load(context.Background(), io.Discard, report.New(io.Discard),
		testKinds(), machine.NewShell(noCapture), blockedFileBundle(t), root, set, false, modeInstall); err == nil {
		t.Fatal("install succeeded even though no file could be written")
	}
	values, err := siteFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the site values: %v", err)
	}
	if values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %v, want the answer stored for an unattended re-run", values)
	}
}

// blockedFileBundle links a bundle whose one file can never be written: the directory it needs
// is already an ordinary file.
func blockedFileBundle(t *testing.T) string {
	t.Helper()
	return linkTestBundle(t, map[string]string{
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
}

// heldSecrets wraps a machine.Machine and reports secrets the machine already holds.
type heldSecrets struct {
	machine.Machine
	names []string
}

func (h heldSecrets) Secrets(context.Context) ([]string, error) { return h.names, nil }
