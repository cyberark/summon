// Package conjurprovider implements Summon's built-in Conjur secrets provider.
package conjurprovider

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/cyberark/conjur-api-go/conjurapi/response"
)

var telemetryData = conjurapi.NewTelemetry("Summon", "idira-secretsmanager", "", "Idira", "")

// BuiltinProviderName is the canonical routing key for the built-in Conjur
// provider. When --provider=conjur (exact match) or when SUMMON_PROVIDER is
// unset/empty, provider routing dispatches to this package instead of an
// external binary.
const BuiltinProviderName = "conjur"

// BuiltinProviderPath is the identifier returned by ConjurProvider.Path and
// stored on SubprocessConfig.Provider for logging and built-in fetch dispatch.
const BuiltinProviderPath = "conjur (built-in)"

// ConjurClient is a test seam that wraps the conjurapi.Client. Declaring this
// interface allows unit tests to inject mocks so no real network calls are made.
type ConjurClient interface {
	RetrieveSecret(variableID string) ([]byte, error)
}

// ConjurProvider is the built-in Conjur secrets provider. It wraps a
// ConjurClient and retrieves secrets via the conjur-api-go library.
type ConjurProvider struct {
	client ConjurClient
}

// loadConjurConfig applies summon built-in provider defaults and loads Conjur
// configuration from the environment. Credential storage mode defaults to
// ReadOnly unless the operator sets CONJUR_CREDENTIAL_STORAGE_MODE.
func loadConjurConfig() (conjurapi.Config, error) {
	if err := validateNetrcPath(); err != nil {
		return conjurapi.Config{}, err
	}
	if err := validateAuthnTokenFile(context.Background()); err != nil {
		return conjurapi.Config{}, err
	}
	// conjur-api-go's WithDefaultCredentialStorageMode is a package-level setter
	// (no return value), not a functional option. LoadConfig reads the default
	// when CONJUR_CREDENTIAL_STORAGE_MODE is unset.
	conjurapi.WithDefaultCredentialStorageMode(conjurapi.CredentialStorageModeReadOnly)
	return conjurapi.LoadConfig()
}

// validateNetrcPath checks CONJUR_NETRC_PATH when set and returns a
// configuration error if the path does not refer to a regular file.
// This prevents conjur-api-go from emitting a misleading OIDC error
// when the operator points the variable at a directory by mistake.
func validateNetrcPath() error {
	path := os.Getenv("CONJUR_NETRC_PATH")
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("configuration error: CONJUR_NETRC_PATH is not accessible: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("configuration error: CONJUR_NETRC_PATH is not a regular file: %s", path)
	}
	return nil
}

// Token-file wait settings. conjur-api-go waits for a missing token file with
// no time limit (it polls every 100ms, MaxWaitTime -1), which hung summon
// forever on a bad path. Summon therefore waits itself, for a bounded time,
// before the client is built.
const (
	authnTokenFileWaitEnv     = "SUMMON_AUTHN_TOKEN_FILE_WAIT"
	defaultAuthnTokenFileWait = 30 * time.Second
)

// tokenFilePollInterval is how often the token file is polled; a variable so
// tests can shorten it.
var tokenFilePollInterval = 100 * time.Millisecond

// authnTokenFileWait parses SUMMON_AUTHN_TOKEN_FILE_WAIT (a Go duration; 0
// disables waiting). Unset or empty yields the default.
func authnTokenFileWait() (time.Duration, error) {
	v := os.Getenv(authnTokenFileWaitEnv)
	if v == "" {
		return defaultAuthnTokenFileWait, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("configuration error: %s must be a non-negative duration such as 30s, got %q", authnTokenFileWaitEnv, v)
	}
	return d, nil
}

