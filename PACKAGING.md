# Release contents

Each Windows release uploads five files:

| File | Purpose |
| --- | --- |
| `ufc-manager-VERSION-windows-x86_64.msi` | Normal installation |
| `ufc-manager-VERSION-runtime-windows-amd64.zip` | In-app updater payload; not a standalone installer |
| `ufc-update.json` | Signed update description |
| `ufc-update.sig` | Update-description signature |
| `SHA256SUMS.txt` | Archive/installer integrity checks |

GitHub automatically provides Source code (zip) and Source code (tar.gz) for the release tag. These supply the corresponding tagged source, build scripts, dependency locks and notices; no duplicate source ZIP is uploaded. Keep release tags at their original source commits. The build manifest remains a local build output, not a release attachment. The tracked-source privacy check remains enabled.

CI transfers only the installer, runtime archive and checksum list between build/sign jobs. Signing extracts the runtime rather than uploading an additional payload tree. It uses the protected signing secret; the public key is safe to distribute. The runtime ZIP and signed metadata names/URLs remain stable for installed clients.

Deleting files on the current branch does not remove older Git history or automatic source archives for old tags. Repository replacement/history cleanup is a separate operation. Preserve the public repository URL, signing key and increasing release sequence when migrating an existing update channel.

See [building](BUILDING.md), [Windows usage](distrib/windows/UFC-README.txt) and the retained [upstream licence](LICENSE). Licence/attribution obligations still apply when distributing a fork or a new repository.
