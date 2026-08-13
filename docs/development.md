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

## Working alongside thinre-cloud

Thinre Cloud (closed source, separate repository) imports this module as a versioned dependency. For local cross-repo development, use an **uncommitted** Go workspace one directory above both checkouts:

```
workspace/
├── thinre/          (this repo)
├── thinre-cloud/
└── go.work          (never committed to either repo)
```

```
go work init ./thinre ./thinre-cloud
```

Rules:

- Never commit a `replace` directive.
- The dependency is one-way: this repository must never reference `thinre-cloud`. CI enforces this (see [ci.md](ci.md)).
- When cloud work needs new code from this repo: merge here first, tag `vX.Y.Z`, then bump the `require` in thinre-cloud.

## Platform notes

The Supervisor targets Linux (systemd, `/var/lib/thinre`, `/etc/thinre`). OS-agnostic unit tests run anywhere, including Windows; Linux-specific integration tests are build-tagged and run in the Docker testbed or CI.
