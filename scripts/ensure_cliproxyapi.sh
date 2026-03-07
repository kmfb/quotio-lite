#!/usr/bin/env bash
set -euo pipefail

# Inspired by robust installer/update patterns from widely-used CLI projects,
# especially Helm (scripts/get-helm-3) and k3d (install.sh):
# - explicit OS/arch detection
# - release/tag resolution
# - checksum verification before install
# - fail-fast with clear logs

STRICT=0
MODE="ensure" # ensure | check | update
REQUESTED_VERSION=""
FORCE=0

if [[ $# -gt 0 ]]; then
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --strict)
        STRICT=1
        ;;
      --check)
        MODE="check"
        ;;
      --update)
        MODE="update"
        ;;
      --latest)
        REQUESTED_VERSION="latest"
        ;;
      --version)
        shift
        if [[ $# -eq 0 ]]; then
          echo "[cliproxy] --version requires a value (e.g. --version v6.8.47)" >&2
          exit 1
        fi
        REQUESTED_VERSION="$1"
        ;;
      --force)
        FORCE=1
        ;;
      --help|-h)
        cat <<'USAGE'
Usage:
  ensure_cliproxyapi.sh [--strict]
  ensure_cliproxyapi.sh --check
  ensure_cliproxyapi.sh --update [--latest | --version vX.Y.Z] [--force]

Modes:
  default    Ensure CLIProxyAPI exists (copy from candidates or optional URL).
  --check    Print current + latest release info.
  --update   Download and install a release from GitHub.

Environment:
  QUOTIO_LITE_CLIPROXY_PATH          Target binary path (default ~/.quotio-lite/bin/CLIProxyAPI)
  QUOTIO_LITE_CLIPROXY_VERSION_DIR   Versioned binary dir (default ~/.quotio-lite/bin/versions)
  QUOTIO_LITE_CLIPROXY_GITHUB_REPO   Release source repo (default router-for-me/CLIProxyAPI)
  QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL  Override download URL (ensure mode fallback)
  QUOTIO_LITE_CLIPROXY_SHA256        Expected SHA256 for override download URL
USAGE
        exit 0
        ;;
      *)
        echo "[cliproxy] unknown argument: $1" >&2
        exit 1
        ;;
    esac
    shift
  done
fi

DEFAULT_TARGET="$HOME/.quotio-lite/bin/CLIProxyAPI"
TARGET="${QUOTIO_LITE_CLIPROXY_PATH:-$DEFAULT_TARGET}"
TARGET_DIR="$(dirname "$TARGET")"
VERSION_DIR="${QUOTIO_LITE_CLIPROXY_VERSION_DIR:-$HOME/.quotio-lite/bin/versions}"
DOWNLOAD_URL_OVERRIDE="${QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL:-}"
EXPECTED_SHA256="${QUOTIO_LITE_CLIPROXY_SHA256:-}"
GITHUB_REPO="${QUOTIO_LITE_CLIPROXY_GITHUB_REPO:-router-for-me/CLIProxyAPI}"

log() {
  printf '[cliproxy] %s\n' "$1"
}

to_lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[cliproxy] required command missing: $cmd" >&2
    exit 1
  fi
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

normalize_tag() {
  local raw="${1:-}"
  if [[ -z "$raw" ]]; then
    return 0
  fi
  if [[ "$raw" == latest ]]; then
    echo "latest"
    return 0
  fi
  if [[ "$raw" == v* ]]; then
    echo "$raw"
  else
    echo "v$raw"
  fi
}

detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) echo "darwin" ;;
    linux) echo "linux" ;;
    mingw*|msys*|cygwin*) echo "windows" ;;
    *)
      echo "unsupported os: $os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported arch: $arch" >&2
      exit 1
      ;;
  esac
}

http_get_text() {
  local url="$1"
  if has_cmd curl; then
    curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location "$url"
  elif has_cmd wget; then
    wget -q -O - "$url"
  else
    echo "[cliproxy] curl or wget is required" >&2
    exit 1
  fi
}

