# Pinned English Wikipedia parser fixtures

These are adapted excerpts from English Wikipedia, not CC0 or an independent source of truth. The contributors to each linked article/list are the authors. [manifest.json](manifest.json) records per-file permanent revision links and hashes; each linked revision's **View history** provides contributor attribution. Content is available under [Creative Commons Attribution-ShareAlike 4.0](https://creativecommons.org/licenses/by-sa/4.0/), subject to [Wikimedia Terms of Use](https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use). Retain these notices, source links and adaptation descriptions when redistributing the fixtures.

Adaptations: list limited to the 26 study cases plus UFC 37.5, UFC 176 and the actual UFC 196; omitted rows and nonessential merged-cell spans removed. Articles retain lead paragraphs and one explicit earlier-billing paragraph for Fight Night 82; unrelated prose, infoboxes, images, navigation, references and fight cards are omitted. HTML citation markup remains to exercise parsing. Expectations are the manually reviewed acquisition facts from the existing study, with page/revision identities added. Fixtures are test data, not an application seed. Synthetic drift/conflict/error variants in tests are project-authored.

Reproduce the measured source-agreement report without network access:

```powershell
$env:UFC_SOURCE_REPORT = (Join-Path $PWD 'docs/research/WIKIPEDIA_SOURCE_RESULTS.json')
go test -count=1 -run '^TestPinnedSourceGate$' -v ./internal/ufc/catalogue
Remove-Item Env:UFC_SOURCE_REPORT
```

The source gate requires all eight scheduled and eight recent cases with expected calendar dates, surfaced unresolved identities and no unsafe current identifiers. Historical agreement and alias recovery are reported separately. This is agreement with the frozen Wikipedia reference, not independent promoter verification or a claim of continuing coverage.
