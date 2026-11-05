# Security Policy

## Reporting a vulnerability

Please **do not open a public issue** for security vulnerabilities.

Report privately through GitHub's vulnerability reporting for this repository: **Security → Report a vulnerability** on [NexGen-X/Route-X](https://github.com/NexGen-X/Route-X/security). If that channel is not available to you, open a minimal public issue asking for a private contact path — without any vulnerability details — and one will be provided.

Route-X is maintained by a single maintainer on a best-effort basis; there is no formal response-time SLA. Reports are triaged as quickly as realistically possible, and fixes for confirmed issues are prioritized over feature work.

## Supported versions

Fixes are applied to the latest release and to `main`. Older releases do not receive backports — if you run Route-X, track the newest release.

## Security model (what you are trusting)

Route-X is a **single-admin, self-hosted** gateway for personal use and small, trusted teams:

- Every authenticated dashboard user is a full administrator. There is **no RBAC and no multi-tenant isolation** between dashboard users. Treat dashboard access as root access to the instance.
- The instance stores **paid upstream provider credentials**. Protect it like any credential vault: keep it behind TLS, restrict who can reach it, and never expose it to untrusted users.

How secrets are handled in code:

- **Gateway API keys** are stored only as **HMAC-SHA256 hashes keyed with the server pepper** (`API_KEY_PEPPER`). They cannot be recovered from the database; a database leak alone does not yield usable keys.
- **Provider credentials** are encrypted at rest with **AES-256-GCM** (`ENCRYPTION_KEY`, with `ENCRYPTION_KEY_PREVIOUS` supported as a rotation grace key).
- **Dashboard passwords** are hashed with **Argon2id**. Sessions are capped at 10 concurrent per user with a sliding TTL and a hard 7-day absolute cap; admin mutations are CSRF-protected; login attempts are rate-limited.
- **Egress** goes through an anti-SSRF dialer that blocks DNS rebinding, RFC 1918 destinations, IPv6 loopback, and cloud metadata endpoints unless explicitly allow-listed.
- **Backup restores** are scanned for psql meta-commands and applied through a scratch-database swap, so a malicious backup file cannot execute commands or half-apply.
- The repository itself contains **code only** — all secrets live in each deployment's own environment. The one historical exception is an initial-admin password that was published in pre-v1.5 documentation and git history; it must be considered **burned** and must never be used. Current code creates no default credential: without `INITIAL_ADMIN_PASSWORD` set, no admin account is created at all.

## Operator responsibilities

- Keep `ENCRYPTION_KEY`, `API_KEY_PEPPER`, and `SESSION_SECRET` strong, private, and backed up separately from the database. Losing `ENCRYPTION_KEY` makes stored provider credentials unrecoverable; leaking `API_KEY_PEPPER` weakens every API key.
- Do **not** rotate `API_KEY_PEPPER` routinely — rotating it invalidates all existing API keys. Follow the rotation procedures in the [Operations Runbook](docs/runbook-operasi.md) for the other secrets.
- The Antigravity/Google OAuth integration embeds Google's public desktop-app client value as an automatic fallback (non-secret by design for installed-app OAuth clients); set `ANTIGRAVITY_CLIENT_SECRET` in the server environment to override it.
- Keep the host, PostgreSQL, and Redis patched, and upgrade Route-X promptly when a security fix is released.