http_get_file() {
  local url="$1"
  local out="$2"
  if has_cmd curl; then
    curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location "$url" -o "$out"
  elif has_cmd wget; then
    wget -q -O "$out" "$url"
  else
    echo "[cliproxy] curl or wget is required" >&2
    exit 1
  fi
}

sha256_file() {
  local path="$1"
  if has_cmd shasum; then
    shasum -a 256 "$path" | awk '{print $1}'
  elif has_cmd sha256sum; then
    sha256sum "$path" | awk '{print $1}'
  else
    echo "[cliproxy] shasum/sha256sum is required for checksum verification" >&2
    exit 1
  fi
}

extract_version_from_binary() {
  local bin="$1"
  if [[ ! -x "$bin" ]]; then
    return 0
  fi
  local out ver
  out="$($bin 2>&1 || true)"
  ver="$(printf '%s\n' "$out" | sed -n 's/.*CLIProxyAPI Version:[[:space:]]*\([^,[:space:]]*\).*/\1/p' | head -n1)"
  if [[ -n "$ver" ]]; then
    echo "$ver"
  fi
}

resolve_latest_tag() {
  local tag=""
  if has_cmd gh; then
    tag="$(gh api "repos/${GITHUB_REPO}/releases/latest" --jq '.tag_name' 2>/dev/null || true)"
  fi

  if [[ -z "$tag" ]]; then
    local url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local payload
    payload="$(http_get_text "$url")"
    tag="$(printf '%s' "$payload" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  fi

  if [[ -z "$tag" ]]; then
    echo "[cliproxy] failed to resolve latest tag from ${GITHUB_REPO}" >&2
    exit 1
  fi
  echo "$tag"
}

release_json_for_tag() {
  local tag="$1"
  local out="$2"

  if has_cmd gh; then
    if gh api "repos/${GITHUB_REPO}/releases/tags/${tag}" > "$out" 2>/dev/null; then
      return 0
    fi
  fi

  http_get_file "https://api.github.com/repos/${GITHUB_REPO}/releases/tags/${tag}" "$out"
}

asset_url_from_release_json() {
  local json_path="$1"
  local asset_name="$2"

  if has_cmd jq; then
    jq -r --arg n "$asset_name" '.assets[] | select(.name==$n) | .browser_download_url' "$json_path" | head -n1
    return
  fi

  if has_cmd python3; then
    python3 - "$json_path" "$asset_name" <<'PY'
import json, sys
jpath, name = sys.argv[1], sys.argv[2]
with open(jpath, 'r', encoding='utf-8') as f:
    data = json.load(f)
for a in data.get('assets', []):
    if a.get('name') == name:
        print(a.get('browser_download_url', ''))
        break
PY
    return
  fi

  echo "[cliproxy] need jq or python3 to parse release metadata" >&2
  exit 1
}

version_key() {
  # normalize for sort -V: strip leading v
  printf '%s' "${1#v}"
}

check_for_existing_binary() {
  if [[ -x "$TARGET" ]]; then
    log "found: $TARGET"
    return 0
  fi

  local candidates=(
    "$HOME/.quotio-lite/bin/CLIProxyAPI"
    "$HOME/Library/Application Support/Quotio/CLIProxyAPI"
    "$HOME/Library/Application Support/CLIProxyAPI/CLIProxyAPI"
    "/Applications/Quotio.app/Contents/MacOS/CLIProxyAPI"
  )

  mkdir -p "$TARGET_DIR"
  for candidate in "${candidates[@]}"; do
    if [[ -x "$candidate" ]]; then
      if [[ "$candidate" == "$TARGET" ]]; then
        log "found: $TARGET"
        return 0
      fi
      cp "$candidate" "$TARGET"
      chmod +x "$TARGET"
      log "copied from: $candidate -> $TARGET"
      return 0
    fi
  done

  return 1
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
}

