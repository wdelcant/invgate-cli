package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wdelcant/invgate-cli/internal/auth"
	"github.com/wdelcant/invgate-cli/internal/cli"
	keyring "github.com/zalando/go-keyring"
)

// TestMain best-effort clears any leftover invgate-cli entries from the
// real OS keychain left by previous (pre-MockKeyring) test runs, then runs
// the suite. This keeps the developer's machine clean.
func TestMain(m *testing.M) {
	for _, key := range []string{auth.KeyClientID, auth.KeyClientSecret, auth.KeyAccessToken} {
		_ = keyring.Delete(auth.ServiceName, key)
	}
	os.Exit(m.Run())
}

// integrationTestEnv bundles the mock servers and temp paths for a test run.
type integrationTestEnv struct {
	apiServer   *httptest.Server
	tokenServer *httptest.Server
	specPath    string
	xdgDir     string
	tokenCalls  int32
}

// newIntegrationEnv stands up a token server, an API server, and a
// temp OAS3 spec whose server URL and tokenUrl point at the mocks.
func newIntegrationEnv(t *testing.T) *integrationTestEnv {
	t.Helper()
	env := &integrationTestEnv{}

	// Token endpoint.
	env.tokenServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&env.tokenCalls, 1)
		_ = r.ParseForm()
		if r.FormValue("client_id") == "" || r.FormValue("client_secret") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "integration-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))

	// API server with a few endpoints.
	env.apiServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer integration-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"missing/invalid token"}`))
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/assets":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"name":"MacBook","status":"active"},{"id":2,"name":"ThinkPad","status":"retired"}]`))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/assets/"):
			id := strings.TrimPrefix(strings.Trim(r.URL.Path, "/"), "assets/")
			if id == "9999" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":%s,"name":"Asset %s"}`, id, id)))
		case r.Method == "POST" && r.URL.Path == "/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":42,"name":"created"}`))
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/assets/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"unknown route"}`))
		}
	}))

	// Write a temp OAS3 spec with the mock server URLs.
	env.specPath = writeTempSpec(t, env.apiServer.URL, env.tokenServer.URL)
	env.xdgDir = t.TempDir()
	return env
}

// writeTempSpec writes a minimal OAS3 spec to a temp file with the
// given server URL and token URL.
func writeTempSpec(t *testing.T, serverURL, tokenURL string) string {
	t.Helper()
	spec := fmt.Sprintf(`openapi: 3.0.0
info:
  title: Integration Test API
  version: 1.0.0
servers:
  - url: %s
components:
  securitySchemes:
    oauth2:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl: %s
          scopes:
            write: write access
paths:
  /assets:
    get:
      tags: [Assets]
      operationId: assets_list
      summary: List assets
      parameters:
        - name: name
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: A list of assets
    post:
      tags: [Assets]
      operationId: assets_create
      summary: Create an asset
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        "201":
          description: Created
  /assets/{id}:
    get:
      tags: [Assets]
      operationId: assets_read
      summary: Get an asset
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: An asset
        "404":
          description: Not found
    delete:
      tags: [Assets]
      operationId: assets_delete
      summary: Delete an asset
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
      responses:
        "204":
          description: Deleted
  /ping:
    get:
      summary: Health check
      operationId: ping_read
      responses:
        "200":
          description: OK
`, serverURL, tokenURL)
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// withEnv sets the required env vars and restores them after the test.
func withEnv(t *testing.T, env *integrationTestEnv, fn func()) {
	t.Helper()
	oldClientID := os.Getenv("INVGATE_CLIENT_ID")
	oldClientSecret := os.Getenv("INVGATE_CLIENT_SECRET")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		os.Setenv("INVGATE_CLIENT_ID", oldClientID)
		os.Setenv("INVGATE_CLIENT_SECRET", oldClientSecret)
		os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	os.Setenv("INVGATE_CLIENT_ID", "test-client")
	os.Setenv("INVGATE_CLIENT_SECRET", "test-secret")
	os.Setenv("XDG_CONFIG_HOME", env.xdgDir)
	fn()
}

// runCLI builds a CLI pointed at the mock servers and captures output.
// A fresh MockKeyring is injected so tests never touch (or read
// stale tokens from) the real OS keychain — keeping them hermetic.
func runCLI(t *testing.T, args []string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runCLIWithKeyring(t, args, auth.NewMockKeyring())
}

// runCLIWithKeyring lets a test supply its own keyring (e.g. for setup tests
// that pre-store credentials).
func runCLIWithKeyring(t *testing.T, args []string, kr auth.Keyring) (stdout, stderr string, exitCode int) {
	t.Helper()
	c := cli.New(args)
	c.Keyring = kr
	var outBuf, errBuf bytes.Buffer
	c.Out = &outBuf
	c.ErrOut = &errBuf
	c.Root.SetOut(&outBuf)
	c.Root.SetErr(&errBuf)
	exitCode = c.Execute(args)
	return outBuf.String(), errBuf.String(), exitCode
}

func TestIntegration_Version(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--version"})
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "invgate-cli v") {
			t.Errorf("version output = %q, want invgate-cli v...", stdout)
		}
	})
}

func TestIntegration_HelpShowsDynamicCommands(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--help"})
		if code != 0 {
			t.Errorf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "assets") {
			t.Errorf("--help should show 'assets' group, got %q", stdout)
		}
	})
}

