# Contributing to Thinre

Thanks for your interest in contributing.

## Ground rules

- **License**: all contributions are accepted under [Apache-2.0](LICENSE).
- **Scope**: this repository holds the open-source edge (Supervisor, CLI, integration spec, shared contracts, SDK). Features belonging to the commercial control plane will not be accepted here — see the boundary section in the [README](README.md).
- **Git history**: we merge pull requests with **true merge commits** — no squashing, no rebasing. Keep branches small and focused so history stays legible.

## Workflow

1. Open an issue or discussion describing the change before writing significant code.
2. Fork/branch, implement, and add tests (unit tests always; integration tests where applicable).
3. Run `make build test lint` locally — CI enforces the same checks plus the boundary check.
4. Open a pull request with a clear description of the *why*.

## Code style

- Keep it simple (KISS); standard library first.
- One package = one responsibility; package doc comments and godoc on all exported symbols.
- Comment the *why* of non-obvious logic, not the *what*.

## Versioning

Semantic versioning via git tags (`vX.Y.Z`). Shared contract packages (`integration-spec/`, `bundle/`, `protocol/`) are frozen at tagged releases; breaking a contract requires a minor (pre-1.0) or major (post-1.0) version bump and a changelog note.
