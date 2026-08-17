---
title: Link protocol
---

# The Thinre Link protocol

Link is the protocol between a supervisor and Thinre Cloud: **one
outbound WebSocket per managed application**, carrying JSON messages.
It is deliberately tiny — three message types cover the whole lifecycle —
because everything interesting (verification, hooks, rollback) happens
on the machine, not on the wire.

```
Supervisor ──── hello ───▶ Cloud        who am I, what have I applied
Supervisor ◀─── state ──── Cloud        the desired-state document
Supervisor ── observed ──▶ Cloud        what is actually running
```

## Transport and authentication

- WebSocket over HTTPS (`wss://`), endpoint **`/v1/link`** on the
  gateway.
- The connection is always **dialed outbound** by the supervisor — the
  machine needs no open inbound ports.
- Authentication happens once, at the HTTP upgrade: the supervisor sends
  its machine token in the **`X-Thinre-Machine-Token`** header. The
  token (obtained at [enrollment](quickstart#5-enroll-and-start), stored
  hashed by the cloud) binds the connection to exactly one runtime and
  organization; nothing in later messages can change that.
- A supervisor managing several applications holds one connection per
  application, each with its own runtime identity.

## Framing and forward compatibility

Every WebSocket text message is **one JSON envelope**:

```json
{ "type": "<hello|state|observed>", "<type>": { … } }
```

Receivers **must ignore envelopes whose `type` they do not know** — that
is how the protocol grows without breaking older supervisors. `hello`
carries `link_version` (currently `1`); it only changes for incompatible
revisions, never for additions.

## `hello` — client → server, once per connection

```json
{
  "type": "hello",
  "hello": {
    "link_version": 1,
    "supervisor_version": "v0.6.0",
    "integration": "myapp",
    "hostname": "edge-paris-01",
    "ip": "10.20.0.14",
    "os": "linux",
    "arch": "amd64",
    "labels": { "env": "production", "dc": "paris" },
    "applied_generation": 41
  }
}
```

- `integration` is the manifest's `metadata.name`, linking the runtime
  to its published contract.
- Host identification and `labels` are best-effort and shown in the
  console; they never influence reconciliation.
- `applied_generation` is the last desired-state generation this
  application applied (`0` = none). The server compares it with the
  current generation and, when they differ, sends `state` immediately —
  a reconnecting supervisor always converges without any request/reply
  round-trip.

## `state` — server → client

The full desired-state document, exactly the `DesiredState` type from
the `protocol` package:

```json
{
  "type": "state",
  "state": {
    "schema_version": "v1",
    "generation": 42,
    "package": {
      "version": "2.0.0",
      "artifact": {
        "url": "https://artifacts.example.com/myapp-2.0.0.tar.gz",
        "sha256": "9f2c…e41a"
      }
    },
    "bundle": {
      "revision": 7,
      "manifest_hash": "c11d…09b3",
      "files": [
        { "id": "main", "content": "listen_port = 9090\n" }
      ]
    }
  }
}
```

- Package version and configuration bundle travel in **one document**,
  keeping them atomic (the bundle-consistency rule).
- Sent on connect when generations differ, and pushed again on every
  change while connected.
- The client applies **latest-wins**: a newer document replaces any
  not-yet-applied one, so a reconnecting or slow supervisor converges on
  the most recent desired state, never replays history.

## `observed` — client → server

The observed-state document, sent whenever reconciliation progresses
**and at least every 30 seconds** — the periodic report doubles as
application-level liveness:

```json
{
  "type": "observed",
  "observed": {
    "schema_version": "v1",
    "generation": 42,
    "version": "2.0.0",
    "config_revision": 7,
    "health": "healthy",
    "status": "installed"
  }
}
```

- `generation` echoes the desired state a report refers to, so the cloud
  can tell current reports from stale ones.
- `status` walks the reconcile phases: `idle`, `downloading`,
  `verifying`, `upgrading`, `staging`, `applying`, `installed`,
  `failed`, `rolled-back`. Terminal outcomes (`failed`, `rolled-back`)
  are durable — they keep being reported until a new desired state
  supersedes them, so a failure can never be masked by a later idle
  heartbeat.
- `health` is `healthy`, `unhealthy`, or `unknown`, straight from the
  manifest's health hook.

## Liveness and reconnection

- The supervisor pings over the WebSocket alongside its 30-second
  reports; the gateway closes connections silent for more than 90
  seconds.
- On any disconnect the supervisor redials with **jittered exponential
  backoff** (1s doubling to a 60s cap, full jitter — a gateway restart
  does not produce a reconnect stampede) and sends a fresh `hello`.
- Disconnection is visible in the console (`DISCONNECTED`, last-seen
  timestamp) but changes nothing on the machine: the supervisor keeps
  enforcing the last applied state and reconciles any missed changes
  the moment `state` arrives after reconnect.

## What is deliberately absent

No request/reply correlation, no capability negotiation, no config
hashes, no partial updates: the desired state is one small document, so
sending it whole on every change is simpler and self-healing. If your
agent can open one outbound WebSocket and read JSON, it can speak Link.
