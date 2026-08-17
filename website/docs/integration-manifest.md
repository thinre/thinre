---
title: Integration manifest
---

# Integration manifest (v1)

The integration manifest is the entire contract between Thinre and your
software. It is a YAML document validated identically by the cloud (when
you publish it) and by the supervisor (when it acts on it). Unknown
fields are rejected — a typo fails at publish time, never silently on a
machine.

## Full shape

```yaml
apiVersion: thinre.io/v1   # required, exactly this value
kind: Integration          # required, exactly this value

metadata:
  name: myapp              # required; links supervisors to this integration

package:
  upgrade:                 # REQUIRED
    executable: /opt/myapp/bin/upgrade.sh
    args: ["{{ artifact.path }}"]
    timeout: 120s
  rollback:                # optional — runs when an upgrade or health fails
    executable: /opt/myapp/bin/rollback.sh
    timeout: 120s
  version:                 # optional — prints the installed version to stdout
    executable: /opt/myapp/bin/version.sh
    timeout: 10s

configuration:             # optional — managed configuration files
  files:
    - id: main             # referenced by bundle files in the cloud
      destination: /opt/myapp/config/app.conf
  validate:                # optional — dry-run check of a staged bundle
    executable: /opt/myapp/bin/validate-config.sh
    args: ["{{ bundle.path }}"]
    timeout: 30s
  apply:                   # optional — tell the app to load the new config
    executable: /opt/myapp/bin/apply-config.sh
    timeout: 30s

health:                    # REQUIRED — exit 0 = healthy
  check:
    executable: /opt/myapp/bin/health.sh
    timeout: 10s
```

## Hooks

Every hook has the same three fields:

| Field | Meaning |
|---|---|
| `executable` | Absolute path of the program to run. Executed directly — **no shell**, no `PATH` lookup, no word splitting. |
| `args` | Optional argument list. Two placeholders are substituted: `{{ artifact.path }}` (the verified artifact) and `{{ bundle.path }}` (the staged bundle directory). |
| `timeout` | How long the hook may run (`10s`, `2m`, …). On expiry the process is killed and the operation counts as failed. |

Hook stdout/stderr is captured (bounded) and surfaced in the runtime's
event timeline, so a failing hook explains itself in the console.

## Semantics worth knowing

- **`package.upgrade`** receives an artifact that has already been
  downloaded and SHA-256-verified — fail-closed, before your code runs.
  The hook's job is only: unpack, install, restart.
- **`package.version`** makes drift visible: the supervisor runs it after
  every operation and periodically, and the cloud compares it against the
  desired version to derive the sync status.
- **`package.rollback`** runs when the upgrade hook fails *or* the health
  check stays red after an upgrade. Without it, a failure leaves the
  machine as the upgrade left it (still reported as FAILED).
- **`configuration`** enables atomic config bundles: the supervisor
  stages all files, runs `validate` against the staged copies, places
  them with backups, runs `apply`, then checks health. Any failure
  restores the backups — a rejected revision changes nothing.
- **`health.check`** is required, not optional: it gates everything —
  upgrades, config changes, and staged rollouts all wait on it. Without a
  health signal Thinre could not tell a successful upgrade from a broken
  one, so the contract insists on it.

## Trying a manifest

Paste any manifest into **Integrations → Validate** in the console for an
immediate dry-run parse, or validate locally with the `thinre` CLI from
this repository.
