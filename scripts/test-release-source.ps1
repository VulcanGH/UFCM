$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-source.ps1')
$root = Split-Path -Parent $PSScriptRoot
$fixture = Join-Path $root ('.dev/security/policy-fixture-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force $fixture | Out-Null
& git init --quiet $fixture
if ($LASTEXITCODE -ne 0) { throw 'Could not initialise isolated source fixture' }
foreach ($path in @('go.mod','internal/main.go','web/package.json','web/dist/.gitkeep','scripts/package-windows.ps1','LICENSE','AGENTS.md','PLANS.md','docs/plans/private.md','docs/LICENSING.md')) {
    $target = Join-Path $fixture $path
    New-Item -ItemType Directory -Force (Split-Path -Parent $target) | Out-Null
    [IO.File]::WriteAllText($target, 'fixture')
}
& git -C $fixture add .
if ($LASTEXITCODE -ne 0) { throw 'Could not stage isolated source fixture' }
[IO.File]::Delete((Join-Path $fixture 'web/dist/.gitkeep'))
[IO.File]::WriteAllText((Join-Path $fixture 'internal/untracked.go'), 'must not ship')
$files = @(Get-ReleaseSourceFiles -Root $fixture)
foreach ($path in @('AGENTS.md','PLANS.md','docs/plans/private.md','internal/untracked.go','web/dist/.gitkeep')) { if ($path -in $files) { throw 'Private, generated or untracked fixture was included' } }
foreach ($path in @('go.mod','internal/main.go','web/package.json','LICENSE','docs/LICENSING.md')) { if ($path -notin $files) { throw 'Required source or notice was excluded' } }
[IO.File]::WriteAllText((Join-Path $fixture 'internal/private.pem'), 'not a real key')
& git -C $fixture add internal/private.pem
$rejected = $false
try { Get-ReleaseSourceFiles -Root $fixture | Out-Null } catch { $rejected = $_.Exception.Message -like 'Private or generated path*' }
if (-not $rejected) { throw 'Credential-shaped tracked file was accepted' }
$assets = @(Get-ReleaseAssetNames -Version '0.1.9' -Signed)
if ($assets.Count -ne 5 -or 'BUILD-MANIFEST.json' -in $assets -or 'ufc-manager-0.1.9-windows-x86_64.zip' -in $assets -or 'ufc-manager-0.1.9-windows-x86_64-source.zip' -in $assets) { throw 'Unexpected public asset set' }
$actual = @(Get-ReleaseSourceFiles -Root $root)
if ($actual.Count -eq 0 -or 'AGENTS.md' -in $actual -or 'PLANS.md' -in $actual) { throw 'Actual checkout source policy failed' }
'PASS: tracked allowlist, private-doc exclusion, retained notices, credential-path rejection and five-asset publication policy'
