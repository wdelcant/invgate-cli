package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wdelcant/invgate-cli/internal/errors"
)

const credFileName = "credentials.json"

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
// credential chain: flags → env → keychain → fallback file.
// If none are found, returns an AppError guiding the user to run setup.
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
	// Fallback to file when the OS keychain is unavailable (e.g. headless Linux).
	if clientID == "" || clientSecret == "" {
		if fc, ok := readCredFile(); ok {
			if clientID == "" {
				clientID = fc.ClientID
			}
			if clientSecret == "" {
				clientSecret = fc.ClientSecret
			}
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
// If the keychain is unavailable, falls back to a restricted-permission
// file in the config directory.
func (r *CredentialResolver) StoreCredentials(clientID, clientSecret string) error {
	if r.Keyring == nil {
		return writeCredFile(clientID, clientSecret)
	}
	if err := r.Keyring.Set(ServiceName, KeyClientID, clientID); err != nil {
		// Keychain unavailable — fall back to file.
		return writeCredFile(clientID, clientSecret)
	}
	if err := r.Keyring.Set(ServiceName, KeyClientSecret, clientSecret); err != nil {
		return writeCredFile(clientID, clientSecret)
	}
	return nil
}

// credFileEntry is the JSON persisted to disk as a keychain fallback.
type credFileEntry struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func credFilePath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "invgate-cli", credFileName), nil
}

func readCredFile() (credFileEntry, bool) {
	path, err := credFilePath()
	if err != nil {
		return credFileEntry{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return credFileEntry{}, false
	}
	var entry credFileEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return credFileEntry{}, false
	}
	return entry, entry.ClientID != "" && entry.ClientSecret != ""
}

func writeCredFile(clientID, clientSecret string) error {
	path, err := credFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create config dir: %w", err)
	}
	data, err := json.Marshal(credFileEntry{ClientID: clientID, ClientSecret: clientSecret})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}