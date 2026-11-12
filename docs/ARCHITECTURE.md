# Architecture

Status: approved target architecture. [Foundation](FOUNDATION_RESULTS.md) implements product identity, isolated settings/migrations and read-only status within the existing application. Remaining UFC workflows are future milestones; [audit](AUTOBRR_AUDIT.md) describes the original inspected revision.

## Shape and boundaries

[Manual acquisition](plans/MANUAL_DOWNLOAD.md) is a lifecycle-owned native Go service using the existing catalogue, feed and downloader services. UFC migration 5 reserves intent and projects safe shared history before remote submission. The durable journal, rather than the event bus, owns idempotency, remote IDs, uncertain outcomes and queue/history reconciliation. [ADR 0014](DECISIONS/0014-manual-acquisition-journal.md) records the boundaries; no legacy filter processing is invoked.

The initial [manual event search](plans/EVENT_SEARCH.md) has a UFC-local orchestration service consuming existing catalogue/feed services. The feed service exposes a bounded read-only query that reuses configured connections without entering feed refresh, release processing, cache writes or actions. Transient results are not durable acquisitions; no scheduler or schema is added.

The initial [matching preview](plans/MATCHING_PREVIEW.md) uses a pure `internal/ufc/matching` package. HTTP reads an effective catalogue snapshot through the existing service/repository and invokes the matcher; it adds no lifecycle worker, schema, external client or acquisition side effect.

One Go process, the existing React frontend, and the existing SQLite or PostgreSQL database. Add focused packages under `internal/ufc/` for catalogue, metadata providers, matching, profiles, search/acquisition, download reconciliation, library inventory, and native processing. Keep external HTTP clients separate from business logic. Consumer packages declare narrow unexported interfaces and constructors return concrete services, following upstream conventions. Do not create a generic plugin framework or another database.

`internal/application.App.setup` constructs repositories/clients/services. `components()` starts workers before HTTP serving and stops them before DB closure. Register cancellation-aware workers with bounded concurrency and graceful stop. UFC failure may degrade UFC functionality while preserving administrative diagnostics; never silently start acquisition without usable persistence. Reuse the scheduler for wake-ups. Persist jobs/reservations/retry times; event bus/SSE only announce changes.

## Data flow

Metadata provider -> canonical catalogue -> profile intent. Manual or scheduled Prowlarr search -> persisted candidates -> matching evidence -> explicit selection or eligible automatic decision -> transactional acquisition reservation -> SAB submission intent -> returned IDs/uncertain submission -> queue/history reconciliation -> completed-media processing job -> verified file operations -> library coverage -> activity/notifications.

Initial library adoption is a separate read-only flow into inventory/review. It never creates downloads or reorganises files. Passive upstream release events may enqueue immutable candidate snapshots; listeners return quickly. UFC paths do not execute legacy filter actions. Operators retain control of advanced autobrr actions to avoid configuring duplicate acquisition authorities.

## Persistence

Use the existing DB handle, transaction conventions, and migrator. Place UFC tables under a `ufc_` prefix and use `ufc_schema_migrations` with `WithTableName` and `WithLegacyConversions(false)`; independent migration IDs avoid collisions with upstream numbering. Run both migration streams sequentially before UFC repositories/workers start. Maintain SQLite/PostgreSQL parity, fresh-install schema and upgrade tests, backups, and restore-based rollback. No destructive down migrations are assumed.

Model canonical event/component identity, source provenance, settings revisions, matching decisions, searches, acquisitions, remote IDs, processing jobs/file-operation journals, library files/coverage, and activity. [DOMAIN_MODEL.md](DOMAIN_MODEL.md) owns relationships and state semantics. Schema details belong to the reviewed milestone introducing each feature.

## API and frontend

Mount authenticated `/api/ufc` handlers in the existing Chi server. Use existing session/browser protections and explicit API-key scopes. Preserve upstream API paths. Typed DTOs, bounded input, cancellation/timeouts, pagination, and validation errors follow existing conventions. Mutations return durable identifiers where work outlives requests. Reconnects/refetches recover from missed SSE events.

Add a UFC route group within the existing TanStack router/query/form stack. UFC dashboard and event pages become the primary experience as delivered; advanced autobrr pages remain accessible. Essential settings, errors, and manual controls ship alongside each workflow, not only at the UI-polish milestone.

## Integration contracts

Prowlarr client: connectivity, eligible Usenet indexer discovery, search, candidate metadata, protected NZB retrieval, rate-limit/timeout handling. Prefer native Prowlarr API for coordinated discovery/search; retain existing Newznab feed/search primitives. Validate exact installed-version contracts with mocks and a separately approved live smoke test.

