# Changelog

## 0.1.0 (unreleased)

- Keep routine encrypted payload-section notices in scan logs instead of counting them as failures/warnings; unreadable metadata/icon warnings remain visible.

- Restore cached metadata with lightweight image existence checks at startup; defer full screenshot decoding and repair to visible scans so slow NAS storage does not block the HTTP listener.

- Show an indeterminate loading bar during page loads, directory browsing and interactive backend requests; keep routine polling quiet.
- Remove the full-configuration copy button; keep individual category feed URL copying.
- Replace oversized dark UI with a compact neutral theme, guided wizard, hidden advanced settings, clearer folder help and actionable errors.
- Show live package and IGDB progress bars on library/admin pages, with stage/count/current-title information and readable warning summaries.
- Publish local package catalogs before IGDB enrichment so initial feeds become usable while optional game details are still loading; retain published packages when enrichment is cancelled.

- Add an EXE-only Windows walkthrough and native Windows service install/start/stop/status/uninstall, limited Local Service account, graceful shutdown and service.log.
- Keep Windows double-click configuration/data beside the EXE and include WINDOWS.md in Windows archives; add real service lifecycle CI coverage.

- Close application instances in three server tests before temporary-directory cleanup, fixing Windows CI writer-lock removal failures.

- Start the server by running the binary directly; remove the serve subcommand and update native/container startup instructions.

- Native Go rewrite with embedded assets and JSON configuration; environment values seed missing fields.
- Added native PS4 parsing, incremental scans, deterministic catalogs, plaintext icon repair and cancellation.
- Added IGDB caches and rich metadata, local screenshots, offline operation and manual overrides.
- Added embedded library, token-gated setup, authenticated administration and range-capable downloads.
- Added copyable FPKGi client configuration, persistent settings, offline metadata restore and CLI lifecycle commands.
- Added six-platform archive creation, Docker/Compose delivery and native CI checks.
- Hardened Windows atomic replacement, repaired truncated images, restored cross-category cover sharing and rejected malformed IGDB responses without negative caching.
- Verified browser setup/admin/library, non-root Compose deployment, restart persistence, HTTP range/HEAD and six release cross-builds.
- Serve an embedded PNG fallback to PS4 clients, alongside the browser SVG fallback.

### Completion audit

- Preserve published downloads across pending mapping edits and hide disabled/repointed cached roots at restart; clean retired disk catalogs on the next scan.
- Retain entries for invalid directory roots, overlapping unavailable aliases and packages changing during failed reads; report scan write failures.
- Enforce cross-process configuration/data writer locks with automatic crash release and setup lock transfer.
- Correct setup listener restart notices, native Windows browsing, wizard Back state, canonical feed links and strict JSON/origin/duration validation.
- Cache immutable feed/library views, distinguish numbered IGDB sequels and repeated words, and report additional encrypted CNT entries without decryption.
- Build both Docker architectures without QEMU and export the OCI index in CI; keep checkout line endings portable.
