package site

import (
	"bytes"
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

	"github.com/Moq77111113/vessel/internal/build"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/internal/site/values"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
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
	if err := at(t, root, nil).job(dir, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
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
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(dir); err == nil {
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
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(dir); err != nil {
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
	job := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).job(dir, nil)
	job.SetFile = dbPassword
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
	root := t.TempDir()
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(twoUnitBundle(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl start db.service web.service") {
		t.Errorf("the install never started the services: %v", stub.calls)
	}
}

func TestInstallAsksSystemdWhetherEveryServiceCameUp(t *testing.T) {
	stub := &podmanStub{}
	root := t.TempDir()
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(twoUnitBundle(t)); err != nil {
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
	if err := at(t, root, nil).install(dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	unit := filepath.Join(root, "etc/containers/systemd/web.container")
	before, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := at(t, root, nil).install(dir); err != nil {
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
	err := at(t, root, []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), []string{"web.service"}}}).install(dir)
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
	dir := twoUnitBundle(t)
	down := []string{"db.service", "web.service"}
	err := at(t, root, []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), down}}).install(dir)
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
	if err := at(t, root, []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), []string{"web.service"}}}).install(dir); err == nil {
		t.Fatal("install succeeded with a service that is down")
	}
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if record.Done() {
		t.Error("the record is closed even though a service failed to start")
	}
}

type downServices struct {
	driver.Driver
	names []string
}

func (d downServices) Running(ctx context.Context, services []delivery.Service) ([]delivery.Service, error) {
	var running []delivery.Service
	for _, service := range services {
		if !slices.Contains(d.names, service.Name) {
			running = append(running, service)
		}
	}
	return running, nil
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
	if !disk.Same(filepath.Join(root, record.Files[0].Path), record.Files[0].Digest) {
		t.Errorf("the record's digest for %s does not match the file on disk", record.Files[0].Path)
	}
}

func recordAfterAnInstall(t *testing.T, root string) release.Record {
	t.Helper()
	if err := at(t, root, nil).install(writeBundle(t, "acme", "1.4.0")); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
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
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if record.Done() {
		t.Error("the record is closed after an install that failed")
	}
	if len(record.Files) == 0 {
		t.Fatal("the record names no file")
	}
	if !disk.Same(filepath.Join(root, record.Files[0].Path), record.Files[0].Digest) {
		t.Errorf("%s never reached disk before the image load failed", record.Files[0].Path)
	}
}

func writeBundle(t *testing.T, delivery, version string) string {
	t.Helper()
	return linkTestBundle(t, map[string]string{
		"vessel.yaml": fmt.Sprintf("name: %s\nversion: %s\n", delivery, version),
	})
}

func installWithABrokenImageLoad(t *testing.T, root, dir string) error {
	t.Helper()
	return at(t, root, []driver.Driver{brokenImages{quadlet.New(root, (&podmanStub{}).run)}}).install(dir)
}

type brokenImages struct {
	driver.Driver
}

var errBrokenImageLoad = errors.New("broken image load")

func (brokenImages) AddImages(context.Context, report.Report, *oci.Images) ([]string, error) {
	return nil, errBrokenImageLoad
}

func noCapture(context.Context, string) ([]byte, error) {
	return nil, errors.New("the shell should not run in this test")
}

type podmanStub struct {
	calls         []string
	secrets       map[string]string
	watchFile     string
	sawFileAtLoad bool
	stdinErr      error
}

func (p *podmanStub) run(_ context.Context, command quadlet.Command) ([]byte, error) {
	stdin, name, args := command.Stdin, command.Line[0], command.Line[1:]
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
	case name == "systemd-creds" && len(args) == 4 && args[0] == "encrypt":
		body, _ := io.ReadAll(stdin)
		return nil, os.WriteFile(args[3], body, 0o600)
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

func linkTestBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	return linkTestDelivery(t, "", files)
}

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
	link := build.Link{Drivers: testKinds("/")}
	if _, err := link.Run(context.Background(), build.LinkRequest{Source: source, Platform: "linux/amd64", Dir: out, Work: report.New(io.Discard)}); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

