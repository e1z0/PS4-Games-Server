# Beginner setup

## Native executable

Windows beginners: see the standalone [Windows EXE and service guide](https://github.com/e1z0/PS4-Games-Server/blob/main/WINDOWS.md), also included as WINDOWS.md in Windows archives.

1. Unpack the archive for your OS and CPU. Use amd64 for most Intel/AMD machines and arm64 for Apple Silicon and most ARM servers. On Windows, open a terminal in the unpacked folder.
2. Run `./ps4-server` (Windows: `ps4-server.exe`). The application creates `config.json` with restricted permissions where the OS supports them.
3. Open the printed setup URL. Treat the token as a temporary administrator secret. If browsing remotely, replace `localhost` with the server's address and retain the fragment beginning `#token=`.
4. Choose a username and a password containing at least 12 characters. Passwords are converted into salted PBKDF2-SHA256 hashes before persistence.
5. Add absolute package folder paths. Every mapping has a stable ID, category and enabled flag. Examples: `/srv/ps4/games`, `C:\Games\PS4`. Several folders may use `games` as their category. IDs and categories permit letters, digits, underscore and hyphen; keep an ID stable to retain incremental cache reuse. Folders are scanned recursively; file symlinks must remain inside their mapping. Directory symlinks inside a mapping are not followed.
6. Select writable data storage and enter your server's LAN URL, such as `http://192.168.1.20:8080`. The PS4 must reach this address. The listener `:8080` means all interfaces; it is not an address to put in FPKGi. Open the selected port in your local firewall as needed.
7. Choose startup scans and a scheduled interval in minutes. Zero disables periodic scanning. IGDB is optional; local parsing works offline. Obtain credentials through the [IGDB setup documentation](https://api-docs.igdb.com/#account-creation).
8. Save setup, sign in, and let the first scan run. The admin screen shows progress, errors and recent logs. The progress bar shows folder discovery, package reading, and optional game-detail fetching. Local packages are published before IGDB requests finish. You can browse during enrichment. Stopping before local publication retains the previous catalog; stopping during enrichment keeps the newly published local packages. Force rescan rereads package metadata without hashing game data.
9. Browse `/library`, use search/category/type filters, open game details, and copy feeds. Insert the copied URLs into the `content_urls` object in your FPKGi configuration. Keep other client settings as needed; see the FPKGi snippet in the library.

Generated catalogs, covers, screenshots and caches live under the data directory, separate from `config.json`. Relative data paths resolve against the process working directory. Back up the configuration and data together. Use the same working directory after restarts or choose absolute paths. A partial scan reports unavailable mappings and retains their previous records.

## Docker

Install Docker Engine with Compose. `compose.yaml` builds the image locally. Set `PACKAGES_PATH` to an existing package folder, then run `make deploy` and `make logs`. If using Docker without Go/Make, use `docker compose up -d --build --wait` and `docker compose logs -f`.

The native application sees host paths. The Docker application sees **container paths**: select `/packages` for the default bind mount, `/data` for writable data and `/config/config.json` for configuration. Additional host folders require additional Compose read-only bind mounts; they cannot be mounted from the web panel. Add a mount such as `/srv/other-games:/extra-games:ro`, recreate the service, then map `/extra-games` in administration.

Two named volumes persist configuration and data. The default runtime UID/GID is 10001. To run with `PUID`/`PGID`, first prepare existing volumes with that ownership. One option is `docker compose run --rm --no-deps --user root --entrypoint chown ps4-server -R 1000:1000 /config /data`, then set `PUID=1000 PGID=1000` and start the service. Substitute your actual IDs. Package directories need read/traverse access for those IDs. Bind-mounted configuration/data directories must also be writable by the runtime UID. The application does not change host package permissions.

`make down` stops containers and retains volumes. Avoid `docker compose down -v` unless deliberately deleting settings and generated data. Restarting during incomplete setup creates a new token; finishing setup invalidates the token immediately. For unattended deployment, set administrator environment seeds and the public URL before the very first launch. Already persisted fields, including empty strings, win over seeds; use web settings or edit the JSON while stopped for later changes.

Use `docker compose exec ps4-server ps4-server health -config /config/config.json` for health. `make upload VERSION=... TAG=...` publishes both supported Linux architectures using an automatically created/reused Buildx builder; authenticate with `docker login` externally. `STABLE=1` explicitly adds `latest`.

## Administration and recovery

`/admin` provides mapping management, settings, scanning, status/logs and JSON metadata overrides. Credentials and IGDB secrets are redacted; blank secret inputs retain their current values. Disable IGDB to stop fetching. Credential changes invalidate all sessions. Listener or data-directory changes require a restart; other settings are captured at the start of each job. Existing unavailable mappings may be retained while editing unrelated settings.

To reset the password, stop the service and run `ps4-server password-reset -config config.json`, type the new password on standard input, then send EOF (Ctrl-D on Unix; Ctrl-Z then Enter on Windows). Input is read verbatim from stdin, so terminals may echo it; use protected input or a secure password pipe if needed. Restart any existing process to invalidate its sessions and load the new hash.

An optional separate download listener requires both `download_listen` and a PS4-reachable `download_url` for that listener. Publish its port in Compose too. This listener exposes only health, package/cover downloads and background; library, feeds and administration remain on the main port.

Custom covers: place a valid PNG/JPEG at `data/covers/<category>/<TitleID>.png` (or `.jpg`/`.jpeg`). DLC uses the complete Content ID in place of Title ID. Missing identifiers use a deterministic key; inspect the generated cover URL. Valid custom covers are retained; missing/corrupt covers are repaired from plaintext package icons. A fallback is served when no valid image exists. Background accepts a server-native PNG/JPEG path. Encrypted entries are reported without decryption.

Generated cache files are internal. If a cache file becomes malformed, stop the service, move it aside, and scan again; never delete your custom covers as part of cache repair. No console has been used to validate this implementation yet.

The wizard retains entered fields when going Back and validates folder access before proceeding. On Windows, the picker starts in the server's working directory and lists available drives. Changing listeners during setup also requires a restart; the wizard and administration screen show this notice. Copy the canonical `/feeds/<category>` URLs from the library; legacy short category aliases remain supported.

Stop a running writer before using one-shot scan or password-reset against the same configuration/data. The application enforces this with OS file locks and automatically releases them after process exit; empty `.lock` sidecar files are expected. Mapping edits saved during a scan take effect in the next published scan, keeping currently published package URLs available until then.

The FPKGi configuration clipboard helper lowercases client category labels and checks the client's limit of 32 sources including games, 32 characters per label, distinct labels ignoring case, and the reserved ps5 label. Feed names remain unchanged. If a collection exceeds those client limits, individual feed URLs can be copied into selected, shorter client category labels.
