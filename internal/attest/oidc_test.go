package attest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOIDCTokenUsesGitLabToken(t *testing.T) {
	source := NewOIDCTokenSource(func(k string) string {
		if k == "VESSEL_SIGSTORE_ID_TOKEN" {
			return "token"
		}
		return ""
	}, nil)
	got, err := source.Token(context.Background())
	if err != nil || got != "token" {
		t.Fatalf("Token: %q, %v", got, err)
	}
}

func TestOIDCTokenUsesGitHubActionsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer request-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": "github-token"})
	}))
	defer server.Close()

	source := NewOIDCTokenSource(func(k string) string {
		switch k {
		case "VESSEL_SIGSTORE_ID_TOKEN":
			return ""
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return server.URL
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "request-token"
		}
		return ""
	}, nil)
	got, err := source.Token(context.Background())
	if err != nil || got != "github-token" {
		t.Fatalf("Token: %q, %v", got, err)
	}
}

func TestOIDCTokenReturnsMissingTokenError(t *testing.T) {
	source := NewOIDCTokenSource(func(k string) string { return "" }, nil)
	_, err := source.Token(context.Background())
	if !errors.Is(err, ErrNoOIDCToken) {
		t.Errorf("got %v, want ErrNoOIDCToken", err)
	}
}

func TestOIDCTokenWithPreexistingQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer request-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Check that both query parameters are present
		if r.URL.Query().Get("audience") != "sigstore" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing or wrong audience"))
			return
		}
		if r.URL.Query().Get("api-version") != "2.0" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing or wrong api-version"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": "github-token"})
	}))
	defer server.Close()

	source := NewOIDCTokenSource(func(k string) string {
		switch k {
		case "VESSEL_SIGSTORE_ID_TOKEN":
			return ""
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return server.URL + "?api-version=2.0"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "request-token"
		}
		return ""
	}, nil)
	got, err := source.Token(context.Background())
	if err != nil || got != "github-token" {
		t.Fatalf("Token: %q, %v", got, err)
	}
}

func TestOIDCTokenErrorContainsNoCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	source := NewOIDCTokenSource(func(k string) string {
		switch k {
		case "VESSEL_SIGSTORE_ID_TOKEN":
			return ""
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return server.URL
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "secret-request-token"
		}
		return ""
	}, nil)
	_, err := source.Token(context.Background())

	if err == nil {
		t.Fatal("expected error")
	}
	errText := err.Error()
	if strings.Contains(errText, "secret-request-token") {
		t.Errorf("error contains request token: %v", err)
	}
	if strings.Contains(errText, server.URL) {
		t.Errorf("error contains URL: %v", err)
	}
}
