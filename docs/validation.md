# Validation record — 2026-10-19

Environment: Linux amd64, Go 1.26.2, Docker Engine 29.2.1, Docker Buildx 0.31.1. Baseline sibling repository at 88a969c: all 15 original Linux tests passed, repository unchanged.

Completed so far:

- Native Go tests and go vet passed across all packages.
- Synthetic fixtures cover PS4 metadata, firmware fallback, homebrew, malformed/truncated/oversized/encrypted entries, PNG repair, unknown categories and distinct DLC keys.
- Scanner tests cover restart reuse, unchanged JSON timestamps, nested Unicode escaping, addition/removal, unavailable mappings, overlap prevention, cancellation, changing files and a sparse 5 GiB package.
- HTTP tests cover setup lifecycle, login, sessions/logout, CSRF, throttling, redaction, edits/restart notices, feeds/legacy aliases, HEAD/ranges, symlink/traversal containment and internal-file exclusions.
- IGDB tests use mocked HTTP only: conservative matching, cache reuse/expiry, manual overrides, token renewal, HTTP 429 retry, screenshot repair and offline retention without negative caching.
- CGO-free archives built for Linux/macOS/Windows on amd64 and arm64. Archive sizes are approximately 2.8–3.1 MiB. The Linux binary is statically linked.

Final race, browser and container smoke results will be appended after execution. No live PS4 testing or hosted cross-platform CI run is claimed.

Final local checks:

- `make check`: formatting, `go vet ./...`, `go test ./...`, `go test -race ./...` and generated source-map check passed. The regression suite also verifies truncated PNG repair, valid custom-cover retention and sharing, malformed IGDB responses without negative caching, and restoration of offline rich metadata when startup scanning is disabled.
- Chromium 145 / Playwright 1.58.2 smoke: completed the six-step wizard, selected a directory, saved setup, logged in and started the first scan, opened package details, searched, checked 390px mobile layout without horizontal overflow, saved an admin scan interval and logged out. No JavaScript page errors. The browser tooling was installed in a temporary directory outside the repository; it is not a build or runtime dependency.
- Built executable smoke: serve, health, validate and one-shot incremental scan passed; the scan reused its existing package metadata. The unconfigured setup token and authenticated persistence flow were exercised through the browser.
- `make docker` and `make deploy` with an isolated Compose project: image built, service became healthy, UID/GID were 10001, configuration/data volumes were writable and the package bind mount was read-only. Fully seeded unattended startup, login, secret redaction, synthetic catalog, ranged GET and HEAD passed. After restart, the scanner reused metadata and retained a web-edited interval of 19 minutes despite an environment seed of 0. Image size is approximately 16 MiB.
- Six release archives were cross-compiled and include one executable, example configuration, GPL license, third-party notices and standalone setup instructions. Extracted native Linux executable passed version, validate, serve, health and restart checks and was confirmed statically linked. Feed fallbacks were checked as valid PNGs for PS4 raster decoders. macOS/Windows/arm64 runtime execution remains external validation.

Smoke tests used only a synthetic tiny PS4-shaped package. Temporary test servers/containers were stopped after checking. Registry uploads, live IGDB requests with credentials, hosted CI and console installation were not performed. See TODO.md for the remaining external checks and operational limits.

Final suite count: 25 native test functions plus the parser fuzz seed corpus. A second browser pass checked the rebuilt release, clipboard-generated FPKGi `content_urls`, persisted native settings over changed environment seeds, and the embedded raster fallback. The final Compose image passed the same download checks after rebuild. Test containers, network and test-only volumes were removed; test servers were stopped. Build artifacts remain under ignored `bin/` and `dist/`.
