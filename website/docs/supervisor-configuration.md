---
title: Supervisor configuration
---

# Supervisor configuration

The supervisor reads one YAML file — `/etc/thinre/supervisor.yaml` by
default. Everything dynamic (desired versions, artifacts, configuration
bundles) arrives over OpAMP; this file only says who to talk to and what
software this machine runs.

```yaml title="/etc/thinre/supervisor.yaml"
# Cloud REST endpoint — used only for enrollment.
api_url: https://api.<your-workspace>

# WebSocket endpoint of the OpAMP gateway — the permanent connection.
opamp_url: wss://opamp.<your-workspace>/v1/opamp

# The integration manifest describing the managed software.
integration_manifest: /etc/thinre/integrations/myapp.yaml

# Writable state directory (identity, downloads, backups).
# Default: /var/lib/thinre
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

## What the supervisor reports

Alongside its observed state (version, health, reconcile status), the
supervisor identifies its host in the console: **hostname**, **primary
IP address**, **operating system and architecture**, its own version,
and your **labels**. Identification is best-effort and read-only — it
never influences reconciliation.
