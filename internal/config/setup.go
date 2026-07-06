package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wdelcant/invgate-cli/internal/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/term"
)

// SetupFlags holds the non-interactive setup parameters.
// When all required fields are populated, no prompts are shown.
type SetupFlags struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	Output       string
	TokenURL     string
	SpecURL      string
}

// SetupOptions configures how RunSetup operates.
type SetupOptions struct {
	Interactive bool
	In          io.Reader
	Out         io.Writer
	Flags       SetupFlags
	Keyring     auth.Keyring
	HTTPClient  *http.Client
}

// DefaultSpecURL is the default path appended to the instance URL to download the spec.
const specPathSuffix = "/public-api/swagger/v2/?format=openapi"

// RunSetup guides the user through configuring invgate-cli.
// In interactive mode it downloads the spec, then prompts for base URL,
// credentials, and output format.
// In non-interactive mode it validates that required flags are set.
// On completion it writes the config file and stores secrets in the
// keychain, then tests the connection.
func RunSetup(opts SetupOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}

	cfg := defaults()
	if opts.Flags.Output != "" {
		cfg.Output = opts.Flags.Output
	}

	specURL := opts.Flags.SpecURL

	if opts.Interactive {
		scanner := bufio.NewScanner(opts.In)
		scanner.Split(bufio.ScanLines)

		// Step 1: Ask for InvGate instance URL, then derive everything from it.
		instanceURL, err := promptURL(opts.Out, scanner, "InvGate instance URL", opts.Flags.BaseURL)
		if err != nil {
			return err
		}
		instanceURL = strings.TrimRight(instanceURL, "/")

		// Build spec URL from instance URL and download spec.
		specURL := fmt.Sprintf("%s%s", instanceURL, specPathSuffix)
		fmt.Fprintf(opts.Out, "Downloading API spec from %s ...\n", specURL)
		specPath, err := downloadSpec(specURL)
		if err != nil {
			fmt.Fprintf(opts.Out, "warning: could not download spec: %v\n", err)
			fmt.Fprintln(opts.Out, "You can specify a local spec file with --spec or set INVGATE_SPEC.")
		} else {
			cfg.SpecPath = specPath
			fmt.Fprintf(opts.Out, "✓ Spec downloaded to %s\n\n", specPath)
		}

		// Derive base URL from instance URL.
		cfg.BaseURL = instanceURL + "/public-api/v2"

		// Step 2: Credentials.
		clientID, err := promptSecret(opts.Out, opts.In, scanner, "Client ID")
		if err != nil {
			return err
		}
		clientSecret, err := promptSecret(opts.Out, opts.In, scanner, "Client Secret")
		if err != nil {
			return err
		}
		opts.Flags.ClientID = clientID
		opts.Flags.ClientSecret = clientSecret

		// Derive token URL from instance URL.
		opts.Flags.TokenURL = instanceURL + "/oauth2/token/"
		cfg.Output, err = promptString(opts.Out, scanner, "Default output format (json, yaml, table, csv, record)", cfg.Output)
		if err != nil {
			return err
		}
		if cfg.Output == "" {
			cfg.Output = DefaultOutput
		}
	} else {
		// Non-interactive: all required flags must be present.
		if opts.Flags.BaseURL == "" {
			return fmt.Errorf("--base-url is required")
		}
		if opts.Flags.ClientID == "" {
			return fmt.Errorf("--client-id is required")
		}
		if opts.Flags.ClientSecret == "" {
			return fmt.Errorf("--client-secret is required")
		}
		cfg.BaseURL = opts.Flags.BaseURL
		// Download spec in non-interactive mode too.
		if specURL != "" {
			specPath, err := downloadSpec(specURL)
			if err != nil {
				fmt.Fprintf(opts.Out, "warning: could not download spec: %v\n", err)
			} else {
				cfg.SpecPath = specPath
			}
		}
	}

	// Persist config YAML (no secrets).
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}

	// Store secrets in keychain (best-effort). Falls back to file
	// when the OS keychain is unavailable (headless Linux, Docker, CI).
	if opts.Keyring != nil {
		if err := opts.Keyring.Set(auth.ServiceName, auth.KeyClientID, opts.Flags.ClientID); err != nil {
			fmt.Fprintf(opts.Out, "warning: could not store client-id in keychain, using file fallback\n")
		}
		if err := opts.Keyring.Set(auth.ServiceName, auth.KeyClientSecret, opts.Flags.ClientSecret); err != nil {
			fmt.Fprintf(opts.Out, "warning: could not store client-secret in keychain, using file fallback\n")
		}
	}
	// Always write file fallback as backup.
	resolver := auth.NewCredentialResolver(opts.Keyring)
	if err := resolver.StoreCredentials(opts.Flags.ClientID, opts.Flags.ClientSecret); err != nil {
		fmt.Fprintf(opts.Out, "warning: could not store credentials: %v\n", err)
	}

	// Optional connection test. Non-fatal: warn on failure.
	if err := testConnection(opts, cfg.BaseURL); err != nil {
		fmt.Fprintf(opts.Out, "warning: connection test failed: %v\n", err)
	} else {
		fmt.Fprintln(opts.Out, "Setup complete. Connection test succeeded.")
	}
	return nil
}

