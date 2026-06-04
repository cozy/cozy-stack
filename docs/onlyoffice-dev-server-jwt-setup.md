[Table of contents](README.md#table-of-contents)

# OnlyOffice Dev Server — JWT Contract and Configuration Guide

This document explains **why** `scripts/start-oo.sh` configures the OnlyOffice Document
Server the way it does, so the contract never has to be re-discovered by hand.

---

## 1. The JWT Contract — 5 Settings in `local.json`

The authoritative Document Server configuration lives in
`/etc/onlyoffice/documentserver/local.json` inside the container, under the
`services.CoAuthoring` key. Five settings establish the correct contract with cozy-stack:

| Setting (path under `services.CoAuthoring`) | Value | Justification |
|---------------------------------------------|-------|---------------|
| `token.enable.request.inbox` | **`false`** (mandatory) | cozy-stack signs the editor config with its own JSON shape (`editor` / `document` keys, not the standard `editorConfig.callbackUrl`). If the DS validates the inbox token it rejects the open with *"The document security token is not correctly formed"* / log *"auth missing required parameter editorConfig.callbackUrl (since 7.1 version)"*. |
| `token.enable.browser` | **`false`** | Same reason as inbox: the browser token validation expects the standard shape that cozy-stack does not produce. |
| `token.enable.request.outbox` | `true` if `OO_SECRET` non-empty, else `false` | When enabled, the DS signs its save-callbacks so cozy-stack's `checkToken` function accepts them. Both sides must share the same secret. |
| `secret.inbox.string` = `secret.outbox.string` = `secret.session.string` | `OO_SECRET` | All three secret strings must equal cozy-stack's `onlyoffice_outbox_secret` / `onlyoffice_inbox_secret`. A mismatch causes the callback to be rejected with HTTP 400. |
| `request-filtering-agent.allowPrivateIPAddress` | `true` | On a Docker bridge network the DS reaches cozy-stack through the gateway (typically `172.17.0.1`), which is a private IP address. OnlyOffice's built-in SSRF filter blocks it by default. See [The SSRF Trap](#3-the-ssrf-trap) below. |

`start-oo.sh` writes `request-filtering-agent` **under `services.CoAuthoring`** (the same
section as `token` and `secret`). That is where OnlyOffice actually reads it — verified against
a working Document Server's `local.json`. A top-level key (sibling of `services`) is silently
ignored and the SSRF filter keeps blocking the docker gateway → *"download failed"*.

---

## 2. The Two Consistent Modes

There are exactly two stable configurations. Mixing settings between modes produces
JWT errors or failed saves.

### Mode A — JWT Fully OFF

Use this when both sides have empty secrets (simplest local dev setup).

| Side | Setting |
|------|---------|
| cozy-stack config | `onlyoffice_inbox_secret: ""` and `onlyoffice_outbox_secret: ""` |
| DS `local.json` | `token.enable.request.inbox = false`, `token.enable.request.outbox = false`, `token.enable.browser = false` |
| DS `local.json` | `request-filtering-agent.allowPrivateIPAddress = true` (still required) |

Launch with: `./scripts/start-oo.sh` (no `OO_SECRET`).

### Mode B — Outbox JWT ON

Use this when cozy-stack has a non-empty `onlyoffice_outbox_secret`. The DS signs
its callbacks so cozy-stack validates them. The inbox token remains off.

| Side | Setting |
|------|---------|
| cozy-stack config | `onlyoffice_inbox_secret: <secret>` and `onlyoffice_outbox_secret: <secret>` |
| DS `local.json` | `token.enable.request.inbox = false`, `token.enable.browser = false` |
| DS `local.json` | `token.enable.request.outbox = true` |
| DS `local.json` | `secret.inbox.string = secret.outbox.string = secret.session.string = <secret>` |
| DS `local.json` | `request-filtering-agent.allowPrivateIPAddress = true` |

Launch with: `OO_SECRET=<secret> ./scripts/start-oo.sh`.

The `<secret>` value must be identical on both sides.

---

## 3. The Env-Var Trap

The Docker environment variables `JWT_ENABLED`, `JWT_SECRET`, and
`ALLOW_PRIVATE_IP_ADDRESS` are **only applied during the container's first
initialization** — when `local.json` is generated from scratch. If `local.json`
already exists (e.g. because the container was previously started, or a volume
persists it), these env vars are **silently ignored**.

Empirical observation: a container started with `-e JWT_ENABLED=false` was found to
still have `token.enable.request.outbox=true` in its `local.json` because the file
had been written on a previous run.

**The reliable fix:** edit `local.json` directly, then run `supervisorctl restart all`
inside the container. `scripts/start-oo.sh` does this automatically after every
container start:

1. Reads the current `local.json` out of the container via `docker exec ... cat`.
2. Transforms it on the **host** with python3 (the DS 9.3.x image ships with no python3).
3. Writes it back via `docker exec -i -u root ... cat > local.json && chown ds:ds ...`.
4. Restarts all DS services with `docker exec ... supervisorctl restart all`.

---

## 4. The SSRF Trap

When a Document Server runs in a Docker bridge network, it reaches cozy-stack
through the Docker gateway IP (typically `172.17.0.1`). OnlyOffice's built-in
request-filtering agent treats this as a private IP address and blocks it:

```
Error: DNS lookup 172.17.0.1 ... is not allowed. Because, It is private IP address.
```

This manifests as a *"Download failed"* error in the editor when it tries to fetch
the document from cozy-stack.

**Fix:** set `request-filtering-agent.allowPrivateIPAddress = true` in `local.json`
at the top level (sibling of `services`). `start-oo.sh` sets this unconditionally
regardless of the JWT mode.

Note: `allowMetaIPAddress` is left `false` for safety (no need to allow cloud
metadata endpoints in a local dev setup).

---

## 5. The Frontend Trap (cozy-drive Companion — not a cozy-stack bug)

**This is NOT a cozy-stack issue.** It is a cozy-drive (frontend) companion concern.

The OnlyOffice editor config produced by cozy-stack uses the keys `editor` and
`document` (see `model/sharing/open_office.go`, the `sign()` function). The registry
version of the cozy-drive app (v1.99.0 at time of writing) expects the standard
`editorConfig` key and does not fall back to `editor`. This produces:

```
"The document security token is not correctly formed"
```

in the browser, even when the DS and cozy-stack JWT configs are correct.

**Fix:** open from a cozy-drive build that maps `editorConfig || editor`. Use the
development build of cozy-drive, not the registry-published version, for local E2E
testing of office document editing.

---

## 6. Secret-Verification Recipe

Use this recipe to confirm that a running cozy-stack instance accepts a given secret
as its `onlyoffice_outbox_secret`, without needing to read the config file.

**What it does:** The DS signs its save-callbacks with an HS256 JWT whose claims
are `{"payload":{"key":K,"status":S,"url":U}}`. Replicate that signature:

```bash
INSTANCE_URL="http://mycozy.localhost:8080"
CANDIDATE_SECRET="changeme-shared-secret"

# Pick any plausible values for key/status/url:
KEY="some-doc-key"
STATUS="2"
CALLBACK_URL="http://localhost/unused"

# Build the JWT claims and sign with the candidate secret.
# Requires python3 + pip install pyjwt (or use another JWT tool).
TOKEN=$(python3 -c "
import jwt, json, os
payload = {'payload': {'key': '$KEY', 'status': $STATUS, 'url': '$CALLBACK_URL'}}
print(jwt.encode(payload, '$CANDIDATE_SECRET', algorithm='HS256'))
")

# POST to the office callback endpoint.
# HTTP 200 -> secret matches onlyoffice_outbox_secret.
# HTTP 400 -> secret does not match (or the endpoint rejected for another reason).
curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$INSTANCE_URL/office/callback" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"key\":\"$KEY\",\"status\":$STATUS,\"url\":\"$CALLBACK_URL\"}"
```

Expected response: `200` if the secret matches, `400` if it does not.

See `model/office/callback.go` — the `checkToken` function skips validation only
when `OutboxSecret == ""`. When `OutboxSecret` is non-empty the token is verified
with that exact value.

---

## 7. Code References

These are the cozy-stack source locations relevant to the JWT contract:

- **`model/sharing/open_office.go`** — the `sign()` function and the `editor` /
  `document` JSON shape sent to the browser. This is the shape the DS inbox would
  try to validate (and reject) if `token.enable.request.inbox` were `true`.

- **`model/office/callback.go`** — the `checkToken` function. It validates the
  incoming save-callback JWT against `OutboxSecret`. When `OutboxSecret == ""` the
  check is skipped entirely (Mode A). When non-empty, the callback's HS256 JWT must
  be signed with the same value (Mode B).

---

*This document encodes the JWT contract as empirically verified against a running
OnlyOffice Document Server 9.3.x (container `onlyoffice/documentserver:latest`)
and a cozy-stack development instance.*
