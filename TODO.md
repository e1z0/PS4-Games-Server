# Outstanding external validation

Implementation and local validation are complete. Remaining checks need additional hardware, accounts or external execution:

- Test feeds, large downloads, metadata dialogs and installation against a real PS4/FPKGi console. No live console validation has been performed.
- Run the configured hosted Linux/macOS/Windows CI matrix. Local execution was Linux amd64; other OS/CPU artifacts were cross-compiled, not executed.
- Exercise live IGDB credentials and real records; automated integration tests use mocked HTTP only.
- Publish/sign releases when requested, with external registry credentials and a Buildx builder supporting Linux amd64/arm64. Nothing has been pushed to a registry. macOS binaries are unsigned.

Known operational limits: directory symlinks inside mappings are not followed; portable release dates use modification time; malformed cache JSON requires moving the affected cache aside while stopped; catalog/cache writes are atomic per file rather than a multi-file transaction. Run one server/CLI scan writer per configuration/data directory. Changing listeners or data-directory settings requires restarting the process.
