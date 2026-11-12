function Test-ReleaseSourcePath {
    param([Parameter(Mandatory = $true)][string]$Path)
    $rootFiles = @('go.mod','go.sum','LICENSE','README.md','README.autobrr.md','CONTRIBUTING.md','CODE_OF_CONDUCT.md','Makefile','config.toml','BUILDING.md','PACKAGING.md','NOTICE.md')
    return $Path -in $rootFiles -or $Path -match '^(cmd|internal|pkg|web|third_party|test|scripts|distrib)/' -or $Path -in @('docs/man/autobrr.1','docs/LICENSING.md','docs/UPSTREAM_CHANGES.md')
}

function Get-ReleaseSourceFiles {
    param([Parameter(Mandatory = $true)][string]$Root)
    $tracked = @(& git -C $Root ls-files --cached)
    if ($LASTEXITCODE -ne 0) { throw 'Could not enumerate tracked source files' }
    foreach ($relative in $tracked) {
        if (-not (Test-ReleaseSourcePath -Path $relative)) { continue }
        if ($relative -eq 'web/dist/.gitkeep') { continue }
        if ($relative -match '(^|/)(AGENTS\.md|PLANS\.md|\.env(?:\..*)?|node_modules|dist|build|\.dev|\.git|\.codex|\.claude)(/|$)|\.(?:pfx|p12|pem|key|sqlite|db|log)$') { throw "Private or generated path in release source: $relative" }
        if ($relative -match '(^|/)(?:id_rsa|id_ed25519|credentials)$') { throw 'Credential file in release source' }
        $path = Join-Path $Root $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Tracked release source is missing: $relative" }
        if ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Release source must not be a link: $relative" }
        $relative
    }
}

function Get-ReleaseAssetNames {
    param([Parameter(Mandatory = $true)][string]$Version, [switch]$Signed)
    "ufc-manager-$Version-windows-x86_64.msi"
    "ufc-manager-$Version-runtime-windows-amd64.zip"
    'SHA256SUMS.txt'
    if ($Signed) { 'ufc-update.json'; 'ufc-update.sig' }
}
