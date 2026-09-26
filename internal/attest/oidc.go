package attest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ErrNoOIDCToken is returned when no OIDC token can be obtained.
var ErrNoOIDCToken = errors.New("no OIDC token available")

// TokenSource obtains OIDC tokens for Sigstore.
type TokenSource interface {
	Token(context.Context) (string, error)
}

type oidcTokenSource struct {
	lookup func(string) string
	client *http.Client
}

// NewOIDCTokenSource creates a token source reading env via lookup; a nil client means http.DefaultClient.
func NewOIDCTokenSource(lookup func(string) string, client *http.Client) TokenSource {
	if client == nil {
		client = http.DefaultClient
	}
	return &oidcTokenSource{lookup: lookup, client: client}
}

func githubRequestURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse OIDC request URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", "sigstore")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Token returns the CI OIDC token, trying GitLab's VESSEL_SIGSTORE_ID_TOKEN before the GitHub Actions endpoint.
func (s *oidcTokenSource) Token(ctx context.Context) (string, error) {
	if token := s.lookup("VESSEL_SIGSTORE_ID_TOKEN"); token != "" {
		return token, nil
	}

	rawURL := s.lookup("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := s.lookup("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if rawURL == "" || requestToken == "" {
		return "", ErrNoOIDCToken
	}

	reqURL, err := githubRequestURL(rawURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OIDC response: %w", err)
	}

	var data struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("failed to decode OIDC response: %w", err)
	}

	return data.Value, nil
}
