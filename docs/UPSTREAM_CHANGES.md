# Upstream change policy

[Manual workflow completion](plans/MANUAL_WORKFLOW_COMPLETION.md) forwards publication timestamps through the existing feed-search DTO and exposes a read-only title in the shared SAB tracking result for verified binding. Existing feed/action pipelines and credentials are reused. Migration 10 affects only UFC tables. UI/API, plain-term matching policy, decision evidence and recovery audit remain in the UFC modules. No new dependency, media operation or automatic acquisition.

[Manual downloads](plans/MANUAL_DOWNLOAD.md) extend the shared SAB client with optional job names, explicit HTTP refusal handling, bounded clones and queue/history lookup. Existing caller settings remain intact. Feed search retains private NZB links/GUIDs only server-side. UFC migration 5 and its repository project safe manual history into existing Releases/action-status tables atomically, with no upstream schema/filter/action changes. Application/HTTP wiring and UFC Downloads UI expose the lifecycle. See [ADR 0014](DECISIONS/0014-manual-acquisition-journal.md).

[Search criteria/quality preferences](plans/SEARCH_CRITERIA.md) add only UFC-local migration 4, catalogue preferences, matching/search checks and UI. The user's updated naming convention permits explicit Main Card inference, independently of accepted resolution. No shared upstream schema, connection settings, filter actions or downloader behaviour change.

[Manual event search](plans/EVENT_SEARCH.md) adds a read-only method to the existing feed service using its saved configuration, transport helpers and protocol clients. It bypasses scheduled refresh/cache/filter/action processing. UFC-local orchestration, API and Events UI are wired through application/HTTP dependencies. No settings duplication, dependency/schema change or SAB submission.

The [read-only matching preview](plans/MATCHING_PREVIEW.md) adds a UFC-local matcher/snapshot and scoped endpoint, sidebar route and screen. Status copy acknowledges the preview. No upstream release parsing/filter/acquisition behaviour, dependencies or migrations change.

Phase 0 changed documentation/environment only. Foundation runtime changes below were subsequently authorised; dependency locks and the external processor remain unchanged.

| Classification | Subsystems | Policy |
|---|---|---|
| Retain | Authentication/API infrastructure, SQL repositories/migrator, lifecycle, logging/SSE, notifications, scheduler, integrations, legacy filters/actions, Docker | Reuse tested behaviour and preserve public paths |
| Adapt | Application/HTTP wiring, product/config/installer/update identity, frontend navigation, technical parsing use, SAB capabilities, credential redaction | Small focused changes with evidence and regression checks |
| Hide from primary UFC navigation | Generic filters, IRC/Matrix, unrelated clients/lists | Keep routes, APIs, configuration and implementation available in advanced navigation |
| Remove | None initially | Require separate justification and review |

For invasive changes/removals record necessity, consumers/dependencies, affected tests, licence impact, upstream merge cost, and why hiding or extension is insufficient. Preserve indexer tombstones where applicable. Do not bulk-rewrite headers, imports, or the Go module path to rebrand the product.

Expected integration hotspots: `internal/application`, `internal/http`, `internal/events`, `internal/notification`, `internal/update`, client additions in `pkg/sabnzbd`, frontend routes/navigation, packaging/build metadata. UFC business logic and persistence remain independently named packages/tables. Keep a per-milestone entry here identifying actual upstream files changed and tests run.

Review official upstream monthly and promptly for security fixes. Fetch and inspect changes on a dedicated integration branch; check overlapping hotspots and migration names; merge without rewriting shared history; run mandatory checks and affected tagged suites. Record the imported revision and unresolved conflicts. Do not merge unreviewed upstream updates into the baseline while measuring it.

Fork PRs target `develop`, use the existing template and Conventional Commit titles, and disclose AI usage truthfully in the designated section. No AI attribution trailers. No remote pushes or PR creation are authorised by this stage.

## Foundation changes

