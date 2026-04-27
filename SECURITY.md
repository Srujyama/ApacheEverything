# Security Policy

## Reporting a vulnerability

If you've found a security issue in Sunny, please **do not** open a public
GitHub issue. Instead, email **security@sunny.dev** with:

- A description of the vulnerability
- The affected version (output of `sunny --version`) or commit SHA
- Steps to reproduce, ideally with a minimal proof of concept
- The impact (data exposure, RCE, auth bypass, DoS, etc.)
- Whether you've notified anyone else about it

We aim to:

- Acknowledge receipt within 72 hours
- Provide an initial assessment within 7 days
- Ship a fix or detailed mitigation within 30 days for high-severity issues
- Credit you in the release notes (or stay anonymous, your call)

## Supported versions

Sunny is pre-1.0; we currently fix security issues only on the latest
released version. Once we tag v1.0.0, we'll support the previous minor
release for at least 6 months.

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | ✅ (latest)        |
| < 0.1   | ❌                 |

## Scope

In scope:

- The `sunny` server binary, including the embedded React frontend
- The `sunny-cli` admin tool
- First-party connectors in `connectors/`
- The Helm chart and Docker image we publish to ghcr.io
- Authentication / session-cookie handling

Out of scope:

- Third-party connectors (report directly to their authors)
- The OS-level packages we depend on (DuckDB, Go runtime — report upstream)
- Self-DoS via misconfiguration (e.g. running 500 connectors on a 1-core VPS)
- Anything requiring physical access to the host

## Threat model (v1)

Sunny v1 is designed for **single-tenant, self-hosted use behind a trusted
edge**. Concretely:

- One deployment, one user, one password (or no auth at all in embedded mode).
- The operator is trusted: the connector config controls what runs
  in-process. Don't load connectors from sources you don't trust.
- DuckDB lives on local disk; back it up with `sunny-cli backup`.
- The HTTP API is HTTPS-terminated by your reverse proxy / ingress, not
  directly. The bundled HTTP listener is plain text.
- Secrets (API keys, the password hash, the session-signing key) are read
  from environment variables. Don't log them.

Multi-tenant deployments, RBAC, and audit logging are explicit non-goals
for v1. Future major versions may add them.

## Push connectors and authentication

The `/api/ingest/<instance>/...` path is **deliberately not gated by the
session cookie**, even when `SUNNY_PASSWORD_HASH` is set. The session
cookie is browser-shaped; webhook callers (vendor automation, other
services) cannot carry it.

Each push connector enforces its own auth. The bundled `webhook`
connector accepts `X-Sunny-Token: <value>` and rejects requests without
it when `requireToken` is configured (or `SUNNY_SECRET_WEBHOOK_TOKEN` is
set). Always configure a token before exposing an ingest endpoint to the
public internet.

## Hardening checklist for self-hosters

- Run behind a reverse proxy with HTTPS.
- Set `SUNNY_PASSWORD_HASH` (use `sunny-cli hash-password`).
- Set `SUNNY_SESSION_KEY` to a 32-byte random value so cookies survive
  restarts and aren't predictable.
- Mount the data dir on a volume with backups (`sunny-cli backup`).
- Restrict the container's outbound network if you don't use connectors
  that need it.
- Run as a non-root user (the bundled Docker image already does).
