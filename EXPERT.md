# Expert guide

For first installation, use the [README](README.md) or [Windows walkthrough](WINDOWS.md). This guide covers optional features and maintenance.

## Optional IGDB details

Package titles, versions, firmware and available covers are read locally. IGDB adds descriptions, storylines, genres, companies, ratings, player information and media when a matching PS4 base game is found.

1. Follow the [IGDB account and application setup instructions](https://api-docs.igdb.com/#account-creation) to obtain a Client ID and Client Secret.
2. In Administration, expand **Optional game details**, enable IGDB, enter the credentials and save.
3. Start a normal scan. Local packages remain available while details and screenshots are added.

Metadata is shared by Title ID across a game's packages. Default positive-cache lifetime is 30 days; unsuccessful matches are cached for 7 days. Network errors do not become negative matches. Existing metadata remains usable offline. Downloaded screenshots and their validation fingerprints are persisted: unchanged files skip decoding across scans and restarts, while missing or changed images are validated and repaired. Older installations need one validating pass to build the image index. Completed lookups are saved even if the rest of a scan is interrupted.

IGDB credentials live in the protected configuration file. Secret fields are redacted in browser responses; leaving an existing secret input blank retains it. Disable IGDB to stop online enrichment. Adjust cache lifetimes and request spacing through the [configuration reference](docs/configuration.md).

Videos already present in a game's `igdb.videos` metadata appear as embedded YouTube players after its screenshots. No extra lookup or API key is needed. Videos stream directly from YouTube and require internet access; they are not downloaded into application data. Closing the details dialog stops playback. A Watch on YouTube link is available when the uploader or browser prevents embedding.

## Configuration and environment variables

Runtime settings live in one `config.json`. Use Administration for ordinary edits. The [full configuration reference](docs/configuration.md) documents every JSON field and supported environment variable. [config.example.json](config.example.json) is a reference, not a required installation step.

**Environment variables seed missing JSON fields once.** Existing values win on later starts, including `false`, `0`, empty strings and empty arrays. Web edits persist across restarts. Copying an example JSON with empty fields makes those fields present, so environment variables will not replace them.

To configure a fresh Docker installation without the wizard, provide these environment entries in addition to the normal paths/listener:

```yaml
environment:
  PS4_DATA_DIR: /data
  PS4_LISTEN: :8080
  PS4_PUBLIC_URL: http://192.168.1.20:3000
  PS4_ADMIN_USERNAME: admin
  PS4_ADMIN_PASSWORD: ${PS4_ADMIN_PASSWORD:?Supply an administrator password}
  PS4_MAPPINGS: '[{"id":"games","path":"/packages","category":"games","enabled":true}]'
```

Supply the password through your deployment environment; it must contain at least 12 characters. The application converts it to a salted hash before saving configuration. Set these before the first launch with a fresh config directory. For an existing installation, edit settings in Administration instead. Keep deployment environment files private and out of Git.

Listener and data-folder changes need a server restart. Mapping, schedule and metadata changes apply to the next job. Only one writer may use a configuration/data directory at a time.

## More package folders and categories

A folder mapping has a stable ID, an absolute server path, a category and an enabled switch. Keep its ID stable for incremental reuse. Several folders can contribute to the same category. Overlapping mappings are deduplicated by canonical file path. Category chooses a feed; the package's own metadata determines game/update/DLC type.

For native use, choose paths on the computer running the binary. For Docker, add a host mount before adding its **container path** in Administration:

```yaml
volumes:
  - /volume1/Games/_PS4/games:/packages:ro
  - /volume2/PS4Extras:/extra-packages:ro
```

Recreate the service after changing Compose, then add `/extra-packages` in Administration. The web picker cannot mount another NAS folder by itself. Nested files are scanned recursively; directory symlinks are not followed, and file symlinks must stay inside the mapped folder. An unavailable mapping reports an error and retains its previous catalog entries.

## Docker ownership, updates and health

The [NAS Compose example](examples/compose.nas.yml) uses bind mounts and `user: "1026:100"`. Run `id` for your intended account and substitute its UID/GID. The account needs read/traverse access to package directories and write access to config/data. For a new install, create the two writable directories and give that account ownership using your NAS permissions UI or `chown`. Existing installations should preserve their ownership and ACLs. For a first install with the example IDs:

```sh
sudo mkdir -p /volume1/docker/dockers/ps4gamesserver/config /volume1/docker/dockers/ps4gamesserver/data
sudo chown 1026:100 /volume1/docker/dockers/ps4gamesserver/config /volume1/docker/dockers/ps4gamesserver/data
```

Use your actual account IDs, or set the equivalent access in the NAS permissions interface.

The repository's [development Compose file](compose.yaml) uses named volumes and UID/GID 10001 by default. If changing to `PUID=1000 PGID=1000`, prepare those volumes first, substituting your IDs:

```sh
docker compose run --rm --no-deps --user root --entrypoint chown ps4-server -R 1000:1000 /config /data
PUID=1000 PGID=1000 docker compose up -d
```

To update the NAS example, back up config/data, select the desired image tag in the YAML, then run:

```sh
docker compose -f ps4-games-server.yml pull
docker compose -f ps4-games-server.yml up -d
docker compose -f ps4-games-server.yml exec ps4-server ps4-server health -config /config/config.json
```

Use `sudo docker-compose` on Synology installations with the older command. The example's `0.1.0-dev` tag is a development build; prefer a versioned stable tag when a stable release is published. Ordinary development uploads do not update `latest`.

Stopping or recreating the container keeps bind-mounted data. For named volumes, `docker compose down` retains them; `down -v` deletes them.

## Covers, backgrounds and manual metadata

Custom PNG/JPEG covers belong at `data/covers/<category>/<TitleID>.png` (or `.jpg`/`.jpeg`). DLC first uses the full Content ID as its filename so different DLC packages can have different artwork. For entries without IDs, inspect the generated cover URL to find the deterministic name. Valid custom covers are retained. Missing or corrupt covers are repaired from readable package icons; a fallback is used when none is available. Encrypted package sections are reported without attempting decryption.

Set a custom PNG/JPEG background path in Administration's advanced connection options. This is a path visible to the server; mount the file in Docker if it is outside existing mounts.

Administration's **Advanced: metadata overrides** accepts JSON keyed by Title ID, for example:

```json
{
  "CUSA12345": {
    "summary": "Your description",
    "genres": ["Adventure"]
  }
}
```

Save and scan to apply it. Overrides are stored in `data/metadata-overrides.json` and merged into cached details.

## Feeds and separate download listeners

Use `/feeds/<category>` URLs from **Game feeds**. Legacy `/<category>` and `/<category>.json` aliases remain supported where they do not collide with application routes. Catalogs keep FPKGi's `{"DATA": {...}}` format, escaped nested paths and original field types. For FPKGi, add selected URLs to its `content_urls` object; see the [Windows example](WINDOWS.md#fpkgi-compatible-clients).

An optional second listener requires both `download_listen` and `download_url`; publish that port in Compose too. It serves only health, packages, covers and background. Library, feeds and administration stay on the main port. Always configure URLs reachable from the PS4, not wildcard bind addresses.

Feeds/downloads are public to devices that can reach the listener. Login protects administration. For remote access, arrange your network access and HTTPS separately; local home-network use does not require router port forwarding.

## Backup and recovery

Back up `config.json` and the entire data directory together. Packages remain in their own folder and are not copied into application data. Stop the server for a consistent filesystem backup. On restore, keep the same container paths or adjust settings, and retain access permissions.

If a cache JSON file is malformed, stop the server, move only the affected cache file aside, and scan again. Package details or online materials may need to be rebuilt. Keep your custom covers and overrides. Empty `.lock` sidecar files are normal; OS locks release when a process exits. Do not remove them while a writer is running.

To reset a forgotten password, stop the server and run:

```sh
./ps4-server password-reset -config /absolute/path/config.json
```

Enter the new password on standard input, then EOF (Ctrl+D on Unix; Ctrl+Z and Enter on Windows). A terminal may echo this input; use a protected input method where needed. Restart the server afterward. The [Windows guide](WINDOWS.md) covers service installation and recovery.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| PS4 cannot connect | Use the PC/NAS LAN address and published port. Check the firewall and guest Wi-Fi/device isolation. Test `/library` from another device. |
| Empty library | Confirm the folder contains PS4 `.pkg` files, its mapping is enabled, and a scan ran. In Docker, use `/packages`, not the host's `/volume1/...` path. |
| Folder unavailable | Check the mount and account permissions. Previous entries are retained; consult Server log. |
| Missing descriptions/screenshots | Enable IGDB and check its credentials. Not every title has a conservative match; native package scanning still works offline. |
| Repeated checks | Scheduled scans look for file changes. Read/reused counts and cache-only log messages distinguish checks from new parsing/fetching. |
| Setup link expired | During incomplete setup, use the new link printed after restarting. After setup, open `/admin` and sign in. |
| Settings need a restart | Listener and data-directory changes take effect on restart; other changes apply between jobs. |

For implementation coverage and external validation limits, see [validation results](docs/validation.md) and [acceptance](docs/acceptance.md). Live PS4 installation has not been validated by this server's test suite.
