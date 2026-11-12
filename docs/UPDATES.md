# UFC Manager updates

Windows packages include a stable launcher and a versioned application runtime. Installer identity, executable names and profile locations remain stable across updates.

The public release channel is `VulcanGH/UFCM`. Ordinary source pushes do not update installations. Only a published stable release with valid signed update metadata is eligible. PostgreSQL installations always remain operator-managed; SQLite is the default and supports in-app installation.

## User flow

Use the launcher-based Windows package once available. Existing pre-launcher private previews require a one-time MSI upgrade after quitting all existing instances and taking a verified backup. Shortcuts and Start on Login point to `ufc-manager-launcher.exe`; the matching operator CLI is in `seed/ufc-manager.exe`.

Settings shows the installed/available versions and release notes. Verified updates download in the background. **Update and Restart** waits for current work, backs up SQLite/configuration and relaunches through a gated trial. **Automatically Install When Idle** is off by default. An interrupted download leaves the current application running. A backup failure restarts the old version and requires an explicit retry; it does not change your automatic-install preference or create a restart loop.

While waiting, new work pauses and admitted operations finish normally. If draining takes more than 30 minutes, the update is postponed and ordinary work resumes; no active operation is forcibly cancelled. The same deferred release requires an explicit retry. The launcher preserves configuration environment variables and the working directory, and independently refuses an old pending request after a newer version has been installed.

Availability uses the existing Application Update notification event. Failed/deferred updates use UFC Attention through the existing durable outbox and configured destinations, including Discord. Notification retries do not repeat downloads, processing or installation.

## Maintainer setup and publication

1. Complete public licensing/signing clearance and isolated installer/session acceptance. Local private packages do not establish distribution clearance.
2. Provision an Ed25519 signing key through your trusted secret-management process. Retain an encrypted offline recovery copy outside the repository. Never paste the private key into source, command arguments, logs or chat. No production key is supplied by this implementation.
3. Configure the repository variable `UFC_UPDATE_PUBLIC_KEY` with the base64 32-byte public key. Configure a protected `ufc-release` GitHub environment with release approval and its `UFC_UPDATE_PRIVATE_KEY` secret containing the base64 64-byte Ed25519 private key. Restrict who can change this environment, workflows and release tags. The signing job receives the private key only through its environment.
4. Commit/review the intended code and create an existing stable tag such as `v1.0.1`. Run **UFC Manager release draft** manually for that tag and an explicitly increasing signing sequence. Use `20000000` for the initial UFCM 1.0.0 release and increase it for every subsequent release; never derive it from a repository workflow-run counter. The read-only build job runs focused checks, prepares matching launcher/runtime/source packages and verifies the pinned WiX tool archive. The protected signing job signs exact metadata and creates a complete draft. Reusing an existing release must fail rather than overwrite its assets.
5. Review the draft, source/notices, focused test evidence and any database migration compatibility implications. Publish the complete draft deliberately. Installed clients then discover it. An unsigned, incomplete, draft, prerelease, incompatible or replayed release is ignored/rejected.

The manifest includes a monotonic maintainer-supplied release sequence, version/commit, platform, launcher protocol, migration-source digest, payload size/hash and every permitted runtime file. Installer and runtime binaries must be built with the same pinned public key. A build without it cannot install updates. Windows Authenticode is separate from manifest signing; select and provision the Windows signing service before public distribution as appropriate.

For local key generation, run `go run ./cmd/ufc-update-keygen` from the repository root. On Windows it creates `%APPDATA%/UFC Manager Signing/private-key.txt` and `public-key.txt`, restricts directory access to the current user and SYSTEM, and prints only the directory location. Files contain the exact base64 formats expected above. It refuses an existing signing directory rather than replacing a key pair. The generator's focused test verifies signature compatibility and no replacement using temporary keys only; no production keys were generated during implementation.

## Recovery and servicing

Runtime selection and private backups live in `%LOCALAPPDATA%/UFC Manager Runtime/<installation-path-hash>/`. One installation is bound to one configuration. Recovery backups have separate attempt directories, so a failed snapshot does not overwrite or invalidate the next attempt. Configuration, database, download history and library media remain outside installer file ownership.

A trial cannot start workers before the launcher commits activation. A failed trial can return to the prior runtime only after confirmed process exit and an unchanged SQLite schema plus both migration histories. Otherwise recovery pauses. No automatic database restore is performed: restoring an old database after external work could replay downloads/copies or lose history. Preserve current journals and use a matching, explicitly reviewed recovery procedure or forward fix.

Quit UFC Manager before installer repair, upgrade or uninstall. MSI maintenance markers exclude launcher/direct-profile starts during file changes and are released after a successful commit. A failed, cancelled or interrupted installer intentionally retains its marker, including through rollback, until that same installer transaction is rerun/repaired successfully. This avoids reopening the app while file restoration may still be in progress. Do not delete a marker while an installer or updater may still be active. Existing legacy pre-launcher instances must all be stopped for the initial transition.

Launcher protocol changes and key rotation/loss/compromise recovery use a separately trusted MSI servicing path initially. Do not change only the application's key and leave the stable launcher trusting an unrelated key. Automated trust-key rotation is not implemented.

Unknown runtime content and recovery backups are retained. Only verified, flat, unreferenced runtime directories can be pruned; installer uninstall conservatively leaves downloaded/runtime recovery storage. No updater or installer cleanup traverses library or completed-download roots. See [the implementation/verification plan](plans/IN_APP_UPDATES.md) and [ADR 0021](DECISIONS/0021-signed-launcher-updates.md).
