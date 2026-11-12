# Development environment

Use the requirements in `go.mod` and `web/package.json`, not guessed versions. The audited revision requires Go >=1.27.0, selects toolchain 1.27.3, requires Node >=24 and pins pnpm 11.24.0. Baseline Node is 24.15.0 to match `.github/workflows/release.yml`. Do not update dependencies or lockfiles to establish the baseline.

The current Windows baseline is measured but not entirely green. Read [the actual results and blockers](DEVELOPMENT_BASELINE.md) before interpreting these commands as a stable green baseline. Foundation was authorised with those known exceptions; its [results](FOUNDATION_RESULTS.md) are a separate record.

## Workspace-local tools

This workspace uses portable verified tools under ignored `.dev/tools`: Go 1.27.3, Node 24.15.0 and pnpm 11.24.0. Go archive SHA-256 comes from official Go release metadata; Node SHA-256 from the release SHASUMS256 manifest; pnpm SHA-512 integrity from npm registry metadata. Verification protects downloaded artifacts relative to these HTTPS manifests; it is not a claim of independent signature verification. Toolchain metadata and diagnostic logs stay under ignored `.dev`.

For a fresh environment obtain those versions from [Go releases](https://go.dev/dl/), [Node distributions](https://nodejs.org/dist/v24.15.0/), and the package-manager version pinned in `web/package.json`; verify their published integrity values before extraction. A system installation is optional. No administrator-level or permanent PATH change is required by the portable setup.

In PowerShell, from the repository root:

```powershell
$devRoot = Join-Path (Get-Location).Path '.dev'
$toolRoot = Join-Path $devRoot 'tools'
$env:PATH = "$toolRoot\go\bin;$toolRoot\node-v24.15.0-win-x64;$toolRoot;C:\Program Files\Git\bin;" + $env:PATH
$env:GOTOOLCHAIN = 'local'
$env:GOFLAGS = '-mod=readonly'
$env:CGO_ENABLED = '0'
$env:GOPATH = Join-Path $devRoot 'gopath'
$env:GOMODCACHE = Join-Path $devRoot 'gomodcache'
$env:GOCACHE = Join-Path $devRoot 'gocache'
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $devRoot 'playwright'
$env:LOCALAPPDATA = Join-Path $devRoot 'user-cache'
$env:TEMP = Join-Path $devRoot 'tmp'
$env:TMP = $env:TEMP
New-Item -ItemType Directory -Force -Path $env:TEMP, $env:LOCALAPPDATA, 'bin/baseline' | Out-Null
go version
node --version
pnpm.cmd --version
go mod download
pnpm.cmd --dir web install --frozen-lockfile --store-dir "$devRoot\pnpm-store"
```

The local `pnpm.cmd` shim launches the verified package with Node. Use `pnpm` normally where installed. Run web scripts from the repo root as `pnpm --dir web <script>`; do not change into `web` and invoke pnpm. Keep script execution-policy overrides process-scoped when required for locally reviewed PowerShell setup scripts; do not change machine policy.

## Mandatory checks and builds

```powershell
pnpm.cmd --dir web lint
pnpm.cmd --dir web test
pnpm.cmd --dir web build
go test ./...
go vet ./...
go build -o bin/baseline/autobrr.exe ./cmd/autobrr
go build -o bin/baseline/autobrrctl.exe ./cmd/autobrrctl
go build -ldflags '-H=windowsgui' -o bin/baseline/autobrr-desktop.exe ./cmd/autobrr-desktop
```

Use ignored `bin/` outputs rather than leaving executables in the repository root. Build `web/dist` before binaries that embed it. Standard `make` targets remain available on environments with their shell prerequisites; `make dev` needs tmux and is not the Windows baseline entry point. Run frontend dev server with `pnpm --dir web dev` and the Go server with an isolated `--config` directory, never production settings.

## Environment-dependent checks

See [TEST_STRATEGY.md](TEST_STRATEGY.md) for tagged SQL/IRC/simulation/browser suites. For Playwright, resolve the exact Go module version, then run `go run github.com/mxschmitt/playwright-go/cmd/playwright@<resolved-version> install chromium`; use Linux `--with-deps` only where appropriate. Record embedded PostgreSQL download/platform constraints. Race tests require their supported C toolchain; `CGO_ENABLED=0` is not a race-test environment. Installer UI/Explorer/logon checks require an interactive disposable Windows account. Do not start services, acquire media or exercise production webhooks to validate setup.

## Workflow

Keep the current baseline revision fixed while measuring it. Use focused branches and PRs to fork `develop`, preserve upstream, update ADRs/plans when decisions change, and keep formatting/migrations/tests in the feature milestone that owns them. Review monthly upstream integrations separately. [PLANS.md](../PLANS.md) defines completion evidence. Foundation follows the [approved plan](plans/FOUNDATION.md); catalogue work remains gated on review.

## Foundation variants and checks

After building the frontend, use the existing entrypoints:

```powershell
go build -o bin/foundation/autobrr.exe ./cmd/autobrr
go build -ldflags '-H=windowsgui' -o bin/foundation/autobrr-desktop.exe ./cmd/autobrr-desktop
go build -ldflags '-X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -o bin/foundation/ufc-manager.exe ./cmd/autobrr
go build -ldflags '-H=windowsgui -X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -o bin/foundation/ufc-manager-desktop.exe ./cmd/autobrr-desktop
go test -tags=integration -count=1 -timeout 10m ./internal/ufc/...
go test -tags=foundation_e2e -count=1 -timeout 5m ./test/foundation
```

UFC defaults to `os.UserConfigDir()/ufc-manager`, port 7475, `UFC_MANAGER__` environment variables and a distinct session cookie. Explicit `--config` and config/environment port settings take precedence. Legacy defaults remain unchanged. Never launch development binaries with real media or production configuration. The UFC build flag is mandatory for UFC artifacts; filenames alone do not select identity. MSI/release automation changes are deferred.

See [the Foundation test guide](../test/foundation/README.md) for browser/registry prerequisites and [results](FOUNDATION_RESULTS.md) for exact observed outcomes.
