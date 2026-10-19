# External follow-ups

The supplied implementation plan and local acceptance gates are complete. The requirement audit is in docs/acceptance.md and actual execution results are in docs/validation.md. No remaining implementation item is being hidden behind the external checks below.

Optional hardware/account checks and publication actions:

- Exercise real feeds, downloads, metadata dialogs and installation on a PS4/FPKGi console. No live console test has occurred.
- Run the configured hosted Linux/macOS/Windows CI matrix after pushing to a chosen repository. Local execution was Linux amd64; all six OS/CPU artifacts cross-compiled. Installed Wine could not initialize the Go runtime because it lacks bcryptprimitives.dll, so it does not count as Windows validation.
- Exercise live IGDB credentials and records; automated IGDB tests intentionally use mocked HTTP only.
- Publish/sign releases when requested. Registry credentials are supplied externally; make upload creates/reuses its named Buildx builder and only adds latest for STABLE=1. No registry upload or release signing was performed.

Operational limits: directory symlinks inside mappings are not walked; contained file symlinks are supported. Release dates use modification time for portability. Malformed derived cache JSON requires moving the affected cache aside while stopped. Publication uses atomic per-file replacement rather than a multi-file filesystem transaction. OS writer locks reject concurrent writers and release after exit; do not remove live lock sidecars. Listener and data-directory edits require a process restart. FPKGi client configuration limits are checked by the clipboard helper; individual feed URLs remain available for custom collections.
