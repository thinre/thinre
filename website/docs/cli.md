---
title: CLI
---

# The `thinre` CLI

A thin client for the Thinre Cloud API, for the things that are nicer
from a terminal than a browser. Download it from the
[releases page](https://github.com/thinre/thinre/releases)
(`thinre_<os>_<arch>`).

## Authentication

Every command needs the cloud API URL and a bearer token, via flags or
environment:

```bash
export THINRE_API_URL=https://api.<your-workspace>
export THINRE_TOKEN=<your token>
# Only needed when you belong to several organizations:
export THINRE_ORG=<organization id>
```

## `thinre publish`

Publishes a locally authored integration manifest to Thinre Cloud as a
new integration version:

```bash
thinre publish -version 2 /etc/thinre/integrations/myapp.yaml
```

- The manifest is validated **locally first**, with the same parser the
  cloud and the supervisor use — a bad manifest never leaves the machine.
- The integration is matched by the manifest's `metadata.name`. If it
  does not exist yet, `-create` creates it in the same run:

```bash
thinre publish -version 1 -create ./myapp.yaml
```

Publishing flows **host → cloud only**. The cloud stores manifests to
validate releases and configuration bundles against them; it can never
push a manifest down to a machine — what runs on a host is decided
solely by the files an operator placed there.
