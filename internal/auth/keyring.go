// Package auth manages OAuth2 credentials and access tokens for
// invgate-cli. Secrets are stored in the OS keychain (via go-keyring),
// never in config files.
package auth

import "errors"

// ErrKeyringUnavailable is returned when the OS keychain cannot be
// accessed (e.g. in CI/Docker environments without a secret store).
var ErrKeyringUnavailable = errors.New("keyring unavailable: no secret store found")

// ErrNotFound is returned when a keychain item is not present.
var ErrNotFound = errors.New("keychain item not found")

// Keyring is the abstraction over the OS secret store, allowing
// mock implementations in tests.
type Keyring interface {
	Get(service, key string) (string, error)
	Set(service, key, value string) error
	Delete(service, key string) error
}

// ServiceName is the keychain service under which all invgate-cli
// secrets are stored.
const ServiceName = "invgate-cli"

// Keychain keys.
const (
	KeyClientID     = "client-id"
	KeyClientSecret = "client-secret"
	KeyAccessToken  = "access-token"
)

// MockKeyring is an in-memory keyring for tests.
type MockKeyring struct {
	Store      map[string]map[string]string
	Unavailable bool
}

// NewMockKeyring creates a fresh in-memory keyring.
func NewMockKeyring() *MockKeyring {
	return &MockKeyring{Store: make(map[string]map[string]string)}
}

func (m *MockKeyring) Get(service, key string) (string, error) {
	if m.Unavailable {
		return "", ErrKeyringUnavailable
	}
	svc, ok := m.Store[service]
	if !ok {
		return "", ErrNotFound
	}
	v, ok := svc[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *MockKeyring) Set(service, key, value string) error {
	if m.Unavailable {
		return ErrKeyringUnavailable
	}
	if m.Store[service] == nil {
		m.Store[service] = make(map[string]string)
	}
	m.Store[service][key] = value
	return nil
}

func (m *MockKeyring) Delete(service, key string) error {
	if m.Unavailable {
		return ErrKeyringUnavailable
	}
	if svc, ok := m.Store[service]; ok {
		delete(svc, key)
	}
	return nil
}