---
slug: /
title: What is Thinre?
---

# What is Thinre?

Thinre is a **universal lifecycle control plane**: it upgrades, configures,
and watches software running on machines you don't control — customer
servers, edge boxes, appliances — without requiring that software to know
anything about Thinre.

It has two halves:

- **Thinre Cloud** — the control plane. You declare *desired state*:
  "fleet X should run release 2.0.0 with configuration revision 7."
- **The Supervisor** (this repository, Apache-2.0) — a single static
  binary on each machine. It connects outbound over WebSocket
  (the [Thinre Link protocol](link-protocol)), receives the
  desired state, and **reconciles** the local software toward it.

```
Thinre Cloud ── desired state ──▶ Supervisor ── download ▶ verify ▶ upgrade ▶ health ──▶ observed state
```

## The black-box contract

Thinre never links against, patches, or introspects your application.
Everything it does goes through the **integration manifest** — a small
YAML file declaring the lifecycle hooks your software already has:

- how to **upgrade** it (a script that receives a verified artifact),
- how to read its **version**,
- how to check its **health**,
- how to **roll back**,
- which **configuration files** it owns and how to validate/apply them.

If you can write those scripts, Thinre can manage the software. The full
contract is in the [integration manifest reference](integration-manifest).

## What the Supervisor guarantees

- **Fail-closed artifacts.** Every artifact download is verified against
  its expected SHA-256 before any hook sees it. A checksum mismatch is
  never retried into acceptance — the upgrade simply does not happen.
- **Rollback on failure.** If the upgrade hook fails or the health check
  stays red, the rollback hook runs and the failure is reported durably.
- **Atomic configuration.** Configuration bundles are staged, validated,
  and placed with backups; a rejected revision changes nothing.
- **Crash safety.** A supervisor killed mid-upgrade resumes cleanly.
- **Outbound-only.** The machine needs no open inbound ports; the
  supervisor dials out and reconnects with backoff.

## Where to go next

- [Quick start](quickstart) — from a downloaded binary to your first
  remote upgrade.
- [Integration manifest](integration-manifest) — the contract between
  Thinre and your software.
- [Supervisor configuration](supervisor-configuration) — every field of
  `supervisor.yaml`.
- [Link protocol](link-protocol) — the three-message wire protocol
  between supervisor and cloud.
