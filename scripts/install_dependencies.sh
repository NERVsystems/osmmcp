#!/usr/bin/env bash
set -euo pipefail

# This script installs Go and pre-fetches all Go modules for offline builds.
# Run this in a setup phase while network access is available.

GO_VERSION="1.24.2"
ARCH="$(dpkg --print-architecture)"
GO_ARCHIVE="go${GO_VERSION}.linux-${ARCH}.tar.gz"

# Install required packages
sudo apt-get update
sudo apt-get install -y curl ca-certificates build-essential git

# Install Go toolchain
curl -fsSL "https://go.dev/dl/${GO_ARCHIVE}" -o "${GO_ARCHIVE}"
sudo tar -C /usr/local -xzf "${GO_ARCHIVE}"
rm "${GO_ARCHIVE}"

# Add Go to PATH for the current session
export PATH="/usr/local/go/bin:${PATH}"

# Pre-download module dependencies so tests can run offline
GOTOOLCHAIN=local go mod download

# Optionally run a quick build/test to verify
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test ./...

