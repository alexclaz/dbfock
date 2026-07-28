#!/usr/bin/env bash
# Build, sign (when a usable certificate is installed), and install DBfock on macOS.
# Usage: ./scripts/install-macos.sh
# Optional: APPLE_SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)' ./scripts/install-macos.sh

set -euo pipefail

readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly APP_NAME="DBfock.app"
readonly BUILD_APP="$PROJECT_ROOT/backend/build/bin/dbfock.app"
readonly INSTALL_APP="/Applications/$APP_NAME"

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

confirm() {
  local prompt="$1" answer=''
  [[ "${DBFOCK_YES:-}" == '1' ]] && return 0
  [[ -r /dev/tty ]] || fail "${prompt} Run again from a terminal, or set DBFOCK_YES=1 to approve dependency installation."
  printf '%s [y/N] ' "$prompt" > /dev/tty
  read -r answer < /dev/tty
  [[ "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  fail 'This installer only supports macOS.'
fi

ensure_brew() {
  if ! command -v brew >/dev/null 2>&1; then
    confirm 'Homebrew is required. Install it now?' || fail 'Homebrew is required to install missing dependencies.'
    /bin/bash -c "$(curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    if [[ -x /opt/homebrew/bin/brew ]]; then
      eval "$(/opt/homebrew/bin/brew shellenv)"
    elif [[ -x /usr/local/bin/brew ]]; then
      eval "$(/usr/local/bin/brew shellenv)"
    fi
  fi
  export PATH="$(brew --prefix)/bin:$PATH"
}

ensure_go() {
  local go_version=''
  local installed_with_brew=false
  if command -v go >/dev/null 2>&1; then
    go_version="$(go env GOVERSION 2>/dev/null || true)"
  fi

  if [[ ! "$go_version" =~ ^go1\.(2[0-9]|[3-9][0-9])\. ]]; then
    ensure_brew
    confirm 'Go 1.20+ is required to download the project toolchain. Install Go with Homebrew?' || fail 'Go is required to build DBfock.'
    printf 'Installing Go with Homebrew...\n'
    HOMEBREW_NO_INSTALL_CLEANUP=1 brew install go
    installed_with_brew=true
  fi

  if [[ "$installed_with_brew" == true ]]; then
    export PATH="$(brew --prefix go)/bin:$PATH"
  fi
  go_version="$(go env GOVERSION)"
  [[ "$go_version" =~ ^go1\.(2[0-9]|[3-9][0-9])\. ]] || fail "Go 1.20+ is required (found $go_version)."
  export GOTOOLCHAIN=auto
}

ensure_node() {
  local node_major=''
  local installed_with_brew=false
  if command -v node >/dev/null 2>&1; then
    node_major="$(node --version | sed -E 's/^v([0-9]+).*/\1/')"
  fi

  if [[ ! "$node_major" =~ ^(2[4-9]|[3-9][0-9])$ ]]; then
    ensure_brew
    confirm 'Node.js 24+ is required. Install it with Homebrew?' || fail 'Node.js is required to build DBfock.'
    printf 'Installing Node.js 24 with Homebrew...\n'
    HOMEBREW_NO_INSTALL_CLEANUP=1 brew install node@24
    installed_with_brew=true
  fi

  if [[ "$installed_with_brew" == true ]]; then
    export PATH="$(brew --prefix node@24)/bin:$PATH"
  fi
  node_major="$(node --version | sed -E 's/^v([0-9]+).*/\1/')"
  [[ "$node_major" =~ ^(2[4-9]|[3-9][0-9])$ ]] || fail "Node.js 24+ is required (found $(node --version))."
}

find_signing_identity() {
  local identities identity certificate_type

  if [[ -n "${APPLE_SIGNING_IDENTITY:-}" ]]; then
    printf '%s' "$APPLE_SIGNING_IDENTITY"
    return
  fi

  identities="$(security find-identity -v -p codesigning 2>/dev/null || true)"
  for certificate_type in 'Developer ID Application' 'Apple Development'; do
    identity="$(printf '%s\n' "$identities" | sed -nE "s/.*\"(${certificate_type}: [^\"]+)\".*/\\1/p" | head -n 1)"
    if [[ -n "$identity" ]]; then
      printf '%s' "$identity"
      return
    fi
  done
}

prepare_desktop_icon() {
  mkdir -p "$PROJECT_ROOT/backend/build"
  cp "$PROJECT_ROOT/backend/appicon.png" "$PROJECT_ROOT/backend/build/appicon.png"
}

ensure_go
ensure_node

printf 'Installing JavaScript dependencies...\n'
cd "$PROJECT_ROOT/frontend"
npm ci

printf 'Building DBfock...\n'
cd "$PROJECT_ROOT/backend"
go mod download
prepare_desktop_icon
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 build

[[ -d "$BUILD_APP" ]] || fail "Build completed but did not produce $BUILD_APP."

signing_identity="$(find_signing_identity)"
if [[ -n "$signing_identity" ]]; then
  printf 'Signing with %s...\n' "$signing_identity"
  if [[ "$signing_identity" == Developer\ ID\ Application:* ]]; then
    codesign --force --deep --options runtime --timestamp --sign "$signing_identity" "$BUILD_APP"
  else
    codesign --force --deep --sign "$signing_identity" "$BUILD_APP"
  fi
else
  printf '%s\n' 'No Apple signing certificate found; applying an ad-hoc local signature.'
  printf '%s\n' 'To distribute outside this Mac, install a Developer ID Application certificate and run again.'
  codesign --force --deep --sign - "$BUILD_APP"
fi

codesign --verify --deep --strict --verbose=2 "$BUILD_APP"

printf 'Installing %s in /Applications (administrator password may be requested)...\n' "$APP_NAME"
sudo rm -rf "$INSTALL_APP"
sudo cp -R "$BUILD_APP" "$INSTALL_APP"
sudo chown -R "$(id -u):$(id -g)" "$INSTALL_APP"
xattr -dr com.apple.quarantine "$INSTALL_APP" 2>/dev/null || true

if [[ -n "$signing_identity" ]]; then
  if [[ "$signing_identity" == Developer\ ID\ Application:* ]]; then
    codesign --force --deep --options runtime --timestamp --sign "$signing_identity" "$INSTALL_APP"
  else
    codesign --force --deep --sign "$signing_identity" "$INSTALL_APP"
  fi
else
  codesign --force --deep --sign - "$INSTALL_APP"
fi

codesign --verify --deep --strict --verbose=2 "$INSTALL_APP"

printf '\nInstalled: %s\n' "$INSTALL_APP"
printf 'Open it with: open "%s"\n' "$INSTALL_APP"
