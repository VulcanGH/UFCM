UFC Manager - private Windows preview

Launch ufc-manager-launcher.exe for the tray application. It has no console.
Open Web UI, Open Config Folder, Start on Login and Quit are in its tray menu.
The normal default URL is http://127.0.0.1:7475.

MSI: installs for the current user, defaulting to Program Files\UFC Manager.
Choose another destination in the installer. Installation requests administrator
permission; the application runs normally afterward. Upgrades retain the previous
installation folder. Existing settings, database and runtime updates stay per-user.
Start on Login is enabled on the installed application's first launch; the tray
toggle and Windows Startup settings control it afterward. Upgrades retain that
preference. Silent installs do not launch the application automatically.

ZIP: extract into a stable folder. No installer registration is written. Use the
tray menu to opt into Start on Login. Moving the folder requires re-enabling it.

Data: configuration, database and logs live separately under
%APPDATA%\ufc-manager unless --config selects another directory. Media stays in
the folders you configured. The installer/uninstaller never owns those locations.
Uninstall removes only installed application files and UFC startup registration.
Downloaded runtimes and private recovery backups are retained conservatively.

Development demo data is NOT automatically adopted. Continue with its explicit
--config path if required; do not merge databases or copy a database while running.
The packaged app and demo cannot share a configuration concurrently.

Before upgrading: Quit the tray, preserve the old package, copy the configuration
files and create a consistent database backup. For SQLite, use:
  seed\ufc-manager.exe db backup --db-path "C:\path\autobrr.db" --output "C:\backup\pre-upgrade.db"
The backup command never migrates the source or replaces an existing destination.
For PostgreSQL use pg_dump and retain a tested restore procedure. A raw file copy
of a running SQLite database can miss its WAL; it is not a valid backup method.

Install the new MSI (or replace ZIP binaries only after Quit), then launch it.
Use existing Downloads review controls for interrupted or uncertain operations.
The upstream automatic updater remains disabled for UFC Manager. Its separate
in-app updater accepts only signed public releases from VulcanGH/UFCM. Builds
without a configured verification key cannot install updates. Settings shows
Update and Restart when a verified package is ready; automatic installation when
idle is opt-in. PostgreSQL users always install upgrades manually.

The launcher keeps runtimes under %LOCALAPPDATA%\UFC Manager Runtime in a folder
bound to the installation path and one configuration. Shortcuts and login entries
must point to the launcher. Do not execute downloaded version folders directly.
For an in-app update, current work finishes before shutdown; SQLite/configuration
are backed up before a gated trial. An uncertain trial never rewinds the database.
Follow recovery guidance if the launcher reports an incompatible migrated schema.

Rollback: Quit first. Restore BOTH the matching old binaries and their matching
pre-upgrade database/configuration backup. Never run an old binary against a newer
schema. Restoring a database does not undo copies already published to Library;
review inventory and remote jobs before re-enabling automatic work.

No Windows service or before-login execution is required. Log files rotate in the
configuration's logs folder. Public distribution, signing and complete licensing
clearance remain Phase 10 work. The inherited tray/executable icon is retained.

The matching source archive and build manifest accompany each local package.
Original upstream notices and LICENSE are retained.
