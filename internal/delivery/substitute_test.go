package delivery

import (
	"errors"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func TestSubstitutePutsTheValueInPlaceOfItsMarker(t *testing.T) {
	files := []descriptor.File{{
		Path: "etc/containers/systemd/web.container",
		Data: []byte("[Container]\nEnvironment=HOST=###PUBLIC_HOST###\n"),
	}}
	got, err := Substitute(files, map[string]string{"PUBLIC_HOST": "dmas.acme.local"}, nil)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	want := "[Container]\nEnvironment=HOST=dmas.acme.local\n"
	if string(got[0].Data) != want {
		t.Errorf("got %q, want %q", got[0].Data, want)
	}
}

func TestSubstituteReplacesEveryOccurrence(t *testing.T) {
	files := []descriptor.File{{Path: "etc/a", Data: []byte("###A### and ###A###")}}
	got, err := Substitute(files, map[string]string{"A": "x"}, nil)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if string(got[0].Data) != "x and x" {
		t.Errorf("got %q, want %q", got[0].Data, "x and x")
	}
}

func TestSubstituteNamesTheFileAndTheVariableItCannotResolve(t *testing.T) {
	files := []descriptor.File{{Path: "etc/a", Data: []byte("###PUBLIC_HOST###")}}
	_, err := Substitute(files, map[string]string{}, nil)
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("got %v, want ErrUnknownVariable", err)
	}
	if got := err.Error(); !strings.Contains(got, "PUBLIC_HOST") || !strings.Contains(got, "etc/a") {
		t.Errorf("got %q, want it to name PUBLIC_HOST and etc/a", got)
	}
}

func TestSubstituteRefusesToWriteASecretIntoAFile(t *testing.T) {
	files := []descriptor.File{{Path: "etc/a", Data: []byte("password = ###DB_PASSWORD###")}}
	_, err := Substitute(files, map[string]string{}, []string{"DB_PASSWORD"})
	if !errors.Is(err, ErrSecretInAFile) {
		t.Fatalf("got %v, want ErrSecretInAFile", err)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Errorf("got %q, want it to name DB_PASSWORD", err.Error())
	}
}

func TestSubstituteLeavesAFileWithNoMarkerUntouched(t *testing.T) {
	files := []descriptor.File{{Path: "etc/a", Data: []byte("# nothing here\n")}}
	got, err := Substitute(files, map[string]string{"A": "x"}, nil)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if string(got[0].Data) != "# nothing here\n" {
		t.Errorf("got %q, want it unchanged", got[0].Data)
	}
}

func TestSubstituteRefusesTheSecretEvenWhenAnotherVariableIsAlsoMissing(t *testing.T) {
	files := []descriptor.File{{
		Path: "etc/a",
		Data: []byte("host = ###PUBLIC_HOST###\npassword = ###DB_PASSWORD###"),
	}}
	_, err := Substitute(files, map[string]string{}, []string{"DB_PASSWORD"})
	if !errors.Is(err, ErrSecretInAFile) {
		t.Fatalf("got %v, want ErrSecretInAFile", err)
	}
}

func TestCheckMarkersRefusesAMarkerNoVariableDeclares(t *testing.T) {
	files := []descriptor.File{{Path: "etc/acme/realm.json", Data: []byte(`{"host":"###PUBLIC_HOST###"}`)}}
	err := CheckMarkers(files, []Variable{{Name: "OTHER"}})
	if !errors.Is(err, ErrUnknownVariable) {
		t.Errorf("got %v, want ErrUnknownVariable", err)
	}
}

func TestCheckMarkersRefusesASecretWrittenIntoAFile(t *testing.T) {
	files := []descriptor.File{{Path: "etc/acme/realm.json", Data: []byte("###DB_PASSWORD###")}}
	err := CheckMarkers(files, []Variable{{Name: "DB_PASSWORD", Secret: true}})
	if !errors.Is(err, ErrSecretInAFile) {
		t.Errorf("got %v, want ErrSecretInAFile", err)
	}
}

func TestCheckMarkersAcceptsAFileWhoseMarkersAreDeclared(t *testing.T) {
	files := []descriptor.File{{Path: "etc/acme/realm.json", Data: []byte("###PUBLIC_HOST###")}}
	if err := CheckMarkers(files, []Variable{{Name: "PUBLIC_HOST", Ask: "address"}}); err != nil {
		t.Errorf("CheckMarkers: %v", err)
	}
}
