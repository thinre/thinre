#!/bin/sh
# Upgrade hook: install the artifact tarball as the new application.
# Usage: upgrade.sh <artifact-path>
#
# Failure injection for tests: `touch /opt/blackbox/fail-upgrade` makes the
# next upgrade exit 1 before touching anything.
set -eu

APP_DIR=/opt/blackbox
ARTIFACT="$1"

if [ -e "$APP_DIR/fail-upgrade" ]; then
    echo "upgrade: failing on request (fail-upgrade marker present)" >&2
    exit 1
fi

# Crash-recovery testing: `touch /opt/blackbox/slow-upgrade` stretches the
# upgrade so a test can kill the supervisor mid-operation.
if [ -e "$APP_DIR/slow-upgrade" ]; then
    sleep 10
fi

# Keep the previous version as the rollback candidate. Hooks and config are
# part of the installation, not the versioned app payload, so only VERSION
# and payload files are replaced.
rm -rf "$APP_DIR/previous"
mkdir -p "$APP_DIR/previous"
for f in VERSION payload; do
    [ -e "$APP_DIR/$f" ] && cp -r "$APP_DIR/$f" "$APP_DIR/previous/$f"
done

tar -xzf "$ARTIFACT" -C "$APP_DIR"
echo "upgraded to $(cat "$APP_DIR/VERSION")"
