# Native Go processing specification

User safety directive, clarified 11 November 2026: processing is copy-only into configured library roots. Never delete, move, rename, overwrite or replace existing library content. Optional cleanup may remove only a specific UFC-owned SABnzbd completed download outside all library roots after every intended copy is verified and durably recorded. Cleanup is off by default; partial or uncertain work retains its source. Database-record resets do not authorise filesystem operations. See the [ordered Phase 6 plan](plans/NATIVE_PROCESSING.md).

Status: explicit copy jobs, review choices, retry, cancellation and restart reconciliation are implemented. Basic multi-file bundles use the existing matcher and distinct part names; ambiguous bundles pause. Opt-in confirmed-SAB handoff and narrowly guarded source cleanup are implemented; focused milestone sign-off remains pending. See [domain model](DOMAIN_MODEL.md), [ADR 0004](DECISIONS/0004-native-processing.md), and [ADR 0009](DECISIONS/0009-conflicts-and-replacement.md).

SAB reports the exact completed storage path in tracked history. The app removes that path natively after verification; SAB history deletion does not remove completed payload files. Cleanup requires every intended media file to have a durably verified Library copy, then checks the exact job marker, root boundaries and regular-file size/presence before removing all regular files and directories inside that completed folder. Full content hashes are computed during copying, not again during cleanup. A link, special entry, missing or size-changed verified file, or uncertain path leaves the source in place. A reviewed cleanup can be retried by the user after the guards run again; it never reruns the copy.

## Responsibilities and inputs

Implement a focused `internal/ufc/processing` subsystem in the shared Go process. Use lifecycle-managed workers, persistent jobs/file-operation journals, the shared catalogue/matcher for identity/components, and library repositories for reconciliation. External Python is a behavioural reference only: no subprocess, interpreter, copied monitor loop, or Python test dependency in the finished application.

Accept SAB completion-confirmed acquisitions and explicit manual requests. Unmanaged directory discovery is opt-in, ignores temporary/underscore paths by default, requires configured settling checks, and still verifies stable files before publication. Downloads being queued, disappearing from the queue, or containing a video extension are not completion evidence. Manual requests must resolve to a configured source root and canonical event/component assignment or enter review.

## Persistent settings and interfaces

Use typed versioned `ProcessingSettings` containing source roots, staging policy, destination/library roots, folder/file templates, component folder rules, copy-only/conflict policies, optional SAB cleanup, extensions, concurrency, retries and settling preferences. Host/auth/DB configuration stays upstream; feature settings live in the existing DB with paired engine migrations. Snapshots contain no credentials. Reuse the existing SAB connection, retrieve its categories for selection, and persist the selected UFC category.

Initial defaults: disabled until configured; no root paths; MKV/MP4/AVI; copy-only; SAB cleanup off; one worker; three total attempts for transient errors with bounded backoff; different-content conflicts `review`. Confirmed SAB completion does not need an arbitrary polling delay. Templates and component folder rules are configurable. The script's paths and ten-second scan loop are not hard-coded application defaults.

API surface under existing authenticated `/api/ufc` routes:

| Operation | Contract |
|---|---|
| `GET/PUT /processing/settings` | Read/validate/save settings, with revision checks and field errors |
| `GET /processing/categories/{clientID}` | List categories through an existing enabled SAB client; no credentials returned |
| `POST /processing/preview` | Identify input and return proposed paths, coverage, conflicts and operation manifest; no media/DB-job side effects |
| `POST /processing/jobs` | Persist an execution request and settings snapshot; return job ID |
| `GET /processing/jobs`, `GET /processing/jobs/{id}` | Paginated status, per-file progress, errors and recovery actions |
| `POST /processing/jobs/{id}/retry`, `/cancel`, `/replan` | Explicit safe job control; replan invalidates old operation authorisations |
| Processing cleanup | Opt-in setting, snapshotted per queued copy; completed verified jobs are claimed by the worker, with no arbitrary path API |

Use existing auth/scopes/encoders and house frontend forms. Saving settings creates no media directories. Validate templates/root containment/overlap, recognised fields, portable filename sanitisation, invalid Windows names and path limits. Stored unavailable roots block execution with diagnostics. Never allow archive/source containment loops or untrusted junction escapes. New requests revalidate even after preview because filesystem state can change.

## Naming compatibility

