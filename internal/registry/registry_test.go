package registry

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// serveOneImage starts an in-memory registry holding acme/web:1.0 and returns its reference.
func serveOneImage(t *testing.T) descriptor.Ref {
	t.Helper()
	server := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(server.Close)

	host, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	image, err := random.Image(256, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	tag, err := name.NewTag(host.Host + "/acme/web:1.0")
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	if err := remote.Write(tag, image); err != nil {
		t.Fatalf("remote.Write: %v", err)
	}
	ref, err := descriptor.ParseRef(tag.String())
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return ref
}

func TestResolveReturnsADigestForATag(t *testing.T) {
	digest, err := New().Resolve(context.Background(), serveOneImage(t), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("got %q, want a sha256 digest", digest)
	}
}

func TestResolveIsStableAcrossTwoCalls(t *testing.T) {
	client, ref := New(), serveOneImage(t)
	first, err := client.Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	second, err := client.Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if first != second {
		t.Errorf("got %q then %q, want the same digest", first, second)
	}
}

func TestResolveNamesTheReferenceItCouldNotReach(t *testing.T) {
	ref, err := descriptor.ParseRef("127.0.0.1:1/acme/web:1.0")
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	_, err = New().Resolve(context.Background(), ref, "")
	if err == nil {
		t.Fatal("Resolve: want an error on an unreachable registry, got nil")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1/acme/web:1.0") {
		t.Errorf("error does not name the reference: %v", err)
	}
}

func TestResolveRejectsAnUnreadablePlatform(t *testing.T) {
	_, err := New().Resolve(context.Background(), serveOneImage(t), "not a platform")
	if !errors.Is(err, ErrPlatform) {
		t.Errorf("got %v, want ErrPlatform", err)
	}
}

func TestFetchWritesTheImageIntoTheLayout(t *testing.T) {
	client, ref := New(), serveOneImage(t)
	digest, err := client.Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dir := t.TempDir()
	if err := client.Fetch(context.Background(), ref.WithDigest(digest), dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Fatalf("no index.json in the layout: %v", err)
	}
}

func TestFetchAnnotatesTheManifestWithTheWholeReference(t *testing.T) {
	client, ref := New(), serveOneImage(t)
	digest, err := client.Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dir := t.TempDir()
	pinned := ref.WithDigest(digest)
	if err := client.Fetch(context.Background(), pinned, dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(index), pinned.String()) {
		t.Errorf("index.json does not carry %q: %s", pinned, index)
	}
}

func TestFetchingTwoImagesLeavesBothInOneLayout(t *testing.T) {
	client := New()
	dir := t.TempDir()
	for range 2 {
		ref := serveOneImage(t)
		digest, err := client.Resolve(context.Background(), ref, "")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if err := client.Fetch(context.Background(), ref.WithDigest(digest), dir); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := strings.Count(string(index), "org.opencontainers.image.ref.name"); got != 2 {
		t.Errorf("annotations: got %d, want 2", got)
	}
}
