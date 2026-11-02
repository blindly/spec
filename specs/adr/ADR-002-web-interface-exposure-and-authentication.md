---
created: 2026-11-01
id: ADR-002
status: proposed
title: Web interface exposure and authentication
type: adr
---

# Web interface exposure and authentication

## Context

`spec web` binds to `web_host` (default `127.0.0.1`), and users can set it
to `0.0.0.0` to reach the UI from other machines. The UI has no
authentication:

- All pages, including spec contents, README, and agent docs, are readable
  by anyone who can reach the port.
- Write forms carry a per-process token. It stops other websites from
  submitting forms in the user's browser (CSRF), but it's embedded in the
  page, so anyone who can load the page can also submit edits.

So binding to `0.0.0.0` gives everyone on the network read and write
access to the specs. REQ-008 says the web UI must not allow that.

Options:

- **A. Loopback only.** Refuse non-loopback binds. Remote access goes
  through an SSH tunnel (`ssh -L 8080:localhost:8080 host`) or a reverse
  proxy that handles auth.
- **B. Loopback by default, with an explicit opt-in.** Allow non-loopback
  binds only with `--insecure-bind` (or `web_insecure: true`) and print a
  warning that the UI is unauthenticated.
- **C. Built-in shared-secret auth.** Generate an access token at startup
  and print it in the URL, like Jupyter does. Store it in a cookie after
  the first visit, and require it for all routes when not bound to
  loopback.
- **D. Keep the current behavior and document the risk.**

## Decision

Proposed: **C**, falling back to **B** if C turns out too heavy.

C keeps the single-binary, no-dependency design (stdlib only: random
token, cookie, a check in one handler) and makes `0.0.0.0` safe for the
trusted-network use case that already exists. Loopback binds keep working
with no login. A token-in-URL flow is a familiar pattern from Jupyter.

## Consequences

- Binding to `0.0.0.0` stops exposing specs to anyone on the network.
- Users get a new URL with a token each time `spec web` restarts, unless
  the token is made configurable.
- This isn't real user management: anyone holding the token has full
  access, and plain HTTP sends it unencrypted. Use an SSH tunnel or a TLS
  proxy on untrusted networks.
- Choosing A or B instead would be simpler, but would break the current
  `web_host: 0.0.0.0` workflow or keep it unsafe.
- Once accepted, implement it as a feature spec that addresses REQ-008.
