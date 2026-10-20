# External follow-ups

The supplied implementation plan and local acceptance gates are complete. The requirement audit is in docs/acceptance.md and actual execution results are in docs/validation.md. No remaining implementation item is being hidden behind the external checks below.

Optional hardware/account checks and publication actions:

- Exercise real feeds, downloads, metadata dialogs and installation on a PS4/FPKGi console. No live console test has occurred.
- Exercise ARM releases on native ARM hardware and Windows service startup after an actual reboot. All six archives cross-compile; the hosted Linux/macOS/Windows matrix and real Windows service lifecycle smoke passed in [run 39341972657](https://github.com/e1z0/PS4-Games-Server/actions/runs/39341972657). The Windows runner was not rebooted.
- Exercise live IGDB credentials and records; automated IGDB tests intentionally use mocked HTTP only.
- Publish/sign releases when requested. Registry credentials are supplied externally; make upload creates/reuses its named Buildx builder and only adds latest for STABLE=1. The 0.1.0-dev image is published to Docker Hub for linux/amd64 and linux/arm64 (see docs/validation.md); stable/latest publication and release signing remain follow-ups.

Operational limits: directory symlinks inside mappings are not walked; contained file symlinks are supported. Release dates use modification time for portability. Malformed derived cache JSON requires moving the affected cache aside while stopped. Publication uses atomic per-file replacement rather than a multi-file filesystem transaction. OS writer locks reject concurrent writers and release after exit; do not remove live lock sidecars. Listener and data-directory edits require a process restart. FPKGi client configuration limits are checked by the clipboard helper; individual feed URLs remain available for custom collections.

Windows service verification: the hosted Windows job passed both handler tests and an elevated real SCM install/start/stop/restart/uninstall smoke. The beginner guide and Windows archives include the tested command sequence. An actual reboot remains a separate follow-up.
