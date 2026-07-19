#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
frontend_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
assets_dir="$frontend_dir/../backend/desktop/assets"

mkdir -p "$assets_dir"
rsync -a --delete --exclude .gitkeep "$frontend_dir/.output/public/" "$assets_dir/"
