package registry

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// access says whether a fixture registry demands credentials.
type access int

const (
	openAccess access = iota
	privateAccess
)

// payload says whether a fixture registry serves a single image or a multi-platform index.
type payload int

const (
	singleImage payload = iota
	multiPlatformIndex
)

// serveRegistry starts an in-memory registry holding acme/web:1.0 and returns its reference.
// A privateAccess registry answers 401 until it is given user:password, and points DOCKER_CONFIG
// at those credentials. A multiPlatformIndex payload serves an index over one linux/amd64 image
// rather than the image directly.
func serveRegistry(t *testing.T, access access, payload payload) descriptor.Ref {
	t.Helper()
	const user, password = "user", "password"
	header := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))

	open := ggcrregistry.New()
	handler := http.Handler(open)
	if access == privateAccess {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != header {
				w.Header().Set("WWW-Authenticate", `Basic realm="vessel"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			open.ServeHTTP(w, r)
		})
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	host, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var writeOptions []remote.Option
	if access == privateAccess {
		dir := t.TempDir()
		config := fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`,
			host.Host, base64.StdEncoding.EncodeToString([]byte(user+":"+password)))
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		t.Setenv("DOCKER_CONFIG", dir)
		writeOptions = append(writeOptions, remote.WithAuth(&authn.Basic{Username: user, Password: password}))
	}

	tag, err := name.NewTag(host.Host + "/acme/web:1.0")
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	if payload == multiPlatformIndex {
		index, err := random.Index(256, 1, 1)
		if err != nil {
			t.Fatalf("random.Index: %v", err)
		}
		if err := remote.WriteIndex(tag, index, writeOptions...); err != nil {
			t.Fatalf("remote.WriteIndex: %v", err)
		}
	} else {
		image, err := random.Image(256, 1)
		if err != nil {
			t.Fatalf("random.Image: %v", err)
		}
		if err := remote.Write(tag, image, writeOptions...); err != nil {
			t.Fatalf("remote.Write: %v", err)
		}
	}

	ref, err := descriptor.ParseRef(tag.String())
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return ref
}

func TestResolveReturnsADigestForATag(t *testing.T) {
	ref := serveRegistry(t, openAccess, singleImage)
	digest, err := New().Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("got %q, want a sha256 digest", digest)
	}
}

func TestResolveReadsARegistryThatDemandsCredentials(t *testing.T) {
	ref := serveRegistry(t, privateAccess, singleImage)
	digest, err := New().Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("got %q, want a sha256 digest", digest)
	}
}

func TestImageReadsARegistryThatDemandsCredentials(t *testing.T) {
	client, ref := New(), serveRegistry(t, privateAccess, singleImage)
	digest, err := client.Resolve(context.Background(), ref, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := client.Image(context.Background(), ref.WithDigest(digest)); err != nil {
		t.Fatalf("Image: %v", err)
	}
}

// The seam the invariant rests on: Resolve names a manifest, Image hands back that same
// manifest, and WriteLayout stores it under the digest Resolve returned. It has to hold whether
// the reference names a single image or an index over several platforms: DMAS shipped an index
// pinned by digest, and it broke on install because the two halves disagreed on which manifest
// the digest named.
func TestImageReturnsTheManifestResolveNamed(t *testing.T) {
	cases := []struct {
		name    string
		payload payload
	}{
		{"a single-platform image", singleImage},
		{"a multi-platform index", multiPlatformIndex},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, ref := New(), serveRegistry(t, openAccess, c.payload)
			digest, err := client.Resolve(context.Background(), ref, "linux/amd64")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			image, err := client.Image(context.Background(), ref.WithDigest(digest))
			if err != nil {
				t.Fatalf("Image: %v", err)
			}
			pulled, err := image.Digest()
			if err != nil {
				t.Fatalf("Digest: %v", err)
			}
			if pulled.String() != digest {
				t.Errorf("Image returned %s, Resolve returned %s", pulled, digest)
			}
		})
	}
}

func TestResolveIsStableAcrossTwoCalls(t *testing.T) {
	client, ref := New(), serveRegistry(t, openAccess, singleImage)
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
	ref := serveRegistry(t, openAccess, singleImage)
	_, err := New().Resolve(context.Background(), ref, "not a platform")
	if !errors.Is(err, ErrPlatform) {
		t.Errorf("got %v, want ErrPlatform", err)
	}
}