func TestInstallRefusesABundleNoMachineCanInstallBeforeWritingAnything(t *testing.T) {
	root := t.TempDir()
	err := at(t, root, nil).install(unknownKindBundle(t))
	if !errors.Is(err, ErrUnknownDriver) {
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

func unknownKindBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: emptyLayout(t),
		Files:  []delivery.File{{Path: "etc/acme/realm.json", Data: []byte("{}")}},
		Config: bundle.Config{Name: "acme", Version: "1.0.0", Machine: "compose", Platform: "linux/amd64"},
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

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
	err := at(t, root, nil).install(hostileBundle(t, "../../../etc/cron.daily"))
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
	kinds := []driver.Driver{heldSecrets{quadlet.New(root, (&podmanStub{}).run), []string{"OLD_PASSWORD"}}}
	job := at(t, root, kinds).job(dir, nil)
	job.SetFile = map[string]string{"NEW_PASSWORD": "hunter2"}
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
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
	first := at(t, root, []driver.Driver{quadlet.New(root, (&podmanStub{}).run)}).job(dir, nil)
	first.SetFile = map[string]string{"NEW_PASSWORD": "hunter2"}
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("first install: %v", err)
	}
	second := []driver.Driver{heldSecrets{quadlet.New(root, (&podmanStub{}).run), []string{"NEW_PASSWORD"}}}
	if err := at(t, root, second).upgrade(dir); err != nil {
		t.Fatalf("second install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !slices.Equal(record.Secrets, []string{"NEW_PASSWORD"}) {
		t.Errorf("got %v, want the secret the first install created still named", record.Secrets)
	}
}

func TestTheRecordNamesThePlatformTheBundleWasLinkedFor(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(writeBundle(t, "acme", "1.4.0")); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
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
	job := at(t, root, testKinds(root)).job(blockedFileBundle(t), set)
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
	job := at(t, root, testKinds(root)).job(blockedFileBundle(t), map[string]string{"PUBLIC_HOST": "dmas.acme.local"})
	err := job.Run(context.Background(), io.Discard, report.New(io.Discard))
	if err == nil {
		t.Fatal("the install succeeded even though no file could be written")
	}
	if !errors.Is(err, ErrPartlyInstalled) {
		t.Errorf("got %v, want ErrPartlyInstalled", err)
	}
}

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
	kinds := []driver.Driver{unreadyMachine{quadlet.New(root, (&podmanStub{}).run)}}
	err := at(t, root, kinds).install(dir)
	if !errors.Is(err, driver.ErrNotReady) {
		t.Fatalf("got %v, want the machine-not-ready error", err)
	}
}

type unreadyMachine struct {
	driver.Driver
}

func (unreadyMachine) Check(context.Context) error { return driver.ErrNotReady }

type heldSecrets struct {
	driver.Driver
	names []string
}

func (h heldSecrets) Secrets(context.Context) ([]string, error) { return h.names, nil }

func TestTheRecordSaysTheBundleIsInsecure(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkInsecureBundle(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !record.Insecure {
		t.Error("the record of an unsigned bundle does not say it is insecure")
	}
}

func linkInsecureBundle(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "web.container"), "[Container]\nImage="+registryHost(t)+"/acme/web:1.0\n")
	body, err := os.ReadFile("testdata/insecure/vessel.yaml")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	writeFile(t, filepath.Join(source, "vessel.yaml"), string(body))
	out := filepath.Join(t.TempDir(), "bundle")
	link := build.Link{Drivers: testKinds("/")}
	if _, err := link.Run(context.Background(), build.LinkRequest{Source: source, Platform: "linux/amd64", InsecureUnsigned: true, Dir: out, Work: report.New(io.Discard)}); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

func TestInstallSaysTheBundleIsInsecure(t *testing.T) {
	var out bytes.Buffer
	job := at(t, t.TempDir(), nil).job(linkInsecureBundle(t), nil)
	if err := job.Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "insecure") {
		t.Errorf("got %q, want the install to say the bundle is insecure", out.String())
	}
}

func TestTheRecordNamesEveryStepAFinishedInstallRan(t *testing.T) {
	entry := recordAfterAnInstall(t, t.TempDir())
	want := []release.Step{release.StepValues, release.StepActions, release.StepFiles,
		release.StepSecrets, release.StepImages, release.StepServices}
	if !slices.Equal(entry.Steps, want) {
		t.Errorf("got %v, want %v", entry.Steps, want)
	}
}

func TestAnInstallCutAtTheImagesNamesTheStepsBefore(t *testing.T) {
	root := t.TempDir()
	if err := installWithABrokenImageLoad(t, root, writeBundle(t, "acme", "1.4.0")); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	want := []release.Step{release.StepValues, release.StepActions, release.StepFiles, release.StepSecrets}
	if !slices.Equal(entry.Steps, want) {
		t.Errorf("got %v, want %v", entry.Steps, want)
	}
}

