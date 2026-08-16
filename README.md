# Thinre

**Thinre (Thin Runtime Enablement)** is a universal lifecycle control plane for distributed software: manage the lifecycle (install, upgrade, rollback, configure, health-check) of any software agent or daemon — treated as a black box — without that software having to implement any Thinre-specific protocol.

**Documentation:** <https://thinre.github.io/thinre/> — start with the [quick start](https://thinre.github.io/thinre/quickstart). The site's source lives in [`website/`](website/).

This repository contains the **open-source edge components**, licensed under [Apache-2.0](LICENSE):

| Component | Path | Purpose |
|---|---|---|
| Supervisor | `cmd/supervisor`, `supervisor/` | Runs next to the managed software; reconciles desired state via OpAMP and executes locally defined lifecycle hooks |
| CLI | `cmd/cli` | Thin API client for Thinre Cloud (shipped under the command name `thinre`) |
| Integration spec | `integration-spec/` | Schema + validation for the Integration contract between the Supervisor and black-box software |
| Bundle format | `bundle/` | Configuration bundle manifest format |
| Protocol types | `protocol/` | Enrollment and OpAMP payload types shared with Thinre Cloud |
| SDK & fixtures | `sdk/` | Integration SDK and local integration test tools |

## The boundary

This repository is and stays fully open source. What belongs here: everything that runs on **your** infrastructure, plus the shared contracts. What will never be here: fleet orchestration, multi-tenancy, approvals, audit, billing, or any other part of the commercial control plane (**Thinre Cloud**, developed separately). The dependency is strictly one-way: Thinre Cloud imports this module; this module never references Thinre Cloud.

The core invariant:

> **Cloud declares WHAT. Supervisor decides HOW. Managed software remains a BLACK BOX.**

## Build

```
make build   # go build ./...
make test    # go test ./...
make lint    # golangci-lint run
```

See [docs/development.md](docs/development.md) for the local development setup and [docs/ci.md](docs/ci.md) for how the CI and release pipelines work.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
