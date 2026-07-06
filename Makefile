.PHONY: build test release clean

VERSION := $(shell cat .version 2>/dev/null || echo "0.0.0")

build:
	go build -ldflags "-X github.com/wdelcant/invgate-cli/internal/version.Version=$(VERSION)" -o invgate-cli ./cmd/invgate-cli/

test:
	go test -race -timeout 120s ./...

clean:
	rm -f invgate-cli coverage.out

# Bump version and create a release tag.
# Usage: make release bump=patch|minor|major
release:
	@./scripts/release.sh $(or $(bump),patch)
