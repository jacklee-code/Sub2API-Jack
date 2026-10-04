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

## Shared contributor workflow

[CONTRIBUTING.md](../CONTRIBUTING.md) is the common development, PR, release and
maintenance workflow for people and coding agents. It defines owner-only access,
automation boundaries, fresh-clone setup, validation and failure handling.
[AGENTS.md](../AGENTS.md) points agents to the same workflow; harness adapters must
not maintain separate release rules. This guide contains the implementation and
recovery details referenced by that workflow.

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

## Jack theme layer

The Jack look (Graphite · 石墨) lives in Jack-owned files: a graphite canvas
holds the sidebar, and the header and page share one paper sheet. Upstream
templates and `src/style.css` are not edited, so upstream UI changes merge as-is
and pick up the theme automatically.

- `frontend/jack/tailwind-theme.js` points the `gray`, `dark`, `primary` and
  `accent` palettes at `--jack-*` CSS variables and sets the fonts (`sans`,
  `mono`, plus a Jack `display` family). Upstream touches:
  `tailwind.config.js` wraps its export in `withJackTheme(...)`, and
  `vite.config.ts` registers `jackTheme(__dirname)`. Keep these the only edits.
  The `frontend/jack/` helpers are JSDoc-typed JavaScript so `vue-tsc -b` does
  not emit build output beside them; keep them as `.js`.
- `frontend/src/jack/theme/fonts.css` bundles the Instrument Sans, Instrument
  Serif and Geist Mono Latin subsets. `fonts-cjk.css` bundles Noto Serif SC
  (titles) and Noto Sans SC (text) as unicode-range slices, so a page only
  fetches the slices for its characters. That is about 1 MB on a first visit,
  then cached. The slices add about 11 MB to the image. Fonts are OFL; the
  licences are beside the files.
  `node jack/vendor-cjk-fonts.mjs` regenerates the slices from the Fontsource
  packages (usage in the script header). It skips Latin and emoji ranges, so
  emoji keep their colour glyphs.
- `frontend/jack/palette.js` defines the 文房 palette. Colour carries meaning
  only: blue-black ink, verdigris, brass and madder, plus an ink grey for
  upstream's decorative purples and pinks. Each hue is one OKLCH base, and all
  eleven shades come from one lightness ladder. `JACK_UI_FAMILIES` maps
  Tailwind's colour families (`blue`, `emerald`, ...) to these hues, and
  `tailwind-theme.js` applies the map. Slate, zinc, neutral and stone follow the
  Jack greys. Retune a colour by editing its base value.
- `frontend/src/jack/theme/tokens.css` holds every colour and surface value;
  `components.css` restyles upstream component classes (`.btn`, `.card`,
  `.input`, `.modal-content`, ...). The Vite plugin appends both to
  `src/style.css` before Tailwind runs, so they join the same layers after
  upstream's rules and template utilities still win. Selectors are written as
  `.x, .dark .x` pairs to match the specificity of upstream `dark:` variants.
  Change the look by editing values in `tokens.css`.
- `frontend/jack/vite-plugin.js` swaps `AppLayout.vue`, `AuthLayout.vue` and
  `HomeView.vue` for the Jack versions in `src/jack/layouts/` and
  `src/jack/views/`, and wraps the `chart.js` package. Only plain imports are swapped; Vue's SFC sub-requests
  (`?vue&type=style`) stay with the original, and the build fails if a replaced
  module ends up importing its own replacement. Login logic stays upstream.
  Administrator home content and the compact home page still render the upstream
  `HomeView`. Vitest does not load the plugin, so upstream tests keep testing
  upstream components.
- `JackAppLayout.vue` composes the upstream `AppSidebar` and `AppHeader` and keeps
  all of their logic (menus, feature flags, tour anchors, titles, balance, user
  menu). The sidebar sits in a `.dark` wrapper, so it renders dark in both colour
  modes. The sheet is the scroll container. The layout mirrors upstream
  `AppLayout`'s onboarding wiring, so review it when upstream changes that file.
  `shell.css` styles the composition. It is unlayered and scoped under
  `.jack-shell`, so it follows upstream utilities. It also resizes
  `TablePageLayout` for the sheet.
