# CI and release pipelines

Any change to a workflow, Makefile target, or this pipeline behavior must update this document in the same commit.

## `.github/workflows/ci.yml` — CI

**Triggers:** every push to `main` and every pull request.

| Job | What it checks |
|---|---|
| `build-test` | `go build ./...` then `go test ./...` |
| `lint` | `golangci-lint run` (config: `.golangci.yml`, default linter set) |
| `boundary` | Greps all Go sources and module files (`*.go`, `go.mod`, `go.sum`) for `thinre-cloud`; any hit fails the build. This enforces the one-way dependency rule: open-source **code** must never depend on the closed-source repository. Documentation may mention the sibling repo by name. |

All three jobs are required status checks on `main` (branch protection). Pull requests merge with **merge commits only** — squash and rebase merging are disabled at the repository level.

## `.github/workflows/release.yml` — Release

**Trigger:** pushing a tag matching `v*` (semver, e.g. `v0.1.0`).

Steps:

1. Build the Supervisor with `CGO_ENABLED=0` for `linux/amd64` and `linux/arm64`, stamping the tag into the binary via `-ldflags "-X main.version=..."`.
2. Create a GitHub release for the tag with auto-generated notes and the two binaries attached.

The same tag doubles as the **Go module version** that `thinre-cloud` pins in its `go.mod` (resolved through proxy.golang.org — this repo is public). Workflow rule: tag here first, then bump the require in thinre-cloud.

## Tagging policy

- `v0.0.x` — early bootstrap churn.
- `v0.1.0` — first contract freeze (integration-spec + protocol stable enough for the cloud to build against), at milestone M2 exit.
- `v0.2.0` — M3 exit (first remote upgrade works end to end).
