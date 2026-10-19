# Progress

Baseline: sibling FPKGi-node-server, commit 88a969cf991303c36be64438a6752955c46b578d. Its 15 Linux tests passed on 2026-10-19. The original repository is unchanged.

- Foundation: independent repository, GPL-2.0-only, versioned JSON, presence-aware environment seeding, atomic writes and salted password hashing implemented.
- Parser/scanner: native bounded CNT/SFO parsing, firmware fallback, homebrew, encrypted warnings, stable mapping IDs, recursive discovery, canonical deduplication, relative-path incremental cache, cover repair, unavailable-mapping retention, cancellation and deterministic catalogs implemented. Synthetic tests include a sparse 5 GiB package, restart reuse, escaped Unicode paths, removal and overlap rejection.
- Metadata/HTTP: optional IGDB enrichment with conservative PS4 matching, Title-ID sharing, token renewal, throttling, positive/negative caches, offline retention, rich schema, overrides and local screenshot repair. Mocked HTTP tests pass. Public catalog aliases, embedded library, authenticated setup/admin, sessions, CSRF, login throttling, directory browsing, range/HEAD serving and containment tests pass.
- Web/CLI milestone: embedded responsive library, six-step setup, directory picker, metadata dialogs, copyable category feeds and FPKGi configuration, admin mapping/settings/override controls and restart notices implemented. Native CLI supports serve, scan, init, validate, password-reset, health and version. Native tests and vet pass; six native release archives have cross-compiled successfully.
