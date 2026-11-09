package quadlet_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

type creds struct {
	lines []string
	stdin []string
}

func (c *creds) run(_ context.Context, command quadlet.Command) ([]byte, error) {
	c.lines = append(c.lines, strings.Join(command.Line, " "))
	if command.Stdin != nil {
		body, _ := io.ReadAll(command.Stdin)
		c.stdin = append(c.stdin, string(body))
	}
	return nil, nil
}

func rootWithUnit(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/keycloak.container")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return root
}

func TestAddSecretEncryptsASecretAUnitLoadsAsACredential(t *testing.T) {
	root := rootWithUnit(t, "[Service]\nLoadCredentialEncrypted=KC_DB_PASSWORD\n")
	system := &creds{}
	if err := quadlet.New(root, system.run).AddSecret(context.Background(), delivery.Secret{Name: "KC_DB_PASSWORD", Value: "a3f9"}); err != nil {
		t.Fatalf("AddSecret: %v", err)
	}
	want := "systemd-creds encrypt --name=KC_DB_PASSWORD - " + filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD")
	if !slices.Equal(system.lines, []string{want}) {
		t.Errorf("got %v, want %q", system.lines, want)
	}
	if !slices.Equal(system.stdin, []string{"a3f9"}) {
		t.Errorf("got %q on stdin, want the value alone", system.stdin)
	}
}

func TestAddSecretKeepsPodmanForASecretAUnitMounts(t *testing.T) {
	root := rootWithUnit(t, "[Container]\nSecret=DB_PASSWORD,type=env\n")
	system := &creds{}
	if err := quadlet.New(root, system.run).AddSecret(context.Background(), delivery.Secret{Name: "DB_PASSWORD", Value: "a3f9"}); err != nil {
		t.Fatalf("AddSecret: %v", err)
	}
	if len(system.lines) != 1 || !strings.HasPrefix(system.lines[0], "podman secret create DB_PASSWORD") {
		t.Errorf("got %v, want a podman secret", system.lines)
	}
}

func TestAddSecretSealsACertificateWithItsLineBreaks(t *testing.T) {
	root := rootWithUnit(t, "[Service]\nLoadCredentialEncrypted=TLS_CERT\n")
	system := &creds{}
	certificate := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	if err := quadlet.New(root, system.run).AddSecret(context.Background(), delivery.Secret{Name: "TLS_CERT", Value: certificate}); err != nil {
		t.Fatalf("AddSecret: %v", err)
	}
	if !slices.Equal(system.stdin, []string{certificate}) {
		t.Errorf("got %q on stdin, want the certificate byte for byte", system.stdin)
	}
}

func TestSecretsNamesACredentialTheMachineHolds(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("sealed"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	secrets, err := quadlet.New(root, (&creds{}).run).Secrets(context.Background())
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if !slices.Contains(secrets, "KC_DB_PASSWORD") {
		t.Errorf("got %v, want the credential named", secrets)
	}
}

func TestRemoveHintNamesTheCredentialFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/credstore.encrypted/KC_DB_PASSWORD")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("sealed"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, want := quadlet.New(root, (&creds{}).run).RemoveHint("KC_DB_PASSWORD"), "rm "+path; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
