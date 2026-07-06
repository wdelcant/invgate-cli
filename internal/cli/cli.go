// Package cli wires together the spec loader, command builder, auth
// manager, HTTP executor, and output formatters into a single cobra
// command tree. It is constructed once at program start.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wdelcant/invgate-cli/internal/auth"
	"github.com/wdelcant/invgate-cli/internal/client"
	"github.com/wdelcant/invgate-cli/internal/commands"
	"github.com/wdelcant/invgate-cli/internal/config"
	errs "github.com/wdelcant/invgate-cli/internal/errors"
	"github.com/wdelcant/invgate-cli/internal/output"
	"github.com/wdelcant/invgate-cli/internal/spec"
	"github.com/wdelcant/invgate-cli/internal/version"
	"github.com/spf13/cobra"
)

// Exit codes per the spec.
const (
	ExitOK     = 0
	ExitError  = 1
	ExitUsage  = 2
)

// CLI bundles the runtime components and the assembled cobra root
// command. A single instance is built by New and shared with the
// command tree via the CLIRunner interface.
type CLI struct {
	Root      *cobra.Command
	Config    *config.Config
	SpecDoc   *openapi3.T
	SpecPath  string
	Auth      auth.Manager
	Executor  *client.Executor
	Out       io.Writer
	ErrOut    io.Writer
	Verbose   bool
	OutputFmt string
	Compact   bool
	Columns   []string
	BaseURL   string
	Timeout   time.Duration
	Scope     []string
	ClientID  string
	ClientSecret string
	// Keyring used for auth and setup. Mockable for tests.
	Keyring auth.Keyring
	// HTTPClient override used by auth/executor (mainly for tests).
	HTTPClient *http.Client
}

