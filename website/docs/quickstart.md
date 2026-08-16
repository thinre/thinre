---
title: Quick start
---

# Quick start

This walkthrough takes one Linux machine from nothing to its **first
remote upgrade**, driven from the Thinre console. Budget about ten
minutes.

## What you need

- A **Thinre Cloud workspace** and access to its console.
- A **Linux machine** (amd64 or arm64) running the software you want to
  manage, with outbound HTTPS/WebSocket access.
- An **upgrade script** for that software — or five minutes to write one.

## 1. Install the Supervisor

Download the static binary from the
[latest release](https://github.com/thinre/thinre/releases) and put it on
the machine:

```bash
curl -fL -o /usr/local/bin/thinre-supervisor \
  https://github.com/thinre/thinre/releases/latest/download/thinre-supervisor_linux_amd64
chmod +x /usr/local/bin/thinre-supervisor
```

(Use `thinre-supervisor_linux_arm64` on ARM machines.)

## 2. Describe your software

Create an integration manifest. It declares the hooks Thinre may call —
your software stays a black box behind them:

```yaml title="/etc/thinre/integrations/myapp.yaml"
apiVersion: thinre.io/v1
kind: Integration

metadata:
  name: myapp

package:
  # Called with the path of a downloaded, checksum-verified artifact.
  upgrade:
    executable: /opt/myapp/bin/upgrade.sh
    args: ["{{ artifact.path }}"]
    timeout: 120s
  # How Thinre reads the currently installed version (prints it to stdout).
  version:
    executable: /opt/myapp/bin/version.sh
    timeout: 10s

health:
  # Exit 0 = healthy. Also gates upgrades and rollouts.
  check:
    executable: /opt/myapp/bin/health.sh
    timeout: 10s
```

Only `package.upgrade` is mandatory; `version` and `health` make the
console dramatically more useful. The
[manifest reference](integration-manifest) covers rollback and managed
configuration files.

## 3. Register the integration in the console

In the console, open **Integrations → Create**, name it `myapp`, then
paste the same manifest on the integration's page and publish it as
version 1. The **Validate** button dry-runs the manifest without
publishing anything.

:::note
The manifest lives in both places on purpose: the cloud validates
releases and bundles against it; the supervisor enforces it on the
machine. The name in `metadata.name` links the two at enrollment.
:::

## 4. Configure the Supervisor

```yaml title="/etc/thinre/supervisor.yaml"
api_url: https://api.<your-workspace>
opamp_url: wss://opamp.<your-workspace>/v1/opamp
integrations:
  - manifest: /etc/thinre/integrations/myapp.yaml

# Optional: tags shown in the console (or via THINRE_LABELS env).
labels:
  env: production
  dc: paris
```

One supervisor can manage several applications on the same host — add
more `integrations` entries and each becomes its own runtime in the
console (see [supervisor configuration](supervisor-configuration)).

## 5. Enroll and start

In the console, open **Runtimes → New enrollment token** and copy the
token — it is single-use and shown only once. Then, on the machine:

```bash
THINRE_ENROLLMENT_TOKEN=thinre_et_… thinre-supervisor
```

The supervisor exchanges the token for a permanent machine identity
(stored under `/var/lib/thinre`), connects, and the runtime appears
**CONNECTED** in the console within seconds — with its hostname, IP,
OS, and your labels. From now on, start it without the token; a systemd
unit is the natural home:

```ini title="/etc/systemd/system/thinre-supervisor.service"
[Unit]
Description=Thinre Supervisor
After=network-online.target

[Service]
ExecStart=/usr/local/bin/thinre-supervisor
Restart=always

[Install]
WantedBy=multi-user.target
```

## 6. Your first remote upgrade

1. Package the new version of your software as a tarball your upgrade
   script understands, and note its checksum: `sha256sum myapp-2.0.0.tar.gz`.
2. In the console, open **Releases → New release**: pick `myapp`, version
   `2.0.0`, and either paste an artifact URL + SHA-256 or upload the file
   directly (managed storage).
3. Open your runtime's page and set the **desired state** to release
   2.0.0.

Watch the event timeline: `downloading → upgrading → installed`, the
observed version flips to 2.0.0, and the sync badge turns **IN_SYNC**.
That's the whole loop — the supervisor verified the artifact's checksum,
ran your upgrade hook, confirmed the version and health, and reported
back.

If anything fails — bad checksum, failing hook, red health check — the
rollback hook runs and the runtime reports **FAILED** with the reason in
its timeline. Fix the cause, set the desired state again, and it
converges.

## Where to go next

- Group runtimes into **fleets** and drive **staged rollouts** with
  canary percentages and approval gates.
- Manage configuration as **atomic bundle revisions** with validate and
  apply hooks.
- Explore the [supervisor configuration](supervisor-configuration) and
  [integration manifest](integration-manifest) references.
