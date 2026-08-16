# Testbed: the Supervisor plus an installed blackbox fixture application,
# in one Linux container. Used for local end-to-end runs and CI.
# Build from the repository root:  docker build -f sdk/testbed/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/thinre-supervisor ./cmd/supervisor

FROM debian:stable-slim
COPY --from=build /out/thinre-supervisor /usr/local/bin/thinre-supervisor
# Install the blackbox app at 1.0.0 with its hooks and manifest.
COPY sdk/fixtures/blackbox/hooks /opt/blackbox/hooks
COPY sdk/fixtures/blackbox/blackbox.yaml /etc/thinre/integrations/blackbox.yaml
COPY sdk/testbed/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /opt/blackbox/hooks/*.sh \
    && mkdir -p /opt/blackbox/config \
    && printf '1.0.0\n' > /opt/blackbox/VERSION \
    && printf 'blackbox payload for version 1.0.0\n' > /opt/blackbox/payload
# A second, independent copy of the app (blackbox-b) for multi-application
# tests: same hooks and manifest with every path and the integration name
# rewritten. The entrypoint only enables it when THINRE_SECOND_APP=1.
RUN mkdir -p /opt/blackbox-b/config \
    && cp -r /opt/blackbox/hooks /opt/blackbox-b/hooks \
    && sed -i 's|/opt/blackbox|/opt/blackbox-b|g' /opt/blackbox-b/hooks/*.sh \
    && sed -e 's|/opt/blackbox|/opt/blackbox-b|g' -e 's|name: blackbox|name: blackbox-b|' \
        /etc/thinre/integrations/blackbox.yaml > /etc/thinre/integrations/blackbox-b.yaml \
    && printf '1.0.0\n' > /opt/blackbox-b/VERSION \
    && printf 'blackbox-b payload for version 1.0.0\n' > /opt/blackbox-b/payload
ENTRYPOINT ["/entrypoint.sh"]
