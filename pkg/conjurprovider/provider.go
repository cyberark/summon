// Package conjurprovider implements Summon's built-in Conjur secrets provider.
package conjurprovider

import (
	"errors"
	"fmt"
	"os"

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
	client, err := conjurapi.NewClientFromEnvironment(cfg, telemetryData)
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