// New assembles the full CLI. It eagerly resolves the spec from
// --spec flag, INVGATE_SPEC env, config, or the default location so
// the dynamic command tree is available before cobra dispatches.
func New(args []string) *CLI {
	c := &CLI{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	c.Config, _ = config.Load()
	c.OutputFmt = c.Config.Output
	c.BaseURL = c.Config.BaseURL
	c.Timeout = c.Config.Timeout
	c.SpecPath = c.Config.SpecPath

	// Eagerly resolve a spec path from raw args / env, since the
	// dynamic command tree must exist before cobra dispatches.
	if sp := scanSpecArg(args); sp != "" {
		c.SpecPath = sp
	} else if env := os.Getenv("INVGATE_SPEC"); env != "" && c.SpecPath == "" {
		c.SpecPath = env
	}

	// Try to load the spec and build the dynamic command tree up front.
	if c.SpecPath != "" {
		loader := spec.NewFileLoader()
		if doc, err := loader.Load(c.SpecPath); err == nil {
			c.SpecDoc = doc
		}
	}

	c.buildRoot()
	return c
}

// buildRoot constructs the cobra root command with persistent global
// flags, the setup/logout subcommands, --version, the dynamic spec
// commands, and the PersistentPreRunE that initializes auth + executor.
func (c *CLI) buildRoot() {
	root := &cobra.Command{
		Use:   "invgate-cli",
		Short: "Runtime OpenAPI/Swagger CLI",
		Long:  "invgate-cli parses an OpenAPI/Swagger spec at runtime and exposes its operations as a command tree.",
		SilenceUsage: true,
		SilenceErrors: true,
		PersistentPreRunE: c.persistentPreRun,
	}
	root.SetOut(c.Out)
	root.SetErr(c.ErrOut)

	// Global persistent flags.
	root.PersistentFlags().String("spec", "", "Path to the OpenAPI/Swagger spec file")
	root.PersistentFlags().String("base-url", "", "API base URL override")
	root.PersistentFlags().String("output", "", "Output format: json, yaml, table, csv, record")
	root.PersistentFlags().Duration("timeout", 0, "HTTP request timeout (e.g. 30s)")
	root.PersistentFlags().Bool("verbose", false, "Verbose error output (request/response details)")
	root.PersistentFlags().String("client-id", "", "OAuth2 client ID (overrides env/keychain)")
	root.PersistentFlags().String("client-secret", "", "OAuth2 client secret (overrides env/keychain)")
	root.PersistentFlags().Bool("compact", false, "Force compact JSON output (no indentation/color)")
	root.PersistentFlags().StringSlice("columns", nil, "Restrict/reorder columns for table/csv output")
	root.PersistentFlags().StringSlice("scope", nil, "OAuth2 scopes (default: write)")

	root.PersistentFlags().BoolP("version", "v", false, "Print version and exit")

	// Static subcommands.
	root.AddCommand(c.buildSetupCmd())
	root.AddCommand(c.buildLogoutCmd())

	// Dynamic spec-driven command tree.
	if c.SpecDoc != nil {
		runner := &cliRunner{cli: c}
		builder := commands.NewBuilder(c.SpecDoc, runner)
		specRoot := builder.Build()
		// Attach each resource group as a direct subcommand of the root.
		for _, sub := range specRoot.Commands() {
			root.AddCommand(sub)
		}
	}

	c.Root = root
}

// persistentPreRun loads overrides from the parsed flags and
// initializes the auth manager + HTTP executor before any leaf
// command runs. It skips setup/logout and --version.
func (c *CLI) persistentPreRun(cmd *cobra.Command, args []string) error {
	// Skip for static helper commands that don't need auth/spec.
	if cmd.Name() == "setup" || cmd.Name() == "logout" || cmd.Name() == "invgate-cli" {
		return nil
	}
	flags := cmd.Flags()
	if v, err := flags.GetString("spec"); err == nil && v != "" {
		c.SpecPath = v
	}
	if v, err := flags.GetString("base-url"); err == nil && v != "" {
		c.BaseURL = v
	}
	if v, err := flags.GetString("output"); err == nil && v != "" {
		c.OutputFmt = v
	}
	if v, err := flags.GetBool("verbose"); err == nil {
		c.Verbose = v
	}
	if v, err := flags.GetBool("compact"); err == nil {
		c.Compact = v
	}
	if v, err := flags.GetStringSlice("columns"); err == nil {
		c.Columns = v
	}
	if v, err := flags.GetString("client-id"); err == nil && v != "" {
		c.ClientID = v
	}
	if v, err := flags.GetString("client-secret"); err == nil && v != "" {
		c.ClientSecret = v
	}
	if v, err := flags.GetDuration("timeout"); err == nil && v > 0 {
		c.Timeout = v
	}
	if v, err := flags.GetStringSlice("scope"); err == nil && len(v) > 0 {
		c.Scope = v
	}

	// Resolve base URL: flag > env > config > spec host+basePath.
	if c.BaseURL == "" {
		if env := os.Getenv("INVGATE_BASE_URL"); env != "" {
			c.BaseURL = env
		} else if c.SpecDoc != nil {
			c.BaseURL = deriveBaseURL(c.SpecDoc, c.Config)
		}
	}

	// Initialize the auth manager + executor unless this is a no-auth
	// operation (the spec may mark security as empty per-op).
	c.initAuthAndExecutor()
	return nil
}

// initAuthAndExecutor builds the credential resolver, auth manager,
// and HTTP executor. If no token URL can be derived, auth is left nil
// so unauthenticated requests can still be made.
func (c *CLI) initAuthAndExecutor() {
	kr := c.Keyring
	if kr == nil {
		kr = auth.NewOSKeyring()
	}
	resolver := auth.NewCredentialResolver(kr)
	resolver.ClientIDFlag = c.ClientID
	resolver.ClientSecretFlag = c.ClientSecret

	tokenURL := ""
	if c.SpecDoc != nil {
		tokenURL = extractTokenURL(c.SpecDoc)
		// If the token URL from the spec is relative (e.g. /oauth2/token/),
		// resolve it against the server's host, not the API base path.
		if tokenURL != "" && !strings.HasPrefix(tokenURL, "http") {
			base := c.BaseURL
			if base == "" {
				base = deriveBaseURL(c.SpecDoc, c.Config)
			}
			tokenURL = resolveRelativeURL(base, tokenURL)
		}
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
		if c.Timeout > 0 {
			httpClient.Timeout = c.Timeout
		} else {
			httpClient.Timeout = config.DefaultTimeout
		}
	}

	var mgr auth.Manager
	if tokenURL != "" {
		m := auth.NewKeychainManager(resolver, tokenURL, kr)
		m.HTTPClient = httpClient
		if len(c.Scope) > 0 {
			m.Scopes = c.Scope
		}
		mgr = m
	}
	c.Auth = mgr
	c.Executor = client.NewExecutor(c.BaseURL, toExecutorAuth(mgr), httpClient)
	c.Executor.Verbose = c.Verbose
	c.Executor.Logf = func(format string, a ...any) {
		fmt.Fprintf(c.ErrOut, "[verbose] "+format+"\n", a...)
	}
}

// toExecutorAuth adapts auth.Manager to the client.AuthManager
// interface (Token + Login only) the executor expects.
func toExecutorAuth(m auth.Manager) client.AuthManager {
	if m == nil {
		return nil
	}
	return &executorAuthAdapter{m: m}
}

type executorAuthAdapter struct{ m auth.Manager }

func (a *executorAuthAdapter) Token(ctx context.Context) (string, error) {
	return a.m.Token(ctx)
}
func (a *executorAuthAdapter) Login(ctx context.Context) error { return a.m.Login(ctx) }

// cliRunner implements commands.CLIRunner, delegating a generated
// leaf command to the HTTP executor and output formatter.
type cliRunner struct{ cli *CLI }

func (r *cliRunner) RunOperation(cmd *cobra.Command, method, path string, op *openapi3.Operation, sf commands.SpecFlags) error {
	return r.cli.runOperation(cmd, method, path, op, sf)
}

// runOperation is the heart of the CLI: it assembles an HTTP request
// from the cobra flags + operation metadata, executes it, and renders
// the response through the selected output formatter.
func (c *CLI) runOperation(cmd *cobra.Command, method, path string, op *openapi3.Operation, sf commands.SpecFlags) error {
	if c.Executor == nil {
		// No spec-derived executor; persistentPreRun should have set it up.
		return fmt.Errorf("no executor initialized (missing spec?)")
	}

	req := &client.Request{
		Method:       method,
		PathTemplate: path,
	}

	// Path args come from positional args in declaration order.
	if len(sf.PathArgs) > 0 {
		req.PathArgs = cmd.Flags().Args()
		if len(req.PathArgs) == 0 {
			// Cobra may store args via cobra.PositionalArgs; read from cmd.ArgsLenAtDash().
			req.PathArgs = nil
		}
	}

	// Query params from flags.
	q := url.Values{}
	flags := cmd.Flags()
	for i := range sf.QueryFlags {
		fd := &sf.QueryFlags[i]
		if fl := flags.Lookup(fd.Name); fl != nil && fl.Changed {
			val := fl.Value.String()
			switch fd.Type {
			case "stringArray":
				// pflag StringArray Value.String returns "[a,b]" — read via StringSlice interface.
				ss, _ := flags.GetStringSlice(fd.Name)
				for _, s := range ss {
					q.Add(fd.Name, s)
				}
			default:
				if fd.Type == "bool" {
					if b, _ := flags.GetBool(fd.Name); b {
						q.Set(fd.Name, "true")
					}
				} else {
					q.Add(fd.Name, val)
				}
			}
		}
	}
	req.Query = q

	// Body from --data if present.
	if sf.BodyFlag != nil {
		if fl := flags.Lookup(sf.BodyFlag.Name); fl != nil && fl.Changed {
			raw, _ := flags.GetString(sf.BodyFlag.Name)
			if raw != "" {
				if strings.HasPrefix(raw, "@") {
					body, err := c.resolveDataFile(raw)
					if err != nil {
						return err
					}
					req.Body = body
				} else {
					if !json.Valid([]byte(raw)) {
						return fmt.Errorf("invalid JSON in --data")
					}
					req.Body = []byte(raw)
				}
				req.ContentType = "application/json"
			}
		}
	}

	ctx := context.Background()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	resp, err := c.Executor.Execute(ctx, req)
	if err != nil {
		return err
	}

	// Empty / 204 responses.
	if resp == nil || len(resp.Body) == 0 || resp.Status == 204 {
		fmt.Fprintln(c.Out, "OK")
		return nil
	}

	return c.renderOutput(resp.Body)
}

// renderOutput parses the JSON response body and renders it through the
// selected formatter. Adds pagination summary when the response is a
// paginated wrapper {count, next, previous, results}.
func (c *CLI) renderOutput(body []byte) error {
	format := c.OutputFmt
	if format == "" {
		format = config.DefaultOutput
	}
	formatter, err := output.Get(format)
	if err != nil {
		return err
	}

	// Parse once into a generic structure so table/csv formatters can
	// introspect keys. If the body isn't JSON, fall back to raw bytes.
	var data any
	if json.Valid(body) {
		if jerr := json.Unmarshal(body, &data); jerr != nil {
			data = string(body)
		}
	} else {
		data = string(body)
	}

	cfg := output.FormatConfig{
		Columns: c.Columns,
		Compact: c.Compact,
		Color:   output.IsTTY(),
	}
	out, err := formatter.Format(data, cfg)
	if err != nil {
		return err
	}
	_, werr := c.Out.Write(out)
	if werr != nil {
		return werr
	}
	// Ensure a trailing newline for text formats when missing.
	if len(out) > 0 && out[len(out)-1] != '\n' {
		fmt.Fprintln(c.Out)
	}
	// Pagination summary for DRF-style wrappers.
	if wrapper, ok := data.(map[string]any); ok {
		if countRaw, hasCount := wrapper["count"]; hasCount {
			if count, ok := countRaw.(float64); ok {
				results := extractResults(wrapper)
				fmt.Fprintf(c.Out, "(showing %d of %.0f total — use --page N for next pages)\n", len(results), count)
			}
		}
	}
	return nil
}

func extractResults(m map[string]any) []any {
	if r, ok := m["results"]; ok {
		if arr, ok := r.([]any); ok {
			return arr
		}
	}
	return nil
}

// resolveDataFile reads an @file.json reference from --data.
func (c *CLI) resolveDataFile(raw string) ([]byte, error) {
	path := strings.TrimPrefix(raw, "@")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read --data file %s: %w", path, err)
	}
	return data, nil
}