// validateAuthnTokenFile checks CONJUR_AUTHN_TOKEN_FILE when set. A path that
// does not exist yet is polled for up to SUMMON_AUTHN_TOKEN_FILE_WAIT (a
// sidecar may create it shortly after start); a path that is not a readable
// regular file fails immediately. It runs once, when the provider is
// constructed; later rotation of the file is left to the library.
func validateAuthnTokenFile(ctx context.Context) error {
	path := os.Getenv("CONJUR_AUTHN_TOKEN_FILE")
	if path == "" {
		return nil
	}
	wait, err := authnTokenFileWait()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(wait)
	var info os.FileInfo
	for {
		info, err = os.Stat(path)
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("configuration error: CONJUR_AUTHN_TOKEN_FILE %s does not exist after waiting %s (see %s)", path, wait, authnTokenFileWaitEnv)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("configuration error: waiting for CONJUR_AUTHN_TOKEN_FILE %s: %w", path, ctx.Err())
		case <-time.After(min(tokenFilePollInterval, time.Until(deadline))):
		}
	}
	if err != nil {
		return fmt.Errorf("configuration error: CONJUR_AUTHN_TOKEN_FILE is not accessible: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("configuration error: CONJUR_AUTHN_TOKEN_FILE is not a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("configuration error: CONJUR_AUTHN_TOKEN_FILE is not readable: %w", err)
	}
	return f.Close()
}

// New loads Conjur configuration from the environment and creates a
// ConjurProvider. Configuration loading, validation, and auth setup are
// handled by conjur-api-go.
func New() (*ConjurProvider, error) {
	cfg, err := loadConjurConfig()
	if err != nil {
		return nil, fmt.Errorf("configuration error: failed to load Conjur configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	// When LoadConfig auto-wired the SPIFFE Workload API as the cert source
	// (SPIFFE_ENDPOINT_SOCKET is set, AuthnType=cert, no static cert files),
	// cfg.ClientCertProvider is non-nil. NewClientFromCertificate accepts a
	// dynamic provider function; NewClientFromEnvironment does not.
	var client *conjurapi.Client
	if cfg.ClientCertProvider != nil {
		client, err = conjurapi.NewClientFromCertificate(cfg, telemetryData)
	} else {
		client, err = conjurapi.NewClientFromEnvironment(cfg, telemetryData)
	}
	if err != nil {
		// Security: propagating err.Error() here does not risk leaking secret
		// material. conjur-api-go validates config before any auth attempt,
		// producing static messages (e.g. "Must specify an ApplianceURL"). If
		// auth itself fails, ConjurError.Error() returns only the HTTP status
		// line (e.g. "Unauthorized") — secret values appear exclusively in
		// 2xx success response bodies handled by SecretDataResponse, not in
		// error types. See conjurapi/response/error.go for detail.
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return &ConjurProvider{client: client}, nil
}

// NewWithClient creates a ConjurProvider with an injected ConjurClient. This
// constructor is intended for use in tests where a mock client is needed.
func NewWithClient(client ConjurClient) *ConjurProvider {
	return &ConjurProvider{client: client}
}

// FetchSecret fetches a Conjur variable by ID. A missing variable is treated
// as a retrieval failure; an empty value is returned as-is (matching the
// behavior of the standalone summon-conjur binary).
func (p *ConjurProvider) FetchSecret(varID string) ([]byte, error) {
	value, err := p.client.RetrieveSecret(varID)
	if err != nil {
		var conjurErr *response.ConjurError
		if errors.As(err, &conjurErr) && conjurErr.Code == 404 {
			// CONJ00036E is Conjur's canonical "authenticator webservice not found
			// in policy" code. Matching on the code rather than Details != nil
			// prevents future Conjur versions that return structured bodies for
			// variable-not-found 404s from being misclassified as auth failures.
			if conjurErr.Details != nil && conjurErr.Details.Code == "CONJ00036E" {
				return nil, fmt.Errorf("authentication failed: authenticator not found or not enabled — check Conjur policy: %w", err)
			}
			return nil, fmt.Errorf("secret retrieval failed: variable '%s' not found or inaccessible", varID)
		}
		// Security: same rationale as New() — ConjurError.Error() returns only
		// the HTTP status line from the error response body, which never
		// contains secret material.
		return nil, fmt.Errorf("secret retrieval failed: %w", err)
	}

	return value, nil
}

// Path returns the built-in Conjur provider identifier (BuiltinProviderPath).
// It satisfies the provider.Provider interface and is used for logging and for
// dispatch in the summon package (fetchSecrets compares Provider to
// BuiltinProviderPath).
func (p *ConjurProvider) Path() string {
	return BuiltinProviderPath
}
