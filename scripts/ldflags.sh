#!/usr/bin/env bash
# Shared version ldflags computation for Makefile and shell scripts.
#
# Source to export VERSION, GIT_COMMIT, BUILD_DATE into the environment.
# Pipe through stdout to get the -X ldflags string directly.
#
# Override via env vars:
#   VERSION=foo GIT_COMMIT=bar BUILD_DATE=baz make build
#   VERSION=foo ./deploy/scripts/build-deploy-package.sh
#
# When sourced, exports: VERSION GIT_COMMIT BUILD_DATE LDFLAGS_VERSION
# When piped, prints: -X 'github.com/.../Version=...' -X '...'

set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

LDFLAGS_VERSION="-X 'github.com/Togather-Foundation/server/cmd/server/cmd.Version=${VERSION}' -X 'github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit=${GIT_COMMIT}' -X 'github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate=${BUILD_DATE}'"

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    # Running as a script: print ldflags to stdout
    echo "${LDFLAGS_VERSION}"
fi
