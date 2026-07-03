package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wdelcant/invgate-cli/internal/auth"
)

// newConnectionTestServer returns an httptest server that satisfies both
// steps of the W3 connection test: the OAuth2 client-credentials token
// endpoint (POST /token) and an authenticated API call (GET /asset-types/).
// It issues access token "test-tok" for matching client_id/secret and
// requires that token on subsequent API calls.
func newConnectionTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.FormValue("client_id") != "test-id" || r.FormValue("client_secret") != "test-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"missing/invalid token"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
}

func TestRunSetup_NonInteractive_AllFlags(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	server := newConnectionTestServer(t)
	defer server.Close()

	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:        &out,
		Flags: SetupFlags{
			BaseURL:      server.URL,
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Output:       "yaml",
			TokenURL:     server.URL + "/token",
		},
		Keyring:    m,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("RunSetup: %v", err)
	}
	// Verify config was written
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != server.URL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, server.URL)
	}
	if cfg.Output != "yaml" {
		t.Errorf("Output = %q, want yaml", cfg.Output)
	}
	// Verify secrets in keychain
	id, _ := m.Get(auth.ServiceName, auth.KeyClientID)
	sec, _ := m.Get(auth.ServiceName, auth.KeyClientSecret)
	if id != "test-id" {
		t.Errorf("keyring client-id = %q, want test-id", id)
	}
	if sec != "test-secret" {
		t.Errorf("keyring client-secret = %q, want test-secret", sec)
	}
	if !strings.Contains(out.String(), "Setup complete") {
		t.Errorf("Output should contain 'Setup complete', got: %s", out.String())
	}
}

// TestRunSetup_ConnectionTestSucceeds confirms that the W3 connection
// test actually fetches a token and calls an authenticated endpoint.
func TestRunSetup_ConnectionTestSucceeds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	server := newConnectionTestServer(t)
	defer server.Close()

	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags: SetupFlags{
			BaseURL:      server.URL,
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			TokenURL:     server.URL + "/token",
		},
		Keyring:    m,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("RunSetup: %v", err)
	}
	if !strings.Contains(out.String(), "Setup complete. Connection test succeeded.") {
		t.Errorf("expected success message, got: %s", out.String())
	}
}

// TestRunSetup_ConnectionTestTokenFetchFails verifies that when the
// token endpoint rejects the credentials, the test reports a token
// fetch failure (warning), not an API call failure.
func TestRunSetup_ConnectionTestTokenFetchFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token endpoint always rejects.
		if r.Method == http.MethodPost && r.URL.Path == "/token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags: SetupFlags{
			BaseURL:      server.URL,
			ClientID:     "wrong-id",
			ClientSecret: "wrong-secret",
			TokenURL:     server.URL + "/token",
		},
		Keyring:    m,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("RunSetup should not fail on connection test: %v", err)
	}
	if !strings.Contains(out.String(), "token fetch failed") {
		t.Errorf("expected 'token fetch failed' in warning, got: %s", out.String())
	}
}

func TestRunSetup_NonInteractive_MissingBaseURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags:       SetupFlags{ClientID: "id", ClientSecret: "sec"},
		Keyring:     auth.NewMockKeyring(),
	})
	if err == nil {
		t.Fatal("expected error for missing base-url")
	}
	if !strings.Contains(err.Error(), "--base-url is required") {
		t.Errorf("error = %v, want --base-url is required", err)
	}
}

func TestRunSetup_NonInteractive_MissingClientID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags:       SetupFlags{BaseURL: "https://api.example.com", ClientSecret: "sec"},
		Keyring:     auth.NewMockKeyring(),
	})
	if err == nil {
		t.Fatal("expected error for missing client-id")
	}
	if !strings.Contains(err.Error(), "--client-id is required") {
		t.Errorf("error = %v", err)
	}
}

func TestRunSetup_NonInteractive_MissingClientSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags:       SetupFlags{BaseURL: "https://api.example.com", ClientID: "id"},
		Keyring:     auth.NewMockKeyring(),
	})
	if err == nil {
		t.Fatal("expected error for missing client-secret")
	}
	if !strings.Contains(err.Error(), "--client-secret is required") {
		t.Errorf("error = %v", err)
	}
}

func TestRunSetup_Interactive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	input := strings.Join([]string{
		server.URL,
		"interactive-id",
		"interactive-secret",
		"table",
	}, "\n")
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: true,
		In:          strings.NewReader(input),
		Out:         &out,
		Keyring:     m,
		HTTPClient:  server.Client(),
	})
	if err != nil {
		t.Fatalf("RunSetup: %v", err)
	}
	id, _ := m.Get(auth.ServiceName, auth.KeyClientID)
	if id != "interactive-id" {
		t.Errorf("keyring client-id = %q, want interactive-id", id)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != server.URL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, server.URL)
	}
	if cfg.Output != "table" {
		t.Errorf("Output = %q, want table", cfg.Output)
	}
}

