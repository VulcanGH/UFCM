# Foundation browser and host acceptance

This suite is separate from mandatory unit/static/build checks and from the upstream `test/e2e` suite. It builds both server identities, launches temporary SQLite configurations, onboards fake users, exercises authentication/scopes and webhook mocks, and uses headless Chromium for the UFC page and legacy navigation. No real credentials, services or media are required.

From the repository root, with the pinned Go/Node/pnpm tools and frozen dependencies installed:

```powershell
pnpm --dir web build
go test -tags=foundation_e2e -count=1 -timeout 5m ./test/foundation
go test -tags=integration -count=1 -timeout 10m ./internal/ufc/...
go test ./internal/product ./internal/config ./internal/meta ./internal/update ./cmd/autobrr-desktop
go test -ldflags '-X github.com/autobrr/autobrr/internal/product.variant=ufc-manager' -run '^TestProductRegistryIsolation$|^TestUFCUpdaterDisabled$' -count=1 ./cmd/autobrr-desktop
```

Browser checks require the Playwright driver/Chromium version matching the existing Go module, child-process permission, loopback ports and writable temporary storage. Set `PLAYWRIGHT_BROWSERS_PATH` and `LOCALAPPDATA` consistently when using a portable installation. Both-engine checks start embedded PostgreSQL 17 and require its binary download/cache, process permission and loopback port. An unavailable PostgreSQL engine is a failure, not a skipped pass. Use a supported environment if prerequisites cannot be met.

Windows registry tests write process-named values under both HKCU product namespaces and remove only those test values. Existing startup-value tests also use process-named entries and cleanup. They require HKCU access. The focused UFC updater test is intentionally skipped in a legacy test build and must also run with the UFC linker flag as shown above.

On Windows the tagged suite also runs all eight artifact cases natively in Go, strips inherited product environment variables and uses only temporary configurations. Artifact smoke matrix: build both server/tray variants as documented in [development](../../docs/DEVELOPMENT.md); launch each with temporary APPDATA/current directory and with an explicit configuration path containing spaces plus a port override. Verify readiness, reported identity/config/port, forced-disabled UFC updates, and zero UFC tables for autobrr versus two for UFC. Inspect tray PE subsystem `WINDOWS_GUI`; launch/stop only owned test processes. Generated legacy configs retain their update default in unit tests; artifact smoke explicitly disables legacy network checks. The recorded Windows run uses the tray host's session-end message for orderly exit, not a real user logout.

These checks do not replace Explorer/Start Menu, tray-menu interaction, login/reboot, MSI upgrade/repair/uninstall or code-signing acceptance. Those remain in the Windows distribution milestone. See [Foundation results](../../docs/FOUNDATION_RESULTS.md).
