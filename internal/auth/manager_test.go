package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// tokenHandler returns a server that issues tokens with the given expiry.
func tokenHandler(t *testing.T, expiry time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = r.ParseForm()
		id := r.FormValue("client_id")
		sec := r.FormValue("client_secret")
		if id == "bad" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client"})
			return
		}
		if id == "" || sec == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + id,
			"token_type":   "Bearer",
			"expires_in":   int(expiry.Seconds()),
		})
	}))
}

func TestManager_TokenCacheHit(t *testing.T) {
	m := NewMockKeyring()
	// Pre-cache a token that expires in 2h.
	ct := cachedToken{Token: "prefetched-tok", Expiry: time.Now().Add(2 * time.Hour)}
	data, _ := json.Marshal(ct)
	_ = m.Set(ServiceName, KeyAccessToken, string(data))

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()
	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "prefetched-tok" {
		t.Errorf("Token = %q, want prefetched-tok (cache hit)", got)
	}
}

func TestManager_TokenCacheMiss_FetchesNew(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "test-id")
	_ = m.Set(ServiceName, KeyClientSecret, "test-secret")

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()

	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "tok-test-id" {
		t.Errorf("Token = %q, want tok-test-id", got)
	}
	// Verify it was cached
	raw, err := m.Get(ServiceName, KeyAccessToken)
	if err != nil {
		t.Errorf("cached token not found: %v", err)
	}
	if raw == "" {
		t.Error("cached token should be non-empty")
	}
}

func TestManager_TokenExpired_Refreshes(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "refresh-id")
	_ = m.Set(ServiceName, KeyClientSecret, "refresh-secret")
	// Pre-cache an expired token.
	ct := cachedToken{Token: "expired-tok", Expiry: time.Now().Add(-1 * time.Hour)}
	data, _ := json.Marshal(ct)
	_ = m.Set(ServiceName, KeyAccessToken, string(data))

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()

	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "tok-refresh-id" {
		t.Errorf("Token = %q, want tok-refresh-id (refreshed)", got)
	}
}

func TestManager_Login_ForcesFetch(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "login-id")
	_ = m.Set(ServiceName, KeyClientSecret, "login-secret")
	// Pre-cache a token.
	ct := cachedToken{Token: "old-tok", Expiry: time.Now().Add(2 * time.Hour)}
	data, _ := json.Marshal(ct)
	_ = m.Set(ServiceName, KeyAccessToken, string(data))

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()

	if err := mgr.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token after login: %v", err)
	}
	if got != "tok-login-id" {
		t.Errorf("Token = %q, want tok-login-id (post-login)", got)
	}
}

func TestManager_InvalidCredentials(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "bad")
	_ = m.Set(ServiceName, KeyClientSecret, "wrong")

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()

	_, err := mgr.Token(context.Background())
	if err == nil {
		t.Fatal("Token should error on invalid credentials")
	}
}

func TestManager_Logout(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "id")
	_ = m.Set(ServiceName, KeyClientSecret, "sec")
	_ = m.Set(ServiceName, KeyAccessToken, "{\"token\":\"x\"}")

	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if err := mgr.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := m.Get(ServiceName, KeyAccessToken); err == nil {
		t.Error("access-token should be deleted after Logout")
	}
	if _, err := m.Get(ServiceName, KeyClientID); err == nil {
		t.Error("client-id should be deleted after Logout")
	}
}

func TestManager_Logout_NoEntries(t *testing.T) {
	m := NewMockKeyring()
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if err := mgr.Logout(); err != nil {
		t.Errorf("Logout with no entries should not error: %v", err)
	}
}

func TestManager_TokenNearExpiry_Refreshes(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "near-id")
	_ = m.Set(ServiceName, KeyClientSecret, "near-secret")
	// Token expires in 30 seconds — within the 60s safety buffer.
	ct := cachedToken{Token: "near-expiry", Expiry: time.Now().Add(30 * time.Second)}
	data, _ := json.Marshal(ct)
	_ = m.Set(ServiceName, KeyAccessToken, string(data))

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = server.Client()

	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got == "near-expiry" {
		t.Error("Token should have been refreshed, not returned near-expiry token")
	}
	if got != "tok-near-id" {
		t.Errorf("Token = %q, want tok-near-id", got)
	}
}

// --- helpers for selective keyring failure ---

var errKeyringFail = errors.New("keyring operation failed")

// keyringFailOn wraps a Keyring, failing Delete/Set for configured keys with
// errKeyringFail (a non-ErrNotFound error). Get passes through.
type keyringFailOn struct {
	inner      Keyring
	failDelete map[string]error
	failSet    map[string]error
}

func (k keyringFailOn) Get(s, key string) (string, error) { return k.inner.Get(s, key) }
func (k keyringFailOn) Set(s, key, v string) error {
	if e, ok := k.failSet[key]; ok {
		return e
	}
	return k.inner.Set(s, key, v)
}
func (k keyringFailOn) Delete(s, key string) error {
	if e, ok := k.failDelete[key]; ok {
		return e
	}
	return k.inner.Delete(s, key)
}

// fakeManager is a stub Manager for TokenSource tests.
type fakeManager struct {
	tok string
	err error
}

func (f *fakeManager) Token(context.Context) (string, error) { return f.tok, f.err }
func (f *fakeManager) Login(context.Context) error          { return nil }
func (f *fakeManager) Logout() error                         { return nil }

