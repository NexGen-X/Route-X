# Contributing to Route-X

Thanks for your interest in improving Route-X. The project is maintained by a small team (effectively a single maintainer plus automation), so the workflow below is deliberately strict: it keeps `main` always releasable and every change reviewable.

## Ground rules

- **`main` is PR-only.** Direct commits and pushes to `main` are forbidden. Every change — code, docs, scripts — lands through a pull request on an isolated branch (`feat/*`, `fix/*`, `docs/*`, `chore/*`), is squash-merged after CI is green, and the branch is deleted.
- **Branch protection requires up-to-date branches.** If `main` moved while your PR was open, update your branch from `main` before merging; a stale branch is rejected.
- **No secrets, ever.** The repository contains code only. Credentials, tokens, and environment files from a real deployment must never be committed.

## Development setup

Prerequisites: **Go 1.27.3**, **Node.js 24**, **PostgreSQL 16**, **Redis 7**.

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
make build                 # web bundle + gateway binary + operations CLI into ./bin

cd web && npm ci           # always a real install — do not symlink node_modules
npm run build && npm test  # tsc + vite build, then the Vitest suite
```

Database-backed Go tests use **disposable `test_*` schemas** (selected via `search_path` in the test DSN) and are cleaned up automatically. Never point tests at the `public` schema or at a development/production database. Tests skip automatically when no test database is configured.

Two environment prerequisites for a fully green local suite: (1) the admin backup tests shell out to **`pg_dump`**, so PostgreSQL client tools must be on `PATH` — the backup test skips with an explicit reason when the binary is absent (CI provides it); (2) PostgreSQL advisory locks are **per-database, not per-schema**, so a test whose work holds or waits on the global migration/partition locks runs in its own throwaway database instead of a schema on the shared test database — otherwise parallel test packages migrating at the same time can starve it of the lock.

## Conventions

- **Go**: `gofmt -l .` must print nothing; `go vet ./...` must pass. Comments and log messages in the codebase are written in **Bahasa Indonesia** — follow the surrounding style.
- **TypeScript/React**: strict mode, no `any` bypasses. `npm run build` is the typecheck gate — Vitest alone does not typecheck, so a green test run is not proof the frontend compiles.
- **Dead code gates**: CI runs a Go deadcode check and `knip` for the web app. If you add an exported function, it must be reachable from production code or be a deliberately documented test seam.
- **Migrations** are append-only: add `internal/database/migrations/NNNN_name.sql` in sequence (currently up to `0019`). Never edit a migration that has been applied anywhere — checksums are verified against the ledger at boot.
- **Commit messages** follow the existing history: conventional prefixes with an Indonesian summary, e.g. `fix(webhook): tanda tangan memakai X-RouteX-Signature`.
- **Documentation language**: `README.md` and the root community files are in English; `docs/` runbooks and guides are in Bahasa Indonesia. Keep terminology consistent within each file, and never let a document claim a feature, env var, command, or number that the code does not back up — verify against the code, not against older documents.

## Verification expectations

Run the gates that match your change before opening the PR; CI runs all of them (jobs `go` and `web` are required; `docker` also runs):

```bash
gofmt -l .
go vet ./...
go test -race ./...        # the race detector must stay at zero data races
cd web && npm run build && npm test && npm run check:deadcode
```

For gateway behavior changes, also run the live probes against a running instance:

```bash
./scripts/probe_e2e_gateway.sh
./scripts/check_metrics.sh
```

Small, single-theme PRs merge fastest. If you discover an unrelated problem while working, note it in your PR description (or open an issue) instead of silently expanding the PR's scope — unrelated fixes get their own PR.

## A note on scope

Route-X is a **single-admin, self-hosted** gateway for personal use and small trusted teams. Contributions that assume enterprise multi-tenancy or RBAC will not fit the project's design; if you are unsure whether an idea fits, open an issue to discuss it before writing code.

For security reports, do **not** open a public issue — follow [`SECURITY.md`](SECURITY.md).
