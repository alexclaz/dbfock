#!/usr/bin/env bash
# Bootstrap DBfock from source on macOS and Linux.
# Run with: curl -fsSL https://raw.githubusercontent.com/alexclaz/dbfock/main/install.sh | bash

set -euo pipefail

readonly REPOSITORY='https://github.com/alexclaz/dbfock'
readonly REF="${DBFOCK_REF:-main}"

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

case "$REF" in
  ''|*[!A-Za-z0-9._/-]*) fail 'DBFOCK_REF must contain only letters, numbers, dots, underscores, dashes, or slashes.' ;;
esac

case "$(uname -s)" in
  Darwin) platform_script='install-macos.sh' ;;
  Linux) platform_script='install-linux.sh' ;;
  MINGW*|MSYS*|CYGWIN*)
    fail 'Windows source bootstrapping is intentionally unsupported. The recommended distribution is a signed installer from https://github.com/alexclaz/dbfock/releases (not published yet).'
    ;;
  *) fail "Unsupported operating system: $(uname -s)." ;;
esac

command -v curl >/dev/null 2>&1 || fail 'curl is required to download DBfock.'
command -v tar >/dev/null 2>&1 || fail 'tar is required to unpack DBfock.'

temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/dbfock-install.XXXXXX")"
cleanup() {
  rm -rf "$temporary_directory"
}
trap cleanup EXIT

archive="$temporary_directory/dbfock.tar.gz"
printf 'Downloading DBfock (%s)...\n' "$REF"
curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error \
  "$REPOSITORY/archive/refs/heads/$REF.tar.gz" \
  --output "$archive"
tar -xzf "$archive" -C "$temporary_directory"

source_directory="$(find "$temporary_directory" -mindepth 1 -maxdepth 1 -type d -name 'dbfock-*' | head -n 1)"
[[ -n "$source_directory" && -x "$source_directory/scripts/$platform_script" ]] || fail 'The downloaded source does not contain a supported installer.'

"$source_directory/scripts/$platform_script"
