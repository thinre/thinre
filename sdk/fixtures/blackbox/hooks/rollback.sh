#!/bin/sh
# Rollback hook: restore the version saved by the last upgrade.
set -eu

APP_DIR=/opt/blackbox

if [ ! -d "$APP_DIR/previous" ]; then
    echo "rollback: no previous version available" >&2
    exit 1
fi

for f in VERSION payload; do
    [ -e "$APP_DIR/previous/$f" ] && cp -r "$APP_DIR/previous/$f" "$APP_DIR/$f"
done
echo "rolled back to $(cat "$APP_DIR/VERSION")"
