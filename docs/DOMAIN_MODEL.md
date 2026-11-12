# Domain model

This is the approved logical model; the Foundation and narrow four-table catalogue importer now have applied schemas. Later acquisition/processing entities remain specifications. Use the existing database and paired engine migrations as described in [architecture](ARCHITECTURE.md).

| Entity | Identity and relationships |
|---|---|
| Event | Stable internal ID; official/display names, event-local date, numbered/Fight Night type, status and headliners. IDs survive renaming, rescheduling, and headliner changes |
| EventIdentifier / Alias | Namespace plus value, provenance and confidence; numbered PPV, Fight Night, ESPN, ABC, provider IDs, and uploader numbering remain distinct |
| Source provenance / Manual correction | Source key/revision/retrieval time and latest normalised core facts; typed protected manual values win. Phase 2 needs no generic observation archive or conflict-job entities |
| Local component template / Event override | Editable defaults by type: numbered Main Card/Prelims/Early Prelims, Fight Night Main Card/Prelims; configurable extras and event inherit/on/off. Resolve locally, never from provider data; detailed file/instance inventory remains later |
| AcquisitionProfile / EventPreference | Wanted components, quality policy and monitoring, defaults plus explicit event overrides; historical events remain unmonitored initially |
| SearchAttempt / CandidateOccurrence | Query/indexer/outcome/time, original title, source GUID/reference, size/date and technical metadata; preserve per-source occurrences |
| Candidate / MatchingDecision | Correlated occurrences with versioned normalisation, identity/component/quality evidence, conflicts, candidates, and separate acquisition decision. A cross-indexer title match is not a content hash |
| Acquisition / Reservation | Selected event/component coverage, intent/idempotency key, chosen occurrence, policy revision, manual override, submission state, remote client IDs, retries, reconciliation history |
| ProcessingSettingsRevision | Validated roots/templates/policies/preferences; immutable revision referenced by job snapshots; no credentials |
| ProcessingJob / FileOperation | Acquisition/manual origin, manifest, source fingerprints, destination/coverage, operation state/verification/retries/errors and separate optional SAB cleanup evidence |
| LibraryFile / FileCoverage | Physical path/root/fingerprint/quality/verification evidence; many-to-many event-component coverage and optional part/episode position |
| Activity / NotificationDelivery | Correlated decisions/transitions/errors and independent delivery state, referencing server-side notification configuration |

## Invariants

- Event identity, component classification, quality, acquisition intent, transfer state, and library availability are separate concepts.
- A bundled candidate or file may cover several components; a component may have several files/qualities. Partial processing cannot mark all bundle coverage available.
- Ignore/monitor preferences persist independently of failed or completed attempts. UI statuses are projections, not one overloaded irreversible enum.
- Reserve needed coverage transactionally before acquisition. Persist submission intent before network I/O and reconcile unknown outcomes after restart.
- Download IDs are opaque strings. Queue disappearance is not completion; consult history and retain an unknown/review state when evidence is insufficient.
- Availability requires verified library evidence. Legacy script flags do not establish it. Offline/inaccessible roots are unknown/offline, not proof that every file is missing.
- Processing jobs snapshot settings. Editing a template never silently replans an in-flight job or renames an existing archive folder.
- Identical content can satisfy coverage without copying again. Different-content conflict handling defaults to review; skip never claims new content was imported.
- Existing library files are never deleted, moved, renamed, overwritten or replaced. Optional SAB cleanup is job-specific, off by default, outside library roots and requires verified full import.
- Notification delivery cannot change a completed file operation back into a copy job.

## State and recovery

Display wanted/searching/found/queued/downloading/downloaded/processing/available/failed/missing/ignored from monitoring, latest attempts, active jobs, and verified coverage. Preserve unknown/review/offline/cancelled distinctions instead of misreporting success or absence.

Processing stages include identification/review, planned, copying, verifying, publishing, inventory reconciliation, optional SAB download cleanup, completed, retry-wait, failed, and cancelled. Journal per-file progress so a multi-file job can report partial success. A cleanup failure after verified full import is recorded independently; it never repeats publication.

Cancellation stops future side effects at safe checkpoints. On restart recheck staged/final/source evidence, reuse verified final content, restart owned partial copies, and require review for uncertainty. [PROCESSING.md](PROCESSING.md) owns detailed operation semantics.

Phase 2 is now the [minimal acquisition catalogue](plans/EVENT_CATALOGUE.md), not a rich metadata service. Stable local IDs and separate numbering namespaces remain mandatory. Overall event sequence, numbered UFC, Fight Night, Vegas, ESPN and ESPN+ numbers must not be conflated.

The importer stores confirmed `wikipedia.en.page_id` references and list/article revisions separately from random opaque local IDs. A source redirect changing a known identity requires review. Calendar dates remain nullable strings, overall sequence stays provenance, and fractional numbered UFC `37.5` is a string. Source/latest suggestions and protected effective values coexist; the editor/API for manual changes remains a later slice.

## Permanent historical catalogue, 10 November 2026

[ADR 0016](DECISIONS/0016-permanent-history.md) and [focused results](plans/HISTORICAL_CATALOGUE.md) supersede observed-only historical adoption. Migration 7 stores the complete list snapshot and separate Events membership; bulk Clear cannot delete canonical records, corrections or library links. Native source refresh imports historical list facts in batches while keeping article enrichment bounded. Library scanning no longer contacts Wikipedia. Existing application lifecycle/API/frontend seams are reused; no upstream migrations, dependencies, real media operations or automatic acquisition changes.
