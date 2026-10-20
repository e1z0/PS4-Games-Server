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

## Completion audit — 2026-10-20

The full supplied plan was checked against current source and behavior; docs/acceptance.md maps each requirement to implementation and verification. The baseline sibling remains at 88a969cf991303c36be64438a6752955c46b578d with a clean working tree. `go list -m all` reports only ps4server.

Final checks after all audit fixes:

- `make check` passed with **49 native test functions**, parser fuzz seeds, vet, formatting, full race tests and source-map checking. The actual CLI subprocess integration starts both listeners, checks public feeds/downloads and private routes, rejects a concurrent CLI writer without changing configuration, kills the child and proves the writer locks release. Additional tests cover schedule changes, failed/changing reads, invalid directory roots, retained alias deduplication, pending mapping downloads, retired catalogs, >4 GiB range offsets, write errors, native directory roots, explicit encrypted-entry reporting and malformed IGDB responses including trailing data.
- Under simultaneous container builds, an initial scheduler test exceeded its 5-second filesystem-completion deadline. The assertion remains unchanged; completion deadlines now allow 30 seconds for slow filesystem I/O. The full suite and race checks passed afterward. No failure was treated as a successful test.
- A bounded fuzz smoke (`go test ./internal/ps4 -fuzz '^FuzzParse$' -fuzztime=3s -parallel=2`) passed with 3 executions; this is not exhaustive fuzzing.
- Chromium again passed the six-step wizard, folder picker, Back-state preservation, login/first scan, details, search, category/type filters, a 390px mobile layout without overflow, admin edits and logout with no page errors. A second pass used the extracted release, checked clipboard content_urls, canonical feed links, lowercase/client-limit validation, login/admin settings and the embedded PNG fallback.
- The extracted Linux release passed serve/version/validate/health; one-shot scan reused metadata after the test server stopped. All six archives were rebuilt. Inspection verified exactly five files per archive (one executable, example JSON, GPL license, setup instructions and notices) and the correct ELF, Mach-O or PE CPU identifier for amd64/arm64.
- The Dockerfile built Linux amd64/arm64 into a verified OCI index using a container Buildx driver on the amd64 host, without QEMU. Both image configs specify user 10001:10001 and an executable health check. A final OCI archive is retained under ignored runtime/ for inspection.
- Final Compose smoke passed unattended startup, login/redaction, synthetic catalogs, range GET, HEAD, health, current embedded UI and persistent web edits over environment seeds. Package mounts were read-only. UID/GID 10001 worked initially; after stopping, applying the documented volume chown and recreating with PUID/PGID=20002, the service stayed healthy and retained its data/settings. The rebuilt image also passed at that override UID/GID.
- All 17 supported application environment seeds were checked against their corresponding JSON-field rows in configuration.md. Normal/stable upload recipes were dry-run reviewed: version tags target both Linux architectures, externally supplied Docker authentication is used, and latest is only added on the explicit stable branch. Nothing was pushed to a registry.

Test servers, the isolated Compose project/volumes and the temporary Buildx builder were removed. The synthetic package was moved out of the local default package directory into an ignored fixture backup. Executables, six native archives, test logs/screenshots and the OCI audit archive remain in ignored output directories. Hosted CI, native Windows/macOS/ARM execution, a live IGDB account and a real console remain unclaimed external follow-ups, rather than omitted local acceptance work.

## Direct startup follow-up — 2026-10-20

`make check` passed after removing the serve subcommand, including the real subprocess server integration, race tests and source-map check. Rebuilt `bin/ps4-server` and launched it with no arguments in an isolated directory; its HTTP health endpoint returned 200. Native/container startup instructions now use the default invocation. The Docker startup command was updated; no new container deployment was run for this change.

## Windows CI cleanup fix — 2026-10-20

The reported Windows go test failures were temporary-directory cleanup failures in three tests that did not close their App instances. Added checked application cleanup before TempDir cleanup and checked previously ignored constructor errors. `make check` passed on Linux, including the full race suite; the three affected tests passed ten consecutive runs. The Windows amd64 server test executable cross-compiled successfully. Windows execution of this fix remains for the hosted CI rerun.

## Windows guide and native service — 2026-10-20

- `make check` passed on Linux: formatting, vet, all tests, full race suite and source inventory. A new real-listener test cancels the service parent context, checks HTTP health and verifies both writer locks are released after shutdown.
- Windows amd64 vet and Windows amd64/arm64 test executable cross-compilation passed. Windows-specific handler tests cover readiness, interrogation, stop, shutdown and nonzero startup failure; these are compiled locally and run by Windows CI.
- All six release archives rebuilt. Both Windows ZIPs contain WINDOWS.md with the exact guide content (six files); other archives retain five files. The Linux Docker image rebuilt successfully with the pinned module/checksum files.
- Added an elevated Windows CI smoke script: no-argument EXE launch from a different working directory; real Local Service registration from paths containing spaces; two start/health/status/stop cycles; uninstall retaining configuration/data. This script has not been executed locally because this host is Linux and has no native Windows SCM. Its result must be checked on the hosted Windows runner before claiming service runtime validation.

