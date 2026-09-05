package bundle

import (
	"errors"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func oneUnit(t *testing.T) descriptor.Manifest {
	t.Helper()
	data := []byte("[Container]\nImage=reg.io/app:1.0\nNetwork=x\n")
	ref, err := descriptor.ParseRef("reg.io/app:1.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return descriptor.Manifest{
		Files: []descriptor.File{{Path: "etc/containers/systemd/app.container", Data: data}},
		Relocs: []descriptor.Relocation{
			{File: 0, Offset: 18, Length: len("reg.io/app:1.0"), Ref: ref},
		},
	}
}

func twoUnitsInOneFile(t *testing.T) descriptor.Manifest {
	t.Helper()
	data := []byte("Image=reg.io/a:1\nX=y\nImage=reg.io/b:2\n")
	first, err := descriptor.ParseRef("reg.io/a:1")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	second, err := descriptor.ParseRef("reg.io/b:2")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return descriptor.Manifest{
		Files: []descriptor.File{{Path: "u.container", Data: data}},
		Relocs: []descriptor.Relocation{
			{File: 0, Offset: 6, Length: len("reg.io/a:1"), Ref: first},
			{File: 0, Offset: 27, Length: len("reg.io/b:2"), Ref: second},
		},
	}
}

func TestPatchReplacesTheReferenceWithItsDigestForm(t *testing.T) {
	files, err := Patch(oneUnit(t), map[string]string{"reg.io/app:1.0": "sha256:aaa"})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	got := string(files[0].Data)
	want := "[Container]\nImage=reg.io/app@sha256:aaa\nNetwork=x\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPatchRewritesEveryReferenceInAFile(t *testing.T) {
	files, err := Patch(twoUnitsInOneFile(t), map[string]string{
		"reg.io/a:1": "sha256:aaa",
		"reg.io/b:2": "sha256:bbb",
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	got := string(files[0].Data)
	want := "Image=reg.io/a@sha256:aaa\nX=y\nImage=reg.io/b@sha256:bbb\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPatchLeavesTheSourceManifestUntouched(t *testing.T) {
	manifest := oneUnit(t)
	before := string(manifest.Files[0].Data)
	if _, err := Patch(manifest, map[string]string{"reg.io/app:1.0": "sha256:aaa"}); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := string(manifest.Files[0].Data); got != before {
		t.Errorf("source changed: got %q, want %q", got, before)
	}
}

func TestPatchNamesTheReferenceItHasNoDigestFor(t *testing.T) {
	_, err := Patch(oneUnit(t), map[string]string{})
	if err == nil {
		t.Fatal("Patch: want an error on a missing digest, got nil")
	}
	if got := err.Error(); got != "no digest for reg.io/app:1.0" {
		t.Errorf("got %q, want it to name the reference", got)
	}
}

func TestPatchErrorMatchesItsSentinel(t *testing.T) {
	_, err := Patch(oneUnit(t), map[string]string{})
	if !errors.Is(err, ErrNoDigest) {
		t.Errorf("got %v, want ErrNoDigest", err)
	}
}
