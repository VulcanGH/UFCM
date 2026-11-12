param(
    [string]$Version = '0.1.0-preview.1',
    [Parameter(Mandatory = $true)][string]$WixRoot,
    [switch]$SkipWebBuild,
    [string]$RepositoryRoot,
    [string]$UpdatePublicKey = $env:UFC_UPDATE_PUBLIC_KEY
)
$ErrorActionPreference = 'Stop'
$repoRoot = if ($RepositoryRoot) { (Resolve-Path -LiteralPath $RepositoryRoot).Path } else { Split-Path -Parent $PSScriptRoot }
Set-Location -LiteralPath $repoRoot
. (Join-Path $PSScriptRoot 'release-source.ps1')
if (-not $UpdatePublicKey) { throw 'UFC_UPDATE_PUBLIC_KEY is required for installable packages; use the public verification key, never the private signing key' }
if ([Convert]::FromBase64String($UpdatePublicKey).Length -ne 32) { throw 'Update public key must be an Ed25519 public key' }
if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)(?:-[0-9A-Za-z.-]+)?$') { throw 'Version must be a numeric triplet with an optional prerelease suffix' }
if ([int]$Matches[1] -gt 255 -or [int]$Matches[2] -gt 255 -or [int]$Matches[3] -gt 65535) { throw 'Version exceeds Windows Installer limits' }
$msiVersion = '{0}.{1}.{2}' -f $Matches[1],$Matches[2],$Matches[3]
$wixPath = (Resolve-Path -LiteralPath $WixRoot).Path
foreach ($tool in @('candle.exe','light.exe')) { if (-not (Test-Path -LiteralPath (Join-Path $wixPath $tool))) { throw "Missing WiX tool: $tool" } }
if (-not $SkipWebBuild) {
    & pnpm --dir web build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
}
if (-not (Test-Path -LiteralPath 'web/dist/index.html')) { throw 'Build the frontend first' }
$output = Join-Path $repoRoot ('bin\packages\ufc-manager-' + $Version)
if (Test-Path -LiteralPath $output) { throw 'Output already exists; choose a new version or move the previous package aside' }
$payload = Join-Path $output 'payload'
New-Item -ItemType Directory -Path $payload | Out-Null
$commit = (& git rev-parse HEAD).Trim()
$builtAt = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
$baseFlags = "-s -w -X main.version=$Version -X main.commit=$commit -X main.date=$builtAt -X github.com/autobrr/autobrr/internal/product.variant=ufc-manager"
$schema = (& go run ./cmd/ufc-update-release --print-schema).Trim()
if ($LASTEXITCODE -ne 0 -or $schema -notmatch '^[a-f0-9]{64}$') { throw 'Migration digest generation failed' }
$baseFlags += " -X github.com/autobrr/autobrr/internal/ufc/updater.SchemaDigest=$schema"
if ($UpdatePublicKey) {
    if ([Convert]::FromBase64String($UpdatePublicKey).Length -ne 32) { throw 'Update public key must be an Ed25519 public key' }
    $baseFlags += " -X github.com/autobrr/autobrr/internal/ufc/updater.PublicKey=$UpdatePublicKey"
}
& go build -trimpath -ldflags "$baseFlags -H windowsgui" -o (Join-Path $payload 'ufc-manager-desktop.exe') ./cmd/autobrr-desktop
if ($LASTEXITCODE -ne 0) { throw 'Tray build failed' }
& go build -trimpath -ldflags $baseFlags -o (Join-Path $payload 'ufc-manager.exe') ./cmd/autobrr
if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
Copy-Item -LiteralPath LICENSE -Destination (Join-Path $payload 'LICENSE.txt')
Copy-Item -LiteralPath distrib/windows/UFC-README.txt -Destination (Join-Path $payload 'README.txt')
$runtimeZip = Join-Path $output ('ufc-manager-' + $Version + '-runtime-windows-amd64.zip')
Compress-Archive -Path (Join-Path $payload '*') -DestinationPath $runtimeZip
& go run ./cmd/ufc-update-release --seed --version $Version --runtime $payload
if ($LASTEXITCODE -ne 0) { throw 'Baseline metadata generation failed' }
$seed = Join-Path $payload 'seed'
New-Item -ItemType Directory -Path $seed | Out-Null
foreach ($name in @('ufc-manager-desktop.exe','ufc-manager.exe','LICENSE.txt','README.txt','seed.json')) {
    Move-Item -LiteralPath (Join-Path $payload $name) -Destination (Join-Path $seed $name)
}
& go build -trimpath -ldflags "$baseFlags -H windowsgui" -o (Join-Path $payload 'ufc-manager-launcher.exe') ./cmd/ufc-manager-launcher
if ($LASTEXITCODE -ne 0) { throw 'Launcher build failed' }
Copy-Item -LiteralPath LICENSE -Destination (Join-Path $payload 'LICENSE.txt')
Copy-Item -LiteralPath distrib/windows/UFC-README.txt -Destination (Join-Path $payload 'README.txt')
$license = (Get-Content -LiteralPath LICENSE -Raw).Replace('\','\\').Replace('{','\{').Replace('}','\}').Replace("`r",'').Replace("`n",'\par ')
$licenseRtf = Join-Path $output 'LICENSE.rtf'
[IO.File]::WriteAllText($licenseRtf, '{\rtf1\ansi\deff0 {\fonttbl{\f0 Courier New;}}\f0\fs16 ' + $license + '}')
$object = Join-Path $output 'ufc-manager.wixobj'
& (Join-Path $wixPath 'candle.exe') -nologo -arch x64 "-dSourceRoot=$repoRoot" "-dPayloadDir=$payload" "-dMsiVersion=$msiVersion" "-dLicenseRtf=$licenseRtf" -out $object distrib/windows/ufc-manager-desktop.wxs
if ($LASTEXITCODE -ne 0) { throw 'MSI authoring compilation failed' }
$stem = 'ufc-manager-' + $Version + '-windows-x86_64'
$msi = Join-Path $output ($stem + '.msi')
& (Join-Path $wixPath 'light.exe') -nologo -ext WixUIExtension -out $msi $object
if ($LASTEXITCODE -ne 0) { throw 'MSI linking/validation failed' }
$files = @(Get-ReleaseSourceFiles -Root $repoRoot)
$manifest = @()
foreach ($relative in $files) {
    if (-not (Test-Path -LiteralPath $relative -PathType Leaf)) { continue }
    $manifest += @{path=$relative;sha256=(Get-FileHash -LiteralPath $relative -Algorithm SHA256).Hash.ToLowerInvariant()}
}
$buildManifest = @{version=$Version;msi_version=$msiVersion;commit=$commit;built_at=$builtAt;source_files=$manifest;go=(& go version);wix=(Get-Item -LiteralPath (Join-Path $wixPath 'candle.exe')).VersionInfo.FileVersion}
$buildManifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $output 'BUILD-MANIFEST.json') -Encoding UTF8
Get-ChildItem -LiteralPath $output -File | Where-Object { $_.Extension -in '.zip','.msi' } | ForEach-Object {
    '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(),$_.Name
} | Set-Content -LiteralPath (Join-Path $output 'SHA256SUMS.txt') -Encoding ASCII
Write-Output $output
