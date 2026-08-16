# blackbox — the fixture application

A minimal "black box" application used to develop and test Thinre
integrations locally, and as the managed software in Thinre's end-to-end
tests. It has no daemon: the application is a directory.

## Layout (installed at `/opt/blackbox`)

```
/opt/blackbox/
├── VERSION          # the installed version, plain text
├── payload          # stand-in for the application binary
├── config/          # managed configuration destination
└── hooks/           # the lifecycle hooks wired by blackbox.yaml
```

## Hooks

| Hook | Behavior |
|---|---|
| `upgrade.sh <artifact>` | Saves the current app to `previous/`, extracts the tarball |
| `rollback.sh` | Restores `previous/` |
| `version.sh` | Prints `VERSION` — the Supervisor's observed-version source |
| `health.sh` | 0 when `VERSION` is readable and no marker present |
| `validate-config.sh <staged-dir>` | Rejects the staged bundle when the `fail-validate` marker is present or the staged dir is empty |
| `apply-config.sh` | Accept-everything stub (nothing to reload) |

## Failure injection

- `touch /opt/blackbox/fail-upgrade` — next upgrade fails before changing anything
- `touch /opt/blackbox/slow-upgrade` — upgrades take 10s (for kill-mid-operation tests)
- `touch /opt/blackbox/fail-validate` — configuration validation rejects every revision
- `touch /opt/blackbox/unhealthy` — health checks fail until removed

## Building release artifacts

```
./make-artifact.sh 2.0.0
```

emits `blackbox-2.0.0.tar.gz` and its SHA-256, ready to be registered as a
release and served from any HTTP endpoint (the dev environment's fixture
server, for instance).
