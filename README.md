# invgate-cli

A runtime OpenAPI/Swagger CLI. Parses any Swagger 2.0 or OpenAPI 3 spec at
startup, builds a Cobra command tree from its operations, and routes each
invocation through OAuth2 client-credentials auth. No code generation —
the spec *is* the source of truth at runtime.

## Installation

### macOS / Linux — Homebrew (recommended)

```bash
brew install wdelcant/tap/invgate-cli
```

### Any OS — npm

```bash
npm install -g invgate-cli@latest
```

### macOS / Linux — one-liner

```bash
curl -fsSL https://raw.githubusercontent.com/wdelcant/invgate-cli/main/install.sh | bash
```

### Windows — Scoop (recommended)

```powershell
scoop bucket add invgate https://github.com/wdelcant/scoop-bucket
scoop install invgate-cli
```

### Windows — manual

Download the latest `.zip` from
[Releases](https://github.com/wdelcant/invgate-cli/releases/latest),
extract, and add `invgate-cli.exe` to your `PATH`.

### Go install

```bash
go install github.com/wdelcant/invgate-cli/cmd/invgate-cli@latest
```

## Quick start

```bash
# Interactive setup — downloads the spec, prompts for credentials
invgate-cli setup

# Non-interactive (CI / scripting)
invgate-cli setup \
  --base-url https://your-instance.invgate.net \
  --client-id "$INVGATE_CLIENT_ID" \
  --client-secret "$INVGATE_CLIENT_SECRET"

# Run commands
invgate-cli asset-types list
invgate-cli assets list --output table
invgate-cli assets list --owner-email "user@corp.com" --output table
invgate-cli people list --output table
invgate-cli vendors list
```

**`invgate-cli setup`** prompts for 3 things:

1. Your InvGate instance URL (e.g. `https://your-company.is.cloud.invgate.net`)
2. Client ID
3. Client Secret

Everything else — spec download, base URL, token URL, connection test — is automatic.

## Features

- **Runtime spec loading** — Swagger 2.0 (auto-converted via `openapi2conv`)
  and OpenAPI 3.0, JSON or YAML.
- **Dynamic command tree** — one Cobra subcommand per operation, grouped by
  tag, with positional path args and typed query/body flags.
- **OAuth2 client-credentials auth** — credential chain
  (`flags → env → keychain`), 24h token caching in the OS keychain,
  auto-refresh, and one retry on `401`.
- **Output formats** — `json` (colored in TTY, compact when piped),
  `yaml`, `table`, and `csv`, with `--columns` selection.
- **Cross-platform binaries** — linux/darwin/windows × amd64/arm64, zero
  dependencies.

## Configuration

Config is stored at `~/.config/invgate-cli/config.yaml`:

```yaml
base_url: https://your-instance.invgate.net/public-api/v2
spec_path: ~/.config/invgate-cli/spec.json
output: json
timeout: 30s
```

**Secrets are NEVER written to config.** Client ID, client secret, and
access token live in the OS keychain (macOS Keychain, Windows Credential
Manager, Linux Secret Service).

## Environment variables

| Variable | Purpose |
|----------|---------|
| `INVGATE_SPEC` | Path to spec file |
| `INVGATE_BASE_URL` | API base URL override |
| `INVGATE_OUTPUT` | Output format (`json`, `yaml`, `table`, `csv`) |
| `INVGATE_TIMEOUT` | Request timeout (e.g. `30s`) |
| `INVGATE_CLIENT_ID` | OAuth2 client ID |
| `INVGATE_CLIENT_SECRET` | OAuth2 client secret |

## Authentication

Credentials are resolved in order:

1. `--client-id` / `--client-secret` flags
2. `INVGATE_CLIENT_ID` / `INVGATE_CLIENT_SECRET` env vars
3. OS keychain (set by `invgate-cli setup`)

First non-empty source wins. Tokens last 24h, cached in the keychain,
and auto-refreshed transparently.

```bash
invgate-cli logout   # clear all stored credentials and tokens
```

## Global flags

| Flag | Description |
|------|-------------|
| `--spec` | Path to spec file |
| `--base-url` | API base URL override |
| `--output` | Output format: `json` (default), `yaml`, `table`, `csv` |
| `--timeout` | HTTP request timeout (e.g. `30s`) |
| `--verbose` | Print request/response details on errors |
| `--client-id` | OAuth2 client ID |
| `--client-secret` | OAuth2 client secret |
| `--compact` | Force compact JSON (no indentation/color) |
| `--columns` | Restrict/reorder columns for table/csv output |
| `--version`, `-v` | Print version and build info |

## Examples

```bash
# List asset types
invgate-cli asset-types list

# List assets with filters
invgate-cli assets list --owner-email "user@corp.com" --output table

# Read a specific asset
invgate-cli assets read 42

# List people
invgate-cli people list --output table

# Search assets by keyword
invgate-cli assets list --keyword "MacBook" --output table

# Compact JSON for piping
invgate-cli assets list | jq '.results[] | {id, name}'

# CSV export
invgate-cli assets list --output csv > assets.csv

# YAML output
invgate-cli vendors list --output yaml

# Table with specific columns
invgate-cli people list --output table --columns id,name,email
```

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Runtime error (auth, network, API) |
| `2` | Usage error (bad flags/args) |

## Development

```bash
git clone https://github.com/wdelcant/invgate-cli
cd invgate-cli

# Tests
go test -race -timeout 120s ./...

# Build
go build -o invgate-cli ./cmd/invgate-cli
```

## License

[MIT](LICENSE)
