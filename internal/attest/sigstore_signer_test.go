package attest

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeTokenSource struct {
	token string
	err   error
}

func (f fakeTokenSource) Token(context.Context) (string, error) {
	return f.token, f.err
}

func TestSigstoreSignerFailsWhenNoTokenIsAvailable(t *testing.T) {
	tokenErr := errors.New("no token")
	factoryCalled := false
	signer := NewSigstoreSigner(fakeTokenSource{err: tokenErr}, func() (*SigstoreServiceConfig, error) {
		factoryCalled = true
		return nil, nil
	})

	_, err := signer.Sign(context.Background(), []byte("data"))
	if !errors.Is(err, tokenErr) {
		t.Fatalf("got %v, want %v", err, tokenErr)
	}
	if factoryCalled {
		t.Error("service config factory called despite missing token")
	}
}

func TestSigstoreSignerFailsWhenServiceConfigFails(t *testing.T) {
	configErr := errors.New("no network")
	signer := NewSigstoreSigner(fakeTokenSource{token: "secret-token"}, func() (*SigstoreServiceConfig, error) {
		return nil, configErr
	})

	_, err := signer.Sign(context.Background(), []byte("data"))
	if !errors.Is(err, configErr) {
		t.Fatalf("got %v, want %v", err, configErr)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Errorf("error leaks the OIDC token: %v", err)
	}
}
