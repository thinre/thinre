#!/bin/sh
# Testbed entrypoint: materialize the supervisor configuration from the
# environment, then hand over to the supervisor (PID 1 via exec).
#
#   THINRE_API_URL           cloud API base URL   (default: host.docker.internal:8080)
#   THINRE_OPAMP_URL         gateway WebSocket    (default: host.docker.internal:8081)
#   THINRE_RUNTIME_NAME      display name         (default: container hostname)
#   THINRE_ENROLLMENT_TOKEN  consumed on first start (read by the supervisor itself)
set -eu

API_URL="${THINRE_API_URL:-http://host.docker.internal:8080}"
OPAMP_URL="${THINRE_OPAMP_URL:-ws://host.docker.internal:8081/v1/opamp}"
NAME="${THINRE_RUNTIME_NAME:-$(hostname)}"

cat > /etc/thinre/supervisor.yaml <<EOF
api_url: $API_URL
opamp_url: $OPAMP_URL
integration_manifest: /etc/thinre/integrations/blackbox.yaml
name: $NAME
EOF

exec /usr/local/bin/thinre-supervisor "$@"
