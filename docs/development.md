# Development

## Prerequisites

- Go (version pinned by `go.mod`)
- GNU make
- golangci-lint (for `make lint`; CI runs it regardless)
- Docker (only for the Linux testbed and integration tests — the Supervisor targets Linux)

## Build, test, lint

```
make build
make test
make lint
```

## Working alongside Thinre Cloud

Thinre Cloud (the closed-source control plane) imports this module as a versioned dependency, resolved from tagged releases through the Go module proxy. Its own development setup is documented in its own repository.

Rules:

- The dependency is one-way: nothing in this repository — code or documentation — may reference the closed-source repository. CI enforces this (see [ci.md](ci.md)).
- Never commit a `replace` directive or a `go.work` file.
- When cloud-side work needs new code from this repo: merge here first, tag `vX.Y.Z`, then bump the requirement on the cloud side.

## Platform notes

The Supervisor targets Linux (systemd, `/var/lib/thinre`, `/etc/thinre`). OS-agnostic unit tests run anywhere, including Windows; Linux-specific integration tests are build-tagged and run in the Docker testbed or CI.
