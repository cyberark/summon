// Package conjurprovider implements Summon's built-in Conjur secrets provider.
package conjurprovider

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// Environment variables read when validating the Conjur provider configuration.
const (
	envSpiffeEndpointSocket = "SPIFFE_ENDPOINT_SOCKET"
	envJWTServiceID         = "CONJUR_AUTHN_JWT_SERVICE_ID"
	envCertServiceID        = "CONJUR_AUTHN_CERT_SERVICE_ID"
	envAuthnJWTToken        = "CONJUR_AUTHN_JWT_TOKEN"
	envJWTTokenPath         = "JWT_TOKEN_PATH"
)

// ConjurClient is a test seam that wraps the conjurapi.Client. Declaring this
// interface allows unit tests to inject mocks so no real network calls are made.
type ConjurClient interface {
	RetrieveSecret(variableID string) ([]byte, error)
}

// requestClient is the part of *conjurapi.Client that lets a retrieval be
// bound to a context: the library has no context-taking RetrieveSecret, but it
// builds the request itself and its SubmitRequest honours the request's
// context. Test doubles that only implement ConjurClient take the fallback.
type requestClient interface {
	RetrieveSecretRequest(variableID string) (*http.Request, error)
	SubmitRequest(req *http.Request) (*http.Response, error)
}

// ConjurProvider is the built-in Conjur secrets provider. It wraps a
// ConjurClient and retrieves secrets via the conjur-api-go library.
type ConjurProvider struct {
	client ConjurClient
	// tokenFile is CONJUR_AUTHN_TOKEN_FILE as seen at construction: daemon
	// mode clears CONJUR_* from the process environment afterwards.
	tokenFile string
}

// loadConjurConfig applies summon built-in provider defaults and loads Conjur
// configuration from the environment. Credential storage mode defaults to
// ReadOnly unless the operator sets CONJUR_CREDENTIAL_STORAGE_MODE.
func loadConjurConfig(maxHTTPTimeout time.Duration) (conjurapi.Config, error) {
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
	cfg, err := conjurapi.LoadConfig()
	if err != nil {
		return cfg, err
	}
	capHTTPTimeout(&cfg, maxHTTPTimeout)
	return cfg, nil
}

