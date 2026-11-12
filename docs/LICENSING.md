# Licensing and redistribution

Status: source-level findings, not release clearance or a legal opinion. Preserve the existing root `LICENSE` and all upstream notices. Open questions below block public distribution, not private documentation work.

## Verified source and practical consequences

The audited root `LICENSE` contains GNU GPL version 2. Source SPDX headers and README grant `GPL-2.0-or-later`; do not describe the project as GPLv2-only. Original notices name Ludvig Lundgren and autobrr contributors. Modifications require truthful notices, relevant dates and retained upstream copyright; record actual contributors/rights holders rather than attributing authorship to an AI tool.

Existing `github.com/coreos/go-oidc/v3 v3.21.0` is Apache-2.0. Apache states compatibility with GPLv3 and incompatibility with GPLv2. The upstream 'or later' permission is therefore material: plan combined distribution under GPLv3-compatible terms after the complete dependency check, without falsely changing original file grants. See [Apache compatibility guidance](https://apache.org/licenses/GPL-compatibility.html) and [dependency licence](https://raw.githubusercontent.com/coreos/go-oidc/v3.21.0/LICENSE).

New project-owned code should retain the repository's `GPL-2.0-or-later` convention with truthful contributor notices. Before distribution, include the GPL version text(s) applicable to the combined work, retain upstream GPLv2 text, and state the chosen distribution terms clearly. Do not assume a licence change to root `LICENSE` alone resolves dependency compatibility.

Provide corresponding source for the actual binaries, including modifications, interface/build files and required scripts. Publish a matching source archive and build instructions beside each executable, installer, and container release rather than relying on an unrelated current branch. Preserve notices and warranty disclaimers; do not add contradictory installer terms. A private build is not the same as distributing binaries. Confirm the precise source-conveyance mechanism for each release with the applicable GPL text.

## Dependency and asset evidence

| Item | Evidence | Status |
|---|---|---|
| React 19.3.1 | [MIT licence](https://raw.githubusercontent.com/facebook/react/v19.3.1/LICENSE) | Sample verified; include notice |
| modernc SQLite 1.61.1 | [BSD-style licence](https://gitlab.com/cznic/sqlite/-/raw/v1.61.1/LICENSE) | Sample verified; include notice |
| TanStack Query / Tailwind | [Query](https://raw.githubusercontent.com/TanStack/query/main/LICENSE), [Tailwind](https://raw.githubusercontent.com/tailwindlabs/tailwindcss/main/LICENSE) | MIT source samples; verify resolved versions before release |
| Bundled `web/public/Inter-Variable.woff2` | [Inter OFL](https://raw.githubusercontent.com/rsms/inter/master/LICENSE.txt) | Identify bundled font version/provenance and provide its notice |
| Local hot-toast implementation | `web/src/components/hot-toast`; [upstream MIT notice](https://raw.githubusercontent.com/timolins/react-hot-toast/main/LICENSE) | Provenance comparison required; do not assume headers alone establish origin |
| Credited Newznab code | Attribution link in `pkg/newznab/feed.go` and Torznab counterpart | Inspect credited revision/licence and preserve required notice |
| Remaining Go/frontend dependencies and container layers | `go.mod`, `go.sum`, `web/package.json`, `web/pnpm-lock.yaml`, Dockerfile | Full resolved direct/transitive inventory pending |

Generated dependency inventories must distinguish shipped/runtime code from build/test tooling, inspect vendored/copied files and assets, collect exceptions/NOTICE requirements, and record unknowns. An install or successful licence scanner does not itself establish legal compatibility. Do not add a dependency without reviewing its exact licence and resulting distribution obligations.

## Installed dependency inventory, 9 November 2026

Frozen installation yielded 858 unique frontend package name/version pairs. Local package declarations report MIT (709), ISC (57), Apache-2.0 (35), BSD-3-Clause (19), BSD-2-Clause (12), BlueOak-1.0.0 (9), Unlicense (5), MPL-2.0 (4), MIT-0 (2), and one each of 0BSD, Python-2.0, CC-BY-4.0, `(BSD-3-Clause AND Apache-2.0)`, CC0-1.0, and `(MIT OR CC0-1.0)`. These are metadata declarations, not a complete notice/compatibility audit or a statement that every package is shipped.

The MPL entries are Lightning CSS 1.32.0/1.33.0 and their Windows native packages; CC-BY-4.0 is `caniuse-lite` 1.0.30001812. `argparse` 2.0.1 declares Python-2.0 as its licence; that label does not imply a Python interpreter dependency in the application. Review build/runtime inclusion and retain required notices/attribution.

The locked Go module graph contains 216 entries excluding the main module. Of these, 118 had downloaded source directories with root licence/notice files; 98 had metadata but no downloaded source directory. All 118 downloaded directories had a root notice file. This establishes notice-file presence, not licence classification, compatibility or transitive release clearance. Go replacements are recorded, including autobrr's SSE fork.

Local raw evidence: `.dev/licensing/frontend-package-licenses.json`, `go-modules.json`, and `go-module-notices.json`. The source-only graph and actual built dependency closure still need release-specific classification. No manifest or lockfile was altered to generate this inventory.

The reference Python source is not shipped or copied into runtime/tests. Its author/rights provenance still matters if future changes copy expressive material; prefer independent native implementation from documented behaviour and sanitised fixtures. Do not copy Sonarr/Radarr/Sportarr code without specific compatibility review. Calling Prowlarr/SAB APIs is distinct from incorporating their implementation.

## Branding and unresolved review

Copyright permission does not grant trademark rights. 'UFC Manager' is provisional; resolve use of UFC names/logos, product name, event imagery, and implication of affiliation before public branding. Retain autobrr acknowledgement and avoid implying upstream endorsement. New icons/artwork need their own provenance. Metadata providers' terms/licences are separate from application code; Wikidata structured data is [CC0](https://www.wikidata.org/wiki/Wikidata:Licensing), while Wikipedia-derived text may require attribution/share-alike handling. Store source/revision provenance and do not bundle unreviewed imagery.

## Release checklist

- [ ] Exact source/binary/container revisions and dependency versions identified.
- [ ] Complete direct/transitive, copied-code, font/image and container licence inventory reviewed.
- [ ] GPL/Apache compatibility and any exceptions resolved; applicable licence texts and NOTICE files included.
- [ ] Truthful upstream/modification copyright and date notices preserved.
- [ ] Corresponding source/build instructions distributed alongside each binary/MSI/image with a durable matching link/archive.
- [ ] Third-party notices accessible from release materials/application distribution.
- [ ] No restrictive EULA or misleading licence/endorsement claims.
- [ ] Public product name, trademarks, imagery and metadata reuse reviewed.
- [ ] No credentials, user databases, reference script secrets, or real media in artifacts.
- [ ] Installer/container/source archives inspected against this checklist.

## Catalogue source gate refinement

The application must not require metadata UI attribution or a paid subscription. TheSportsDB/TMDB are excluded by the current requirements. [Wikidata structured entity data](https://www.wikidata.org/wiki/Wikidata:Licensing) is CC0; linked Wikipedia article content and media are not covered by that dedication. Read-only research comparison notes retain source links and are not a production event seed. Do not bulk-copy Wikipedia content/compilation and label it attribution-free or CC0. An independent curated feed needs confirmed provenance/reuse rights before publication. Source quality is also a gate: see [the measured spike](research/WIKIDATA_COVERAGE.md).

## Selected Wikipedia source, 10 November 2026

The later explicit Wikipedia source selection supersedes the earlier source hold/attribution exclusion for this importer. It does not make Wikipedia CC0 or waive its terms. Adapted minimal HTML fixtures retain contributor/history attribution, per-revision source links and CC BY-SA 4.0 notice in their [README](../internal/ufc/catalogue/testdata/wikipedia/README.md) and manifest. Runtime source references/revisions and catalogue status retain source/licence attribution; no article prose, images or fight-card archive is persisted as catalogue content. No dependency manifest or lockfile changed; `golang.org/x/net/html` and Unicode normalisation use existing dependencies. Public-distribution licence/branding review remains open; this slice is not release clearance.

The application UI attribution is available in the collapsed **Settings → Application → Catalogue data and licences** section. Event names link to their source revisions. This user-requested relocation removes the Events footer while retaining contributor/licence links and identification of local adaptations; it does not change the outstanding distribution review.

## Phase 9 build tooling

The private Windows package builder uses the official [WiX 3.14.1 portable distribution](https://github.com/wixtoolset/wix3/releases/tag/wix3141rtm), with its Microsoft Reciprocal License read from the bundled LICENSE.TXT before use. It is build-only and adds no Go/frontend runtime dependency. Windows Installer UI material and all other final package notices/source obligations remain part of the Phase 10 distribution inventory; private artifacts do not establish public redistribution clearance. Exact compiler/archive hash and package evidence are in `plans/WINDOWS_PACKAGING.md`.

## Local systray correction

`third_party/systray` preserves github.com/gogpu/systray v0.3.0 sources and its MIT licence/copyright. Only the Windows NIF_SHOWTIP flags change; see PATCHES.md. This is a local replacement for the existing dependency, with no new runtime dependency. Source and licence are included in package source snapshots; final distribution inventory remains pending.
