#!/bin/sh
# Version hook: print the currently installed version to stdout.
# This is how the Supervisor learns the black box's observed version.
set -eu
cat /opt/blackbox/VERSION
