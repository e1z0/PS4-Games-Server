# Feature parity and acceptance checklist

Baseline: FPKGi-node-server commit 88a969c. Its 15 Linux tests passed before implementation; it was not modified. The Go implementation uses synthetic CNT/SFO fixtures and mocked HTTP; no game packages are shipped.

| Requirement | Status / evidence |
| --- | --- |
| Native bounded CNT/SFO, IDs/titles/version/region/firmware | Implemented; parser tests and fuzz corpus; firmware fallback and unknown SFO types covered |
| Homebrew without CUSA; reject other container magic | Implemented; native parser tests |
| Separate feed category and package type; DLC identities | Implemented; scanner/parser tests and content-ID cover keys |
| Valid custom covers, plaintext extraction, repair/fallback/background | Implemented; scanner repair tests, image validation, explicit background route |
| Encrypted metadata/icons | Reported; never decrypted; parser tests |
| Recursive stable mapping IDs, overlap deduplication | Implemented; sorted canonical paths; scanner tests |
| Relative incremental metadata/icon offsets; no game hashing | Implemented; stat size/mtime/platform timestamps; restart reuse tests |
| Startup/scheduled/forced/cancellable scans and overlap prevention | Implemented; single-job engine and scheduler; cancellation/overlap tests |
| Removal and unavailable-mapping retention | Implemented; scanner tests |
| FPKGi DATA and original primitive field types | Implemented; golden catalog test; decimal-string sizes, nullable identifiers/firmware/cover |
| Nested escaped URL paths; deterministic unchanged files | Implemented; Unicode/path/timestamp tests |
| Optional IGDB rich metadata, sharing, caches, overrides/screenshots | Implemented; entirely mocked HTTP tests including renewal, 429, expiry/offline and screenshot repair |
| Library/feeds/downloads, legacy category aliases, HEAD/ranges | Implemented; HTTP tests |
| One default port, optional dedicated downloads | Implemented; restricted second handler |
| Explicit PS4-reachable public URL | Required; rejects wildcard origins |
| JSON config, environment missing-field seeds, hashed credentials | Implemented; presence/validation/persistence/permissions tests |
| Setup token lifecycle, wizard and unattended startup | Implemented; server/CLI tests |
| Protected directory browser; Docker/native distinction | Implemented; server tests and wizard help |
| Library search/filter/dialogs/copy feeds and client snippet | Implemented in embedded vanilla JavaScript |
| Admin settings/mappings/status/logs/scans/overrides/restart notice | Implemented; API tests, responsive UI |
| Session auth, CSRF, logout, throttling; public PS4 routes | Implemented; security tests |
| No arbitrary config/cache exposure, traversal/symlink containment | Implemented; route whitelist and os.OpenRoot; HTTP tests |
| One executable, embedded web, Windows SCM service | Implemented; CGO-free release targets; x/sys compiled into Windows EXE |
| CLI setup/validate/reset/health/version | Implemented; CLI tests and smoke validation |
| GPLv2, notices, docs and generated source inventory | Included; source-map check |
| Linux/macOS/Windows amd64/arm64 archives | Release tooling builds six archives; see actual results |
| Multi-stage non-root Docker, persistence, read-only packages, health | Included; see actual Compose smoke results |
| Cross-platform test CI and release compilation | CI configured; hosted CI results are not claimed |
| Live PS4 acceptance | Outstanding: requires a console |

Portable release dates use file modification date, not Node's platform-dependent birthtime. Directory symlinks within mappings are not traversed; contained file symlinks are supported. The native web UI replaces Electron/terminal-menu desktop workflows. No Node configuration importer is provided, as requested. Current validation limitations are listed in TODO.md.
