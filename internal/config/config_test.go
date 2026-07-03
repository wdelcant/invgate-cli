package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfgYAML := "base_url: https://api.example.com\nspec_path: /tmp/spec.json\noutput: yaml\ntimeout: 60s\n"
	if err := os.WriteFile(path, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want https://api.example.com", cfg.BaseURL)
	}
	if cfg.SpecPath != "/tmp/spec.json" {
		t.Errorf("SpecPath = %q, want /tmp/spec.json", cfg.SpecPath)
	}
	if cfg.Output != "yaml" {
		t.Errorf("Output = %q, want yaml", cfg.Output)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestLoadFromPath_defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Minimal config: only base_url, output and timeout should default.
	if err := os.WriteFile(path, []byte("base_url: https://api.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Output = %q, want default %q", cfg.Output, DefaultOutput)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want default %v", cfg.Timeout, DefaultTimeout)
	}
}

func TestSaveToPath_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &Config{
		BaseURL:  "https://my.invgate.com",
		SpecPath: "/home/user/spec.json",
		Output:   "table",
		Timeout:  45 * time.Second,
	}
	if err := cfg.SaveToPath(path); err != nil {
		t.Fatalf("SaveToPath: %v", err)
	}
	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if loaded.BaseURL != cfg.BaseURL {
		t.Errorf("BaseURL = %q, want %q", loaded.BaseURL, cfg.BaseURL)
	}
	if loaded.SpecPath != cfg.SpecPath {
		t.Errorf("SpecPath = %q, want %q", loaded.SpecPath, cfg.SpecPath)
	}
	if loaded.Output != cfg.Output {
		t.Errorf("Output = %q, want %q", loaded.Output, cfg.Output)
	}
	if loaded.Timeout != cfg.Timeout {
		t.Errorf("Timeout = %v, want %v", loaded.Timeout, cfg.Timeout)
	}
}

func TestSave_NoSecrets(t *testing.T) {
	// Even if someone stuffs secrets into the struct, only yaml-tagged
	// fields are marshaled. But we also verify there's no client_id/
	// client_secret in the struct itself — the YAML tags prove it.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &Config{
		BaseURL: "https://api.example.com",
		Output: "json",
		Timeout: 30 * time.Second,
	}
	if err := cfg.SaveToPath(path); err != nil {
		t.Fatalf("SaveToPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if contains(content, "client_id") || contains(content, "client_secret") || contains(content, "access_token") {
		t.Errorf("config file should not contain secrets: %s", content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestConfigDir_XDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	want := filepath.Join(dir, "invgate-cli")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_HomeFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	want := filepath.Join(home, ".config", "invgate-cli")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(dir, "invgate-cli", "config.yaml")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestLoad_WithEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgYAML := "base_url: https://file.example.com\noutput: yaml\n"
	configDir := filepath.Join(dir, "invgate-cli")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INVGATE_BASE_URL", "https://env.example.com")
	t.Setenv("INVGATE_OUTPUT", "csv")
	t.Setenv("INVGATE_TIMEOUT", "90s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://env.example.com" {
		t.Errorf("BaseURL = %q, env should override", cfg.BaseURL)
	}
	if cfg.Output != "csv" {
		t.Errorf("Output = %q, env should override", cfg.Output)
	}
	if cfg.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v, env should override", cfg.Timeout)
	}
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := &Config{
		BaseURL: "https://api.example.com",
		Output:  "json",
		Timeout: 30 * time.Second,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	configFile := filepath.Join(dir, "invgate-cli", "config.yaml")
	if _, err := os.Stat(configFile); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load should not fail on missing file: %v", err)
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Output = %q, want default %q", cfg.Output, DefaultOutput)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want default %v", cfg.Timeout, DefaultTimeout)
	}
}

// TestDefaultSpecPath exercises DefaultSpecPath which resolves the
// default spec location under the XDG config directory.
func TestDefaultSpecPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := DefaultSpecPath()
	if err != nil {
		t.Fatalf("DefaultSpecPath: %v", err)
	}
	want := filepath.Join(dir, "invgate-cli", "spec.json")
	if got != want {
		t.Errorf("DefaultSpecPath() = %q, want %q", got, want)
	}
}

// --- Error-path tests ---

