package auth

import (
	"github.com/zalando/go-keyring"
)

// OSKeyring wraps github.com/zalando/go-keyring, implementing the
// Keyring interface. If the OS has no secret store, it returns
// ErrKeyringUnavailable.
type OSKeyring struct{}

// NewOSKeyring creates an OSKeyring wrapper.
func NewOSKeyring() *OSKeyring {
	return &OSKeyring{}
}

func (o *OSKeyring) Get(service, key string) (string, error) {
	v, err := keyring.Get(service, key)
	if err == keyring.ErrNotFound {
		return "", ErrNotFound
	}
	if err != nil {
		// keyring library returns a generic error when no backend is available.
		return "", ErrKeyringUnavailable
	}
	return v, nil
}

func (o *OSKeyring) Set(service, key, value string) error {
	err := keyring.Set(service, key, value)
	if err != nil {
		return ErrKeyringUnavailable
	}
	return nil
}

func (o *OSKeyring) Delete(service, key string) error {
	err := keyring.Delete(service, key)
	if err == keyring.ErrNotFound {
		return ErrNotFound
	}
	if err != nil {
		return ErrKeyringUnavailable
	}
	return nil
}