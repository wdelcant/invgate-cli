package auth

import (
	"fmt"
	"os"

	"github.com/wdelcant/invgate-cli/internal/errors"
)

// CredentialResolver reads credentials in priority order:
// 1. Explicit flag values (set via SetClientID/SetClientSecret)
// 2. Environment variables INVGATE_CLIENT_ID / INVGATE_CLIENT_SECRET
// 3. OS keychain entries client-id / client-secret
type CredentialResolver struct {
	ClientIDFlag     string
	ClientSecretFlag string
	Keyring          Keyring
}

// NewCredentialResolver creates a resolver using the given keyring.
func NewCredentialResolver(kr Keyring) *CredentialResolver {
	return &CredentialResolver{Keyring: kr}
}

// Resolve returns the best client ID and secret following the
// credential chain. If none are found, returns an AppError guiding
// the user to run setup.
func (r *CredentialResolver) Resolve() (clientID, clientSecret string, err error) {
	clientID = r.firstNonEmpty(
		r.ClientIDFlag,
		os.Getenv("INVGATE_CLIENT_ID"),
	)
	clientSecret = r.firstNonEmpty(
		r.ClientSecretFlag,
		os.Getenv("INVGATE_CLIENT_SECRET"),
	)
	if clientID == "" && r.Keyring != nil {
		if v, e := r.Keyring.Get(ServiceName, KeyClientID); e == nil {
			clientID = v
		}
	}
	if clientSecret == "" && r.Keyring != nil {
		if v, e := r.Keyring.Get(ServiceName, KeyClientSecret); e == nil {
			clientSecret = v
		}
	}
	if clientID == "" || clientSecret == "" {
		return "", "", errors.NewError(0, "no credentials found. Run 'invgate-cli setup' or set INVGATE_CLIENT_ID")
	}
	return clientID, clientSecret, nil
}

func (r *CredentialResolver) firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// StoredCredentialsPresent reports whether both client ID and secret
// are present in the keychain. It does NOT report flag/env presence.
func (r *CredentialResolver) StoredCredentialsPresent() bool {
	if r.Keyring == nil {
		return false
	}
	id, err1 := r.Keyring.Get(ServiceName, KeyClientID)
	if err1 != nil || id == "" {
		return false
	}
	sec, err2 := r.Keyring.Get(ServiceName, KeyClientSecret)
	return err2 == nil && sec != ""
}

// StoreCredentials writes client ID and secret to the keychain.
// Flags/env-only callers use this to persist after setup.
func (r *CredentialResolver) StoreCredentials(clientID, clientSecret string) error {
	if r.Keyring == nil {
		return fmt.Errorf("keyring not available")
	}
	if err := r.Keyring.Set(ServiceName, KeyClientID, clientID); err != nil {
		return fmt.Errorf("could not store client-id: %w", err)
	}
	if err := r.Keyring.Set(ServiceName, KeyClientSecret, clientSecret); err != nil {
		return fmt.Errorf("could not store client-secret: %w", err)
	}
	return nil
}