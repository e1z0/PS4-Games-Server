# Architecture and source guide

`cmd/ps4-server/main.go` is the single application entrypoint. Its CLI selects serving, one-shot scans, initialization/validation, password reset, health and version. Signal cancellation shuts down HTTP and scan jobs. `main_test.go` exercises command behavior.

| Package / location | Responsibility and tests |
| --- | --- |
| `internal/config` | Schema/version, defaults, presence-aware environment seeding, validation, redaction, PBKDF2 credentials. Tests preserve false/zero, persist web edits, validate seeds and remove plaintext. |
| `internal/storage` | Atomic temp/sync/rename writes, unchanged-file preservation, safe containment resolution and OS-owned writer locks (flock / LockFileEx). Storage tests cover permissions and concurrent atomic readers. |
| `internal/ps4` | Bounded ReaderAt CNT table and SFO parsing, firmware, type/region, image validation and plaintext extraction. Tests cover malformed/truncated/encrypted containers, homebrew, firmware and fuzzing. |
| `internal/testpkg` | Synthetic SFO/CNT fixture construction, including valid tiny PNGs. No game content. |
| `internal/scanner` | Single active job, cancellation/progress/logs, recursive mapping discovery, incremental stat fingerprints, icon repair, deterministic catalogs and persisted cache. Tests cover restart reuse, timestamp preservation, unavailable mappings, removal, overlap, Unicode and a sparse 5 GiB package. |
| `internal/metadata` | Injectable HTTP client, token renewal/rate limiting, PS4 matching, rich schema, positive/negative caches, overrides and screenshot repair. Tests use only httptest servers, including 401/429/offline failures. |
| `internal/server` | HTTP routing, sessions/CSRF/throttling, setup, settings persistence, protected directory browsing, downloads and scheduling. HTTP tests cover token lifecycle, auth, redaction, traversal, symlinks, range and HEAD. |
| `internal/server/ui` | Embedded HTML/CSS/vanilla JavaScript library, wizard and administration. No build step. All metadata text enters the DOM through textContent; URLs are restricted to HTTP(S). |
| `tools/source-map.go` | Complete tracked-source inventory generation and stale-file check. Includes CI/config/docs; excludes runtime/build outputs and .git. |
| `tools/release` | CGO-free six-platform build and archive generation using Go zip/tar/gzip. |

## HTTP routes

| Method / route | Access / purpose |
| --- | --- |
| GET `/`, `/library` | Public browser library |
| GET `/login` | Login form |
| GET `/setup` | Wizard shell; data and setup actions require token-derived setup session |
| GET `/admin` | Administrator session; otherwise redirects to login |
| GET `/app.js`, `/style.css`, `/fallback.svg` | Explicit embedded assets only |
| GET/HEAD `/health` | Public liveness; binary `health` command is container probe |
| GET `/api/v1/library`, `/api/v1/feeds` | Public catalogs / configured feed URLs |
| GET/HEAD `/<category>`, `/<category>.json`, `/feeds/<category>` | Public FPKGi feeds and legacy aliases |
| GET/HEAD `/pkg/<mapping-id>/<escaped-relative-path>` | Public package; must appear in snapshot and match an enabled configured root |
| GET/HEAD `/covers/<relative-image-path>`, `/background` | Public validated image files; built-in fallback at `/covers/fallback.png` |
| POST `/api/v1/setup/open` | Secret setup token → setup session and CSRF token |
| PUT `/api/v1/setup` | Setup session + CSRF; validates/saves then invalidates setup token and sessions |
| POST `/api/v1/auth/login` | Throttled login → HttpOnly SameSite session + CSRF token |
| GET `/api/v1/auth/session` | Session bootstrap; includes redacted settings |
| POST `/api/v1/auth/logout` | Admin session + CSRF; invalidates session |
| GET/PUT `/api/v1/settings` | Admin; writes require CSRF, secrets redacted/retained when blank |
| GET `/api/v1/directories?path=...` | Admin or setup session; absolute server directory browser, including native Windows drive roots |
| GET `/api/v1/status`, `/api/v1/logs` | Admin; active job, counts, restart notice and last 200 log events |
| POST `/api/v1/scan`, `/api/v1/scan/cancel` | Admin + CSRF; force flag or cancellation |
| GET/PUT `/api/v1/metadata/overrides` | Admin; writes require CSRF, keyed JSON objects; next scan applies |

