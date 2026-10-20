# Progress

Baseline: sibling FPKGi-node-server, commit 88a969cf991303c36be64438a6752955c46b578d. Its 15 Linux tests passed on 2026-10-19. The original repository is unchanged.

- Foundation: independent repository, GPL-2.0-only, versioned JSON, presence-aware environment seeding, atomic writes and salted password hashing implemented.
- Parser/scanner: native bounded CNT/SFO parsing, firmware fallback, homebrew, encrypted warnings, stable mapping IDs, recursive discovery, canonical deduplication, relative-path incremental cache, cover repair, unavailable-mapping retention, cancellation and deterministic catalogs implemented. Synthetic tests include a sparse 5 GiB package, restart reuse, escaped Unicode paths, removal and overlap rejection.
- Metadata/HTTP: optional IGDB enrichment with conservative PS4 matching, Title-ID sharing, token renewal, throttling, positive/negative caches, offline retention, rich schema, overrides and local screenshot repair. Mocked HTTP tests pass. Public catalog aliases, embedded library, authenticated setup/admin, sessions, CSRF, login throttling, directory browsing, range/HEAD serving and containment tests pass.
- Web/CLI milestone: embedded responsive library, six-step setup, directory picker, metadata dialogs, copyable category feeds and FPKGi configuration, admin mapping/settings/override controls and restart notices implemented. Running the binary starts the server; CLI commands support scan, init, validate, password-reset, health and version. Native tests and vet pass; six native release archives have cross-compiled successfully.
- Final validation: native checks including race tests pass; Chromium smoke passed setup/login/directory selection/first scan/library/search/dialogs/mobile/settings/logout. Docker image and Compose deployment passed non-root health, read-only package mount, authenticated settings, catalog/range/HEAD and restart persistence checks. Six archives built. Added Windows replacement retries/delete-sharing reads, complete image validation and cross-category cover sharing during final review. External console, hosted OS matrix, live IGDB and registry publishing remain explicitly unclaimed.
- Final result: 25 native tests plus parser fuzz seeds, full race suite, vet/format/source-map checks, rebuilt release browser checks and final Compose image smoke pass. Test processes and isolated Compose resources were cleaned up; executable and all six archives remain available locally. The independent repository contains five coherent milestone commits.

## Follow-up completion audit — 2026-10-20

The previous implementation turn made concrete progress, but completion was re-audited against the full supplied plan. Regression fixes now cover mapping publication/restart behavior, failed reads, invalid roots, retained overlap deduplication, setup listener notices, native directory selection and cross-process exclusion. Added an actual CLI subprocess/two-listener integration test, >4 GiB tail range test, schedule test, cached catalog refresh, stricter IGDB matching and negative-cache expiry/outage checks. Both Linux image platforms built into a verified non-root OCI index without emulation. Chromium passed the setup/library/admin flow with wizard Back-state preservation. Final rebuild/race/browser/container checks are being recorded in validation.md; external verification remains explicitly separate.

The completion audit is finished. All local acceptance gates passed with 49 native tests and race checks, browser wizard/library/admin and client-config checks, actual CLI/two-listener/cross-process tests, six inspected native archives, both Docker architectures without QEMU, and Compose persistence/health/downloads at default and overridden UID/GID. Source-map and environment-document coverage checks passed. Test resources were cleaned up. At that audit the implementation was stdlib-only and the original Node repository is untouched. External hardware/account actions are explicitly separated in TODO.md.

## Direct server startup — 2026-10-20

Removed the serve subcommand. Running the binary directly starts the server, with optional -config. Docker, make run, setup instructions and the real subprocess integration test use this invocation. Administrative utility commands remain available.

Verification: make check passed; the rebuilt executable started with no arguments and returned HTTP 200 from /health in an isolated smoke test.

## Windows test cleanup — 2026-10-20

The three server tests reported by Windows CI omitted App.Close, leaking their data writer locks into temporary-directory cleanup. They now check construction errors and register cleanup that closes the application and reports close errors. All other successful App.New calls in the server tests were reviewed for cleanup.

Verification: make check passed, the affected tests passed ten consecutive runs, and the Windows amd64 server test binary cross-compiled. A native Windows CI rerun is still required to confirm the reported cleanup errors are resolved on that runner.

