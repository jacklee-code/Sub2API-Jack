# Sub2API-Jack agent entrypoint

The shared workflow for **people and all coding agents** is
[CONTRIBUTING.md](CONTRIBUTING.md). Read it before editing. Read
[docs/JACK-DEVELOPMENT.md](docs/JACK-DEVELOPMENT.md) for fleet invariants, deployment
or recovery work. These files travel with the repository; a previous chat or
private agent memory is not required. Keep workflow changes in the shared guide.

## Apply the shared workflow

- Work in `jacklee-code/Sub2API-Jack`; `Wei-Shaw/sub2api` is upstream. Verify remotes
  before GitHub operations. Preserve unrelated work and start branches from current
  `origin/main`. Do not rewrite shared history or discard Jack changes.
- This is an owner-maintained repository with no external PRs. Agents act using
  `jacklee-code`'s authorized session; cloning does not grant write/publish access.
  Repository-owned GitHub Actions perform the delegated sync and release work.
- Ordinary PRs run checks but do not auto-merge by default. Follow the session's
  authorized scope for merging or deploying; reuse authorization already given.
  Every merge into `main`, including docs, triggers verification and publication.
- Use the validation commands in `CONTRIBUTING.md`. For documentation-only changes,
  validate links, paths and consistency locally; required CI still runs. Report
  failures or unavailable checks instead of disabling protection.

## Code and data boundaries

- Keep Jack behavior in focused modules: `backend/internal/jackfleet`,
  `backend/jackmigrations`, `backend/internal/handler/admin/fleet_handler.go`, and
  `frontend/src/views/admin/FleetsView.vue`. Keep upstream integration edits small
  and preserve upstream Go module/import paths.
- Keep the Jack frontend look in `frontend/jack/` and `frontend/src/jack/`; do not
  restyle upstream `.vue` files or `src/style.css`. See the theme layer section in
  `docs/JACK-DEVELOPMENT.md`.
- Append custom migrations to the separate Jack ledger; never rewrite applied
  migrations. Review data and binary rollback compatibility for persistent changes.
- Preserve subscription IDs and usage on adoption, history on removal, in-window
  usage on rejoin, and key strings on transfer. Date changes do not reset quota.
  Independent expiry is a membership setting, not an administrator-role bypass.
- Preserve transactions, revision checks, idempotency and cache invalidation.
  Test the affected behavior and authorization boundaries with isolated fixtures.
- A source clone is not the live deployment directory. Obtain the current host
  runbook for operations; never commit production data, credentials or backups.
- For operations on Jack's VPS, read the canonical policy on that host:
  `/home/jack/wikis/jack-vps-knowledge-base/concepts/vps-infrastructure/multi-vps-proxy-policy.md`.
  Keep it in that location. Its absence on a development machine does not block
  ordinary source work.
- Report changes, validation, PR/release status and remaining operator actions.
  Do not describe a merged PR as a successful release without checking publication.