install_from_download_url_override() {
  local tmp_file expected actual
  tmp_file="$(mktemp)"
  trap 'rm -f "${tmp_file:-}"' RETURN

  log "downloading from QUOTIO_LITE_CLIPROXY_DOWNLOAD_URL"
  http_get_file "$DOWNLOAD_URL_OVERRIDE" "$tmp_file"

  if [[ -n "$EXPECTED_SHA256" ]]; then
    expected="$(to_lower "$EXPECTED_SHA256")"
    actual="$(to_lower "$(sha256_file "$tmp_file")")"
    if [[ "$actual" != "$expected" ]]; then
      echo "[cliproxy] sha256 mismatch" >&2
      echo "[cliproxy] expected: $expected" >&2
      echo "[cliproxy] actual:   $actual" >&2
      exit 1
    fi
  fi

  mkdir -p "$TARGET_DIR"

  local extracted tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -f "${tmp_file:-}"; rm -rf "${tmp_dir:-}"' RETURN

  extracted=""
  case "$DOWNLOAD_URL_OVERRIDE" in
    *.tar.gz|*.tgz)
      tar -xzf "$tmp_file" -C "$tmp_dir"
      extracted="$(find "$tmp_dir" -type f \( -name 'CLIProxyAPI*' -o -name 'cli-proxy-api*' \) | head -n1 || true)"
      ;;
    *.zip)
      require_cmd unzip
      unzip -q "$tmp_file" -d "$tmp_dir"
      extracted="$(find "$tmp_dir" -type f \( -name 'CLIProxyAPI*' -o -name 'cli-proxy-api*' \) | head -n1 || true)"
      ;;
    *)
      extracted="$tmp_file"
      ;;
  esac

  if [[ -z "$extracted" || ! -f "$extracted" ]]; then
    echo "[cliproxy] override download does not contain CLIProxyAPI executable" >&2
    exit 1
  fi

  install -m 755 "$extracted" "$TARGET"
  log "installed to: $TARGET"
}

install_release() {
  local tag="$1"
  local os arch version_no_v ext asset_name

  os="$(detect_os)"
  arch="$(detect_arch)"
  version_no_v="${tag#v}"

  if [[ "$os" == "windows" ]]; then
    ext="zip"
    asset_name="CLIProxyAPI_${version_no_v}_${os}_${arch}.zip"
  else
    ext="tar.gz"
    asset_name="CLIProxyAPI_${version_no_v}_${os}_${arch}.tar.gz"
  fi

  local tmp_dir release_json asset_url checksums_url asset_file checksums_file
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir:-}"' RETURN

  asset_file="$tmp_dir/$asset_name"
  checksums_file="$tmp_dir/checksums.txt"

  if has_cmd gh; then
    log "downloading $asset_name via gh release"
    gh release download "$tag" --repo "$GITHUB_REPO" --pattern "$asset_name" --dir "$tmp_dir" --clobber >/dev/null
    # checksums.txt may not exist in all releases, so this is best-effort
    gh release download "$tag" --repo "$GITHUB_REPO" --pattern "checksums.txt" --dir "$tmp_dir" --clobber >/dev/null 2>&1 || true
  else
    release_json="$tmp_dir/release.json"
    release_json_for_tag "$tag" "$release_json"

    asset_url="$(asset_url_from_release_json "$release_json" "$asset_name")"
    if [[ -z "$asset_url" || "$asset_url" == "null" ]]; then
      echo "[cliproxy] asset not found in release $tag: $asset_name" >&2
      exit 1
    fi

    checksums_url="$(asset_url_from_release_json "$release_json" "checksums.txt")"

    log "downloading $asset_name"
    http_get_file "$asset_url" "$asset_file"

    if [[ -n "$checksums_url" && "$checksums_url" != "null" ]]; then
      http_get_file "$checksums_url" "$checksums_file"
    fi
  fi

  if [[ ! -f "$asset_file" ]]; then
    echo "[cliproxy] failed to download $asset_name" >&2
    exit 1
  fi

  local expected_sha actual_sha
  expected_sha=""
  if [[ -n "$EXPECTED_SHA256" ]]; then
    expected_sha="$EXPECTED_SHA256"
  elif [[ -f "$checksums_file" ]]; then
    expected_sha="$(grep -E "[[:space:]]${asset_name}$" "$checksums_file" | awk '{print $1}' | head -n1 || true)"
  fi

  if [[ -n "$expected_sha" ]]; then
    actual_sha="$(sha256_file "$asset_file")"
    if [[ "$(to_lower "$actual_sha")" != "$(to_lower "$expected_sha")" ]]; then
      echo "[cliproxy] checksum verification failed for $asset_name" >&2
      echo "[cliproxy] expected: $expected_sha" >&2
      echo "[cliproxy] actual:   $actual_sha" >&2
      exit 1
    fi
  else
    log "warning: checksum not verified (no checksums.txt and no QUOTIO_LITE_CLIPROXY_SHA256)"
  fi

  local extract_dir binary_path
  extract_dir="$tmp_dir/extracted"
  mkdir -p "$extract_dir"

  if [[ "$ext" == "zip" ]]; then
    require_cmd unzip
    unzip -q "$asset_file" -d "$extract_dir"
  else
    tar -xzf "$asset_file" -C "$extract_dir"
  fi

  binary_path="$(find "$extract_dir" -type f \( -name 'CLIProxyAPI*' -o -name 'cli-proxy-api*' \) | head -n1 || true)"
  if [[ -z "$binary_path" || ! -f "$binary_path" ]]; then
    echo "[cliproxy] failed to locate CLIProxyAPI binary after extracting $asset_name" >&2
    exit 1
  fi

  mkdir -p "$VERSION_DIR" "$TARGET_DIR"

  local versioned_binary symlink_tmp
  versioned_binary="$VERSION_DIR/CLIProxyAPI_${version_no_v}_${os}_${arch}"
  install -m 755 "$binary_path" "$versioned_binary"

  symlink_tmp="${TARGET}.new"
  ln -sfn "$versioned_binary" "$symlink_tmp"
  mv -f "$symlink_tmp" "$TARGET"

  log "installed $tag -> $versioned_binary"
  log "active target -> $TARGET"
}