// buildSetupCmd creates the `invgate-cli setup` subcommand.
func (c *CLI) buildSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure invgate-cli (base URL, credentials, output format)",
		Long:  "Downloads the API spec and prompts for base URL, client credentials, and output format. Use --base-url/--client-id/--client-secret for non-interactive setup.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL, _ := cmd.Flags().GetString("base-url")
			clientID, _ := cmd.Flags().GetString("client-id")
			clientSecret, _ := cmd.Flags().GetString("client-secret")
			outFmt, _ := cmd.Flags().GetString("output")
			tokenURL, _ := cmd.Flags().GetString("token-url")
			specURL, _ := cmd.Flags().GetString("spec-url")

			// If no required flags are provided, run interactively.
			interactive := baseURL == "" && clientID == "" && clientSecret == ""

			kr := c.Keyring
			if kr == nil {
				kr = auth.NewOSKeyring()
			}
			opts := config.SetupOptions{
				Interactive: interactive,
				In:          os.Stdin,
				Out:         c.ErrOut,
				Flags: config.SetupFlags{
					BaseURL:      baseURL,
					ClientID:     clientID,
					ClientSecret: clientSecret,
					Output:       outFmt,
					TokenURL:     tokenURL,
					SpecURL:      specURL,
				},
				Keyring: kr,
			}
			if c.HTTPClient != nil {
				opts.HTTPClient = c.HTTPClient
			}
			return config.RunSetup(opts)
		},
	}
	cmd.Flags().String("base-url", "", "API base URL")
	cmd.Flags().String("client-id", "", "OAuth2 client ID")
	cmd.Flags().String("client-secret", "", "OAuth2 client secret")
	cmd.Flags().String("token-url", "", "OAuth2 token URL override")
	cmd.Flags().String("spec-url", "", "Swagger/OpenAPI spec URL to download (default: InvGate public API)")
	return cmd
}