// capHTTPTimeout lowers cfg.HTTPTimeout (whole seconds) to limit, rounded up,
// when limit is positive and shorter than the effective timeout. It never
// raises a timeout the operator configured.
func capHTTPTimeout(cfg *conjurapi.Config, limit time.Duration) {
	if limit <= 0 {
		return
	}
	secs := int((limit + time.Second - 1) / time.Second)
	if secs < cfg.GetHttpTimeout() {
		cfg.HTTPTimeout = secs
	}
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
	path = filepath.Clean(path)
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

// validateSpiffeConfig validates the SPIFFE Workload API settings before any
// network call so the operator gets an error that names the variable at fault:
//
//   - SPIFFE_ENDPOINT_SOCKET set but empty while JWT-SVID auth is configured
//     (CONJUR_AUTHN_JWT_SERVICE_ID with no static token): without this check the
//     library reports "Must specify a JWT token", which never mentions the socket.
//   - SPIFFE_ENDPOINT_SOCKET set but no authenticator service ID: the SVID cannot
//     be targeted at any Conjur authenticator.
//
// It only reads the environment; see normalizeSpiffeEndpointEnv for the rewrite.
func validateSpiffeConfig() error {
	socket, set := os.LookupEnv(envSpiffeEndpointSocket)
	if socket == "" {
		if set && os.Getenv(envJWTServiceID) != "" &&
			os.Getenv(envAuthnJWTToken) == "" &&
			os.Getenv(envJWTTokenPath) == "" {
			return fmt.Errorf(
				"configuration error: SPIFFE_ENDPOINT_SOCKET is set but empty — " +
					"set it to the SPIFFE Workload API socket (for example unix:///run/spire/agent.sock) " +
					"or unset it and provide CONJUR_AUTHN_JWT_TOKEN or JWT_TOKEN_PATH",
			)
		}
		return nil
	}
	if os.Getenv(envJWTServiceID) == "" &&
		os.Getenv(envCertServiceID) == "" {
		return fmt.Errorf(
			"configuration error: SPIFFE_ENDPOINT_SOCKET is set but no authenticator service ID is configured — " +
				"set CONJUR_AUTHN_JWT_SERVICE_ID (for JWT-SVID) or CONJUR_AUTHN_CERT_SERVICE_ID (for X.509-SVID)",
		)
	}
	return nil
}

// normalizeSpiffeEndpointEnv rewrites SPIFFE_ENDPOINT_SOCKET in the process
// environment when it is a bare absolute path (no scheme): conjur-api-go accepts
// it but go-spiffe rejects it at connect time, so it becomes unix://<path>. A
// bare relative path is a configuration error. An unset or empty value is left
// alone.
func normalizeSpiffeEndpointEnv() error {
	socket := os.Getenv(envSpiffeEndpointSocket)
	if socket == "" {
		return nil
	}
	normalized, err := normalizeSpiffeSocket(socket)
	if err != nil {
		return err
	}
	if normalized == socket {
		return nil
	}
	return os.Setenv(envSpiffeEndpointSocket, normalized)
}

// normalizeSpiffeSocket converts a bare absolute socket path to a unix:// URL.
// Values that already carry a scheme are returned unchanged for conjur-api-go
// to validate.
func normalizeSpiffeSocket(socket string) (string, error) {
	if strings.Contains(socket, "://") {
		return socket, nil
	}
	if !strings.HasPrefix(socket, "/") {
		return "", fmt.Errorf(
			"configuration error: SPIFFE_ENDPOINT_SOCKET %q has no scheme and is not an absolute path — "+
				"use unix:///absolute/path or an absolute socket path", socket)
	}
	return "unix://" + socket, nil
}

// validateCertFiles returns a configuration error when CONJUR_AUTHN_CERT_SERVICE_ID
// is set for static file-based cert auth (i.e. SPIFFE_ENDPOINT_SOCKET is absent)
// but the configured cert or key file does not exist on disk. This check runs
// before any network call so the operator receives a file-system error rather
// than a Conjur API rejection.
func validateCertFiles() error {
	if os.Getenv(envCertServiceID) == "" {
		return nil
	}
	// SPIFFE socket provides the cert dynamically; skip static file validation.
	if os.Getenv(envSpiffeEndpointSocket) != "" {
		return nil
	}
	certFile := os.Getenv("CONJUR_AUTHN_CERT_FILE")
	keyFile := os.Getenv("CONJUR_AUTHN_CERT_KEY_FILE")
	if certFile == "" {
		return fmt.Errorf("configuration error: CONJUR_AUTHN_CERT_SERVICE_ID is set but CONJUR_AUTHN_CERT_FILE is not configured")
	}
	if keyFile == "" {
		return fmt.Errorf("configuration error: CONJUR_AUTHN_CERT_SERVICE_ID is set but CONJUR_AUTHN_CERT_KEY_FILE is not configured")
	}
	if _, err := os.Stat(certFile); err != nil {
		return fmt.Errorf("configuration error: cannot read certificate file %s: %w", certFile, err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return fmt.Errorf("configuration error: cannot read key file %s: %w", keyFile, err)
	}
	return nil
}

// New loads Conjur configuration from the environment and creates a
// ConjurProvider. Configuration loading, validation, and auth setup are
// handled by conjur-api-go.
//
// The client's HTTP timeout is capped at maxHTTPTimeout (see capHTTPTimeout);
// zero leaves it as configured. Daemon mode passes SUMMON_REFRESH_TIMEOUT so
// the library's token refresh, which cannot be cancelled through a context, is
// still bounded by it.
func New(maxHTTPTimeout time.Duration) (*ConjurProvider, error) {
	if err := validateSpiffeConfig(); err != nil {
		return nil, err
	}
	if err := normalizeSpiffeEndpointEnv(); err != nil {
		return nil, err
	}
	if err := validateCertFiles(); err != nil {
		return nil, err
	}
	cfg, err := loadConjurConfig(maxHTTPTimeout)
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

	return &ConjurProvider{client: client, tokenFile: os.Getenv("CONJUR_AUTHN_TOKEN_FILE")}, nil
}

// NewWithClient creates a ConjurProvider with an injected ConjurClient. This
// constructor is intended for use in tests where a mock client is needed.
func NewWithClient(client ConjurClient) *ConjurProvider {
	return &ConjurProvider{client: client}
}

// FetchSecret fetches a Conjur variable by ID. A missing variable is treated
// as a retrieval failure; an empty value is returned as-is (matching the
// behavior of the standalone summon-conjur binary).
//
// With the real client the secret request carries ctx, so cancelling it aborts
// the connection, the wait for response headers and the body read. What ctx
// cannot reach is the library's token refresh (authentication) that may precede
// the request; it is bounded by the client's HTTP timeout instead (see New).
func (p *ConjurProvider) FetchSecret(ctx context.Context, varID string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.checkTokenFile(); err != nil {
		return nil, err
	}
	rc, ok := p.client.(requestClient)
	if !ok {
		value, err := p.client.RetrieveSecret(varID)
		return value, retrievalError(varID, err)
	}
	req, err := rc.RetrieveSecretRequest(varID)
	if err != nil {
		return nil, retrievalError(varID, err)
	}
	resp, err := rc.SubmitRequest(req.WithContext(ctx))
	if err != nil {
		return nil, ctxOr(ctx, retrievalError(varID, err))
	}
	value, err := response.DataResponse(resp)
	return value, ctxOr(ctx, retrievalError(varID, err))
}

// checkTokenFile fails when a configured CONJUR_AUTHN_TOKEN_FILE is no longer a
// readable regular file. The library stats the file on every retrieval and
// dereferences the result unchecked, so a file that vanished after the
// provider was built (or never appeared) must be reported here instead of
// reaching it. The next retrieval succeeds again once the file is back.
func (p *ConjurProvider) checkTokenFile() error {
	if p.tokenFile == "" {
		return nil
	}
	tokenFile := filepath.Clean(p.tokenFile)
	info, err := os.Stat(tokenFile)
	if err != nil {
		return fmt.Errorf("authentication token file unavailable: CONJUR_AUTHN_TOKEN_FILE is not accessible: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("authentication token file unavailable: CONJUR_AUTHN_TOKEN_FILE is not a regular file: %s", tokenFile)
	}
	f, err := os.Open(tokenFile)
	if err != nil {
		return fmt.Errorf("authentication token file unavailable: CONJUR_AUTHN_TOKEN_FILE is not readable: %w", err)
	}
	return f.Close()
}

// ctxOr reports ctx's error when it ended, so callers see a cancellation or
// deadline rather than the transport's wording; otherwise err.
func ctxOr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("secret retrieval failed: %w", ctx.Err())
	}
	return err
}

// retrievalError maps a retrieval error from the library to Summon's wording;
// nil stays nil.
func retrievalError(varID string, err error) error {
	if err == nil {
		return nil
	}
	var conjurErr *response.ConjurError
	if errors.As(err, &conjurErr) && conjurErr.Code == 404 {
		// CONJ00036E is Conjur's canonical "authenticator webservice not found
		// in policy" code. Matching on the code rather than Details != nil
		// prevents future Conjur versions that return structured bodies for
		// variable-not-found 404s from being misclassified as auth failures.
		if conjurErr.Details != nil && conjurErr.Details.Code == "CONJ00036E" {
			return fmt.Errorf("authentication failed: authenticator not found or not enabled — check Conjur policy: %w", err)
		}
		return fmt.Errorf("secret retrieval failed: variable '%s' not found or inaccessible", varID)
	}
	// Security: same rationale as New() — ConjurError.Error() returns only
	// the HTTP status line from the error response body, which never
	// contains secret material.
	return fmt.Errorf("secret retrieval failed: %w", err)
}

// IsUnauthorized reports whether err carries a Conjur HTTP 401 response, i.e.
// the server rejected the credentials. 403 (not permitted), 404 and transport
// errors are not authentication failures.
func IsUnauthorized(err error) bool {
	var conjurErr *response.ConjurError
	return errors.As(err, &conjurErr) && conjurErr.Code == 401
}

// Path returns the built-in Conjur provider identifier (BuiltinProviderPath).
// It satisfies the provider.Provider interface and is used for logging and for
// dispatch in the summon package (fetchSecrets compares Provider to
// BuiltinProviderPath).
func (p *ConjurProvider) Path() string {
	return BuiltinProviderPath
}