Extend `pkg/sabnzbd` rather than inventing another client stack: typed submission validation/IDs, NZB upload, category validation, queue/history, retry/cancellation. Store submission intent before remote calls. On uncertain responses reconcile a stable acquisition correlation marker and remote state; unresolved cases require review, never blind replay. Candidate duplication across indexers is evidence to combine, not proof of identical content.

Native processing shares catalogue/matching/classification, owns file safety and persistent jobs, and reconciles inventory. It never uses Python. [PROCESSING.md](PROCESSING.md) defines preview, copy-only conflict policy, optional verified SAB download cleanup and failure recovery; configured library content is permanent.

## Configuration and security

Keep host/DB/auth bootstrap configuration in the existing file/environment layer. Store feature settings in the shared DB, with revisions and job snapshots. No hard-coded user paths or credentials. Processing settings refer to existing notification destinations; webhooks remain server-side and must be redacted in API/log/error/export paths. Existing storage is not assumed encrypted at rest; protect DB/config/backups with OS permissions and document that boundary.

Validate external metadata and filenames as untrusted data. Restrict file operations to configured roots, including resolved symlink/junction targets. Restrict NZB retrieval to configured integration references and validate redirects; never relay browser-supplied arbitrary URLs with stored credentials. Preserve authentication and state-change protections. Tests never contact production integrations or media roots.

## Deployment and delivery

Tray-first Windows deployment reuses the existing host/MSI. It runs while logged in and stops on logout; no mandatory service. Keep Docker. Fork-specific config/installer/registry/update identity prevents interference with autobrr. Disable upstream replacement until a fork update channel is explicitly configured. [WINDOWS_DEPLOYMENT.md](WINDOWS_DEPLOYMENT.md) owns packaging details.

The first useful workflow is manual event search -> diagnosed selection -> one SAB submission -> restart-safe tracking. Adopt the existing library read-only and deliver native processing before enabling automatic acquisition. [ROADMAP.md](ROADMAP.md) specifies acceptance gates.

## Implemented Foundation seam

`internal/product` selects legacy autobrr by default or `ufc-manager` through a linker flag. UFC uses its own configuration/environment/desktop registry/session cookie namespace and port 7475; updates are disabled at configuration, server checker, tray menu and executable replacement boundaries. Existing CLI and tray entrypoints share the same lifecycle.

Only UFC builds construct `internal/ufc/database` on the existing DB handle, then `internal/ufc/foundation`. Startup validates the disabled singleton before HTTP serving; migration/settings failure aborts startup and closes the DB. The Foundation itself adds no worker; the subsequent catalogue importer registers a manual-refresh lifecycle component. `/api/ufc/status` requires the current authenticated caller with `ufc:read` for keys. The primary UFC tab opens an authenticated workspace at `/ufc`, defaulting to `/ufc/events`; its sidebar's `/ufc/settings` view combines component defaults and the existing Foundation status, while `/ufc/manager` redirects. See [ADR 0010](DECISIONS/0010-foundation-isolation.md) and the [workspace refinement](plans/EVENTS_SCREEN.md#ufc-workspace-refinement).

## Revised Phase 2 boundary

The user reduced catalogue scope to acquisition facts: stable ID, numbered/Fight Night type, name/date/headliners, verified aliases/numbering, status, provenance and protected corrections. No rich fight, artwork, venue or biography metadata is required. A single catalogue service with a thin source reader replaces the earlier generic metadata subsystem proposal. The [revised plan](plans/EVENT_CATALOGUE.md) proposes four tables and one idempotent refresh worker; acquisition and processing durability remain later concerns.

Wanted components are generated locally from editable event-type templates and event overrides: numbered = Main Card/Prelims/Early Prelims; Fight Night = Main Card/Prelims. Optional extras remain configurable; provider data never defines components or clears overrides.

English Wikipedia is now selected under [ADR 0012](DECISIONS/0012-wikipedia-catalogue-source.md). Its pinned gate passes: [measured source agreement](research/WIKIPEDIA_COVERAGE.md). The native reader discovers both list tables and resolves dedicated page IDs, enriching explicit current aliases/headliners through bounded article reads. The four-table importer uses existing lifecycle/DB/auth routes; manual refresh coalesces and periodic sync stays disabled. No Wikidata fallback/provider framework. Source outages/omissions cannot delete events, clear facts or change protected fields. Wikipedia provenance/attribution remains distinct from CC0; TheSportsDB/TMDB remain excluded.

