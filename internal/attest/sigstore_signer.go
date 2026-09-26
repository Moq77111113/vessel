package attest

import (
	"context"
	"fmt"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"
)

// SigstoreServiceConfig bundles the trusted material a signer needs to reach Fulcio, Rekor and a timestamp authority.
type SigstoreServiceConfig struct {
	TrustedRoot   *root.TrustedRoot
	SigningConfig *root.SigningConfig
}

// SigstoreServiceConfigFactory builds a SigstoreServiceConfig, injected so tests can avoid the network.
type SigstoreServiceConfigFactory func() (*SigstoreServiceConfig, error)

// ProductionSigstoreServiceConfig fetches the production public-good trusted root and signing config from TUF.
func ProductionSigstoreServiceConfig() (*SigstoreServiceConfig, error) {
	trustedRoot, err := root.FetchTrustedRoot()
	if err != nil {
		return nil, fmt.Errorf("fetch Sigstore trusted root: %w", err)
	}
	signingConfig, err := root.FetchSigningConfig()
	if err != nil {
		return nil, fmt.Errorf("fetch Sigstore signing config: %w", err)
	}
	return &SigstoreServiceConfig{TrustedRoot: trustedRoot, SigningConfig: signingConfig}, nil
}

type sigstoreSigner struct {
	tokens  TokenSource
	config  SigstoreServiceConfigFactory
	timeout time.Duration
}

// NewSigstoreSigner signs artifacts through Fulcio keyless certificates, Rekor and a timestamp authority.
func NewSigstoreSigner(tokens TokenSource, config SigstoreServiceConfigFactory) ArtifactSigner {
	return &sigstoreSigner{tokens: tokens, config: config, timeout: 30 * time.Second}
}

// Sign requests a Fulcio certificate for an ephemeral keypair, signs data, and returns a standard Sigstore bundle.
func (s *sigstoreSigner) Sign(ctx context.Context, data []byte) ([]byte, error) {
	token, err := s.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtain OIDC token: %w", err)
	}

	cfg, err := s.config()
	if err != nil {
		return nil, fmt.Errorf("load Sigstore service configuration: %w", err)
	}

	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral keypair: %w", err)
	}

	opts, err := s.bundleOptions(ctx, cfg, token)
	if err != nil {
		return nil, err
	}

	bundle, err := sign.Bundle(&sign.PlainData{Data: data}, keypair, *opts)
	if err != nil {
		return nil, fmt.Errorf("build Sigstore bundle: %w", err)
	}

	bundleJSON, err := protojson.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshal Sigstore bundle: %w", err)
	}
	return bundleJSON, nil
}

// bundleOptions selects the Fulcio, Rekor and timestamp services from the signing config for this signing request.
func (s *sigstoreSigner) bundleOptions(ctx context.Context, cfg *SigstoreServiceConfig, token string) (*sign.BundleOptions, error) {
	fulcioService, err := root.SelectService(cfg.SigningConfig.FulcioCertificateAuthorityURLs(), sign.FulcioAPIVersions, time.Now())
	if err != nil {
		return nil, fmt.Errorf("select Fulcio service: %w", err)
	}

	tsaServices, err := root.SelectServices(cfg.SigningConfig.TimestampAuthorityURLs(),
		cfg.SigningConfig.TimestampAuthorityURLsConfig(), sign.TimestampAuthorityAPIVersions, time.Now())
	if err != nil {
		return nil, fmt.Errorf("select timestamp authority services: %w", err)
	}

	rekorServices, err := root.SelectServices(cfg.SigningConfig.RekorLogURLs(),
		cfg.SigningConfig.RekorLogURLsConfig(), sign.RekorAPIVersions, time.Now())
	if err != nil {
		return nil, fmt.Errorf("select Rekor services: %w", err)
	}

	opts := &sign.BundleOptions{
		Context:                    ctx,
		TrustedRoot:                cfg.TrustedRoot,
		CertificateProvider:        sign.NewFulcio(&sign.FulcioOptions{BaseURL: fulcioService.URL, Timeout: s.timeout, Retries: 1}),
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: token},
	}
	for _, tsaService := range tsaServices {
		opts.TimestampAuthorities = append(opts.TimestampAuthorities,
			sign.NewTimestampAuthority(&sign.TimestampAuthorityOptions{URL: tsaService.URL, Timeout: s.timeout, Retries: 1}))
	}
	for _, rekorService := range rekorServices {
		opts.TransparencyLogs = append(opts.TransparencyLogs,
			sign.NewRekor(&sign.RekorOptions{BaseURL: rekorService.URL, Timeout: s.timeout, Retries: 1, Version: rekorService.MajorAPIVersion}))
	}
	return opts, nil
}
