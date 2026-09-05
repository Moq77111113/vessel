package descriptor

import (
	"errors"
	"testing"
)

func TestParseRefSplitsRegistryRepositoryAndTag(t *testing.T) {
	ref, err := ParseRef("registry.example.com/acme/web:1.4.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if got, want := ref.Registry, "registry.example.com"; got != want {
		t.Errorf("registry: got %q, want %q", got, want)
	}
	if got, want := ref.Repository, "acme/web"; got != want {
		t.Errorf("repository: got %q, want %q", got, want)
	}
	if got, want := ref.Tag, "1.4.0"; got != want {
		t.Errorf("tag: got %q, want %q", got, want)
	}
}

func TestParseRefDefaultsToLatestWithoutATag(t *testing.T) {
	ref, err := ParseRef("registry.example.com/acme/web")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if got, want := ref.Tag, "latest"; got != want {
		t.Errorf("tag: got %q, want %q", got, want)
	}
}

func TestParseRefKeepsAPortInTheRegistry(t *testing.T) {
	ref, err := ParseRef("registry.example.com:9004/acme/web:1.4.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if got, want := ref.Registry, "registry.example.com:9004"; got != want {
		t.Errorf("registry: got %q, want %q", got, want)
	}
	if got, want := ref.Tag, "1.4.0"; got != want {
		t.Errorf("tag: got %q, want %q", got, want)
	}
}

func TestParseRefReadsAReferenceAlreadyPinnedToADigest(t *testing.T) {
	ref, err := ParseRef("reg.io/app@sha256:abc123")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	if got, want := ref.Digest, "sha256:abc123"; got != want {
		t.Errorf("digest: got %q, want %q", got, want)
	}
}

func TestParseRefRejectsAnEmptyString(t *testing.T) {
	if _, err := ParseRef(""); err == nil {
		t.Fatal(`ParseRef(""): want an error, got nil`)
	}
}

func TestParseRefRejectsAReferenceWithoutARegistry(t *testing.T) {
	if _, err := ParseRef("alpine:3.20"); err == nil {
		t.Fatal("ParseRef: want an error on a bare name, got nil")
	}
}

func TestRefWithDigestPrintsTheDigestForm(t *testing.T) {
	ref, err := ParseRef("registry.example.com/acme/web:1.4.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	got := ref.WithDigest("sha256:abc123").String()
	want := "registry.example.com/acme/web@sha256:abc123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseRefErrorsMatchTheirSentinel(t *testing.T) {
	if _, err := ParseRef(""); !errors.Is(err, ErrEmptyRef) {
		t.Errorf("got %v, want ErrEmptyRef", err)
	}
	if _, err := ParseRef("alpine:3.20"); !errors.Is(err, ErrNoRegistry) {
		t.Errorf("got %v, want ErrNoRegistry", err)
	}
}