### Hosted native verification

[GitHub Actions run 39341972657](https://github.com/e1z0/PS4-Games-Server/actions/runs/39341972657) for commit 66ac814 passed the Linux, macOS and Windows native jobs, including vet, normal tests, race tests and source-map checks. The Windows service smoke step passed: an actual no-argument EXE launch from a different working directory, registration from a path containing spaces under Local Service with automatic startup, two real start/health/status/stop cycles, and uninstall retaining configuration/data. This supersedes the pending Windows execution status above. The runner was not rebooted; actual boot-time behavior and real-console installation are not claimed.

## Docker Hub development publication — 2026-10-20

Published source commit c10a41a using `make upload VERSION=0.1.0-dev TAG=0.1.0-dev` to [nulldevil/ps4-games-server:0.1.0-dev](https://hub.docker.com/r/nulldevil/ps4-games-server/tags?name=0.1.0-dev). Registry index digest: `sha256:014e868f6ad1f097fd4a08391ac93efd449fb541eb487dabae6421e3d92038aa`. Registry inspection confirmed linux/amd64 and linux/arm64 manifests plus build attestations. The latest tag was not published.

Pulled the image from Docker Hub, checked its 0.1.0-dev version, and started the published image by digest on amd64. Binary health passed and runtime UID was 10001. The temporary container and anonymous config/data volumes were removed. The arm64 image was built and its manifest verified; ARM runtime execution remains unclaimed. Registry credentials were provided externally and were not printed or committed.

## Compact wizard and scan progress — 2026-10-20

Local make check passed after scanner/UI changes, including race tests. A blocked metadata fixture verifies that local catalogs and persisted cache are usable before enrichment returns, that completed metadata is added, and that cancelling enrichment retains local packages. Real mocked IGDB requests verify per-title progress and Title ID sharing. Public progress tests exclude server paths and raw private errors. Chromium passed six-step setup, inline password validation, invalid-directory feedback, folder selection, Back-state preservation, login/first scan, admin/public determinate metadata progress, library details and 390px layout without page errors. A second pass verified seeded IGDB credentials remain usable without re-entering a redacted secret. Screenshots were inspected locally. The existing media feed was independently observed publishing 111 packages (99 enriched) after its old invisible first scan finished. Deployment results follow below when verified.

Additional Chromium checks delayed library, save and directory responses: loading bars appeared during the wait, disappeared on completion/error, and a malformed 502 response produced actionable text. Individual feed URL Copy remained; full configuration copying was absent. Admin settings saved with a previously stored IGDB secret without exposing or requiring it again.

### Live NAS deployment and final verification

- Published source dfa89fd as nulldevil/ps4-games-server:0.1.0-dev for Linux amd64/arm64. Final registry index: sha256:088936b0f56a0c365d794800794cffa38c9bb0d68650ab4ebd2376762f970f71. Latest was not updated.
- Backed up configuration/data to /volume1/docker/dockers/ps4gamesserver/before-compact-ui-20261020.tar.gz (mode 0600). Kept existing credentials, IGDB settings, mappings, public URL, bind mounts and UID/GID 1026:100.
- Pulled and recreated the authorized media deployment from /volume1/docker/ps4-games-server.yml. Non-interactive SSH could not find make; used the exact sudo/env/docker-compose up command in the Makefile target. Final container is healthy with image config sha256:21b15330d8209596dd24ed1477478c45d8eeecca942378781b769f562e31d511.
- Initial restart exposed expensive screenshot decoding before listener startup. After adding cache-only metadata restoration, the HTTP progress endpoint was available while reading packages; its observed successful check took about 3 seconds. Full validation remained in the visible scan.
- Verified the live feed contains 111 packages, 109 populated min_fw values and 99 records with IGDB details. Real package Range bytes=0-3 returned HTTP 206 and the expected CNT magic.
- Chromium verified the actual NAS library has 111 cards, an animated backend-loading bar, individual feed URL Copy only, mobile layout and no page errors. The live metadata stage showed 22 of 98 titles processed with 111 packages already available and zero warnings. Routine encrypted CNT-section notices are retained in logs; actual metadata/icon issues still count as warnings.
- Final [GitHub Actions run 39439106980](https://github.com/e1z0/PS4-Games-Server/actions/runs/39439106980) passed all native OS tests/race checks, Windows service smoke and release/container builds. Six local release archives were rebuilt. Local synthetic UI servers were stopped.

## Clipboard copying on plain HTTP — 2026-10-20

Chromium tested the updated script on the real insecure media.efn.lt origin: the modern secure-context Clipboard API was unavailable, while the synchronous copy-command fallback successfully copied feed URLs and package URLs inside the existing details modal. Paste-back checks verified actual clipboard contents. Forced browser-policy failure selected the original feed URL in place and displayed a keyboard-copy hint, with no extra dialog or leftover hidden textarea. No browser page errors occurred. Browser clipboard API references: [writeText](https://developer.mozilla.org/en-US/docs/Web/API/Clipboard/writeText), [copy command](https://developer.mozilla.org/en-US/docs/Web/API/Document/execCommand).
