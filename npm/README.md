# invgate-cli

Runtime OpenAPI/Swagger CLI for InvGate Asset Management. Parses any Swagger 2.0
or OpenAPI 3 spec at startup and exposes it as a command tree. No code generation.

## Install

```bash
npm install -g invgate-cli@latest
```

Also available via: `brew`, `scoop`, `go install`, or one-liner shell script.

## Usage

```bash
invgate-cli setup              # first time
invgate-cli asset-types list   # list asset types
invgate-cli assets list --output table --owner-email "user@corp.com"
```

## Features

- 5 output formats: json (colored), yaml, table, csv, record
- OAuth2 client credentials with auto-refresh
- OS keychain credential storage (file fallback on headless Linux)
- Pagination support (`--page N`)
- Cross-platform: macOS, Linux, Windows

## Docs

Full documentation at [github.com/wdelcant/invgate-cli](https://github.com/wdelcant/invgate-cli).
