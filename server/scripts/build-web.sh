#!/bin/sh
# Bundle the phone app into the server. Run from server/ with the mobile app
# in ../mobile (or pass its path).
set -e
MOBILE="${1:-../mobile}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/internal/webapp/dist"
rm -rf "$OUT"/* "$OUT"/_expo
(cd "$MOBILE" && npx expo export --platform web --output-dir "$OUT")
if [ ! -f "$OUT/index.html" ]; then
  echo "expo export wrote no index.html into $OUT; the server would have no app" >&2
  exit 1
fi
touch "$OUT/.gitkeep"
echo "web app bundled into $OUT; rebuild the server to embed it"