func TestRunSetup_Interactive_InvalidURL_Reprompts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()

	// First invalid, then valid.
	input := "not-a-url\nhttps://valid.example.com\nid\nsecret\njson\n"
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: true,
		In:          strings.NewReader(input),
		Out:         &out,
		Keyring:     m,
	})
	if err != nil {
		t.Fatalf("RunSetup: %v", err)
	}
	if !strings.Contains(out.String(), "not a valid URL") {
		t.Errorf("should re-prompt on invalid URL, got: %s", out.String())
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"https://api.example.com", true},
		{"http://localhost:8080", true},
		{"not-a-url", false},
		{"", false},
		{"ftp://example.com", false},
		{"https://", true}, // parseable scheme+empty host still "valid" in format check
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := isValidURL(tt.in); got != tt.want {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestPromptString_Default(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	got, err := promptString(&out, scanner, "Name", "default-val")
	if err != nil {
		t.Fatalf("promptString: %v", err)
	}
	if got != "default-val" {
		t.Errorf("promptString = %q, want default-val", got)
	}
}

func TestPromptString_Input(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("hello\n"))
	got, err := promptString(&out, scanner, "Name", "")
	if err != nil {
		t.Fatalf("promptString: %v", err)
	}
	if got != "hello" {
		t.Errorf("promptString = %q, want hello", got)
	}
}

func TestRunSetup_ConnectionTestFailure_NonFatal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	var out bytes.Buffer
	// Use a closed httptest server to simulate unreachable — connection
	// test fails quickly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	server.Close() // close immediately so connections fail
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags: SetupFlags{
			BaseURL:      server.URL,
			ClientID:     "id",
			ClientSecret: "sec",
		},
		Keyring:    m,
		HTTPClient: &http.Client{Timeout: 1 * time.Second},
	})
	if err != nil {
		t.Fatalf("RunSetup should not fail on connection test failure: %v", err)
	}
	if !strings.Contains(out.String(), "warning") {
		t.Errorf("Output should contain warning, got: %s", out.String())
	}
}

// --- Additional branch-coverage tests ---

// errReader is an io.Reader that always returns an error, used to drive
// bufio.Scanner into its error branch.
type errReader struct{ msg string }

func (e *errReader) Read(p []byte) (int, error) { return 0, errors.New(e.msg) }

// failOnSecondSetKeyring accepts the first Set call (client-id) but
// fails on the second (client-secret), exercising the second keyring
// error branch in RunSetup.
type failOnSecondSetKeyring struct{ calls int }

func (k *failOnSecondSetKeyring) Get(service, key string) (string, error) {
	return "", auth.ErrNotFound
}
func (k *failOnSecondSetKeyring) Set(service, key, value string) error {
	k.calls++
	if k.calls >= 2 {
		return errors.New("keychain rejected second set")
	}
	return nil
}
func (k *failOnSecondSetKeyring) Delete(service, key string) error { return nil }

// TestPromptString_NoDefault exercises the no-default prompt branch
// (label printed without the "[default]" hint) returning user input.
func TestPromptString_NoDefault(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("myvalue\n"))
	got, err := promptString(&out, scanner, "Name", "")
	if err != nil {
		t.Fatalf("promptString: %v", err)
	}
	if got != "myvalue" {
		t.Errorf("promptString = %q, want myvalue", got)
	}
	if !strings.Contains(out.String(), "Name:") {
		t.Errorf("output should contain bare label, got: %s", out.String())
	}
	if strings.Contains(out.String(), "[") {
		t.Errorf("output should not include a default hint, got: %s", out.String())
	}
}

// TestPromptString_EOF_NoDefault hits the scan-end path with no default:
// Scan() returns false, Err() is nil, so the default ("") is returned.
func TestPromptString_EOF_NoDefault(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(""))
	got, err := promptString(&out, scanner, "Name", "")
	if err != nil {
		t.Fatalf("promptString: %v", err)
	}
	if got != "" {
		t.Errorf("promptString = %q, want empty (EOF, no default)", got)
	}
}

// TestPromptString_ScannerError drives the reader into an error so
// scanner.Err() returns a non-nil error that promptString wraps.
func TestPromptString_ScannerError(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(&errReader{msg: "read boom"})
	_, err := promptString(&out, scanner, "Name", "def")
	if err == nil {
		t.Fatal("promptString should return the scanner error")
	}
	if !strings.Contains(err.Error(), "read boom") {
		t.Errorf("error = %v, want wrapped 'read boom'", err)
	}
}

