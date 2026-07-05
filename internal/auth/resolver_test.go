package auth

import (
	"os"
	"testing"
)

func TestResolve_FlagsWin(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "keyring-id")
	_ = m.Set(ServiceName, KeyClientSecret, "keyring-secret")
	t.Setenv("INVGATE_CLIENT_ID", "env-id")
	t.Setenv("INVGATE_CLIENT_SECRET", "env-secret")
	r := &CredentialResolver{ClientIDFlag: "flag-id", ClientSecretFlag: "flag-secret", Keyring: m}
	id, sec, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if id != "flag-id" {
		t.Errorf("id = %q, want flag-id", id)
	}
	if sec != "flag-secret" {
		t.Errorf("sec = %q, want flag-secret", sec)
	}
}

func TestResolve_EnvOverKeyring(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "keyring-id")
	_ = m.Set(ServiceName, KeyClientSecret, "keyring-secret")
	t.Setenv("INVGATE_CLIENT_ID", "env-id")
	t.Setenv("INVGATE_CLIENT_SECRET", "env-secret")
	r := NewCredentialResolver(m)
	id, sec, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if id != "env-id" {
		t.Errorf("id = %q, want env-id", id)
	}
	if sec != "env-secret" {
		t.Errorf("sec = %q, want env-secret", sec)
	}
}

func TestResolve_KeyringFallback(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "keyring-id")
	_ = m.Set(ServiceName, KeyClientSecret, "keyring-secret")
	os.Unsetenv("INVGATE_CLIENT_ID")
	os.Unsetenv("INVGATE_CLIENT_SECRET")
	r := NewCredentialResolver(m)
	id, sec, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if id != "keyring-id" {
		t.Errorf("id = %q, want keyring-id", id)
	}
	if sec != "keyring-secret" {
		t.Errorf("sec = %q, want keyring-secret", sec)
	}
}

func TestResolve_NoCreds(t *testing.T) {
	m := NewMockKeyring()
	os.Unsetenv("INVGATE_CLIENT_ID")
	os.Unsetenv("INVGATE_CLIENT_SECRET")
	// Also ensure no file-based credentials from previous tests.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := NewCredentialResolver(m)
	_, _, err := r.Resolve()
	if err == nil {
		t.Fatal("Resolve should error without credentials")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestStoreCredentials(t *testing.T) {
	m := NewMockKeyring()
	r := NewCredentialResolver(m)
	if err := r.StoreCredentials("id1", "sec1"); err != nil {
		t.Fatalf("StoreCredentials: %v", err)
	}
	id, _ := m.Get(ServiceName, KeyClientID)
	sec, _ := m.Get(ServiceName, KeyClientSecret)
	if id != "id1" || sec != "sec1" {
		t.Errorf("stored = (%q, %q), want (id1, sec1)", id, sec)
	}
}

func TestStoredCredentialsPresent(t *testing.T) {
	m := NewMockKeyring()
	r := NewCredentialResolver(m)
	if r.StoredCredentialsPresent() {
		t.Error("should be false when empty")
	}
	_ = m.Set(ServiceName, KeyClientID, "id")
	_ = m.Set(ServiceName, KeyClientSecret, "sec")
	if !r.StoredCredentialsPresent() {
		t.Error("should be true when both present")
	}
}

func TestStoredCredentialsPresent_NilKeyring(t *testing.T) {
	r := &CredentialResolver{Keyring: nil}
	if r.StoredCredentialsPresent() {
		t.Error("should be false when keyring is nil")
	}
}

func TestStoredCredentialsPresent_OnlyClientID(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "id")
	// client-secret missing
	r := NewCredentialResolver(m)
	if r.StoredCredentialsPresent() {
		t.Error("should be false when only client-id is present")
	}
}

func TestStoredCredentialsPresent_OnlyClientSecret(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientSecret, "sec")
	// client-id missing → Get returns ErrNotFound → false
	r := NewCredentialResolver(m)
	if r.StoredCredentialsPresent() {
		t.Error("should be false when only client-secret is present")
	}
}

func TestStoreCredentials_NilKeyring(t *testing.T) {
	r := &CredentialResolver{Keyring: nil}
	// Nil keyring now falls back to file storage, so it should succeed.
	err := r.StoreCredentials("id", "sec")
	if err != nil {
		t.Fatalf("StoreCredentials with nil keyring should write to file: %v", err)
	}
}

func TestStoreCredentials_ClientIDSetFails(t *testing.T) {
	m := NewMockKeyring()
	m.Unavailable = true
	r := NewCredentialResolver(m)
	// Keychain unavailable now falls back to file storage.
	err := r.StoreCredentials("id", "sec")
	if err != nil {
		t.Fatalf("StoreCredentials should fall back to file: %v", err)
	}
}

func TestStoreCredentials_ClientSecretSetFails(t *testing.T) {
	m := NewMockKeyring()
	kr := keyringFailOn{inner: m, failSet: map[string]error{KeyClientSecret: errKeyringFail}}
	r := NewCredentialResolver(kr)
	// Client-secret Set fails → falls back to file storage.
	err := r.StoreCredentials("id", "sec")
	if err != nil {
		t.Fatalf("StoreCredentials should fall back to file: %v", err)
	}
}