// promptString prints a label and reads a single-line response from the
// shared scanner. If the user enters an empty line the default is returned.
func promptString(out io.Writer, scanner *bufio.Scanner, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	if scanner.Scan() {
		v := strings.TrimSpace(scanner.Text())
		if v == "" {
			return def, nil
		}
		return v, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("could not read input: %w", err)
	}
	return def, nil
}

// promptSecret reads a line of input without echoing characters to the
// terminal. Falls back to the shared scanner when stdin is not a terminal
// (e.g. piped input in CI).
func promptSecret(out io.Writer, in io.Reader, scanner *bufio.Scanner, label string) (string, error) {
	fmt.Fprintf(out, "%s: ", label)

	f, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		// Fall back to the shared scanner for non-TTY (CI, piped).
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text()), nil
		}
		return "", scanner.Err()
	}

	secret, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(out) // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("could not read secret: %w", err)
	}
	return strings.TrimSpace(string(secret)), nil
}
func promptURL(out io.Writer, scanner *bufio.Scanner, label, def string) (string, error) {
	for {
		v, err := promptString(out, scanner, label, def)
		if err != nil {
			return "", err
		}
		if v == "" {
			fmt.Fprintln(out, "  error: URL is required")
			continue
		}
		if !isValidURL(v) {
			fmt.Fprintf(out, "  error: %q is not a valid URL. Please try again.\n", v)
			continue
		}
		return v, nil
	}
}

// isValidURL returns true if v is a parseable HTTP(S) URL.
func isValidURL(v string) bool {
	if v == "" {
		return false
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return false
	}
	req, err := http.NewRequest("GET", v, nil)
	if err != nil {
		return false
	}
	_ = req
	return true
}

// testConnection verifies the configured credentials end-to-end:
//  1. Uses the provided client-id/client-secret to fetch an OAuth2
//     client-credentials token from the token URL.
//  2. Calls a simple API endpoint (/asset-types/) with that token.
//
// If either step fails, an error describing which step failed is
// returned. Failures are non-fatal — RunSetup reports them as
// warnings rather than aborting setup.
func testConnection(opts SetupOptions, baseURL string) error {
	tokenURL := opts.Flags.TokenURL
	clientID := opts.Flags.ClientID
	clientSecret := opts.Flags.ClientSecret
	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return fmt.Errorf("skipped: token URL or credentials missing")
	}

	var client *http.Client
	if opts.HTTPClient != nil {
		client = opts.HTTPClient
	} else {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Step 1: fetch an OAuth2 token using client_credentials grant.
	cfg := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
	}
	tokCtx := context.WithValue(ctx, oauth2.HTTPClient, client)
	tok, err := cfg.Token(tokCtx)
	if err != nil {
		return fmt.Errorf("token fetch failed: %w", err)
	}

	// Step 2: call a simple authenticated endpoint with the token.
	apiURL := strings.TrimRight(baseURL, "/") + "/asset-types/"
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("could not build API request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API call returned status %d", resp.StatusCode)
	}
	return nil
}

// downloadSpec fetches the spec from the given URL and saves it to the
// default spec path (~/.config/invgate-cli/spec.json). The config
// directory is created if needed. Returns the saved path on success.
func downloadSpec(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("spec download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("spec download returned status %d", resp.StatusCode)
	}

	specPath, err := DefaultSpecPath()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(specPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not create config directory: %w", err)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB max
	if err != nil {
		return "", fmt.Errorf("could not read spec body: %w", err)
	}

	if err := os.WriteFile(specPath, data, 0o600); err != nil {
		return "", fmt.Errorf("could not write spec file: %w", err)
	}

	return specPath, nil
}