// buildLogoutCmd creates the `invgate-cli logout` subcommand.
func (c *CLI) buildLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear all stored credentials and tokens from the keychain",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr := c.Keyring
			if kr == nil {
				kr = auth.NewOSKeyring()
			}
			resolver := auth.NewCredentialResolver(kr)
			mgr := auth.NewKeychainManager(resolver, "", kr)
			if err := mgr.Logout(); err != nil {
				return err
			}
			fmt.Fprintln(c.Out, "Logged out. All credentials and tokens cleared.")
			return nil
		},
	}
}

// printVersion handles --version, run from main before cobra dispatch.
func (c *CLI) printVersion() {
	fmt.Fprintln(c.Out, version.Info())
}

// Execute runs the cobra root with the given args and returns the exit code.
func (c *CLI) Execute(args []string) int {
	// Handle --version early: cobra would otherwise show help.
	for _, a := range args {
		if a == "--version" || a == "-v" {
			c.printVersion()
			return ExitOK
		}
		if a == "--" {
			break
		}
	}
	c.Root.SetArgs(args)
	if err := c.Root.Execute(); err != nil {
		fmt.Fprintln(c.ErrOut, errs.FormatError(err, c.Verbose))
		// Usage errors (bad flags) are surfaced by cobra with a flag.ErrHelp
		// or a cobra.CommandError; we treat unknown-flag as usage (exit 2).
		if isUsageError(err) {
			return ExitUsage
		}
		return ExitError
	}
	return ExitOK
}

