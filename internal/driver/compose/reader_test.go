package compose_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/Moq77111113/vessel/internal/driver/compose"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

func station(t *testing.T) delivery.Manifest {
	t.Helper()
	manifest, err := compose.NewReader().Read(os.DirFS("testdata/station"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return manifest
}

func paths(files []delivery.File) []string {
	list := make([]string, len(files))
	for i, file := range files {
		list[i] = file.Path
	}
	return list
}

func TestDetectFindsAComposeFile(t *testing.T) {
	if !compose.NewReader().Detect(os.DirFS("testdata/station")) {
		t.Error("got false, want the station directory detected")
	}
}

func TestDetectIgnoresADirectoryWithoutComposeFile(t *testing.T) {
	dir := fstest.MapFS{"web.container": {Data: []byte("[Container]\n")}}
	if compose.NewReader().Detect(dir) {
		t.Error("got true, want a quadlet directory left alone")
	}
}

func TestReadPlacesTheComposeFileUnderItsProjectName(t *testing.T) {
	manifest := station(t)
	if got, want := manifest.Files[0].Path, "etc/compose/station/compose.yaml"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadFindsTheOffsetOfEveryImage(t *testing.T) {
	manifest := station(t)
	want := []string{
		"docker.io/victoriametrics/victoria-metrics:v1.110.0",
		"docker.io/library/busybox:1.37",
		"docker.io/grafana/grafana:11.5.0",
		"docker.io/library/busybox:1.37",
	}
	var refs []string
	for _, reloc := range manifest.Relocs {
		data := manifest.Files[reloc.File].Data
		text := string(data[reloc.Offset : reloc.Offset+reloc.Length])
		if text != reloc.Ref.String() {
			t.Errorf("bytes at %d: got %q, want %q", reloc.Offset, text, reloc.Ref.String())
		}
		refs = append(refs, text)
	}
	if !slices.Equal(refs, want) {
		t.Errorf("got %v, want %v", refs, want)
	}
}

func TestReadPinsAnImageSharedThroughAnAnchorOnce(t *testing.T) {
	manifest := station(t)
	count := 0
	for _, reloc := range manifest.Relocs {
		if reloc.Ref.Repository == "victoriametrics/victoria-metrics" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("got %d relocations for the anchored image, want 1", count)
	}
}

func TestReadCarriesWhatTheComposeFileUsesBesideIt(t *testing.T) {
	want := []string{
		"etc/compose/station/compose.yaml",
		"etc/compose/station/.env",
		"etc/compose/station/dashboards.json",
		"etc/compose/station/grafana/datasources/store.yaml",
		"etc/compose/station/screen.env",
		"etc/compose/station/sensor.env",
		"etc/compose/station/station.conf",
	}
	if got := paths(station(t).Files); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadDeclaresOneServicePerProject(t *testing.T) {
	want := []delivery.Service{{Name: "station", Path: "etc/compose/station/compose.yaml"}}
	if got := station(t).Services; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadNamesEverySecretAServiceReads(t *testing.T) {
	if got, want := station(t).Secrets, []string{"SCREEN_PASSWORD"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func refuses(t *testing.T, dir string, want error) {
	t.Helper()
	_, err := compose.NewReader().Read(os.DirFS("testdata/refused/" + dir))
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestReadRefusesAServiceItWouldHaveToBuild(t *testing.T) {
	refuses(t, "build", compose.ErrBuild)
}

func TestReadRefusesAServiceWithoutImage(t *testing.T) {
	refuses(t, "no-image", compose.ErrNoImage)
}

func TestReadRefusesAComposeFileWithoutName(t *testing.T) {
	refuses(t, "no-name", compose.ErrProjectName)
}

func TestReadRefusesAProjectNameComposeRejects(t *testing.T) {
	refuses(t, "bad-name", compose.ErrProjectName)
}

func TestReadRefusesAnImageNamedThroughAVariable(t *testing.T) {
	refuses(t, "variable", compose.ErrImageVariable)
}

func TestReadRefusesASecretTakenFromTheEnvironment(t *testing.T) {
	refuses(t, "secret-environment", compose.ErrSecretFile)
}

func TestReadRefusesASecretTheFileDoesNotDeclare(t *testing.T) {
	refuses(t, "undeclared-secret", compose.ErrSecretFile)
}

func TestReadRefusesTwoOwnersForOneSecret(t *testing.T) {
	refuses(t, "two-owners", compose.ErrSecretOwner)
}

func TestReadRefusesABindThatLeavesTheProject(t *testing.T) {
	refuses(t, "outside", compose.ErrOutsideProject)
}

func TestReadRefusesToCarryTheSecretsDirectory(t *testing.T) {
	refuses(t, "carries-secrets", compose.ErrCarriesSecrets)
}

func TestReadRefusesASecretNameThatLeavesTheSecretsDirectory(t *testing.T) {
	refuses(t, "secret-name", compose.ErrSecretName)
}

func TestReadRefusesALinkThatLeadsOutsideTheSource(t *testing.T) {
	source := t.TempDir()
	if err := os.CopyFS(source, os.DirFS("testdata/station")); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}
	if err := os.Remove(filepath.Join(source, "sensor.env")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(source, "sensor.env")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := compose.NewReader().Read(os.DirFS(source)); !errors.Is(err, compose.ErrLink) {
		t.Errorf("got %v, want compose.ErrLink", err)
	}
}