// TestPromptURL_EmptyThenInvalidThenValid covers both re-prompt loops:
// an empty value yields "URL is required", an invalid value yields
// "not a valid URL", and the third valid value is returned.
func TestPromptURL_EmptyThenInvalidThenValid(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("\nnot-a-url\nhttps://valid.com\n"))
	got, err := promptURL(&out, scanner, "Base URL", "")
	if err != nil {
		t.Fatalf("promptURL: %v", err)
	}
	if got != "https://valid.com" {
		t.Errorf("promptURL = %q, want https://valid.com", got)
	}
	if !strings.Contains(out.String(), "URL is required") {
		t.Errorf("output should warn about empty URL, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "not a valid URL") {
		t.Errorf("output should warn about invalid URL, got: %s", out.String())
	}
}

// TestPromptURL_ScannerError propagates a promptString error out of the
// promptURL loop.
func TestPromptURL_ScannerError(t *testing.T) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(&errReader{msg: "read boom"})
	_, err := promptURL(&out, scanner, "Base URL", "")
	if err == nil {
		t.Fatal("promptURL should propagate the scanner error")
	}
	if !strings.Contains(err.Error(), "read boom") {
		t.Errorf("error = %v, want wrapped 'read boom'", err)
	}
}

// TestRunSetup_Interactive_KeyringSetFails makes the keyring fail on
// the very first Set (client-id), so RunSetup returns a wrapped keychain
// error before reaching the connection test.
func TestRunSetup_Interactive_KeyringSetFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := auth.NewMockKeyring()
	m.Unavailable = true

	input := "https://api.example.com\nid\nsecret\njson\n"
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: true,
		In:          strings.NewReader(input),
		Out:         &out,
		Keyring:     m,
	})
	if err == nil {
		t.Fatal("RunSetup should fail when keyring.Set fails")
	}
	if !strings.Contains(err.Error(), "client-id in keychain") {
		t.Errorf("error = %v, want 'client-id in keychain'", err)
	}
}

// TestRunSetup_Interactive_KeyringSetSecretFails lets the client-id Set
// succeed but fails on the client-secret Set, covering the second
// keyring error branch.
func TestRunSetup_Interactive_KeyringSetSecretFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	input := "https://api.example.com\nid\nsecret\njson\n"
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: true,
		In:          strings.NewReader(input),
		Out:         &out,
		Keyring:     &failOnSecondSetKeyring{},
	})
	if err == nil {
		t.Fatal("RunSetup should fail when the second keyring.Set fails")
	}
	if !strings.Contains(err.Error(), "client-secret in keychain") {
		t.Errorf("error = %v, want 'client-secret in keychain'", err)
	}
}

// TestRunSetup_NonInteractive_MissingClientID_Direct is the focused,
// direct variant of the existing RunSetup-level missing-client-id test.
func TestRunSetup_NonInteractive_MissingClientID_Direct(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out bytes.Buffer
	err := RunSetup(SetupOptions{
		Interactive: false,
		Out:         &out,
		Flags:       SetupFlags{BaseURL: "https://api.example.com", ClientSecret: "sec"},
		Keyring:     auth.NewMockKeyring(),
	})
	if err == nil {
		t.Fatal("expected error for missing --client-id")
	}
	if !strings.Contains(err.Error(), "--client-id is required") {
		t.Errorf("error = %v, want '--client-id is required'", err)
	}
}

// TestTestConnection_MissingTokenURL exercises the "skipped" early
// return when token URL or credentials are absent.
func TestTestConnection_MissingTokenURL(t *testing.T) {
	err := testConnection(SetupOptions{}, "https://api.example.com")
	if err == nil {
		t.Fatal("testConnection should report 'skipped' when the token URL is missing")
	}
	if !strings.Contains(err.Error(), "skipped") {
		t.Errorf("error = %v, want 'skipped'", err)
	}
}

// TestTestConnection_Non2xxResponse covers the default-http-client
// branch (HTTPClient == nil) and the non-2xx API status branch: the
// token endpoint succeeds but the API endpoint returns 500.
func TestTestConnection_Non2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Deliberately omit HTTPClient so the default client path is taken.
	err := testConnection(SetupOptions{
		Flags: SetupFlags{
			BaseURL:      server.URL,
			ClientID:     "id",
			ClientSecret: "sec",
			TokenURL:     server.URL + "/token",
		},
	}, server.URL)
	if err == nil {
		t.Fatal("testConnection should fail on a non-2xx API response")
	}
	if !strings.Contains(err.Error(), "returned status 500") {
		t.Errorf("error = %v, want 'returned status 500'", err)
	}
}

// TestIsValidURL_Empty is the explicit empty-input case for isValidURL.
func TestIsValidURL_Empty(t *testing.T) {
	if isValidURL("") {
		t.Errorf("isValidURL(\"\") = true, want false")
	}
}