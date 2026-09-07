package delivery

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestReadTakesTheWholeDeclaration(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte(`
name: dmas
version: 1.4.0
units: ./units
files:
  - source: realm.json
    target: /etc/dmas/realm.json
variables:
  - name: PUBLIC_HOST
    ask: public address of this machine
  - name: DB_PASSWORD
    secret: true
    from: openssl rand -hex 32
actions:
  - sh -c 'test -f /etc/dmas/certs/ca.crt || true'
`)}}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Name != "dmas" || got.Version != "1.4.0" || got.Units != "./units" {
		t.Errorf("got %+v, want dmas 1.4.0 ./units", got)
	}
	if len(got.Files) != 1 || got.Files[0].Target != "/etc/dmas/realm.json" {
		t.Errorf("got files %+v", got.Files)
	}
	if len(got.Variables) != 2 || !got.Variables[1].Secret || got.Variables[1].From == "" {
		t.Errorf("got variables %+v", got.Variables)
	}
	if len(got.Actions) != 1 {
		t.Errorf("got actions %+v", got.Actions)
	}
}

func TestReadSaysWhenThereIsNoDeclaration(t *testing.T) {
	if _, err := Read(fstest.MapFS{}); !errors.Is(err, ErrNoDelivery) {
		t.Errorf("got %v, want ErrNoDelivery", err)
	}
}

func TestUnitsDefaultsToTheSourceDirectory(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\n")}}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Units != "." {
		t.Errorf("got %q, want %q", got.Units, ".")
	}
}

func TestReadRejectsAVariableNameThatIsNotUpperSnakeCase(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nvariables:\n  - name: publicHost\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrVariableName) {
		t.Errorf("got %v, want ErrVariableName", err)
	}
}

func TestReadRejectsARelativeFileTarget(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: etc/a\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetNotAbsolute) {
		t.Errorf("got %v, want ErrTargetNotAbsolute", err)
	}
}

func TestReadRejectsAFileTargetThatLeavesTheRoot(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /etc/../../a\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetEscapes) {
		t.Errorf("got %v, want ErrTargetEscapes", err)
	}
}

func TestReadRejectsAFileTargetWithATrailingDotDot(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /etc/..\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetEscapes) {
		t.Errorf("got %v, want ErrTargetEscapes", err)
	}
}

func TestReadRejectsAFileTargetWithADotSegment(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /etc/./a\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetNotClean) {
		t.Errorf("got %v, want ErrTargetNotClean", err)
	}
}

func TestReadRejectsAFileTargetWithADoubledSlash(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /etc//a\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetNotClean) {
		t.Errorf("got %v, want ErrTargetNotClean", err)
	}
}

func TestReadRejectsAFileTargetThatIsTheRoot(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetIsRoot) {
		t.Errorf("got %v, want ErrTargetIsRoot", err)
	}
}

func TestReadRejectsAFileWithNoSource(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - target: /etc/a\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrFileSource) {
		t.Errorf("got %v, want ErrFileSource", err)
	}
}

func TestReadRejectsADeclarationWithNoName(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("version: 1.0\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrNoName) {
		t.Errorf("got %v, want ErrNoName", err)
	}
}

func TestReadRejectsANameThatLeavesTheRoot(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: ../../etc\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestReadRejectsANameWithASlash(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas/prod\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestReadRejectsANameThatIsDotDot(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: ..\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestReadAcceptsAnOrdinaryName(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\n")}}
	if _, err := Read(dir); err != nil {
		t.Errorf("Read: %v", err)
	}
}

func TestReadRejectsASecretThatAsksTheOperator(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte(`
name: dmas
variables:
  - name: DB_PASSWORD
    secret: true
    ask: the database password
`)}}
	if _, err := Read(dir); !errors.Is(err, ErrSecretAsks) {
		t.Errorf("got %v, want ErrSecretAsks", err)
	}
}

func TestReadReportsAFileItCannotRead(t *testing.T) {
	if _, err := Read(unreadable{}); errors.Is(err, ErrNoDelivery) {
		t.Errorf("got ErrNoDelivery, want the read failure reported instead: %v", err)
	}
}

// unreadable is a directory that holds vessel.yaml and refuses to give it up.
type unreadable struct{}

func (unreadable) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestReadRejectsAFileTargetWithADotDotInTheMiddle(t *testing.T) {
	dir := fstest.MapFS{Name: {Data: []byte("name: dmas\nfiles:\n  - source: a\n    target: /etc/../etc/dmas/realm.json\n")}}
	if _, err := Read(dir); !errors.Is(err, ErrTargetEscapes) {
		t.Errorf("got %v, want ErrTargetEscapes", err)
	}
}

func TestCheckNameRefusesAPathThatLeavesTheRoot(t *testing.T) {
	if err := CheckName("../../etc"); !errors.Is(err, ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestCheckNameRefusesAnEmptyName(t *testing.T) {
	if err := CheckName(""); !errors.Is(err, ErrNoName) {
		t.Errorf("got %v, want ErrNoName", err)
	}
}

func TestSecretNamesLeavesThePlainVariablesOut(t *testing.T) {
	got := SecretNames([]Variable{{Name: "PUBLIC_HOST"}, {Name: "DB_PASSWORD", Secret: true}})
	if len(got) != 1 || got[0] != "DB_PASSWORD" {
		t.Errorf("got %v, want [DB_PASSWORD]", got)
	}
}