| Review unit | Existing upstream seams changed | Verification |
|---|---|---|
| Migrator prerequisite | `pkg/migrator/migrator.go`: opt out of historical autobrr conversion for independent streams; propagate initial/incremental commit failures | Real deferred-constraint tests; repeated isolated histories; both-engine UFC integration |
| Product and UFC Foundation | CLI/tray entrypoints, config/meta/update, application/HTTP/config/scopes/web route, React API/routes/settings; `.gitignore` root config scope and paired UFC SQL exceptions | Mandatory checks, both-engine preservation/failure tests, API/browser scopes, registry isolation, eight CLI/tray launches |
| Webhook credentials | Domain JSON, notification update/test/factory, Discord/webhook diagnostics, notification form | Retain/replace/explicit clear, API/log/error redaction, local mock delivery, upstream simulation |

New business packages are limited to `internal/product`, `internal/ufc/database` and `internal/ufc/foundation`; the credential diagnostic helper stays in notification services. No upstream repositories, old migrations, OIDC eviction code, MSI definitions, release workflows or dependency locks were modified. Review [units](changes/README.md) and [results](FOUNDATION_RESULTS.md) independently. Existing source notices and module paths are retained.

## Finalisation review, 10 November 2026

A separate shared OIDC fix orders pending-login eviction by insertion sequence under the existing mutex, keeping the original regression assertions. Product review preserves legacy integer parsing and container behaviour while UFC stays loopback by default; generated settings use the correct namespace. UFC alone validates exact migration history before migration/status. Webhook diagnostics additionally cover JSON-encoded/overlapping credentials, and the native acceptance harness strips product configuration from the inherited environment. See [review](FOUNDATION_REVIEW.md) and [tracked items](FOLLOW_UPS.md). No old SQL repository/migration test was weakened.

## Source evaluation / Phase 2 simplification

The Wikidata spike and revised four-table plan are documentation/research only. No runtime source, frontend, schema, dependency or provider integration changed. The earlier TheSportsDB-first proposal is superseded by the user-directed source gate and acquisition-only scope. See [ADR 0011](DECISIONS/0011-minimal-acquisition-catalogue.md).

## Narrow Wikipedia importer slice

Focused additions under `internal/ufc/catalogue` and `internal/ufc/database` provide native source parsing, local facts/protection/components and transactional persistence. Four new `ufc_` tables use the isolated history (new ID 2). Existing application/HTTP wiring mounts only UFC status/manual refresh with read/invoke scopes; the API-key form exposes the existing invoke access level. Native launch tests expect the four additional tables in UFC builds and still require zero in legacy builds. Existing upstream SQL repositories/migrations, download clients, updater, MSI and dependency locks are unchanged. No Python runtime, fallback provider or extra process is introduced.

## Protected event editing

The existing UFC HTTP handler adds scoped detail GET and field PATCH. `internal/domain/api.go` now allows `ufc:write`, with existing write-implies-read/invoke semantics; unrelated key scopes remain denied. The shared frontend HTTP client's existing error gains a numeric `status` property so the editor can handle 409 without parsing human-readable text. UFC types/API/queries/Events UI add the inline editor. The native Foundation browser assertion now follows the previously delivered UFC top tab instead of its removed Application settings link. No upstream SQL/migration, application lifecycle, updater, dependency or packaging change. See [focused results](plans/EVENT_EDITING.md).

Subsequent Events presentation refinements reuse the existing Settings `Section`. `settings/Application.tsx` gains a UFC-only collapsed catalogue-attribution panel, relocated from the Events footer; legacy application controls are unchanged. Upcoming-only review summaries and timestamp placement use existing local list data. Focused UI/browser tests, lint and builds cover these adjustments; see [Events refinement results](plans/EVENTS_SCREEN.md).

## Component preferences

Native additions under `internal/ufc` expose existing template/override storage and resolver through user-only settings routes and the scoped event editor. Existing frontend API/query/router seams add `/ufc/settings` and retain `/ufc/manager` as a redirect. The existing status panel is reused within Settings. Focused HTTP, both-engine and UI/browser tests cover preferences, preservation, invalid/stale writes and rollback. No shared upstream SQL/migration, worker, source reader, updater, installer or dependency changes. See [the component preference plan/results](plans/COMPONENT_PREFERENCES.md).

