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
   dispatches publication, without a person or an LLM. Conflicts listed in
   `.jack/merge-policy.json` are resolved mechanically: `upstream_wins` takes the
   official file, `regenerate` rebuilds `pnpm-lock.yaml` and
   `backend/cmd/server/wire_gen.go` from the merged inputs (Wire uses readonly
   module metadata), and `union` keeps both
   sides of `go.sum`. `module_versions` retains the newer stable same-major dependency version only when both conflict sides contain identical dependency paths and indirect markers. `jack_locales` reapplies only the known fleet/chat imports and spreads when these are the entire Jack delta from the merge base. Any other conflict opens a draft assigned to
   `jacklee-code` and fails that run so GitHub notifies the owner; later runs
   fail while its PR is a draft, still has conflict markers, records the wrong upstream release, or lacks the target upstream/latest main ancestry. A repaired non-draft PR targeting main in this repository resumes verification of its exact head, followed by merge and publication dispatch on success. Existing branches are never overwritten.
5. When Go or vitest tests fail, `Jack checks` runs the same tests on the
   upstream commit in `.jack/upstream.json` (cached per commit). Failures the
   official release also has are inherited: they are allowed and listed in
   warnings, the sync PR and the release notes. Failures that only Jack has
   still block. Lint, type-check and build are never compared.
   Run `python3 scripts/jack/sync-upstream.py --dry-run` for a read-only check.

Do not reset this repository to upstream or use GitHub's discard-changes sync.
The `.jack/upstream.json` commit must remain an ancestor of every release. Preserve
upstream import/module paths; changing them would create unnecessary merge churn.
Never overwrite release tags. A failed publication can resume its unpublished tag.

The default sync push uses `GITHUB_TOKEN`, which can be rejected when an upstream
release changes `.github/workflows/`; `contents: write` does not grant workflow
write access. Failed commands log their captured stdout/stderr with credentials
redacted and retain their nonzero status. Workflow permission rejections include
an actionable owner-facing error.

