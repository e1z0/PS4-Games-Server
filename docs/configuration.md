# Configuration reference

One versioned JSON file stores all runtime configuration. Null values and unknown configuration fields are rejected. Use `-config /absolute/path/config.json` on every CLI command to select it. Generated application data is separate. `config.example.json` is a readable starting point; starting without a file allows all environment seeds. Copying an example with empty fields makes those fields present and therefore prevents their environment seeds from replacing them.

| JSON field | Default / meaning | Environment seed |
| --- | --- | --- |
| `version` | `1`, required supported schema | none |
| `configured` | false; automatically true when credentials and public URL are valid | none |
| `listen` | `:8080`, main host:port | `PS4_LISTEN` |
| `download_listen` | empty; optional separate download host:port | `PS4_DOWNLOAD_LISTEN` |
| `public_url` | empty; explicit PS4-reachable HTTP(S) origin | `PS4_PUBLIC_URL` |
| `download_url` | empty; defaults to public_url | `PS4_DOWNLOAD_URL` |
| `data_dir` | `data`, writable generated data directory | `PS4_DATA_DIR` |
| `background` | empty; server-native PNG/JPEG file | `PS4_BACKGROUND` |
| `mappings` | `[]`, array of directory mappings | `PS4_MAPPINGS` (JSON array) |
| `scan.startup` | true | `PS4_SCAN_STARTUP` |
| `scan.interval_minutes` | 0; periodic interval, zero disables | `PS4_SCAN_INTERVAL_MINUTES` |
| `admin.username` | empty | `PS4_ADMIN_USERNAME` |
| `admin.password` | write-only plaintext input, 12–1024 bytes; removed before persistence | `PS4_ADMIN_PASSWORD` |
| `admin.password_hash` | salted PBKDF2-SHA256, 210000 iterations | none |
| `igdb.enabled` | false | `PS4_IGDB_ENABLED` |
| `igdb.client_id` | empty | `PS4_IGDB_CLIENT_ID` |
| `igdb.client_secret` | empty; redacted from API | `PS4_IGDB_CLIENT_SECRET` |
| `igdb.positive_days` | 30 | `PS4_IGDB_POSITIVE_DAYS` |
| `igdb.negative_days` | 7 | `PS4_IGDB_NEGATIVE_DAYS` |
| `igdb.interval_ms` | 300, request spacing | `PS4_IGDB_INTERVAL_MS` |

These are all supported application environment variables. Docker/Make additionally use `PACKAGES_PATH`, `PORT`, `BIND_ADDRESS`, `PUID`, `PGID`, `IMAGE`, `TAG`, `VERSION`, `BUILDER` and `STABLE`; those configure deployment/build tools, not JSON fields. Compose forwards the common seeds explicitly; add any other supported `PS4_*` variable to its `environment` section to use it inside the container.

Environment values **only seed absent JSON keys**. False, zero, empty strings and empty arrays count as present. Seeds are persisted immediately on server startup, `scan` and `init`. Later web edits survive restarts and win over seeds. Explicit seeds are validated even when an existing key wins. Booleans use Go boolean syntax (prefer `true`/`false`); intervals are nonnegative integers. URLs must be HTTP(S) origins without credentials, paths, queries or fragments, and cannot be wildcard hosts. Use externally supplied secrets; the application does not log them. A persisted password hash prevents password environment reseeding. An unconfigured launch prints a fresh random token; fully seeded credentials and public URL skip setup.

Each mapping has `id`, `path`, `category`, `enabled`. Paths must be absolute. IDs must be unique; IDs/categories match `[A-Za-z0-9][A-Za-z0-9_-]{0,79}`. Category describes the feed; SFO package category independently determines game/update/DLC type. Unknown nonempty SFO categories remain unknown; folder names are only a fallback when the SFO category is absent. Overlaps are resolved in sorted mapping ID order by canonical path. Keep mapping IDs and paths stable for cache reuse. Files are fingerprinted by mapping-relative path, size, modification time and available stat change timestamps. File content is never fully hashed. Package file byte size is a decimal string in feeds. Release uses file modification date in `MM-DD-YYYY` format because portable Go file creation dates are unavailable.

Directories beneath data_dir:

- `catalogs/<category>.json`: deterministic FPKGi `{"DATA":{...}}` feeds; only rewritten when content changes.
- `covers/<category>/...`: extracted/custom PNG/JPEG covers.
- `covers/igdb/screenshots/<TitleID>/...`: local screenshots, repaired when invalid.
- `scan-cache.json`: versioned metadata and icon offsets, without package data.
- `metadata-cache.json`: positive/negative IGDB results and expiration.
- `metadata-overrides.json`: objects keyed by Title ID, merged recursively into rich metadata.

Atomic temporary-file replacement protects persistent writes and preserves unchanged timestamps. Configuration is mode 0600 and generated directories 0700 on systems honoring Unix permissions. Use filesystem ACLs on Windows. IGDB secrets must remain usable for API requests and are stored in the protected configuration file. No internal JSON file is exposed by static routing.

CLI: run `ps4-server` to start the server; `scan [-force]`; `init` (refuses overwrite); `validate` (read-only); `password-reset` (stdin); `health [-url http://host:port]`; `version`; `help`. All accept `-config`. Health defaults to the main listener's loopback address when it binds all interfaces.

The server holds OS-backed writer locks in `<config-file>.lock` and `<data_dir>/.writer.lock`. A concurrent server, scan or password-reset process fails before changing the active configuration/data. Locks are released by the OS after exit, including crashes; their empty sidecar files remain and must not be removed while a writer is running. Setup transfers the data lock if a different writable data directory is selected. Read-only validate/health commands remain available.

Live mapping edits publish with the next completed scan, preserving existing downloads while a job finishes. On restart, cached entries whose mapping is disabled or points elsewhere are hidden immediately, and the next scan clears their retired disk catalogs. The incremental key remains mapping ID plus relative path; a canonical path hint only deduplicates retained entries when an overlapping mapping becomes unavailable.

Feed links copied from the UI use `/feeds/<category>`, so names such as `admin` do not conflict with application pages. Existing `/<category>` and `/<category>.json` aliases remain available where they do not collide with a page route. Images are limited to 32 MiB and 16 megapixels; screenshots use a 25 MiB download limit. Request spacing and cache/schedule intervals must fit a Go duration; configuration rejects integer values that would overflow.

The FPKGi configuration clipboard helper lowercases client category labels and checks the client's limit of 32 sources including games, 32 characters per label, distinct labels ignoring case, and the reserved ps5 label. Feed names remain unchanged. If a collection exceeds those client limits, individual feed URLs can be copied into selected, shorter client category labels.