- `src/jack/__tests__/themeHooks.spec.ts` fails, and `vite build` stops, when an
  upstream sync renames a hooked file, removes a Tailwind directive from
  `style.css`, changes a replaced layout's slots or adds props to a replaced
  component. Fix the Jack side (paths in `vite-plugin.js`, slots in the Jack
  layout); do not edit upstream files to satisfy the hook.
- `frontend/jack/style-hooks.js` lists upstream markup the look relies on, such as
  sidebar classes, the header's `<h1>` and `h-16`, and dashboard figure classes.
  It also records the reviewed hash of upstream `AppLayout.vue`. When these
  drift, `node jack/check-style-hooks.js` in `Jack checks` and `vite build` only
  warn. A styling drift never blocks an upstream sync. Read the warning in the
  job summary, update the Jack selector or the reviewed hash, and open a normal PR.
- After a sync that adds UI, look at the new pages in both colour modes.
  Hard-coded colours (hex values, `bg-white`) do not follow the theme; fix a
  visible mismatch with a rule in `components.css`, not in the `.vue` file.
  Chart.js paints on a canvas, so CSS cannot recolour it. The plugin resolves
  upstream `import ... from 'chart.js'` to `src/jack/charts/chartjs.ts`. That
  wrapper re-exports chart.js and registers `jackChartTheme`. Before each update,
  `jackChartTheme` maps every Tailwind default colour in datasets, axes and the
  legend to the `JACK_CHART_FAMILIES` colour of the same shade. That is a wider
  categorical set, so series stay distinct; greys use `--jack-gray-*`. Defaults
  are recognised through the static copy in `jack/tailwind-defaults.js`, and the
  hook check warns if the installed Tailwind ever differs from it.

## Traditional Chinese locale

The language menu offers English, 简中 (`zh`, upstream) and 繁中 (`zh-Hant`,
Jack). Upstream maintains only `en` and `zh`; the Traditional messages are
generated from upstream `zh` at build time, so an upstream sync needs no locale
work and new keys appear in 繁中 automatically.

- `frontend/jack/vite-plugin-zh-hant.js` resolves
  `@/i18n/locales/zh?jack-zh-hant` (and the relative imports inside that folder)
  to marked copies of the upstream modules and converts their source with
  `toTraditional` from `frontend/jack/zh-hant-convert.js`: OpenCC `cn` → `tw`
  (character forms only, no regional vocabulary), then the small
  `JACK_ZH_HANT_CHARACTERS` table (臺 → 台, 賬 → 帳). `opencc-js` is a dev
  dependency; the browser only receives the converted chunk, and only when
  繁中 is selected. Both `vite.config.ts` and `vitest.config.ts` register it.
- `frontend/src/jack/i18n/zhHant.ts` is the locale module. Fix a wrong message
  by adding it to `zhHant.overrides.ts` (same shape as upstream `zh`); add a
  character to the table only when the fix applies everywhere.
- Upstream touches: `src/i18n/index.ts` (locale code, loader, browser detection
  for `zh-TW`/`zh-HK`/`zh-MO`/`zh-Hant`, menu entries with a `short` label),
  the button label in `LocaleSwitcher.vue`, and a few `=== 'zh'` checks relaxed
  to `startsWith('zh')`. Keep new Chinese checks prefix-based. The 繁中 entry
  uses the 🇭🇰 flag (Windows shows it as "HK", like "US" and "CN").
- `src/jack/i18n/localeMigration.ts` runs once per browser (marker
  `jack_locale_zh_hant_migrated`): a `zh` saved before 繁中 existed becomes
  `zh-Hant` when the browser language is Traditional Chinese. Saved English,
  Simplified browsers and later choices are kept.
- `src/jack/__tests__/zhHantLocale.spec.ts` checks that the generated keys equal
  upstream `zh`, every message is converted and compiles, and every override
  key still exists upstream.
- Not converted: Chinese hard-coded in upstream templates, server-provided text
  (site settings, compliance phrases) and backend e-mails, which treat any
  `zh*` `Accept-Language` as Simplified. Traditional text renders with the
  bundled Noto SC faces, which cover every character the converted messages use
  (glyph shapes follow the mainland standard); no TC font is bundled.

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