// --- TokenSource.Token ---

func TestTokenSource_Token(t *testing.T) {
	ts := &TokenSource{Manager: &fakeManager{tok: "ts-tok"}}
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok == nil || tok.AccessToken != "ts-tok" {
		t.Errorf("Token = %+v, want AccessToken ts-tok", tok)
	}
}

func TestTokenSource_TokenError(t *testing.T) {
	ts := &TokenSource{Manager: &fakeManager{err: errors.New("nope")}}
	_, err := ts.Token()
	if err == nil {
		t.Fatal("Token should propagate error")
	}
}

// --- Logout branches ---

func TestManager_Logout_NilKeyring(t *testing.T) {
	mgr := &KeychainManager{Keyring: nil}
	if err := mgr.Logout(); err != nil {
		t.Errorf("Logout with nil keyring should return nil, got %v", err)
	}
}

func TestManager_Logout_AccessTokenDeleteFails(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyAccessToken, "x")
	kr := keyringFailOn{inner: m, failDelete: map[string]error{KeyAccessToken: errKeyringFail}}
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", kr)
	err := mgr.Logout()
	if err == nil {
		t.Fatal("Logout should error when access-token delete fails (non-ErrNotFound)")
	}
	if !errors.Is(err, errKeyringFail) {
		t.Errorf("Logout error should wrap fail error, got %v", err)
	}
}

func TestManager_Logout_ClientIDDeleteFails_NonFatal(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyAccessToken, "x")
	_ = m.Set(ServiceName, KeyClientID, "id")
	_ = m.Set(ServiceName, KeyClientSecret, "sec")
	kr := keyringFailOn{inner: m, failDelete: map[string]error{KeyClientID: errKeyringFail}}
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", kr)
	if err := mgr.Logout(); err != nil {
		t.Errorf("Logout should succeed (non-fatal) when client-id delete fails, got %v", err)
	}
}

func TestManager_Logout_ClientSecretDeleteFails_NonFatal(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyAccessToken, "x")
	_ = m.Set(ServiceName, KeyClientID, "id")
	_ = m.Set(ServiceName, KeyClientSecret, "sec")
	kr := keyringFailOn{inner: m, failDelete: map[string]error{KeyClientSecret: errKeyringFail}}
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", kr)
	if err := mgr.Logout(); err != nil {
		t.Errorf("Logout should succeed (non-fatal) when client-secret delete fails, got %v", err)
	}
}

// --- readCachedToken branches ---

func TestManager_ReadCachedToken_NilKeyring(t *testing.T) {
	mgr := &KeychainManager{Keyring: nil}
	if _, ok := mgr.readCachedToken(); ok {
		t.Error("readCachedToken with nil keyring should return false")
	}
}

func TestManager_ReadCachedToken_GetError(t *testing.T) {
	m := NewMockKeyring() // no token set → ErrNotFound
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if _, ok := mgr.readCachedToken(); ok {
		t.Error("readCachedToken should return false when Get errors")
	}
}

func TestManager_ReadCachedToken_EmptyRaw(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyAccessToken, "")
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if _, ok := mgr.readCachedToken(); ok {
		t.Error("readCachedToken should return false when raw is empty")
	}
}

func TestManager_ReadCachedToken_MalformedJSON(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyAccessToken, "{not valid json")
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if _, ok := mgr.readCachedToken(); ok {
		t.Error("readCachedToken should return false on malformed JSON")
	}
}

func TestManager_ReadCachedToken_EmptyTokenField(t *testing.T) {
	m := NewMockKeyring()
	ct := cachedToken{Expiry: time.Now().Add(time.Hour)} // Token empty
	data, _ := json.Marshal(ct)
	_ = m.Set(ServiceName, KeyAccessToken, string(data))
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	if _, ok := mgr.readCachedToken(); ok {
		t.Error("readCachedToken should return false when token field is empty")
	}
}

// --- writeCachedToken branches ---

func TestManager_WriteCachedToken_NilKeyring(t *testing.T) {
	mgr := &KeychainManager{Keyring: nil}
	if err := mgr.writeCachedToken(cachedToken{Token: "x"}); err != nil {
		t.Errorf("writeCachedToken with nil keyring should return nil, got %v", err)
	}
}

func TestManager_WriteCachedToken_SetFails(t *testing.T) {
	m := NewMockKeyring()
	m.Unavailable = true
	mgr := NewKeychainManager(NewCredentialResolver(m), "http://localhost", m)
	err := mgr.writeCachedToken(cachedToken{Token: "x", Expiry: time.Now().Add(time.Hour)})
	if err == nil {
		t.Fatal("writeCachedToken should error when keyring Set fails")
	}
}

// --- fetchAndCache with nil HTTPClient (covers the nil branch) ---

func TestManager_Token_NilHTTPClient(t *testing.T) {
	m := NewMockKeyring()
	_ = m.Set(ServiceName, KeyClientID, "noclient-id")
	_ = m.Set(ServiceName, KeyClientSecret, "noclient-secret")

	server := tokenHandler(t, time.Hour)
	defer server.Close()

	mgr := NewKeychainManager(NewCredentialResolver(m), server.URL, m)
	mgr.HTTPClient = nil // exercise the nil branch in fetchAndCache

	got, err := mgr.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "tok-noclient-id" {
		t.Errorf("Token = %q, want tok-noclient-id", got)
	}
}