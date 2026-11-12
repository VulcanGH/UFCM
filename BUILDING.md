# Building UFC Manager

Windows x64 is the supported application target. Use the Go version/toolchain in `go.mod`, Node.js 24 and the pnpm version in `web/package.json`.

From the repository root:

```powershell
pnpm --dir web install --frozen-lockfile
pnpm --dir web build
go generate ./cmd/autobrr-desktop
go build -trimpath -ldflags '-H windowsgui -X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -o ufc-manager-desktop.exe ./cmd/autobrr-desktop
go build -trimpath -ldflags '-X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -o ufc-manager.exe ./cmd/autobrr
go build -trimpath -ldflags '-H windowsgui -X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -o ufc-manager-launcher.exe ./cmd/ufc-manager-launcher
```

The interface is embedded in the Go application. Build it before compiling Go. `go generate` uses the pinned Windows resource generator and supplied artwork. The local MIT-licensed tray correction is documented in `third_party/systray/PATCHES.md`.

For installer packages, use WiX 3.14.1 and the checked-in package builder:

```powershell
./scripts/package-windows.ps1 -Version 1.0.0 -WixRoot C:/tools/wix3141 -UpdatePublicKey $env:UFC_UPDATE_PUBLIC_KEY
```

GitHub's automatic source downloads provide the tagged source and these build instructions. Use a Git checkout for installer packaging and its local build manifest. The public verification key is required for installable packages; the private signing key belongs only in the protected release-signing environment. Neither binary compilation from a source archive nor normal application use requires a private key.

For focused checks, run `pnpm --dir web lint`, `pnpm --dir web test`, and relevant Go package tests. Integration tests use build tags and isolated test databases. Never use real media folders as test fixtures.