The optional download listener exposes only `/health`, `/pkg/`, `/covers/` and `/background`. Static routes never serve a configuration or arbitrary data directory. Unknown APIs/methods fail closed. Write requests require same-origin when Origin is supplied, JSON request bodies and a session CSRF header. Login/setup-open use JSON and origin checks. Sessions last 12 hours; failed login IPs are throttled for 15 minutes after five attempts. Proxy headers are not trusted; public HTTPS origins set Secure cookies.

## Data flows

Startup loads JSON, seeds missing fields from validated environment, hashes plaintext passwords and atomically saves. Fully configured deployments skip setup. Otherwise a random in-memory setup token is logged as a URL fragment; successful setup invalidates it. Configuration edits are validated and persisted before becoming active. Running scans capture their configuration at job start. Listener and data-directory changes are persisted but retain current runtime values until restart.

Scanning loads relative-path cache entries, sorts mapping IDs, walks recursively and deduplicates canonical paths. A stat fingerprint determines reuse. Parsing reads only bounded headers/metadata/icon entries; no full-package hashing. A second stat rejects files that changed while inspecting. Missing or corrupt images are repaired; valid custom images are retained. Unavailable mapping walks retain old entries and emit errors. Completion publishes deterministic catalogs and cache with atomic per-file replacements, then swaps the in-memory snapshot. Cancellation before publication retains the prior snapshot. Files may already have had covers repaired; publication is atomic per file, not a multi-file filesystem transaction.

Metadata selects PS4 base games for matching, shares results by Title ID across updates/DLC, and caches successful matches and definitive misses separately. Network failures retain expired positive records and do not write negative matches. Overrides merge recursively. Screenshot URL rewriting uses independent clones to keep source cache URLs portable across public-address edits. Cached rich metadata is restored even if startup scanning is disabled.

Catalog URLs use explicitly configured origins and escaped nested relative segments. Requests whitelist package snapshot entries, check containment, then use Go os.OpenRoot to prevent symlink escapes during open. HTTP ServeContent provides efficient streaming, HEAD, conditional dates and ranges without loading packages into memory. Download paths are never inferred from arbitrary user filesystem input.

See SOURCE_MAP.md for the generated file inventory, docs/validation.md for actual check results and docs/parity.md for compatibility details.

`tools/assets` regenerates the embedded fallback PNG using only Go. Feed fallback URLs use PNG so native PS4 raster decoders can load them; the browser can also use the SVG fallback. Embedded assets use ServeContent for HEAD and ranges.

## Lifecycle and portability audit

The CLI acquires the configuration lock before loading/seeding mutable commands. The HTTP app locks its data directory before restoring caches; one-shot scan acquires the same data lock. Setup transfers this lock when selecting a new directory. Tests exercise a second process and abrupt process exit to verify lock exclusion and release. Close cancels jobs before releasing the data lock.

The HTTP download whitelist uses published snapshot roots. Pending mapping edits cannot invalidate in-flight/current-catalog downloads; a completed scan publishes the new roots. On restart, scanner.New hides disabled/repointed cached roots and retains their records privately only for the next disk-catalog cleanup/removal count. Unavailable mappings keep canonical hints for deduplication across aliases. Failed parses are followed by a stat check too, so packages changing during a failed read retain their previous record. Invalid directory roots and write failures report errors without silently publishing an empty/replacement snapshot.

Feed/library catalogs cache their immutable published view per configuration/revision. Image validation stays outside the job mutex and does not repeat for each browser/feed request. Public-URL edits and completed scans invalidate this view. Numbered IGDB sequels are distinguished, repeated words cannot inflate match scores, edition suffixes can match the base title, and malformed/trailing API responses cannot become definitive misses.

The Docker runtime stage executes no target-CPU commands: a native build stage cross-compiles Go, and a native files stage prepares certificates/accounts/directories for both target images. CI uses [Docker's Buildx setup action](https://github.com/docker/setup-buildx-action) and exports a Linux amd64/arm64 OCI image archive without QEMU. LF checkout attributes keep Go golden tests and source-map checks portable; the inventory checker also tolerates CRLF on its existing Markdown input. A Go subprocess integration test covers the real CLI, two actual listeners, public feeds/downloads, private APIs and cross-process exclusion on each native CI OS.