## Manual catalogue and metadata schedule

The [completion slice](plans/CATALOGUE_COMPLETION.md) extends only UFC model/repository/HTTP/UI seams, adding paired UFC migration 3, nullable provider references, protected alias snapshots and a lifecycle-owned metadata timer. Frontend API/types, Events/New event/editor and UFC Settings expose these controls. Existing test expectations are updated for supported type edits, the appended migration and the Save settings label. Shared upstream migrations, scheduler, application wiring, updater, installer, dependencies and source-reader discovery rules are unchanged. Milestone testing is explicitly deferred; only compile/lint and narrow new data/auth/scheduling checks run now.

## Read-only library adoption, 10 November 2026

See [the focused plan/results](plans/LIBRARY.md) and [ADR 0015](DECISIONS/0015-read-only-library.md). Native library service/repository and paired UFC migration 6 persist roots, durable scan state and file observations. Existing lifecycle, scoped HTTP, frontend routing/API and manual acquisition reservation seams are extended. Shared upstream migrations, filters/actions, download connections, media files, updater, installer and dependencies are unchanged. Reference Python naming was read only. No runtime Python dependency.

## Permanent historical catalogue, 10 November 2026

[ADR 0016](DECISIONS/0016-permanent-history.md) and [focused results](plans/HISTORICAL_CATALOGUE.md) supersede observed-only historical adoption. Migration 7 stores the complete list snapshot and separate Events membership; bulk Clear cannot delete canonical records, corrections or library links. Native source refresh imports historical list facts in batches while keeping article enrichment bounded. Library scanning no longer contacts Wikipedia. Existing application lifecycle/API/frontend seams are reused; no upstream migrations, dependencies, real media operations or automatic acquisition changes.

Event browsing/removal refinement: paired UFC migration 8 adds only a download visibility flag. The existing download service/HTTP/API/UI seams support scoped card dismissal; the retained journal still reconciles and prevents duplicates. Catalogue browsing reuses the local reader and existing compact event cells in separate Upcoming/Past routes. No upstream migration, dependency, SAB cancellation or media operation. See [focused results](plans/EVENT_VIEWS_AND_DOWNLOAD_REMOVAL.md).

## Signed launcher updates, 12 November 2026

UFC desktop builds use a separate updater/launcher with pinned signed metadata from `VulcanGH/UFCM`; the upstream self-replacement updater remains disabled for UFC and available to legacy builds. New Windows packages retain the UFC UpgradeCode while replacing direct-runtime MSI components with a launcher and seed components. Existing profile/configuration and user startup preferences are retained. Shared application, API, scheduler/feed/release/action/list/notification entry points acquire a maintenance lease; asynchronous handoffs retain the lease until their work finishes. The new application activation gate precedes component startup. SQLite backup implementation is shared with the existing CLI without changing its command contract.

The inherited release publication job is disabled only for `VulcanGH/UFCM` to prevent collision with the explicit UFC draft workflow; other repositories retain the inherited condition. Existing build/test jobs and legacy application functionality remain. No dependency manifest or upstream database migration changes. See ADR 0021 and the focused updater verification record.

## UFC header navigation, 11 November 2026

Hide the Filters menu link in UFC builds in the shared desktop/mobile navigation list. The route, page and legacy autobrr navigation remain available. No upstream functionality was removed.

## Phase 7 shared seams, 11 November 2026

Feed refresh has an optional page observer before its existing cache acknowledgement; enabled Newznab UFC discovery journals candidates there. Original feed schedules, cache semantics and legacy filter/action processing remain. Application lifecycle wires the observer and durable UFC worker only in UFC builds. Existing notifications gain three selectable UFC events, Discord rendering, safe structured webhook data, inbox labels/filtering and synchronous error reporting for durable callers. Generic notification routing and existing asynchronous Send callers are unchanged. No new dependency or upstream migration.