Default event display title is the validated event label/number plus known headliner, followed by canonical event year. Main-card media lives directly in its event folder; supplementary content defaults to `Featurettes` and appends ` - <Component>` to the filename. Preserve the original media extension.

```text
<Event title> (<year>)/
  <Event title> (<year>).mkv
  Featurettes/
    <Event title> (<year>) - Prelims.mkv
    <Event title> (<year>) - Early Prelims.mkv
    <Event title> (<year>) - Countdown.mkv
    <Event title> (<year>) - Embedded.mkv
```

Use restricted Go templates with allowlisted data/functions, not arbitrary code. Missing required year/identity enters review; do not use current year or a shared headliner-only key. Multiple files and numbered extras need explicit part/episode fields in their plan, not blanket processed flags. Existing library folders are never renamed when metadata improves.

The [synthetic fixture set](fixtures/processing_naming.json) records numbered, Fight Night, ESPN, ABC, Freedom, rematch, extension-case and main/Featurettes layouts. It also identifies missing-year, ambiguous, multi-file, conflict and partial-copy corrections. The reference script's parser, state JSON, automatic suffixes, folder rename and copy retry loop are not compatibility requirements. Keep its successful folder layout as the default while using canonical event identity and verified per-file evidence. Never copy the original script, local paths, state data or webhook into fixtures.

## Conflict policies

Identical content is established by verification, not same filename or size. It can be reconciled as existing coverage without copying. Different content follows the selected policy:

| Policy | Result |
|---|---|
| `review` (default) | Pause the operation, preserve both existing/source files, present evidence and choices |
| `skip` | Record explicit skipped outcome; do not claim the incoming content was imported or assume the existing file matches |
| `keep_both` | Choose and reserve an available ` (n)` suffix, publish without replacement, keep inventory for both files |

No policy can overwrite or replace a library file. For keep both, reserve a new filename and recheck it without a replacement race. Identical verified content may be reconciled as existing coverage, but it does not by itself justify deleting a SAB download when other files in the job remain unprocessed.

## Safe operation protocol

1. Resolve allowed roots and validate identity, component coverage, source stability and destination policy. Persist the manifest and acquire a bounded job lease/reservation.
2. Copy to a job-owned staging path on the destination filesystem. Check cancellation, source identity and size changes. Never expose a partial final filename.
3. Close/flush writes and verify source/destination byte counts and SHA-256. Record evidence; unstable or mismatched content is not available.
4. Recheck destination and use publication semantics that cannot replace an unreviewed file. Where the filesystem cannot provide the required no-replace operation, fail safely into review rather than approximating with an unsafe check-then-overwrite.
5. Commit file evidence and coverage in the DB. Multi-file jobs expose partial outcomes; a bundle is complete only for verified coverage.
6. For an opted-in UFC-owned SAB job only, evaluate cleanup after all intended files have verified copies and durable Library evidence. Bind the action to the exact SAB job and confirm its completed storage is outside library roots. A cleanup failure never triggers another copy.
7. Emit correlated activity and independently retryable notifications. Notification failure does not undo publication or re-run processing.

Persist per-file transitions around filesystem actions. On restart reconcile recorded fingerprints against source/staging/final locations. Reuse verified published files, restart owned incomplete copies, and send uncertainty to review. Discard only recognised application-owned temporary staging content; do not prune arbitrary files. An offline root pauses reconciliation and does not mark the collection missing.

Retries apply to classified transient I/O/integration errors. Unknown identity, invalid configuration, changed fingerprints, and unresolved conflicts await user correction. Cancellation stops at safe checkpoints and preserves the SAB source and completed library files.

## Verification and acceptance

Unit tests exercise parsing/planning/templates/policy without filesystem mutation. Filesystem integration tests use temporary fixture trees only and inject failures at copy, flush, hash, publish, DB-record and SAB-cleanup boundaries. Cover permissions, disk-full, locked files, source mutation, zero-byte media, invalid names, traversal/junction escapes, unavailable roots, repeated jobs, cancellation and crash recovery. Tests must prove no library replacement and no cleanup after partial or uncertain import.

API/frontend tests cover validation/revisions, side-effect-free preview, all permitted conflict choices and opt-in cleanup gating. End-to-end mock acquisition -> processing -> inventory must pass without Python or real media. Compatibility successes and deliberate bug corrections must be distinguishable in the fixture dataset. SAB's completed-history delete does not remove completed payload files; native cleanup uses exact storage and verified per-file evidence. SAB `+Delete` is unrelated unpack cleanup.
