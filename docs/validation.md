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
