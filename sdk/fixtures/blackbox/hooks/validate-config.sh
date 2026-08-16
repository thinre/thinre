#!/bin/sh
# Configuration validate hook: receives the staged bundle directory as $1
# and accepts or rejects the complete revision before anything goes live.
#
# Failure injection for tests: `touch /opt/blackbox/fail-validate` rejects
# every revision until the marker is removed.
set -eu

STAGED="${1:?staged bundle directory expected}"

if [ -e /opt/blackbox/fail-validate ]; then
    echo "validation failing on request (fail-validate marker present)" >&2
    exit 1
fi
if [ ! -d "$STAGED" ] || [ -z "$(ls -A "$STAGED")" ]; then
    echo "staged bundle is missing or empty" >&2
    exit 1
fi
exit 0
