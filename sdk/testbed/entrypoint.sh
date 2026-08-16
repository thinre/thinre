#!/bin/sh
# Testbed entrypoint: materialize the supervisor configuration from the
# environment, then hand over to the supervisor (PID 1 via exec).
#
#   THINRE_API_URL           cloud API base URL   (default: host.docker.internal:8080)
#   THINRE_OPAMP_URL         gateway WebSocket    (default: host.docker.internal:8081)
#   THINRE_RUNTIME_NAME      display name         (default: container hostname)
#   THINRE_ENROLLMENT_TOKEN  consumed on first start (read by the supervisor itself)
#   THINRE_SECOND_APP        "1" also manages the blackbox-b copy (multi-app tests)
set -eu

API_URL="${THINRE_API_URL:-http://host.docker.internal:8080}"
OPAMP_URL="${THINRE_OPAMP_URL:-ws://host.docker.internal:8081/v1/opamp}"
NAME="${THINRE_RUNTIME_NAME:-$(hostname)}"

cat > /etc/thinre/supervisor.yaml <<EOF
api_url: $API_URL
opamp_url: $OPAMP_URL
name: $NAME
integrations:
  - manifest: /etc/thinre/integrations/blackbox.yaml
EOF
if [ "${THINRE_SECOND_APP:-}" = "1" ]; then
    cat >> /etc/thinre/supervisor.yaml <<EOF
  - manifest: /etc/thinre/integrations/blackbox-b.yaml
EOF
fi

exec /usr/local/bin/thinre-supervisor "$@"