// isUsageError reports whether err is a usage/flag error (exit 2).
func isUsageError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "unknown flag") ||
		strings.Contains(s, "unknown command") ||
		strings.Contains(s, "required flag") ||
		strings.Contains(s, "invalid argument")
}

// scanSpecArg pulls a --spec value out of the raw args before cobra
// parses, so the dynamic tree can be built eagerly.
func scanSpecArg(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--spec" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--spec=") {
			return strings.TrimPrefix(a, "--spec=")
		}
		// Stop scanning at the first non-flag so we don't read into a subcommand's flags.
		// (Spec is a global persistent flag; keep scanning only before subcommand boundaries.)
		if !strings.HasPrefix(a, "-") && a != "" {
			// First positional argument → subcommand boundary; stop.
			return ""
		}
	}
	return ""
}

// extractTokenURL reads the OAuth2 client-credentials tokenUrl from
// the OAS3 document, if present.
func extractTokenURL(doc *openapi3.T) string {
	if doc == nil || doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return ""
	}
	for _, schRef := range doc.Components.SecuritySchemes {
		s := schRef.Value
		if s == nil || s.Flows == nil || s.Flows.ClientCredentials == nil {
			continue
		}
		if s.Flows.ClientCredentials.TokenURL != "" {
			return s.Flows.ClientCredentials.TokenURL
		}
	}
	return ""
}

// resolveRelativeURL resolves a relative path against the scheme and host
// of the base URL. For example, base="https://example.com/api/v2" and
// path="/oauth2/token/" yields "https://example.com/oauth2/token/".
func resolveRelativeURL(baseURL, relPath string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" {
		return baseURL + relPath
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, relPath)
}
func deriveBaseURL(doc *openapi3.T, cfg *config.Config) string {
	if doc != nil && len(doc.Servers) > 0 && doc.Servers[0].URL != "" {
		return strings.TrimRight(doc.Servers[0].URL, "/")
	}
	if cfg != nil {
		return cfg.BaseURL
	}
	return ""
}