## Phase 9 Windows packaging seams

Upstream release/MSI sources remain intact. A separate UFC WiX source and PowerShell build script produce private per-user MSI/ZIP and matching source/hash artifacts. Existing tray login registration initialises once only for the installed executable; an installer-only maintenance command removes UFC startup registration without constructing the app. The tray now fails closed if its instance lock cannot be established. Existing database maintenance gains a read-only-source SQLite snapshot command that does not migrate or replace files. No new runtime dependency or database migration. Installer/session acceptance remains deferred; see ADR 0020 and the Windows packaging plan.

Fork branch convention: UFC Manager uses main for development and PRs. Verification workflows include main; inherited Docker jobs are disabled for VulcanGH/UFCM so branch pushes do not publish upstream-style images. Upstream branch references remain supported.

Windows-only release scope: the inherited GoReleaser distribution matrix is skipped for VulcanGH/UFCM, alongside inherited Docker and upstream release publication. The UFC draft workflow owns Windows amd64 launcher/MSI/runtime packaging. Linux-hosted static/unit checks do not produce supported Linux application packages.

Repository landing page: README.md now describes the implemented Windows UFC Manager workflows and current preview/release limitations. The original upstream README is preserved in README.autobrr.md with its original relative-link base; upstream copyright/licence notices remain unchanged.

CI regression repair: UFC UI fixtures follow current labels/switches/navigation; historical database upgrade fixtures seed their historical schema directly. Processing resolves completed SAB storage and source paths consistently before containment checks so Windows short aliases cannot cause false escape detection. Existing source/destination identity, conflict and copy-only checks remain.

Large-library correction: local library matching can use a consistent explicit folder year with the filename's headliner evidence when article enrichment alone is pending/unavailable. Substantive catalogue conflicts and ambiguous identities remain review-only; acquisition matching and stored alias verification are unchanged. Added 601-folder/database-group and 13-page regressions, included in the Windows draft checks.

CI job labels now say Build App Interface and Test App Interface. These build/test the React interface embedded in the Windows app; they are not a separate platform release. The earlier pagination test TypeScript error is fixed and current frontend build/tests pass.

Historical library matching now combines current saved billing/number aliases with explicit year evidence for unenriched history, without promoting stored aliases or changing acquisition matching. Unknown file identity retains independent explicit component/resolution observations and clearer review reasons. Library walking continues after individual directory/file metadata errors, marks affected roots incomplete and never infers missing files from a partial walk. Saved group sorting and the Library explanation make clear that unidentified folders follow identified events.

Canonical namespace correction: numbering is parsed from the start of source billing names; Noche's embedded UFC number is not an ordinary numbered-event identifier. Library matching uses an isolated copy of saved aliases with canonical provider-title/type checks to quarantine stale non-manual cross-family claims and prioritise a unique plain-numbered identity. Persisted aliases/manual overrides and acquisition verification remain intact; true ownership/identity conflicts remain reviewable. Library resolution confirmation selector removed; missing resolution stays optional metadata. Generated 400-number and cross-family/provenance regressions supplement realistic persistence and filename cases.

UFCM release polish: supplied logo/browser/executable/tray artwork; product-specific tray icons preserve upstream tray icons. Explicit update-check progress, signing-configuration failures and no-public-release feedback retain signed-package verification. Compact metadata scheduling toggle and optional resolution display preserve stored settings. UFC MSI adds destination selection and elevated per-user Program Files default; existing install paths and per-user runtime/profile storage remain intact. See `plans/UI_RELEASE_POLISH.md` for focused verification and deferred installer acceptance. Discord is unchanged.

Header SVG viewBox restored for responsive rendering. Installable Windows packages now reject a missing public verification key. Local MIT systray v0.3.0 replacement adds NIF_SHOWTIP on Windows Show/modifyIcon so configured hover text survives taskbar restoration/theme changes; original notices and patch rationale retained.