func TestIntegration_ListAssets(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, stderr, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "MacBook") {
			t.Errorf("stdout should contain MacBook: %q", stdout)
		}
		// A token should have been fetched from the mock token endpoint.
		if atomic.LoadInt32(&env.tokenCalls) == 0 {
			t.Error("token endpoint was not called")
		}
	})
}

func TestIntegration_ReadAsset(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "read", "7"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "Asset 7") {
			t.Errorf("stdout should contain 'Asset 7': %q", stdout)
		}
	})
}

func TestIntegration_CreateAsset(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "create", "--data", `{"name":"NewAsset"}`})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "created") {
			t.Errorf("stdout should contain 'created': %q", stdout)
		}
	})
}

func TestIntegration_DeleteAsset(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "delete", "5"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("204 should print 'OK': %q", stdout)
		}
	})
}

func TestIntegration_OutputFormatYAML(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--output", "yaml", "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "- id: 1") && !strings.Contains(stdout, "id: 1") {
			t.Errorf("YAML output should be YAML: %q", stdout)
		}
	})
}

func TestIntegration_OutputFormatTable(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--output", "table", "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "MacBook") {
			t.Errorf("table should contain MacBook: %q", stdout)
		}
	})
}

func TestIntegration_OutputFormatCSV(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--output", "csv", "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		// CSV must have a header row.
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 2 {
			t.Fatalf("CSV should have header + data rows, got %q", stdout)
		}
		if !strings.Contains(lines[0], "id") {
			t.Errorf("CSV header should contain 'id': %q", lines[0])
		}
	})
}

func TestIntegration_CompactJSON(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		stdout, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--compact", "assets", "read", "1"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		// Compact JSON = no newlines within the JSON body.
		jsonPart := strings.TrimSpace(stdout)
		if strings.Contains(jsonPart, "\n  ") {
			t.Errorf("compact JSON should not be indented: %q", jsonPart)
		}
	})
}

func TestIntegration_404Error(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		_, stderr, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "read", "9999"})
		if code != 1 {
			t.Errorf("404 should exit 1, got %d", code)
		}
		if !strings.Contains(stderr, "404") {
			t.Errorf("stderr should mention 404: %q", stderr)
		}
	})
}

func TestIntegration_NoCredentials(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	// Explicitly clear credentials.
	os.Setenv("INVGATE_CLIENT_ID", "")
	os.Setenv("INVGATE_CLIENT_SECRET", "")
	os.Setenv("XDG_CONFIG_HOME", env.xdgDir)
	stdout, stderr, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "assets", "list"})
	if code != 1 {
		t.Errorf("missing credentials should exit 1, got %d (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	_ = stderr
}

func TestIntegration_VerboseError(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		_, stderr, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--verbose", "assets", "read", "9999"})
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr, "404") {
			t.Errorf("verbose stderr should mention status: %q", stderr)
		}
	})
}

func TestIntegration_401AutoRefresh(t *testing.T) {
	env := newIntegrationEnv(t)
	// Override the API server to require a DIFFERENT token on first call,
	// then accept the integration token after a Login refresh.
	var attempt int32
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempt, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"expired"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer integration-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"MacBook"}]`))
	}))
	defer apiServer.Close()
	defer env.tokenServer.Close()
	// Override the token server so the second token issued is the accepted one.
	env.apiServer.Close()
	env.apiServer = apiServer

	// Rewrite spec with the new API server URL.
	env.specPath = writeTempSpec(t, apiServer.URL, env.tokenServer.URL)
	withEnv(t, env, func() {
		stdout, stderr, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", apiServer.URL, "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0 after retry; stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "MacBook") {
			t.Errorf("stdout should contain MacBook after refresh: %q", stdout)
		}
	})
}

func TestIntegration_SpecFlagFromEnv(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		// Also set INVGATE_SPEC env.
		os.Setenv("INVGATE_SPEC", env.specPath)
		stdout, _, code := runCLI(t, []string{"--base-url", env.apiServer.URL, "assets", "list"})
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(stdout, "MacBook") {
			t.Errorf("stdout should contain MacBook: %q", stdout)
		}
	})
}

// Quick smoke test that setup non-interactive writes config (uses mock keyring-free path).
func TestIntegration_SetupNonInteractive(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		_, stderr, code := runCLI(t, []string{"setup", "--base-url", env.apiServer.URL, "--client-id", "test-client", "--client-secret", "test-secret"})
		_ = code
		// Setup calls testConnection which hits the API without auth token → likely warns.
		// We only assert it doesn't hard-crash on the config write.
		_ = stderr
	})
	// The config file should exist after setup.
	configPath := filepath.Join(env.xdgDir, "invgate-cli", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file should be written after setup: %v", err)
	}
}

// Ensure timeout flag is accepted and parses.
func TestIntegration_TimeoutFlag(t *testing.T) {
	env := newIntegrationEnv(t)
	defer env.apiServer.Close()
	defer env.tokenServer.Close()
	withEnv(t, env, func() {
		_, _, code := runCLI(t, []string{"--spec", env.specPath, "--base-url", env.apiServer.URL, "--timeout", "10s", "assets", "list"})
		if code != 0 {
			t.Errorf("exit = %d, want 0 with --timeout", code)
		}
	})
}

// Quiet the time import if all timeout tests are skipped.
var _ = time.Second