func TestAFinishedActionCarriesItsEndInTheRecord(t *testing.T) {
	root := t.TempDir()
	job := at(t, root, nil).job(linkTestBundle(t, fixture(t, "one-action")), nil)
	job.Shell = disk.NewShell(quietShell)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if len(entry.Actions) != 1 || entry.Actions[0].Command != "mkdir -p /srv/acme" || entry.Actions[0].End.IsZero() {
		t.Errorf("got %+v, want the one action with its end", entry.Actions)
	}
}

func TestAnActionThatFailsLeavesNoOpenActionInTheRecord(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "one-action"))); err == nil {
		t.Fatal("the install succeeded with an action that failed")
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if action, open := entry.OpenAction(); open {
		t.Errorf("got %+v open, want a failed action taken off the record", action)
	}
	if entry.Has(release.StepActions) {
		t.Error("the record says the actions finished though one failed")
	}
}

func TestAnActionKilledBySignalStaysOpenInTheRecord(t *testing.T) {
	root := t.TempDir()
	job := at(t, root, nil).job(linkTestBundle(t, fixture(t, "one-action")), nil)
	job.Shell = disk.NewShell(func(ctx context.Context, _ string) ([]byte, error) {
		return disk.Sh(ctx, "kill -9 $$")
	})
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with an action killed by a signal")
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if _, open := entry.OpenAction(); !open {
		t.Errorf("got %+v, want the killed action open", entry.Actions)
	}
}

func fixture(t *testing.T, name string) map[string]string {
	t.Helper()
	dir := filepath.Join("testdata", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the fixture %s: %v", name, err)
	}
	files := make(map[string]string, len(entries))
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(body)
	}
	return files
}

func quietShell(context.Context, string) ([]byte, error) { return nil, nil }

func TestInstallRefusesAMachineAnInstallLeftHalfway(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	stub := &podmanStub{}
	err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).install(dir)
	if !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
	if errors.Is(err, ErrPartlyInstalled) {
		t.Error("a refusal before any change says the machine changed")
	}
	for _, call := range stub.calls {
		if call != "podman --version" && call != "podman image ls" {
			t.Errorf("the refusal ran %q", call)
		}
	}
}

func TestInstallRefusesAnInstallAnOlderVesselLeftHalfway(t *testing.T) {
	root := t.TempDir()
	body, err := os.ReadFile("testdata/unfinished-main/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	if err := at(t, root, nil).install(writeBundle(t, "acme", "1.4.0")); !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
}

func TestTheRefusalOffersResume(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	err := at(t, root, nil).install(dir)
	if err == nil || !strings.Contains(err.Error(), "run resume") {
		t.Errorf("got %v, want resume offered", err)
	}
}

func TestInstallRefusesASecretPassedWithSet(t *testing.T) {
	job := at(t, t.TempDir(), nil).job(dryRunBundle(t), dbPassword)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, values.ErrSecretOnCommandLine) {
		t.Fatalf("got %v, want values.ErrSecretOnCommandLine", err)
	}
}

func TestInstallRefusesAPlainValuePassedWithSetFile(t *testing.T) {
	job := at(t, t.TempDir(), nil).job(linkTestBundle(t, fixture(t, "one-value")), nil)
	job.SetFile = map[string]string{"PUBLIC_HOST": "a.acme.local"}
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, values.ErrSetFilePlain) {
		t.Fatalf("got %v, want values.ErrSetFilePlain", err)
	}
}

func TestInstallRefusesAFileNoDeliveryPutThere(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/acme/a.txt"), "written by the admin\n")
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "acme-1.3"))); !errors.Is(err, ErrFileExists) {
		t.Fatalf("got %v, want ErrFileExists", err)
	}
}

func TestInstallRefusesAFileAnotherDeliveryOwns(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "acme-1.3"))); err != nil {
		t.Fatalf("install acme: %v", err)
	}
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "billing"))); !errors.Is(err, ErrFileTaken) {
		t.Fatalf("got %v, want ErrFileTaken", err)
	}
}

func TestInstallWritesTheModeTheDescriptorDeclares(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "file-mode"))); err != nil {
		t.Fatalf("install: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "etc/acme/token"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("got %o, want 600", info.Mode().Perm())
	}
}

func TestInstallRecordsTheServicesItStarts(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, []driver.Driver{quadlet.New(root, (&podmanStub{}).run)}).install(twoUnitBundle(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	record, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	var names []string
	for _, service := range record.Services {
		names = append(names, service.Name)
	}
	if got, want := strings.Join(names, " "), "db.service web.service"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
