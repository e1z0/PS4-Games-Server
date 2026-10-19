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