The optional repository Actions secret `JACK_SYNC_PUSH_TOKEN` supports a dedicated
fine-grained PAT scoped only to `jacklee-code/Sub2API-Jack`, with Contents and
Workflows read/write permission. The owner provisions and rotates it; its support
does not mean it is configured. The script removes the secret from its environment
before starting children and supplies it only to `git push` through temporary Git
configuration in that process's environment. It is not written to disk or command
arguments, and merged dependency/regeneration children do not inherit it. PR,
status, merge and publication dispatch continue to use `GITHUB_TOKEN`. This is a
trusted-upstream workflow, not a sandbox for malicious code. Without that secret,
the owner can review and push the sync branch from an authorized local session,
then follow the required checks, merge-commit and release verification procedure in
[CONTRIBUTING.md](../CONTRIBUTING.md#官方上游更新). Never copy a local `gh` OAuth token
into Actions or bypass branch protection to work around the rejection.

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

## Fleet upstream reset synchronization

Fleet Management can opt each fleet into following one ChatGPT OAuth parent
account bound to its group. Enabling requires a fresh seven-day baseline and
preserves existing member usage. Changing sources starts a new observation
generation. Disabling preserves usage and starts a new local seven-day timer.
The setting defaults to off; migration does not adopt or reset any fleet.

Normal gateway response snapshots (including the existing WebSocket/header
paths), genuine admin usage reads and successful manual reset-credit operations
feed the Jack observer. There is no periodic upstream polling. A confirmed
seven-day rollover, or an early drop from at least 5% used to at most 1%, clears
current members' daily/weekly/monthly subscription counters. Detection queues and
wakes an immediate usage-only confirmation query, without a fixed waiting period.
An early drop is confirmed when the query differs from the first low sample by
at most two percentage points and still lies below the pre-reset usage. Both early
and scheduled resets require the same identity/plan; scheduled resets also require
an upstream reset deadline within one minute of the candidate. Request headers
alone cannot confirm a reset,
and busy request traffic cannot overtake an in-flight confirmation's timestamp.
The durable one-shot job also survives restart. The same upstream deadline may
contain multiple early resets. Missing windows, stale/out-of-order observations,
identity/plan changes and computed display values never grant quota.

An authenticated request rejected by a managed subscription's quota can enqueue
an asynchronous usage-only check. The request retains its ordinary quota error;
a subsequent retry works after synchronization. Jobs coalesce by source account,
with a database-enforced 60-second cooldown for ordinary quota checks and
restartable leases. Reset confirmation bypasses that cooldown; repeated signals
share the same job and lease. A mismatched confirmation retains usage and enters
`needs_review`, without repeatedly querying for every low request snapshot. A later
explicit query can confirm it, or a new high sample can establish a new baseline.
Idle accounts and API-key-level limits do not create queries. Failed reads retain
usage and show an unavailable state until another event permits a retry.

Manual reset credits are an explicit trigger independent of the seven-day
percentage heuristic: an upstream `reset` (or the supported `ok`/`success` aliases) with
`windows_reset > 0` clears all three local counters. A zero-window or unknown
outcome does not reset usage. Completed redemption operations are never replayed
to retroactively clear later usage. The admin endpoint accepts `Idempotency-Key`;
the panel retains the key in session storage across a lost response. The database records
the redemption id, reference-account identity and target fleet generations before
calling upstream. Ambiguous outcomes retry the same redemption id, including
after restart; a changed account identity stops recovery. Confirmed outcomes and
fleet mutations commit together. The regular account recovery/display refresh
cannot discard a successful redemption. The panel distinguishes credit usage
from pending fleet cache synchronization. No automatic-credit configuration is
changed by this feature.

Jack tables store source configuration, observations, jobs, redemption operations
and reset events. Detection, manual fleet resets, transfers and redemption results
serialize with the existing fleet transaction lock. A manual reset cancels older
candidates; successful redemption establishes a barrier for stale observations
and rebaselines on the next real sample. Existing subscription IDs, API-key
strings, expiries and usage logs are preserved. New charges are counted after the
reset transaction; historic charges are not reconstructed. Inactive memberships
are not reset, and the existing rejoin behavior applies the latest fleet reset.

Policy-aware subscription reads, validation, progress and maintenance suppress
local weekly rollover for enabled memberships. The repository additionally guards
weekly reset writes from stale callers. Daily midnight and ordinary monthly
maintenance continue. Weekly deadlines come from the upstream observation rather
than the time of local synchronization. Source removal or unbinding suspends
synchronization without choosing a replacement or re-enabling local weekly resets.

This is an additive Jack migration. Before rolling back to a binary without this
feature, disable synchronization for all fleets and resolve pending redemptions;
older binaries do not enforce the external weekly policy or recover these jobs.
Do not delete pending operation rows as a recovery technique. Production rollout
and enablement remain operator actions. Tests use isolated PostgreSQL and local
mock HTTP upstreams only: never consume an operator's real reset credits to verify
this feature.

## Account reserve energy

OpenAI account editing includes an opt-in **Reserve energy / 後備能源** plan.
`internal/jackreserve` owns its configuration; the gateway integration lives in
`internal/service/jack_reserve_energy.go`, and the editor in
`frontend/src/jack/accounts/ReserveEnergySettings.vue`. Small hooks reuse the
upstream account APIs, scheduler entry point and request failover loops.

- `accounts.extra.jack_reserve_enabled` defaults to false;
  `jack_reserve_account_ids` holds up to 16 distinct, ordered account IDs. Only
  existing non-shadow OpenAI accounts may be sources or destinations. Self
  references are rejected. Plans are single-level: a destination's own plan is
  never recursively expanded.
- The primary's group bindings authorize the explicit reserve delegation. A
  reserve need not join those groups; its own bindings are unchanged. Within
  the primary's groups the listed destinations are excluded from ordinary
  weighted/sticky scheduling, including while the primary is healthy. Other
  groups retain their normal scheduling. Multiple exhausted primary plans are
  considered by primary priority, then ID; each plan preserves its exact order.
- An upstream quota cooldown, local API-key quota or quota scheduling pause
  activates the plan. Manual disablement, expiry and credential/proxy errors on
  the primary do not activate it. Each new request checks the primary again;
  quota recovery resumes ordinary primary scheduling, even if sticky state
  points at a reserve.
- Spendable Codex **Credit takes precedence over reserves**. A positive or
  unlimited upstream `codex_credits_snapshot` prevents quota auto-pause from
  preempting those points. At a full subscription window the primary also gets
  an upstream attempt when the balance snapshot is missing or stale: 100%
  subscription usage alone does not prove that paid Credit is unavailable.
  An actual upstream 429 retains its normal cooldown and permits fallback;
  the balance display never clears that cooldown. Reset cards are a separate
  feature, and explicit administrator scheduling restrictions remain effective.
- Scheduling uses the first eligible destination, waits on its concurrency
  limit, and advances only when it is unavailable or the existing request
  failover policy excludes it after an error. The request's failed-ID set
  prevents repeating a destination. Explicit reserve chains extend the ordinary
  account-switch budget only as far as their remaining entries. Existing
  cancellation, partial-output replay guards and first-output timeout caps stay
  effective. Account-bound `previous_response_id` continuations that cannot move
  are not replayed through another account.
- The selected account is the real reserve account, with its own credentials,
  base URL, model mapping, proxy (or direct connection), concurrency slot and
  usage accounting. The primary's proxy is never inherited. User/group billing,
  model/channel restrictions, privacy requirements, profit controls and target
  availability checks still apply. Account configuration is reread after slot
  acquisition. No credentials are sent to the browser by this feature.
- Create/update, bulk update and extra-merge APIs validate the plan. Unrelated
  extra updates preserve existing plan keys. Scoped administrator API keys do
  not expose these extra writes. Data imports clear reserve configuration:
  numeric IDs belong to their original installation, so operators configure the
  plan again after importing. Ordinary same-installation account copies retain
  configuration.

This feature uses existing JSONB storage and needs no schema migration. Older
binaries ignore the plan; disable it before rollback if reserves also have
ordinary group bindings, since older schedulers do not reserve those IDs.
An empty discovery result is cached for one second to avoid adding a database
lookup to every request on installations without plans. Local plan writes
invalidate this after commit; other instances discover activation within one
second. Active plans and primary quota state are always read afresh.
Verification uses synthetic accounts and mock upstreams, never real operator
Credit, credentials or production records. Publication and deployment follow
the ordinary contributor workflow and do not enable plans automatically.

## Chat mode

`/chat` lets users talk to models and generate images in the panel. Conversations,
messages and attachment records live in `jack_chat_*` tables
(`backend/jackmigrations/002_chat.sql`); model calls are ordinary gateway requests.

- Each (user, group) pair gets one hidden API key with the `sk-jackchat-` prefix
  (`internal/pkg/jackchatkey`), created on first use. `internal/jackchat` serves
  the request in-process through the router's real `/v1` chain, so API-key auth,
  subscription/balance checks, scheduling, proxies, fingerprints, rate multipliers,
  reasoning-effort policy and usage billing are exactly those of an API call with
  that key. Usage rows therefore carry the chosen group; the usage DTO adds
  `chat_mode`, and the table shows "<group> 聊天模式".
- The prefix is the only marker. Key lists and counts exclude it, custom keys
  cannot use it, DTOs mask it, and both API-key middlewares reject it unless the
  request context carries the in-process marker. Key strings never reach the browser.
- Only `openai` groups the user can bind are offered; the default is the stored
  preference while it stays available, otherwise the newest group. Each
  conversation keeps its own mode, group, model and options; changes become the
  user's preference.
- Chat mode sends `/v1/responses` (`reasoning.effort`, optional `web_search`
  tool; citations are stored per message). Image mode lists only `gpt-image*`
  models and sends one `n=1` request per image (1-4, bounded by user concurrency),
  using `/v1/images/edits` multipart when reference images are attached. Codex
  image accounts ignore `size` (every requested size returned the same image), but
  follow a ratio stated in the prompt, returning about 1.5 megapixels at that ratio
  (1:1 1254x1254, 16:9 1672x941, 4:3 1448x1086, 2:3 1024x1536 in testing). So no
  size is sent; `ImagePrompt` prepends a ratio sentence for the chosen ratio (none
  for auto). The gateway bills by output size, so these images land in its 2K
  tier. Captions show the real size.
- Each chat request carries `prompt_cache_key: jackchat:<conversation id>`, which
  pins the conversation to one upstream account and prompt cache. History is
  estimated against `max_context_tokens` (admin setting, default 200,000); on
  overflow the conversation's `context_start_id` moves forward to fit 70% of the
  budget, so the request prefix stays stable for later turns. Upstream
  context-length errors are stored as `context_length_exceeded` and shown as a hint.
- Finished replies set `last_reply_at`; a conversation is unread while that is
  newer than `read_at`. Opening it (or seeing the reply finish) marks it read; the
  list can mark it unread again. `GET /chat/unread` feeds the sidebar dot.
  Web search is stored per conversation and new conversations start with it on.
- Images and PDFs, and generated images, are stored in the async image object
  storage (`ImageStorageSettingService.JackChatStore`); text files are kept in the
  database. Without storage, only text attachments work. Files are served through
  `/api/v1/chat/attachments/:id/content` after an ownership check. Reused images are
  copied as rows and the object is deleted only when no row references it; unsent
  uploads are removed after a day.
- Replies run in the background (`jackchat.Runs`), not on the browser request:
  leaving the page, switching conversations or closing the tab only detaches.
  `GET /chat/conversations/:id/stream` reattaches with a snapshot plus live
  events, and `POST .../stop` cancels. Streaming text is saved every 1.5 s, so a
  reload (or another instance) still shows progress. Runs live in process memory,
  are limited to one per conversation and to 30 minutes; replies left
  `streaming` after that (for example across a restart) are marked interrupted.
- The `/api/v1/chat` routes skip the audit middleware because their bodies are
  conversation content. `/admin/chat-settings` stores the switch, an optional
  system prompt and the limits.

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
- `JackHomeView.vue` is the public `/home` page. Its copy lives in
  `src/jack/home/messages.ts` as component-local i18n messages (en, zh, zh-Hant
  written by hand), so the upstream locale files are untouched; shared keys such
  as `home.login` still come from the global messages. The page follows public
  settings: the model catalogue, hero diagram providers and snippet model names
  come from the Model Plaza only when the plaza flag is on and the visitor may
  see it (`model_plaza_require_auth`); a failed or empty plaza response leaves
  the catalogue out. Sign-up calls to action need `registration_enabled`, and
  snippets use `api_base_url` or the current origin. `src/jack/home/InkCanvas.vue`
  draws the 墨暈 ink effect; it pauses off screen and in hidden tabs, and paints
  a still image under `prefers-reduced-motion`.
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
The `v2-` runtime fingerprint covers the Docker runtime definition, entrypoints and
external runtime resources. The default model-pricing JSON is embedded in the Go
binary (`backend/resources/jack_embedded.go`), so price/catalog changes travel with
binary updates and rollbacks without changing this fingerprint. Its README and Go
source files are excluded too; any newly added external resource still changes it.
A changed image requirement continues to require `switch-image.sh`.

The shipped fallback path (`./resources/model-pricing/model_prices_and_context_window.json`,
or its absolute equivalent) uses the binary's embedded data when the image file is
missing or unmodified. Release packaging writes a sibling `.jack-sha256` digest to
identify the image's original data even after the binary advances. Edited or mounted
files that differ from that digest, and custom paths, keep their file-based behavior.
Remote catalog precedence, configured overrides and custom-file hot reload are kept.
Bundled fallback-only startup does not write a remote catalog cache: an old bundled
cache would otherwise outrank a later binary's data after update/rollback.

Migration from the legacy fingerprint (including v0.2.18-jack.1 and v0.2.19-jack.1)
requires one image switch to a release using the `v2-` scheme. Old updaters cannot
interpret the new compatibility boundary. Do not rewrite old manifests or force
matching IDs. After that switch, bundled price updates can use the panel. Rollback
to a legacy-fingerprint release still requires the corresponding image; compatible
panel rollback retains the existing upstream-base/schema-epoch restrictions.

The panel saves
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