The later [read-only Events slice](plans/EVENTS_SCREEN.md) narrows routine source resolution/enrichment to upcoming and ten recent past candidates, preserving older stored identities. The authenticated Events screen displays effective upcoming events and at most ten explicitly completed events, with aliases/review state and manual refresh. It introduces no schema, editing or acquisition workflow.

The subsequent [editing slice](plans/EVENT_EDITING.md) adds local event details and revision-checked field patches. `catalogue.ApplyEdit` distinguishes protected sets/unknowns from source resets; the repository atomically stores the effective fields and existing correction JSON through `saveEvent`. Omitted fields, aliases, component preferences and source identity remain unchanged. HTTP derives correction actor from the existing caller; keys need `ufc:write` to mutate. The editor retains drafts through refresh and 409 responses and requires explicit latest-revision review before retry. No schema, source retrieval or lifecycle change is needed.

The [component preferences slice](plans/COMPONENT_PREFERENCES.md) exposes the existing templates JSON and override table. UFC → Settings (`/ufc/settings`, with the old Manager URL redirecting) contains default wanted components and the reused Foundation status. User-only catalogue settings GET/PUT accepts templates and an expected singleton revision; event PATCH retains `ufc:write` and partial Inherit/On/Off changes under the event revision. Event detail reads resolve defaults/overrides/wanted values in the existing snapshot transaction. Default changes intentionally affect inherited choices without rewriting event revisions; explicit choices remain independent. Source refresh never writes templates or overrides. No migration, new dependency or worker is added.

[ADR 0013](DECISIONS/0013-manual-catalogue-and-refresh.md) extends this with protected manual records/type/alias edits and opt-in metadata scheduling. Migration 3 permits null source references and persists next due. Event POST requires `ufc:write`; source IDs remain immutable and manual records are never auto-merged. The existing catalogue lifecycle owns a lightweight timer, an atomic due claim and the shared refresh coalescing lock. Settings saves wake the timer; failures/restart retain the next interval, with no catch-up burst. Acquisition and processing remain disabled. Phase 2 milestone acceptance is deferred by user instruction.

## Read-only library adoption, 10 November 2026

See [the focused plan/results](plans/LIBRARY.md) and [ADR 0015](DECISIONS/0015-read-only-library.md). Native library service/repository and paired UFC migration 6 persist roots, durable scan state and file observations. Existing lifecycle, scoped HTTP, frontend routing/API and manual acquisition reservation seams are extended. Shared upstream migrations, filters/actions, download connections, media files, updater, installer and dependencies are unchanged. Reference Python naming was read only. No runtime Python dependency.

## Permanent historical catalogue, 10 November 2026

[ADR 0016](DECISIONS/0016-permanent-history.md) and [focused results](plans/HISTORICAL_CATALOGUE.md) supersede observed-only historical adoption. Migration 7 stores the complete list snapshot and separate Events membership; bulk Clear cannot delete canonical records, corrections or library links. Native source refresh imports historical list facts in batches while keeping article enrichment bounded. Library scanning no longer contacts Wikipedia. Existing application lifecycle/API/frontend seams are reused; no upstream migrations, dependencies, real media operations or automatic acquisition changes.

## Phase 7 feed-driven acquisition, 11 November 2026

The existing feed scheduler invokes an optional pre-cache durable observer. UFC candidates are journalled before a page is acknowledged and consumed by a lifecycle worker through existing server-owned selection validation and `acquisition.Service.Submit`. Reservation transactions additionally guard the default-off application switch, admitted membership, current wanted intent, revisions and saved SAB target. Manual and automatic jobs share download history, duplicate protection, reconciliation and native processing handoff. Independent notification outbox delivery uses existing notification destinations. Migration 17 is paired for SQLite/PostgreSQL. Event admission/archive state is distinct from source status; see ADR 0019 and the Phase 7 plan.

## Launcher updates, 12 November 2026

[ADR 0021](DECISIONS/0021-signed-launcher-updates.md) separates the installer-owned launcher/seed from signed runtime versions. `internal/ufc/updater` owns release verification/staging, the durable coordinator and bootstrap recovery; paired migration 18 stores update state without replacing acquisition/processing journals. A shared activity gate covers API changes and scheduled/asynchronous work. The desktop supplies orderly shutdown; application setup binds and validates the trial before worker activation. A child-held profile lock prevents concurrent writers after launcher loss. The launcher backs up SQLite/configuration and commits selection through nonce/PID/version/schema-verified local IPC; it never rewinds a database automatically. PostgreSQL never enters in-app installation. Existing notification outbox/destinations deliver update availability and attention messages. Production trust remains disabled until maintainer provisioning.
