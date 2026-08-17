# blackbox-win

The Windows twin of the `blackbox` fixture: the same black-box lifecycle
semantics (VERSION file, payload, marker-file failure injection) with
PowerShell hooks and zip artifacts instead of shell scripts and tarballs.

Install layout is fixed at `C:\ThinreFixture\blackbox` (the manifest's
hook paths are absolute); the hooks themselves resolve the app directory
from their own location, so a relocated copy only needs a matching
manifest.

Failure injection markers, created in the app directory:

- `fail-upgrade` — the next upgrade exits 1 before touching anything
- `slow-upgrade` — stretches the upgrade for crash-recovery tests
- `unhealthy` — health reports unhealthy until removed
- `fail-validate` — every configuration revision is rejected

Build an artifact: `powershell -File make-artifact.ps1 2.0.0 <out-dir>`.