mode_check() {
  local current_version latest_tag
  if [[ -x "$TARGET" ]]; then
    current_version="$(extract_version_from_binary "$TARGET")"
  else
    current_version=""
  fi

  log "target: $TARGET"
  if [[ -n "$current_version" ]]; then
    log "current: $current_version"
  else
    log "current: (not installed or version unreadable)"
  fi

  if latest_tag="$(resolve_latest_tag 2>/dev/null)"; then
    log "latest: $latest_tag"
    if [[ -n "$current_version" ]]; then
      local cv lv
      cv="$(version_key "$current_version")"
      lv="$(version_key "$latest_tag")"
      if [[ "$cv" == "$lv" ]]; then
        log "status: up-to-date"
      else
        local higher
        higher="$(printf '%s\n%s\n' "$cv" "$lv" | sort -V | tail -n1)"
        if [[ "$higher" == "$cv" ]]; then
          log "status: current appears newer than latest release tag"
        else
          log "status: update available"
        fi
      fi
    fi
  else
    log "latest: (failed to resolve)"
  fi
}

mode_update() {
  local desired_tag current_version

  if [[ "$REQUESTED_VERSION" == "latest" || -z "$REQUESTED_VERSION" ]]; then
    desired_tag="$(resolve_latest_tag)"
  else
    desired_tag="$(normalize_tag "$REQUESTED_VERSION")"
  fi

  current_version="$(extract_version_from_binary "$TARGET" || true)"
  if [[ "$FORCE" -eq 0 && -n "$current_version" ]]; then
    if [[ "$(version_key "$current_version")" == "$(version_key "$desired_tag")" ]]; then
      log "already on requested version: $current_version"
      return 0
    fi
  fi

  install_release "$desired_tag"

  local installed_version
  installed_version="$(extract_version_from_binary "$TARGET" || true)"
  if [[ -n "$installed_version" ]]; then
    log "active binary version: $installed_version"
  fi
}

mode_ensure() {
  if check_for_existing_binary; then
    return 0
  fi

  if [[ -n "$DOWNLOAD_URL_OVERRIDE" ]]; then
    install_from_download_url_override
    return 0
  fi

  warn_missing
}

case "$MODE" in
  ensure) mode_ensure ;;
  check) mode_check ;;
  update) mode_update ;;
  *)
    echo "[cliproxy] unknown mode: $MODE" >&2
    exit 1
    ;;
esac
