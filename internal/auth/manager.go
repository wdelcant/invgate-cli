package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Manager provides authentication operations for the CLI.
type Manager interface {
	// Token returns a valid access token (cached or freshly fetched).
	Token(ctx context.Context) (string, error)
	// Login forces a fresh token acquisition, overwriting any cached one.
	Login(ctx context.Context) error
	// Logout clears all keychain entries for this service.
	Logout() error
}

// cachedToken is the JSON shape persisted in the keychain under KeyAccessToken.
type cachedToken struct {
	Token  string    `json:"token"`
	Expiry time.Time `json:"expiry"`
}

// KeychainManager implements Manager using the keychain for credential
// and token caching, and client-credentials OAuth2 for token fetching.
type KeychainManager struct {
	Resolver   *CredentialResolver
	TokenURL   string
	Scopes     []string
	HTTPClient *http.Client
	Keyring    Keyring
}

// NewKeychainManager constructs a manager with sensible defaults.
func NewKeychainManager(resolver *CredentialResolver, tokenURL string, kr Keyring) *KeychainManager {
	return &KeychainManager{
		Resolver:   resolver,
		TokenURL:   tokenURL,
		Scopes:     []string{"write"},
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Keyring:    kr,
	}
}

// Token returns a cached token if still valid (with a 60-second buffer),
// otherwise fetches a new one via the client-credentials grant. Caching
// to the keychain is best-effort: when the keyring is unavailable
// (CI/Docker), the freshly-fetched token is returned directly so that
// env-only credential mode still works.
func (m *KeychainManager) Token(ctx context.Context) (string, error) {
	if cached, ok := m.readCachedToken(); ok {
		if cached.Expiry.Sub(time.Now()) > 60*time.Second {
			return cached.Token, nil
		}
	}
	return m.fetchAndCache(ctx)
}

// Login forces a fresh token, ignoring any cached value.
func (m *KeychainManager) Login(ctx context.Context) error {
	// Clear existing cached token.
	if m.Keyring != nil {
		_ = m.Keyring.Delete(ServiceName, KeyAccessToken)
	}
	_, err := m.fetchAndCache(ctx)
	return err
}

// Logout removes all keychain entries for the service.
func (m *KeychainManager) Logout() error {
	if m.Keyring == nil {
		return nil
	}
	if err := m.Keyring.Delete(ServiceName, KeyAccessToken); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("could not delete access token: %w", err)
	}
	if err := m.Keyring.Delete(ServiceName, KeyClientID); err != nil && !errors.Is(err, ErrNotFound) {
		// non-fatal
		_ = err
	}
	if err := m.Keyring.Delete(ServiceName, KeyClientSecret); err != nil && !errors.Is(err, ErrNotFound) {
		_ = err
	}
	return nil
}

// fetchAndCache performs the client-credentials token request, persists
// the token + expiry to the keychain (best-effort), and returns the
// token. Keyring write failures are ignored so env-only mode works.
func (m *KeychainManager) fetchAndCache(ctx context.Context) (string, error) {
	clientID, clientSecret, err := m.Resolver.Resolve()
	if err != nil {
		return "", err
	}
	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     m.TokenURL,
		Scopes:       m.Scopes,
	}
	tokCtx := ctx
	if m.HTTPClient != nil {
		tokCtx = context.WithValue(ctx, oauth2.HTTPClient, m.HTTPClient)
	}
	tok, err := cfg.Token(tokCtx)
	if err != nil {
		return "", fmt.Errorf("could not fetch token: %w", err)
	}
	ct := cachedToken{Token: tok.AccessToken, Expiry: tok.Expiry}
	_ = m.writeCachedToken(ct)
	return ct.Token, nil
}

// readCachedToken reads and parses the cached token from the keychain.
func (m *KeychainManager) readCachedToken() (cachedToken, bool) {
	if m.Keyring == nil {
		return cachedToken{}, false
	}
	raw, err := m.Keyring.Get(ServiceName, KeyAccessToken)
	if err != nil || raw == "" {
		return cachedToken{}, false
	}
	var ct cachedToken
	if err := json.Unmarshal([]byte(raw), &ct); err != nil {
		return cachedToken{}, false
	}
	if ct.Token == "" {
		return cachedToken{}, false
	}
	return ct, true
}

// writeCachedToken persists the token to the keychain as JSON.
func (m *KeychainManager) writeCachedToken(ct cachedToken) error {
	if m.Keyring == nil {
		return nil
	}
	data, err := json.Marshal(ct)
	if err != nil {
		return fmt.Errorf("could not encode cached token: %w", err)
	}
	if err := m.Keyring.Set(ServiceName, KeyAccessToken, string(data)); err != nil {
		return fmt.Errorf("could not cache token: %w", err)
	}
	return nil
}

// TokenSource wraps the Manager as an oauth2.TokenSource, so the
// HTTP client's transport can auto-refresh on 401 if desired.
type TokenSource struct {
	Manager Manager
}

// Token implements oauth2.TokenSource.
func (ts *TokenSource) Token() (*oauth2.Token, error) {
	ctx := context.Background()
	tokStr, err := ts.Manager.Token(ctx)
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{AccessToken: tokStr}, nil
}