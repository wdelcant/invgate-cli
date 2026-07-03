# invgate-cli

A runtime OpenAPI/Swagger CLI. `invgate-cli` parses a Swagger 2.0 or
OpenAPI 3 spec at startup, builds a Cobra command tree from its
operations, and routes each invocation through an OAuth2
client-credentials executor. No code generation step — the spec *is*
the source of truth at runtime.

## Features

- **Runtime spec loading** — Swagger 2.0 (auto-converted to OpenAPI 3
  via `openapi2conv`) and OpenAPI 3.0, JSON or YAML.
- **Dynamic command tree** — one Cobra subcommand per operation, grouped
  by tag, with positional path args and typed query/body flags.
- **OAuth2 client-credentials auth** — credential chain
  (`flags → env → keychain`), token caching in the OS keychain,
  auto-refresh within a 60-second safety window, and one retry on `401`.
- **Output formats** — `json` (pretty in TTY, compact when piped),
  `yaml`, `table`, and `csv`, with `--columns` selection.
- **Static binaries** — `CGO_ENABLED=0`, cross-compiled for
  linux/darwin/windows × amd64/arm64.

## Installation

### Go install

```sh
go install github.com/invgate/invgate-cli/cmd/invgate-cli@latest
```

### Homebrew (once the tap is published)

```sh
brew install invgate/tap/invgate-cli
```

### From source

```sh
git clone https://github.com/invgate/invgate-cli
cd invgate-cli
go build -o invgate-cli ./cmd/invgate-cli
```

### Build with version info

```sh
VERSION=v0.1.0
go build -ldflags "\
  -X github.com/invgate/invgate-cli/internal/version.Version=$VERSION" \
  -o invgate-cli ./cmd/invgate-cli
```

## Quick start

```sh
# First-time configuration (interactive)
invgate-cli setup

# Or non-interactive
invgate-cli setup \
  --base-url https://your.invgate.com \
  --client-id "$INVGATE_CLIENT_ID" \
  --client-secret "$INVGATE_CLIENT_SECRET"

# Discover available commands from the spec
invgate-cli --spec ./invgate-swagger-v2.json --help

# Run an operation
invgate-cli --spec ./invgate-swagger-v2.json assets-lite list

# Pipe-friendly output (compact JSON)
invgate-cli assets-lite list | jq .

# Other formats
invgate-cli --output yaml assets-lite read 1
invgate-cli --output table assets-lite list --columns id,name,status
invgate-cli --output csv assets-lite list > assets.csv
```

## Configuration

`invgate-cli` reads YAML config from the XDG config directory:

- `$XDG_CONFIG_HOME/invgate-cli/config.yaml`, or
- `~/.config/invgate-cli/config.yaml`

```yaml
base_url: https://your.invgate.com
spec_path: ~/.config/invgate-cli/spec.json
output: json       # json | yaml | table | csv
timeout: 30s
```

**Secrets are NEVER written to the config file.** Client ID, client
secret, and the cached access token live in the OS keychain
(macOS Keychain, Windows Credential Manager, or Linux Secret Service)
under the `invgate-cli` service.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `INVGATE_SPEC` | Path to the spec file |
| `INVGATE_BASE_URL` | API base URL override |
| `INVGATE_OUTPUT` | Output format |
| `INVGATE_TIMEOUT` | Request timeout (e.g. `30s`) |
| `INVGATE_CLIENT_ID` | OAuth2 client ID |
| `INVGATE_CLIENT_SECRET` | OAuth2 client secret |

## Authentication

`invgate-cli` resolves credentials in priority order:

1. `--client-id` / `--client-secret` flags
2. `INVGATE_CLIENT_ID` / `INVGATE_CLIENT_SECRET` environment variables
3. OS keychain entries (`invgate-cli/client-id`, `invgate-cli/client-secret`)

The first non-empty source wins. The token endpoint comes from the spec's
OAuth2 `clientCredentials` `tokenUrl`. Tokens are cached as
`{"token":"...","expiry":"..."}` in the keychain and reused while
`expiry - 60s > now`. A `401` response triggers a single token refresh
and retry.

```sh
# Clear all stored credentials and tokens
invgate-cli logout
```

## Global flags

| Flag | Description |
|------|-------------|
| `--spec` | Path to the OpenAPI/Swagger spec file |
| `--base-url` | API base URL override |
| `--output` | Output format: `json`, `yaml`, `table`, `csv` |
| `--timeout` | HTTP request timeout (e.g. `30s`) |
| `--verbose` | Print request/response details on errors |
| `--client-id` | OAuth2 client ID |
| `--client-secret` | OAuth2 client secret |
| `--compact` | Force compact JSON (no indentation/color) |
| `--columns` | Restrict/reorder columns for `table`/`csv` output |
| `--scope` | OAuth2 scopes (default: `write`) |
| `--version`, `-v` | Print version, commit, and build date |

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Runtime error (auth, network, API) |
| `2` | Usage error (bad flags/args) |

## Development

```sh
# Run the full test suite (race detector + coverage)
go test -race -timeout 120s -coverpkg=./internal/... ./internal/... ./tests/

# Vet
go vet ./...

# Build a local binary
go build -o ./invgate-cli ./cmd/invgate-cli
```

### Project layout

```
invgate-cli/
├── cmd/invgate-cli/main.go        # entry-point shim
├── internal/
│   ├── cli/         cli.go        # root command assembly, RunOperation
│   ├── spec/        loader.go     # Swagger2 → OAS3 loader
│   ├── commands/    builder.go naming.go flags.go
│   ├── auth/        manager.go resolver.go keyring.go oskeyring.go
│   ├── client/      executor.go request.go
│   ├── output/      formatter.go json.go yaml.go table.go csv.go
│   ├── config/      config.go setup.go
│   ├── errors/      errors.go    # AppError
│   └── version/      version.go   # ldflags
├── tests/           integration_test.go
└── .goreleaser.yaml
```

## License

[MIT](LICENSE) — Copyright (c) 2026 InvGate