## Windows beginner guide and service — 2026-10-20

Added WINDOWS.md with an EXE-only setup walkthrough, complete package folder/feed example, firewall instructions, permissions, troubleshooting and optional Windows service setup. Windows no-argument launches use the executable folder. Added native SCM install/start/stop/status/uninstall, delayed automatic startup under Local Service, captured absolute paths/working directory, cancellable server shutdown and file logging. x/sys v0.30.0 is the sole additional Go module and is linked into the Windows EXE. Added Windows handler tests and a hosted service lifecycle smoke step; external Windows runtime results remain unclaimed until CI executes.

Validation: Linux make check and Docker image build passed; six archives rebuilt and Windows guide inclusion verified; Windows amd64 vet and amd64/arm64 test cross-compilation passed. Hosted native Windows service execution remains pending.

Hosted verification: run [39341972657](https://github.com/e1z0/PS4-Games-Server/actions/runs/39341972657) passed all three native OS jobs. Windows passed the real Local Service install/start/health/status/stop/restart/uninstall smoke, including no-argument launch from a different working directory and paths containing spaces. Native Windows service execution is now verified on the hosted runner.

## Docker Hub publication — 2026-10-20

Published nulldevil/ps4-games-server:0.1.0-dev from c10a41a for Linux amd64/arm64, verified both registry manifests, and pulled/smoke-tested the published amd64 image with healthy startup as UID 10001. Latest was unchanged. Digest and cleanup evidence are recorded in docs/validation.md.

## Compact setup and visible scanning — 2026-10-20

Inspected the authorized media deployment over SSH. Existing configuration and mounts were valid; the initial feed stayed empty until a long IGDB pass finished. The existing deployment eventually published 111 packages with IGDB details for 99. Reworked the UI with compact controls and a light neutral theme, a six-step guided sidebar, progressive disclosure, actionable directory/API errors, and visible stage/count progress on public library/admin. Local parsing now publishes catalogs before optional metadata work. Added regression coverage for early publication, enrichment cancellation, real metadata progress callbacks and public progress redaction.

User follow-ups: removed the full FPKGi configuration-copy action and unused helper; individual category URL copying remains. Added a separate request-loading indicator and initial page placeholder for slow NAS responses. Folder dialogs show a local loading bar, while routine polling stays quiet. Browser delay tests verified page, folder and save loading states and cleanup after a 502 response. NAS configuration/data were backed up before deployment.

Deployment exposed a separate CPU bottleneck: cached screenshot decoding blocked listener startup on the NAS. Startup now restores cached metadata and image URLs using lightweight existence checks; full image validation remains in visible scans and image serving. Added a regression proving startup makes no network requests and defers decoding, while normal scans still reject damaged image files.

Live-cache inspection confirmed all 111 displayed scan warnings were routine additional encrypted CNT sections, not parse failures. Those notices remain in logs and cached parser details, while the attention count now includes only actionable warnings.

Delivered to the authorized media NAS and Docker Hub from dfa89fd. Configuration/data backup retained, container healthy at UID/GID 1026:100, all 111 packages preserved, 109 min_fw values and 99 rich metadata records verified, and real range download passed. Live Chromium confirmed animated request loading, public metadata progress (22/98 while packages were available), zero scan warnings, individual feed Copy only and mobile layout. Hosted run 39439106980 passed all jobs. Registry digest and full verification are in docs/validation.md.

## Clipboard behavior on LAN HTTP — 2026-10-20

The Copy button previously relied only on the secure-context Clipboard API, so plain HTTP opened a manual-copy dialog. Added a synchronous clipboard-command fallback, preserving focus and supporting package details inside a modal. If both browser methods are blocked, select the URL in place with a keyboard-copy hint; no extra dialog is opened and success is only reported after copying succeeds.

Clipboard fix deployed to media and Docker Hub from 2db09ee. Live Chromium paste-back verified feed and package copying on the insecure HTTP origin, with no extra dialogs. Browser-blocked copying selected the URL in place. Health, native CI and release/container builds passed.

## Feed placement — 2026-10-20

Renamed the shared feed section to Game feeds and moved library feed URLs above search/category/type filters and game cards. Kept individual Copy controls and compact spacing.

Feed placement deployed to media and Docker Hub from 65605c3. Live browser checks confirmed Game feeds above filters and all 111 games on desktop/mobile, with URL Copy retained, no overflow/page errors and healthy backend.

## Unified administration log — 2026-10-20

Replaced the separate warning list and collapsed recent-message log with one visible Server log directly below scan controls. Server warnings already flow through the shared timestamped log, so messages are shown once. Changed log content scrolls to its last line; unchanged polls preserve the current view and selection.

Unified log deployed to Docker Hub and media from eddf968. Browser checks against deployed assets (mocked authenticated API), backend tests, release builds and NAS binary health passed.

## Reusing completed scan and metadata work — 2026-10-20

NAS inspection found fresh metadata entries (30-day positive / 7-day negative caches) but every five-minute scan decoded every screenshot again and presented all title IDs as fetching work. Added persistent relative-path image validation fingerprints (size, modification time, platform change time), retained full validation and repair for new/changed files, and count only due lookups or screenshot repairs in metadata progress. Existing caches warm their validation index once. Successful lookups checkpoint immediately; cancellation also saves completed image validation. Unchanged packages retain parser reuse without repeating encrypted-section notices. Logs report read/reused counts and cache-only metadata completion.

Deployed becc0b2 to Docker Hub and media. Live initial indexing saved 703 validated screenshots; restart reused the persisted index, with zero metadata/repair work and zero warnings. Startup scan completion improved from 107 seconds for index creation to 22 seconds after restart (includes native file checks/catalog work). All 111 packages, firmware fields and metadata records were preserved; native builds/checks and deployed UI checks passed.

## Illustrated consumer documentation — 2026-10-20

Reworked README around PS4 Games Installer and browser browsing, features, native startup, a NAS Compose setup and the production folder layout. Added EXPERT.md for configuration/IGDB/mappings/maintenance and DEVELOPER.md for builds/checks/releases. Simplified the archive setup handout, updated Windows connection instructions, corrected the obsolete configuration-copy reference and retained the complete field reference. The password-free examples/compose.nas.yml follows the authorized NAS bind mounts, UID/GID example and port 3000, with a generic LAN address. Captured the live populated production library and game details, plus a separate authenticated local admin example with 111 synthetic packages and empty secret fields. Images were visually reviewed; capture sessions logged out.

Documentation validation passed: reviewed three secret-free browser captures, verified Markdown links and README/example agreement, and smoke-tested fresh Compose setup, scanning, downloads, health and persistent restart with local test mounts/ports/IDs. Removed the temporary capture containers. Production configuration and packages were unchanged.

## Game feed wording — 2026-10-20

Changed the shared library/admin feed guidance to “Copy the feed URL you want to use in PS4 Games Installer.”

Published 7248b87 to Docker Hub and deployed it to media. Exact wording checked in live Library and the authenticated local example admin. Refreshed the README captures without credentials; health passed and the NAS scan retained 111 packages with zero warnings.

## Project footer — 2026-10-20

Updated the shared page footer with server/installer GitHub links, v0.1.0, © 2026 e1z0 and linked GPLv2 license terms.

Published 14b1ec4 to Docker Hub and deployed it to media. Exact footer text, server/installer/license links and desktop/mobile wrapping passed live browser checks; binary health passed. Refreshed the credential-free admin screenshot and rebuilt six native archives.

## Embedded game videos — 2026-10-20

Added a Videos section after screenshots using existing igdb.videos fields, including direct IDs and supported YouTube URL forms. Valid IDs are deduplicated and only the fixed youtube-nocookie.com embed host is allowed. Players load lazily, provide fullscreen and direct YouTube links, and are removed when the dialog closes (including Escape). Per-frame origin-only referrers support YouTube identification while keeping the application's no-referrer default. Updated README/EXPERT descriptions.

Full Go checks and browser regression checks passed. Real YouTube playback passed after reserving screenshot layout space to avoid slow image loads shifting the player. Browser coverage includes source filtering, lazy loading/referrer/CSP, phone layout and dialog-close cleanup.
