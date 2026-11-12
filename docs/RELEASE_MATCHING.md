# Release matching specification

Status: approved target design; the initial [read-only title preview](plans/MATCHING_PREVIEW.md) is implemented. Broader matching/eligibility work remains future scope. Event identity, component classification, quality assessment, and acquisition eligibility must remain independently testable.

## Pipeline

1. Preserve the original title/source metadata. Normalise separators, case, Unicode/accents, and common abbreviations without destroying number namespaces, rematch numbers, or evidence.
2. Extract structured observations: event labels/identifiers, date, headliners/aliases, component phrases, source/resolution/codec, size and publication information. Reuse `Release.ParseString` technical parsing where fixtures validate it.
3. Generate candidates from canonical identifiers, aliases, headliners and date context. Never equate bare `293`, `UFC 293`, and `Fight Night 293`.
4. Validate candidates against metadata and expose supporting/conflicting observations. Rescheduled dates/headliner changes retain historical aliases with provenance.
5. Classify a set of components independently. Resolve `Early Prelims` before the broader `Prelims`; explicit bundled coverage is a set, not duplicate event records. Under the user's [release-naming policy](plans/SEARCH_CRITERIA.md), infer Main Card only with confident event identity and no other component or excluded-programming label; show the inference explicitly.
6. Assess quality with provenance; filename resolution is an assertion, not verified container metadata. Unknown quality remains unknown.
7. Produce a versioned matching decision, then separately evaluate monitoring, wanted coverage, existing inventory/reservations, quality policy, and integration availability.

Use deterministic evidence rules initially rather than a fictitious probability. Automatic acceptance requires one validated event, no unresolved identity conflicts, known wanted coverage, eligible quality, and no existing satisfied/reserved coverage. Namespaced ID plus corroboration or an unambiguous canonical alias/date/headliner combination can qualify; ambiguous headliners/rematches or unfamiliar uploader numbering require review. Quality never repairs an ambiguous identity. Keep regex available as an advanced criterion, not a giant replacement for canonical matching.

Rank eligible candidates by configured quality preferences, complete useful coverage, then technical/source evidence; use stable tie-breaking and report the applied policy. No fallback, upgrade, or acquisition of ignored/historical components is enabled implicitly.

## Manual controls and diagnostics

The Matching Laboratory accepts a title and optional source/date context without searching, submitting, or writing media. Return original/normalised title, extracted observations, candidate events, component set, quality, positive/conflicting evidence, rule/matcher version, identity decision, and separate acquisition eligibility/reasons.

Manual search allows a user to assign event/components when matching fails, inspect size/source/age/quality, and explicitly submit. Store the original automatic decision and manual override; never mutate catalogue aliases automatically from a single override. Manual assignment bypasses matching uncertainty, not credential/path safety or replacement authorisation.

## Regression dataset

Use sanitised JSON fixtures with title, catalogue context, expected identity/components/quality, evidence expectations, and automatic/manual decision. Include punctuation/order variants such as `UFC.Fight.Night.293.Prelims.1080p`, `UFC_FN_293_Prelims_1080p`, a matching headliner variant, and headliner-only titles. These are naming examples, not proof that all numbering systems agree; tests must supply validated catalogue context.

Cover PPV/Fight Night collisions, ESPN/ABC/Freedom labels, aliases/accents, reversed fighters, rematches, missing dates, postponed/cancelled events, contradictory IDs, bundles, repeated extras, unknown components, and unknown quality. Every confirmed bug becomes a permanent regression. File identification reuses the same canonical engine, with filename/path context explicitly identified as evidence.
