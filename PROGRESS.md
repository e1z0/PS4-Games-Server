# Progress

Baseline: sibling FPKGi-node-server, commit 88a969cf991303c36be64438a6752955c46b578d. Its 15 Linux tests passed on 2026-10-19. The original repository is unchanged.

- Foundation: independent repository, GPL-2.0-only, versioned JSON, presence-aware environment seeding, atomic writes and salted password hashing implemented.
- Parser/scanner: native bounded CNT/SFO parsing, firmware fallback, homebrew, encrypted warnings, stable mapping IDs, recursive discovery, canonical deduplication, relative-path incremental cache, cover repair, unavailable-mapping retention, cancellation and deterministic catalogs implemented. Synthetic tests include a sparse 5 GiB package, restart reuse, escaped Unicode paths, removal and overlap rejection.
