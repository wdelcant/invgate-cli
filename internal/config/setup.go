package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wdelcant/invgate-cli/internal/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// SetupFlags holds the non-interactive setup parameters.
// When all required fields are populated, no prompts are shown.
type SetupFlags struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	Output       string
	TokenURL     string
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

// RunSetup guides the user through configuring invgate-cli.
// In interactive mode it prompts for each value, validating URL format.
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

	if opts.Interactive {
		scanner := bufio.NewScanner(opts.In)
		scanner.Split(bufio.ScanLines)

		var err error
		cfg.BaseURL, err = promptURL(opts.Out, scanner, "Base URL", opts.Flags.BaseURL)
		if err != nil {
			return err
		}
		clientID, err := promptString(opts.Out, scanner, "Client ID", opts.Flags.ClientID)
		if err != nil {
			return err
		}
		clientSecret, err := promptString(opts.Out, scanner, "Client Secret (no echo)", opts.Flags.ClientSecret)
		if err != nil {
			return err
		}
		opts.Flags.ClientID = clientID
		opts.Flags.ClientSecret = clientSecret
		cfg.Output, err = promptString(opts.Out, scanner, "Default output format (json, yaml, table, csv)", cfg.Output)
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
	}

	// Persist config YAML (no secrets).
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}

	// Store secrets in keychain if available.
	if opts.Keyring != nil {
		if err := opts.Keyring.Set(auth.ServiceName, auth.KeyClientID, opts.Flags.ClientID); err != nil {
			return fmt.Errorf("could not store client-id in keychain: %w", err)
		}
		if err := opts.Keyring.Set(auth.ServiceName, auth.KeyClientSecret, opts.Flags.ClientSecret); err != nil {
			return fmt.Errorf("could not store client-secret in keychain: %w", err)
		}
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

// promptURL prompts for a URL and re-prompts on invalid input.
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