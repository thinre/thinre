#!/bin/sh
# Health hook: exit 0 when healthy, non-zero otherwise.
#
# Failure injection for tests: `touch /opt/blackbox/unhealthy` makes the
# application report unhealthy until the marker is removed.
set -eu

APP_DIR=/opt/blackbox

if [ -e "$APP_DIR/unhealthy" ]; then
    echo "unhealthy (marker present)" >&2
    exit 1
fi
if [ ! -r "$APP_DIR/VERSION" ]; then
    echo "unhealthy: VERSION missing" >&2
    exit 1
fi
echo "healthy $(cat "$APP_DIR/VERSION")"
