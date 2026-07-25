#!/usr/bin/env bash
# Build and install DBfock for the current Linux user.

set -euo pipefail

readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly INSTALL_BIN="$HOME/.local/bin/dbfock"
readonly DESKTOP_FILE="$HOME/.local/share/applications/dbfock.desktop"
readonly ICON_FILE="$HOME/.local/share/icons/hicolor/512x512/apps/dbfock.png"

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

go_is_current() {
  command -v go >/dev/null 2>&1 && go env GOVERSION 2>/dev/null | grep -Eq '^go1\.(2[0-9]|[3-9][0-9])\.'
}

node_is_current() {
  command -v node >/dev/null 2>&1 && node --version | grep -Eq '^v(2[4-9]|[3-9][0-9])\.'
}

install_go() {
  local architecture release archive temporary_go_directory
  case "$(uname -m)" in
    x86_64) architecture='amd64' ;;
    aarch64|arm64) architecture='arm64' ;;
    *) fail "Unsupported Linux architecture: $(uname -m)." ;;
  esac

  confirm 'Go 1.20+ is required to download the project toolchain. Install the current stable Go release for this user?' || fail 'Go is required to build DBfock.'
  release="$(curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error 'https://go.dev/dl/?mode=json')"
  archive="$(printf '%s\n' "$release" | grep -oE "\"filename\": ?\"go[0-9.]+\.linux-${architecture}\.tar\.gz\"" | head -n 1 | cut -d '"' -f 4 || true)"
  [[ -n "$archive" ]] || fail 'Could not find a stable Go download for this architecture.'

  temporary_go_directory="$(mktemp -d "${TMPDIR:-/tmp}/dbfock-go.XXXXXX")"
  curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error \
    "https://go.dev/dl/$archive" --output "$temporary_go_directory/$archive"
  tar -xzf "$temporary_go_directory/$archive" -C "$temporary_go_directory"

  mkdir -p "$HOME/.local/lib"
  rm -rf "$HOME/.local/lib/dbfock-go"
  mv "$temporary_go_directory/go" "$HOME/.local/lib/dbfock-go"
  rm -rf "$temporary_go_directory"
  export PATH="$HOME/.local/lib/dbfock-go/bin:$PATH"
}

install_node() {
  local nvm_directory="$HOME/.local/share/dbfock/nvm" installer
  confirm 'Node.js 24+ is required. Install Node.js 24 for this user?' || fail 'Node.js is required to build DBfock.'
  mkdir -p "$(dirname "$nvm_directory")"
  if [[ ! -s "$nvm_directory/nvm.sh" ]]; then
    installer="$(mktemp "${TMPDIR:-/tmp}/dbfock-nvm.XXXXXX")"
    curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error \
      https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh --output "$installer"
    PROFILE=/dev/null NVM_DIR="$nvm_directory" bash "$installer"
    rm -f "$installer"
  fi
  # shellcheck disable=SC1090
  source "$nvm_directory/nvm.sh"
  nvm install 24
  nvm use 24
}

install_linux_build_dependencies() {
  WAILS_TAGS=''
  if command -v pkg-config >/dev/null 2>&1; then
    if pkg-config --exists gtk+-3.0 webkit2gtk-4.1; then
      WAILS_TAGS='webkit2_41'
      return
    elif pkg-config --exists gtk+-3.0 webkit2gtk-4.0; then
      return
    fi
  fi

  if command -v apt-get >/dev/null 2>&1; then
    if apt-cache show libwebkit2gtk-4.1-dev >/dev/null 2>&1; then
      WAILS_TAGS='webkit2_41'
      linux_packages=(build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev)
    else
      linux_packages=(build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.0-dev)
    fi
    confirm "Install Linux build dependencies: ${linux_packages[*]}?" || fail 'Linux desktop dependencies are required.'
    sudo apt-get update
    sudo apt-get install -y "${linux_packages[@]}"
  elif command -v dnf >/dev/null 2>&1; then
    WAILS_TAGS='webkit2_41'
    linux_packages=(gcc pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel)
    confirm "Install Linux build dependencies: ${linux_packages[*]}?" || fail 'Linux desktop dependencies are required.'
    sudo dnf install -y "${linux_packages[@]}"
  elif command -v pacman >/dev/null 2>&1; then
    WAILS_TAGS='webkit2_41'
    linux_packages=(base-devel pkgconf gtk3 webkit2gtk-4.1)
    confirm "Install Linux build dependencies: ${linux_packages[*]}?" || fail 'Linux desktop dependencies are required.'
    sudo pacman -S --needed "${linux_packages[@]}"
  else
    fail 'Unsupported Linux package manager. Install GCC, pkg-config, GTK3, and WebKit2GTK, then run this installer again.'
  fi
}

[[ "$(uname -s)" == 'Linux' ]] || fail 'This installer only supports Linux.'
command -v curl >/dev/null 2>&1 || fail 'curl is required to install missing build tools.'

go_is_current || install_go
go_is_current || fail "Go 1.20+ is required (found $(go version 2>/dev/null || printf 'none'))."
export GOTOOLCHAIN=auto

node_is_current || install_node
node_is_current || fail "Node.js 24+ is required (found $(node --version 2>/dev/null || printf 'none'))."

install_linux_build_dependencies

printf 'Installing JavaScript dependencies...\n'
cd "$PROJECT_ROOT/frontend"
npm ci

printf 'Building DBfock...\n'
cd "$PROJECT_ROOT/backend"
go mod download
if [[ -n "$WAILS_TAGS" ]]; then
  go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 build -tags "$WAILS_TAGS"
else
  go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.1 build
fi

build_binary="$PROJECT_ROOT/backend/build/bin/DBfock"
[[ -x "$build_binary" ]] || fail "Build completed but did not produce $build_binary."

mkdir -p "$(dirname "$INSTALL_BIN")" "$(dirname "$DESKTOP_FILE")" "$(dirname "$ICON_FILE")"
install -m 755 "$build_binary" "$INSTALL_BIN"
install -m 644 "$PROJECT_ROOT/frontend/public/branding/favicon/android-chrome-512x512.png" "$ICON_FILE"
cat > "$DESKTOP_FILE" <<EOF
[Desktop Entry]
Type=Application
Name=DBfock
Comment=MySQL workspace
Exec=$INSTALL_BIN
Icon=dbfock
Terminal=false
Categories=Development;Database;
EOF
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$(dirname "$DESKTOP_FILE")" || true

printf '\nInstalled: %s\n' "$INSTALL_BIN"
printf 'Open DBfock from your application menu, or run: %s\n' "$INSTALL_BIN"
