# Windows deployment

Use the x64 MSI from Releases for a normal installation. Fresh installations default to Program Files and offer a destination picker; upgrades retain the saved install directory. The installer requests administrator permission, while configuration, SQLite database, logs and downloaded runtimes remain per-user.

Launch UFC Manager from the Start menu. Its tray menu opens the interface/config folder and controls login startup. The default interface is http://127.0.0.1:7475. See the [package README](../distrib/windows/UFC-README.txt) for backup and recovery guidance.

In Settings, choose Check for Updates, wait for Ready to Update, then choose Update and Restart. Signed public stable releases are eligible; drafts are ignored. PostgreSQL installations require manual updates.

Uninstalling application binaries must not remove configured library media or application data. Keep backups before updates. See [building](../BUILDING.md) for package compilation and [release contents](../PACKAGING.md) for what is distributed.