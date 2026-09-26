package delivery

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
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/link"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestInstallPutsTheSetValueIntoTheFile(t *testing.T) {
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
	if err := jobFor(t, root, dir, nil, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/acme/realm.json"))
	if err != nil {
		t.Fatalf("the file never reached the root: %v", err)
	}
	if got, want := string(body), `{"realm":"dmas.acme.local"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestInstallWritesNothingWhenAValueIsMissing(t *testing.T) {
	dir := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nvariables:\n  - name: PUBLIC_HOST\n"})
	root := t.TempDir()
	stub := &podmanStub{}
	if err := runInstall(t, root, dir, []machine.Machine{quadlet.New(stub.run)}); err == nil {
		t.Fatal("the install succeeded with no value for PUBLIC_HOST")
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

func TestInstallLoadsImagesAfterWritingFiles(t *testing.T) {
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
	if err := runInstall(t, root, dir, []machine.Machine{quadlet.New(stub.run)}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !stub.sawFileAtLoad {
		t.Error("podman load ran before the file reached the root")
	}
}

func TestInstallCreatesASecretWithoutStoringIt(t *testing.T) {
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
	job := jobFor(t, root, dir, []machine.Machine{quadlet.New(stub.run)}, set)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if stub.stdinErr != nil {
		t.Fatalf("read the secret podman was handed: %v", stub.stdinErr)
	}
	value, ok := stub.secrets["DB_PASSWORD"]
	if !ok {
		t.Fatal("podman secret create was never called for DB_PASSWORD")
	}
	if value != "hunter2" {
		t.Errorf("got %q, want %q", value, "hunter2")
	}
	body, err := os.ReadFile(filepath.Join(root, "var/lib/vessel/acme/values"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(body), "DB_PASSWORD") || strings.Contains(string(body), "hunter2") {
		t.Errorf("the store carries the secret: %s", body)
	}
}

func TestInstallStartsEveryServiceTheUnitsGenerate(t *testing.T) {
	stub := &podmanStub{}
	if err := runInstall(t, t.TempDir(), twoUnitBundle(t), []machine.Machine{quadlet.New(stub.run)}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl start db.service web.service") {
		t.Errorf("the install never started the services: %v", stub.calls)
	}
}

func TestInstallAsksSystemdWhetherEveryServiceCameUp(t *testing.T) {
	stub := &podmanStub{}
	if err := runInstall(t, t.TempDir(), twoUnitBundle(t), []machine.Machine{quadlet.New(stub.run)}); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, call := range []string{"systemctl is-active db.service", "systemctl is-active web.service"} {
		if !slices.Contains(stub.calls, call) {
			t.Errorf("the install reported success without %q: %v", call, stub.calls)
		}
	}
}

func TestASecondInstallLeavesAFileItDidNotChangeAlone(t *testing.T) {
	root, dir := t.TempDir(), writeBundle(t, "acme", "1.4.0")
	if err := runInstall(t, root, dir, nil); err != nil {
		t.Fatalf("first install: %v", err)
	}
	unit := filepath.Join(root, "etc/containers/systemd/web.container")
	before, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := runInstall(t, root, dir, nil); err != nil {
		t.Fatalf("second install: %v", err)
	}
	after, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("the second install rewrote a file whose content had not changed")
	}
}

// twoUnitBundle links a bundle carrying two units, so a claim about every service has two to name.
func twoUnitBundle(t *testing.T) string {
	t.Helper()
	return linkTestBundle(t, map[string]string{
		"vessel.yaml":  "name: acme\nversion: 1.4.0\n",
		"db.container": "[Container]\nContainerName=db\n",
	})
}

func TestInstallFailsWhenAServiceDidNotStart(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	err := runInstall(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}})
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
	err := runInstall(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), down}})
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
	if err := runInstall(t, root, dir, []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}}); err == nil {
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

// downServices reports the named services as not running, whatever the machine did.
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

func TestInstallWritesARecordNamingTheVersionItPut(t *testing.T) {
	record := recordAfterAnInstall(t, t.TempDir())
	if record.Version != "1.4.0" {
		t.Errorf("got %q, want %q", record.Version, "1.4.0")
	}
}

func TestInstallClosesTheRecordWhenEveryServiceStarted(t *testing.T) {
	if record := recordAfterAnInstall(t, t.TempDir()); !record.Done() {
		t.Error("the record is still open after an install that succeeded")
	}
}

func TestTheRecordNamesEveryFileTheInstallPut(t *testing.T) {
	if record := recordAfterAnInstall(t, t.TempDir()); len(record.Files) == 0 {
		t.Error("the record names no file")
	}
}

func TestTheRecordNamesEveryImageTheInstallPut(t *testing.T) {
	if record := recordAfterAnInstall(t, t.TempDir()); len(record.Images) == 0 {
		t.Error("the record names no image")
	}
}

func TestTheRecordDigestMatchesTheFileOnDisk(t *testing.T) {
	root := t.TempDir()
	record := recordAfterAnInstall(t, root)
	if len(record.Files) == 0 {
		t.Fatal("the record names no file")
	}
	if !machine.Same(filepath.Join(root, record.Files[0].Path), record.Files[0].Digest) {
		t.Errorf("the record's digest for %s does not match the file on disk", record.Files[0].Path)
	}
}

// recordAfterAnInstall installs one bundle under root and returns the record it left.
func recordAfterAnInstall(t *testing.T, root string) record.Record {
	t.Helper()
	if err := runInstall(t, root, writeBundle(t, "acme", "1.4.0"), nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, found, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !found {
		t.Fatal("no record after an install that succeeded")
	}
	return record
}

func TestAFailedInstallLeavesAnOpenRecord(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")

	err := installWithABrokenImageLoad(t, root, dir)
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

// jobFor builds the install a test drives, through kinds or a working podman stub when nil.
func jobFor(t *testing.T, root, dir string, kinds []machine.Machine, set map[string]string) Install {
	t.Helper()
	if kinds == nil {
		kinds = []machine.Machine{quadlet.New((&podmanStub{}).run)}
	}
	artifact, err := bundle.OpenNamed(dir)
	if err != nil {
		t.Fatalf("open the bundle: %v", err)
	}
	return Install{Artifact: artifact, Root: root, Set: set, Machines: kinds, Shell: machine.NewShell(noCapture)}
}

// runInstall installs the bundle in dir under root, printing nothing.
func runInstall(t *testing.T, root, dir string, kinds []machine.Machine) error {
	t.Helper()
	return jobFor(t, root, dir, kinds, nil).Run(context.Background(), io.Discard, report.New(io.Discard))
}

// runUpgrade upgrades the bundle in dir under root, printing nothing.
func runUpgrade(t *testing.T, root, dir string, kinds []machine.Machine) error {
	t.Helper()
	return jobFor(t, root, dir, kinds, nil).Upgrade(context.Background(), io.Discard, report.New(io.Discard))
}

// installWithABrokenImageLoad installs through a machine whose AddImages fails.
func installWithABrokenImageLoad(t *testing.T, root, dir string) error {
	t.Helper()
	return runInstall(t, root, dir, []machine.Machine{brokenImages{quadlet.New((&podmanStub{}).run)}})
}

// brokenImages fails AddImages, after the files and before the images.
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

// podmanStub answers podman well enough to drive an install without podman, remembering every call.
type podmanStub struct {
	calls         []string
	secrets       map[string]string
	watchFile     string
	sawFileAtLoad bool
	stdinErr      error
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
		body, err := io.ReadAll(stdin)
		if err != nil {
			p.stdinErr = err
		}
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

// linkTestBundle links a one-unit source plus these files into a bundle, against a local registry.
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
	if err := link.Link(context.Background(), report.New(io.Discard), report.New(io.Discard), testKinds(), source, out, "linux/amd64", "", ""); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

func TestSetRefusesAnEmptyValue(t *testing.T) {
	if _, err := ParseSet([]string{"PUBLIC_HOST="}); !errors.Is(err, ErrSetIsEmpty) {
		t.Errorf("got %v, want ErrSetIsEmpty", err)
	}
}

func TestInstallRefusesABundleNoMachineCanInstallBeforeWritingAnything(t *testing.T) {
	root := t.TempDir()
	err := runInstall(t, root, unknownKindBundle(t), nil)
	if !errors.Is(err, machine.ErrUnknownMachine) {
		t.Fatalf("got %v, want ErrUnknownMachine", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/realm.json")); err == nil {
		t.Error("load wrote a file for a bundle it cannot install")
	}
}

func TestABundleThatCarriesNoNameIsRefused(t *testing.T) {
	_, err := bundle.OpenNamed(namelessBundle(t))
	if !errors.Is(err, bundle.ErrBundleHasNoName) {
		t.Errorf("got %v, want bundle.ErrBundleHasNoName", err)
	}
}

// unknownKindBundle writes a bundle naming a machine kind this build cannot install.
func unknownKindBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: emptyLayout(t),
		Files:  []descriptor.File{{Path: "etc/acme/realm.json", Data: []byte("{}")}},
		Config: bundle.Config{Name: "acme", Version: "1.0.0", Machine: "compose", Platform: "linux/amd64"},
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

// namelessBundle writes a bundle whose config carries no name, as an older vessel produced one.
func namelessBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: emptyLayout(t),
		Config: bundle.Config{Version: "1.0.0", Machine: "quadlet", Platform: "linux/amd64"},
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
	err := runInstall(t, root, hostileBundle(t, "../../../etc/cron.daily"), nil)
	if !errors.Is(err, descriptor.ErrDeliveryName) {
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
		Config: bundle.Config{Name: name, Version: "1.0.0", Machine: "quadlet"},
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
	set := map[string]string{"NEW_PASSWORD": "hunter2"}
	if err := jobFor(t, root, dir, kinds, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
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
	if err := jobFor(t, root, dir, first, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("first install: %v", err)
	}
	second := []machine.Machine{heldSecrets{quadlet.New((&podmanStub{}).run), []string{"NEW_PASSWORD"}}}
	if err := runUpgrade(t, root, dir, second); err != nil {
		t.Fatalf("second install: %v", err)
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
	if err := runInstall(t, root, writeBundle(t, "acme", "1.4.0"), nil); err != nil {
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
	job := jobFor(t, root, blockedFileBundle(t), testKinds(), set)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded even though no file could be written")
	}
	values, err := siteFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the site values: %v", err)
	}
	if values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %v, want the answer stored for an unattended re-run", values)
	}
}

func TestAnInstallThatDiesWritingAFileSaysTheMachineIsPartwayThrough(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/acme"), "an ordinary file where the delivery wants a directory")
	job := jobFor(t, root, blockedFileBundle(t), testKinds(), map[string]string{"PUBLIC_HOST": "dmas.acme.local"})
	err := job.Run(context.Background(), io.Discard, report.New(io.Discard))
	if err == nil {
		t.Fatal("the install succeeded even though no file could be written")
	}
	if !errors.Is(err, ErrPartlyInstalled) {
		t.Errorf("got %v, want ErrPartlyInstalled", err)
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

func TestInstallReportsTheMachineNotReadyEvenWhenTheRecordCannotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permissions cannot make the record unreadable")
	}
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	recordDir := filepath.Join(root, "var/lib/vessel/acme")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(recordDir, "record.json")
	if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(recordPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(recordPath, 0o644); err != nil {
			t.Errorf("chmod: %v", err)
		}
	})
	kinds := []machine.Machine{unreadyMachine{quadlet.New((&podmanStub{}).run)}}
	err := runInstall(t, root, dir, kinds)
	if !errors.Is(err, quadlet.ErrNotReady) {
		t.Fatalf("got %v, want the machine-not-ready error", err)
	}
}

// unreadyMachine wraps a machine.Machine and fails Check, as an operator's own machine would.
type unreadyMachine struct {
	machine.Machine
}

func (unreadyMachine) Check(context.Context, string) error { return quadlet.ErrNotReady }

// heldSecrets wraps a machine.Machine and reports secrets the machine already holds.
type heldSecrets struct {
	machine.Machine
	names []string
}

func (h heldSecrets) Secrets(context.Context) ([]string, error) { return h.names, nil }
