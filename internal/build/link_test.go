package build

import (
	"context"
	"slices"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

type line struct {
	verb    string
	subject string
}

type recorder struct {
	calls []string
	lines []line
}

func (r *recorder) Line(verb, subject string) {
	r.calls = append(r.calls, "line")
	r.lines = append(r.lines, line{verb: verb, subject: subject})
}

func (r *recorder) Resolve(context.Context, delivery.Ref, string) (string, error) {
	r.calls = append(r.calls, "resolve")
	return "sha256:deadbeef", nil
}

func (r *recorder) Image(context.Context, delivery.Ref) (v1.Image, error) {
	r.calls = append(r.calls, "image")
	return random.Image(64, 1)
}

func oneRelocation(t *testing.T) []delivery.Relocation {
	t.Helper()
	ref, err := delivery.ParseRef("registry.test/acme/web:1.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return []delivery.Relocation{{Ref: ref}}
}

func TestPullAnnouncesBeforeItResolvesAndBeforeItPulls(t *testing.T) {
	work := &recorder{}
	if _, err := (pull{client: work, work: work, platform: "linux/amd64", layout: t.TempDir()}).resolve(context.Background(), oneRelocation(t)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"line", "resolve", "line", "image"}
	if !slices.Equal(work.calls, want) {
		t.Errorf("got %v, want %v: an announcement before Resolve and another before the pull", work.calls, want)
	}
}

func TestPullNamesTheDigestInThePullingAnnouncement(t *testing.T) {
	work := &recorder{}
	if _, err := (pull{client: work, work: work, platform: "linux/amd64", layout: t.TempDir()}).resolve(context.Background(), oneRelocation(t)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	present := false
	for _, announced := range work.lines {
		if announced.subject == "registry.test/acme/web@sha256:deadbeef" {
			present = true
		}
	}
	if !present {
		t.Errorf("no announcement names the digest: %v", work.lines)
	}
}
