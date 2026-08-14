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
ENTRYPOINT ["/entrypoint.sh"]
