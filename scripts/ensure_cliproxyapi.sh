#!/usr/bin/env bash
set -euo pipefail

STRICT=0
if [[ "${1:-}" == "--strict" ]]; then
  STRICT=1
fi

DEFAULT_TARGET="$HOME/Library/Application Support/Quotio/CLIProxyAPI"
TARGET="${QUOTIO_LITE_CLIPROXY_PATH:-$DEFAULT_TARGET}"
TARGET_DIR="$(dirname "$TARGET")"

DOWNLOAD_URL="${QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL:-}"
EXPECTED_SHA256="${QUOTIO_LITE_CLIPROXY_SHA256:-}"

log() {
  printf '[cliproxy] %s\n' "$1"
}

warn_missing() {
  log "CLIProxyAPI not available at: $TARGET"
  log "login/probe endpoints may fail without it."
  log "Options:"
  log "  1) Install Quotio and keep default path"
  log "  2) Export QUOTIO_LITE_CLIPROXY_PATH to existing binary"
  log "  3) Export QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL for auto-download"
  if [[ "$STRICT" -eq 1 ]]; then
    exit 1
  fi
  return 0
}

if [[ -x "$TARGET" ]]; then
  log "found: $TARGET"
  exit 0
fi

mkdir -p "$TARGET_DIR"

CANDIDATES=(
  "$HOME/Library/Application Support/Quotio/CLIProxyAPI"
  "$HOME/Library/Application Support/CLIProxyAPI/CLIProxyAPI"
  "/Applications/Quotio.app/Contents/MacOS/CLIProxyAPI"
)

for candidate in "${CANDIDATES[@]}"; do
  if [[ -x "$candidate" ]]; then
    if [[ "$candidate" == "$TARGET" ]]; then
      log "found: $TARGET"
      exit 0
    fi
    cp "$candidate" "$TARGET"
    chmod +x "$TARGET"
    log "copied from: $candidate -> $TARGET"
    exit 0
  fi
done

if [[ -n "$DOWNLOAD_URL" ]]; then
  tmp_file="$(mktemp)"
  trap 'rm -f "$tmp_file"' EXIT

  log "downloading from QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL"
  curl -fsSL "$DOWNLOAD_URL" -o "$tmp_file"

  if [[ -n "$EXPECTED_SHA256" ]]; then
    actual_sha="$(shasum -a 256 "$tmp_file" | awk '{print $1}')"
    if [[ "$actual_sha" != "$EXPECTED_SHA256" ]]; then
      log "sha256 mismatch"
      log "expected: $EXPECTED_SHA256"
      log "actual:   $actual_sha"
      exit 1
    fi
  fi

  install -m 755 "$tmp_file" "$TARGET"
  log "downloaded to: $TARGET"
  exit 0
fi

warn_missing
