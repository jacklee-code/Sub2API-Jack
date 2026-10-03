# Sub2API-Jack

This downstream follows official stable releases using merge commits. `main` is
the published development branch; do not rebase it or force-push it. Develop new
features on branches, add regression tests, and merge through a pull request.

Fleet management owns selected existing subscriptions without replacing their
IDs or billing history. A group belongs to one fleet; users may belong to several
fleets. Members normally inherit the fleet expiry, with an explicit independent
expiry option. Moving a member also rebinds that member's source-group API keys.
Leaving disables access but retains per-fleet usage for rejoining. Date changes
never reset quota. Reset actions only affect current managed memberships.

Custom SQL migrations have a separate ledger and run after upstream migrations.
No production users, account credentials, or deployment secrets belong here.

Upstream synchronization and Jack releases are automated after checks pass.
Production deployment remains operator-triggered. The panel's update and rollback
source is this repository. Docker must preserve the installed executable across
container recreation; image changes and executable changes are separate actions.

## Daily workflow

1. Branch from `main`, implement a feature and add behavior-level tests.
2. Push and open a PR. `Jack verified` covers backend tests, frontend tests/build,
   and deployment metadata checks. Merge after reviewing the result.
3. `Publish Jack` creates the next `v<upstream>-jack.N`, Linux ARM64/AMD64 archives,
   checksums, `jack-release.json`, and GHCR images. Production is not changed.
4. `Sync upstream stable release` runs hourly at minute 17. It creates a merge PR,
   runs the same checks against its exact commit, merges on success and explicitly
   dispatches publication. An existing failed or draft sync PR is left for repair.
   Run `python3 scripts/jack/sync-upstream.py --dry-run` for a read-only check.

Do not reset this repository to upstream or use GitHub's discard-changes sync.
The `.jack/upstream.json` commit must remain an ancestor of every release. Preserve
upstream import/module paths; changing them would create unnecessary merge churn.
Never overwrite release tags. A failed publication can resume its unpublished tag.

## Fleet behavior

Each group has at most one live fleet. Membership owns the subscription even after
leaving: the row is suspended, not deleted, so rejoining keeps the same ID and
in-window usage. Existing personal subscriptions require explicit adoption.
Independent expiry is a per-membership setting, not an administrator-role bypass.
It participates in resets but not fleet date changes. Removal always suspends it.
Transfers between platforms are rejected; moving within the same platform rebinds
only the member's source-group keys. Existing destination membership is retained.

Direct term changes to managed subscriptions, and deleting/changing the platform
or billing type of bound groups, are guarded. Remove members and archive the fleet
before retiring a group. Archived subscription ownership is retained as history.
Members of archived fleets cannot be silently adopted into a different fleet.

Operations serialize through a PostgreSQL advisory lock and check fleet versions.
The idempotency record commits with the business changes. A cache outage returns
`cache_pending`; a persisted worker retries every ten seconds, including after a
restart. Retrying the same key does not repeat the business operation. Do not
manually delete pending operation records.

## Deployment and recovery

Copy `deploy/jack/switch-image.sh` into the existing deployment directory and run
`./switch-image.sh 0.2.13-jack.1`. It requires Bash, Python 3, flock and Docker Compose.
The script preserves the existing Compose network, environment and volumes. It
owns `docker-compose.override.yml` only when that file has `x-jack-managed: true`;
an unrelated override must be merged explicitly before using the script.

The entrypoint seeds `/app/data/jack-runtime/sub2api` only when absent. Panel updates
atomically replace that persistent file. A normal container recreation does not
silently revert it. The image-switch script explicitly replaces both image and
persistent executable, including when intentionally selecting an older image.

Panel updates require a same-runtime fingerprint and a mandatory SHA-256 checksum.
The fingerprint covers the upstream Docker runtime definition, entrypoints and
runtime resources. A changed runtime requires `switch-image.sh`. The panel saves
a database dump, executable and available config under
`/app/data/jack-runtime/update-backups/` before replacing the executable. These
sensitive local backups use private permissions; include them in existing encrypted
off-host backup policy and monitor disk usage. They are not automatically pruned.

The image switch first pulls the candidate, then stops application writers and
saves a PostgreSQL custom-format dump, app data, previous executable/image identity,
and deployment files into `.jack-backups/<UTC timestamp>/` (mode 0700). It starts
only the application service and requires a healthy container. Failure retains the
backup and reports its path. Before replacing the executable, an installed
`backup-jack-offsite.py` hook encrypts and verifies the snapshot off-host. A backup
failure resumes the unchanged old application. Database restoration is never automatic.

For a recovery that requires database restoration, stop the application, restore
the saved deployment files and previous image, restore the app-data archive into
the existing data volume, and restore the database dump with PostgreSQL `pg_restore
--clean --if-exists --no-owner` (image-switch backup) or `gzip -dc ... | psql`
(panel backup). This rewinds writes since the snapshot and must be done during a
maintenance window. Start the app, verify version/health, memberships, key bindings,
expiry and quota before reopening access. A compatible binary-only rollback does
not restore data. Panel rollback candidates are limited to the last three Jack
releases on the same upstream base, schema epoch and runtime fingerprint.

Initial adoption is an explicit call to the fleet API, never a schema migration.
Production member identities and private snapshots stay outside the repository.

## First publication and package access

GitHub Container Registry packages start private even when their source repository
is public. After the first successful publication, the owner sets the package
visibility to Public in GitHub's Package settings once. Public Release binaries
are independently available. Do not add a GitHub token to application settings to
work around package visibility: the panel updater downloads public Release assets.

For this repository the package is `ghcr.io/jacklee-code/sub2api-jack`. Source-linked
workflow access is supplied by the image's `org.opencontainers.image.source` label.
Main is protected by `Jack verified`; push feature branches and merge their PRs.
The hourly upstream workflow preserves failed/conflicting PRs for repair and does
not deploy to a VPS. Release tags and published versions must never be overwritten.

## Verification recorded for the first release

- Real PostgreSQL transactions cover adoption, independent expiry, source-key
  transfer, leave/rejoin, reset scope, idempotency and concurrent stale revisions.
- Browser checks cover desktop/mobile pages, member transfer controls and the
  drag/drop confirmation flow. Ordinary requests and SSE through isolated mock
  providers switch destinations while preserving the same API key.
- HTTP checks exercise removal/rejoin, expiry and renewal, reset-all, managed
  subscription guards, administrator authorization and repeated operation keys.
- An isolated restoration of the existing deployment's database was migrated and
  adopted successfully with all subscription IDs, counters and billing windows
  preserved. No application workers ran against that restored production data.
- Executable replacement and compatible rollback run in subprocess tests. Bad
  checksums leave the executable unchanged. Container recreation retains an
  updated persistent executable. Runtime changes are rejected for binary updates.
- Real local Git fixtures verify successful upstream merges preserve custom
  features, and conflicting merges produce a draft without discarding local work.
