package cli

import (
	"context"
	"slices"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// line is one announcement a recorder saw, and the order it saw it in.
type line struct {
	verb    string
	subject string
}

// recorder stands in for a registry and a report.Report at once, so one log shows whether an
// announcement came before or after the call it describes.
type recorder struct {
	calls []string
	lines []line
}

func (r *recorder) Line(verb, subject string) {
	r.calls = append(r.calls, "line")
	r.lines = append(r.lines, line{verb: verb, subject: subject})
}

func (r *recorder) Resolve(context.Context, descriptor.Ref, string) (string, error) {
	r.calls = append(r.calls, "resolve")
	return "sha256:deadbeef", nil
}

func (r *recorder) Image(context.Context, descriptor.Ref) (v1.Image, error) {
	r.calls = append(r.calls, "image")
	return random.Image(64, 1)
}

func oneRelocation(t *testing.T) []descriptor.Relocation {
	t.Helper()
	ref, err := descriptor.ParseRef("registry.test/acme/web:1.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return []descriptor.Relocation{{Ref: ref}}
}

// install runs on a client site pulling images over a slow link; if the announcement waited
// until after the call returned, the operator would watch a silent terminal.
func TestResolveAllAnnouncesBeforeItResolvesAndBeforeItPulls(t *testing.T) {
	work := &recorder{}
	if _, err := resolveAll(context.Background(), work, work, oneRelocation(t), "linux/amd64", t.TempDir()); err != nil {
		t.Fatalf("resolveAll: %v", err)
	}
	want := []string{"line", "resolve", "line", "image"}
	if !slices.Equal(work.calls, want) {
		t.Errorf("got %v, want %v: an announcement before Resolve and another before the pull", work.calls, want)
	}
}

// The digest stays visible while the image is still being pulled: an operator compares it
// against the install sheet.
func TestResolveAllNamesTheDigestInThePullingAnnouncement(t *testing.T) {
	work := &recorder{}
	if _, err := resolveAll(context.Background(), work, work, oneRelocation(t), "linux/amd64", t.TempDir()); err != nil {
		t.Fatalf("resolveAll: %v", err)
	}
	found := false
	for _, announced := range work.lines {
		if announced.subject == "registry.test/acme/web@sha256:deadbeef" {
			found = true
		}
	}
	if !found {
		t.Errorf("no announcement names the digest: %v", work.lines)
	}
}