// corruptYAML is syntactically invalid YAML that viper will refuse to
// parse (colon in a bare value plus an unterminated flow sequence).
const corruptYAML = "base_url: : :\n\tbad: [oops\n"

// TestLoad_CorruptYAML verifies that Load surfaces a parse error rather
// than silently falling back to defaults when the config file exists but
// cannot be parsed.
func TestLoad_CorruptYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configDir := filepath.Join(dir, "invgate-cli")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(corruptYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail on corrupt YAML")
	}
	if !strings.Contains(err.Error(), "could not read config file") {
		t.Errorf("error = %v, want 'could not read config file'", err)
	}
}

// TestLoadFromPath_CorruptYAML exercises the explicit-path loader's
// error branch: any ReadInConfig failure is wrapped and returned.
func TestLoadFromPath_CorruptYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(corruptYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("LoadFromPath should fail on corrupt YAML")
	}
	if !strings.Contains(err.Error(), "could not read config file") {
		t.Errorf("error = %v, want 'could not read config file'", err)
	}
}

// TestLoadFromPath_NonExistent confirms that, unlike Load(), the explicit
// loader treats a missing file as a hard error (no default fallback).
func TestLoadFromPath_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.yaml")
	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("LoadFromPath should fail on a non-existent file")
	}
	if !strings.Contains(err.Error(), "could not read config file") {
		t.Errorf("error = %v, want 'could not read config file'", err)
	}
}

// TestSave_UnwritableDir points XDG at a regular file so MkdirAll on a
// child path fails with "not a directory".
func TestSave_UnwritableDir(t *testing.T) {
	dir := t.TempDir()
	// Create a file and use its path as XDG_CONFIG_HOME; ConfigDir will
	// try to create a directory *inside* this file, which cannot succeed.
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", file)
	cfg := &Config{BaseURL: "https://api.example.com", Output: "json", Timeout: 30 * time.Second}
	err := cfg.Save()
	if err == nil {
		t.Fatal("Save should fail when the config directory cannot be created")
	}
	if !strings.Contains(err.Error(), "could not create config directory") {
		t.Errorf("error = %v, want 'could not create config directory'", err)
	}
}

// TestSaveToPath_UnwritablePath writes to a path that is an existing
// directory; the underlying create/open fails with EISDIR.
func TestSaveToPath_UnwritablePath(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{BaseURL: "https://api.example.com", Output: "json", Timeout: 30 * time.Second}
	err := cfg.SaveToPath(dir) // dir is a directory, not a file
	if err == nil {
		t.Fatal("SaveToPath should fail when the target path is not a regular file")
	}
	if !strings.Contains(err.Error(), "could not write config file") {
		t.Errorf("error = %v, want 'could not write config file'", err)
	}
}

// --- Home-directory resolution error paths ---

// withNoHome unsets both XDG_CONFIG_HOME and HOME (and USERPROFILE on
// Windows) so os.UserHomeDir returns an error, exercising the error
// branches of ConfigDir and every function that depends on it.
func withNoHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "") // Windows
}

func TestConfigDir_HomeError(t *testing.T) {
	withNoHome(t)
	_, err := ConfigDir()
	if err == nil {
		t.Fatal("ConfigDir should fail when HOME is unset")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %v, want home directory error", err)
	}
}

func TestConfigPath_HomeError(t *testing.T) {
	withNoHome(t)
	_, err := ConfigPath()
	if err == nil {
		t.Fatal("ConfigPath should fail when ConfigDir fails")
	}
}

func TestDefaultSpecPath_HomeError(t *testing.T) {
	withNoHome(t)
	_, err := DefaultSpecPath()
	if err == nil {
		t.Fatal("DefaultSpecPath should fail when ConfigDir fails")
	}
}

func TestLoad_ConfigDirError(t *testing.T) {
	withNoHome(t)
	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail when the config path cannot be resolved")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %v", err)
	}
}

func TestSave_ConfigDirError(t *testing.T) {
	withNoHome(t)
	cfg := &Config{BaseURL: "https://api.example.com", Output: "json", Timeout: 30 * time.Second}
	err := cfg.Save()
	if err == nil {
		t.Fatal("Save should fail when ConfigDir fails")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %v", err)
	}
}