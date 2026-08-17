---
title: Supervisor configuration
---

# Supervisor configuration

The supervisor reads one YAML file — `/etc/thinre/supervisor.yaml` by
default on Linux, `%ProgramData%\Thinre\supervisor.yaml` on Windows.
Everything dynamic (desired versions, artifacts, configuration bundles)
arrives over OpAMP; this file only says who to talk to and what software
this machine runs.

```yaml title="/etc/thinre/supervisor.yaml"
# Cloud REST endpoint — used only for enrollment.
api_url: https://api.<your-workspace>

# WebSocket endpoint of the OpAMP gateway — the permanent connection.
opamp_url: wss://opamp.<your-workspace>/v1/opamp

# The applications this supervisor manages — one entry per app, each
# with its own runtime identity, reconcile loop, and state directory.
integrations:
  - manifest: /etc/thinre/integrations/myapp.yaml
  - manifest: /etc/thinre/integrations/log-shipper.yaml
    # Optional display-name override; see "Runtime names" below.
    name: logs-edge-paris-01

# Writable state directory (identity, downloads, backups).
# Default: /var/lib/thinre (Linux), %ProgramData%\Thinre\data (Windows)
data_dir: /var/lib/thinre

# Display name in the console. Default: the machine's hostname.
name: edge-paris-01

# One-time enrollment token; the THINRE_ENROLLMENT_TOKEN environment
# variable overrides it so tokens never need to be written to disk.
enrollment_token: thinre_et_…

# Operator-defined tags, shown in the console next to the reported
# hostname, IP, and OS.
labels:
  env: production
  dc: paris
```

Unknown fields are rejected at startup — a typo fails loudly instead of
silently changing behavior.

## Multiple applications per host

One supervisor install manages every listed integration: each entry
enrolls as its **own runtime** in the cloud (own desired state, sync
status, health, rollouts) and keeps its state in its own subdirectory of
`data_dir` (`/var/lib/thinre/<integration-name>/…`), so applications on
the same host can never interfere with each other. A single enrollment
token enrolls all of them in one exchange.

**Runtime names:** with one integration, the runtime is named after the
host (`name`, defaulting to the hostname). With several, each runtime
defaults to `<host name>/<integration name>` — override per entry with
the `name` field if you prefer.

## Environment variables

| Variable | Effect |
|---|---|
| `THINRE_ENROLLMENT_TOKEN` | Overrides `enrollment_token`. The token is single-use: it is consumed on first start and replaced by a permanent machine identity in `data_dir`. |
| `THINRE_LABELS` | `key=value,key=value` pairs merged **over** the file's `labels` (the environment wins on conflicts). Malformed pairs fail startup. |

## Enrollment and identity

On first start the supervisor exchanges the enrollment token for a
machine identity (an opaque token, stored with `0600` permissions under
`data_dir`). Every later start reuses that identity; the enrollment
token is never needed again. Revoking the identity in the console cuts
the machine off immediately.

## Windows

The supervisor is a native Windows binary
(`thinre-supervisor_windows_amd64.exe` on the releases page) with the
same behavior; only the default paths differ (above). Hooks are any
executable — for PowerShell scripts, point `executable` at
`powershell.exe` (absolute path) and pass the script via `args`; see the
Windows example in the [manifest reference](integration-manifest).

Two platform caveats worth knowing:

- Placing configuration files fails if the managed application holds
  them open with an exclusive lock (Windows file semantics). The
  revision reports as failed and the backups are restored — release the
  file in your `apply` flow or accept a restart.
- A hook that exceeds its timeout has its process killed; child
  processes it spawned are not reaped until job-object support lands
  with the production-hardening milestone.

## What the supervisor reports

Alongside its observed state (version, health, reconcile status), the
supervisor identifies its host in the console: **hostname**, **primary
IP address**, **operating system and architecture**, its own version,
and your **labels**. Identification is best-effort and read-only — it
never influences reconciliation.
