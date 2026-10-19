# PS4 Games Server

A native Go PS4 package server with an embedded browser library, setup wizard and authenticated administration. One executable; no runtime Go installation, Node.js, Python, Orbis tools or database. GPL-2.0-only.

This is an independent rewrite of FPKGi-node-server at `88a969c`. The original repository stays untouched. There is no configuration importer.

## Start natively

Download and unpack an archive for your operating system and CPU. Run:

```sh
./ps4-server serve
```

On Windows, run `ps4-server.exe serve` in a terminal. Open the setup URL printed in the terminal. Replace `localhost` with the server's LAN address if using another computer. The secret setup token appears after `#`; it expires after setup completes or the process restarts.

The wizard asks for administrator credentials, package folders, writable application storage, a PS4-reachable URL such as `http://192.168.1.20:8080`, and optional IGDB details. Sign in to start the first scan. Open `/library` to browse and copy category feed URLs into FPKGi. Feeds and package downloads do not require login.

Read [beginner setup](docs/setup.md), [configuration and environment reference](docs/configuration.md), and [validation results](docs/validation.md).

## Docker Compose

Docker is needed only for container commands. Set `PACKAGES_PATH` to an existing host directory. Packages are mounted read-only at `/packages`.

```sh
export PACKAGES_PATH=/srv/ps4-packages
make deploy
make logs
```

Open the setup URL from the logs and select `/packages` in the wizard. Persistent named volumes store configuration and generated data. The container runs as UID/GID 10001 by default. For a different UID/GID, see the volume preparation instructions in [setup](docs/setup.md).

To start unattended on a **fresh configuration volume**, provide `PS4_PUBLIC_URL`, `PS4_ADMIN_USERNAME`, a password of at least 12 characters in `PS4_ADMIN_PASSWORD`, and `PS4_MAPPINGS`:

```sh
export PS4_PUBLIC_URL=http://192.168.1.20:8080
export PS4_ADMIN_USERNAME=admin
# Supply PS4_ADMIN_PASSWORD through your deployment environment.
export PS4_MAPPINGS='[{"id":"main","path":"/packages","category":"games","enabled":true}]'
make deploy
```

Environment variables seed missing JSON fields once. Persistent web edits take precedence on every subsequent restart. Do not commit passwords or deployment environment files.

## Develop and distribute

Go 1.24+ is sufficient for native development; CI and Docker use Go 1.26. `make` builds `bin/ps4-server`. `make check` runs formatting verification, vet, tests, race tests and source-map checks. Race checks require the host C compiler used by Go’s race detector; normal CGO-free builds and tests need only Go. `make release VERSION=0.1.0` produces six self-contained archives for Linux, macOS and Windows on amd64/arm64 using Go alone. macOS executables are unsigned.

Other targets: `run`, `test`, `fmt`, `source-map`, `docker`, `deploy`, `logs`, `down`. `make upload VERSION=0.1.0 TAG=0.1.0` publishes Linux amd64/arm64 images to `nulldevil/ps4-games-server` with externally supplied Docker registry credentials. `STABLE=1` additionally publishes `latest`; ordinary uploads never do. Upload creates or reuses a named Docker Buildx builder with the docker-container driver (`BUILDER=...` selects another builder). Both Linux image architectures build on the host CPU without QEMU. CI also produces a multi-architecture OCI image archive.

See the [requirement audit](docs/acceptance.md), [feature parity](docs/parity.md), [architecture](docs/architecture.md), [source inventory](SOURCE_MAP.md), [progress](PROGRESS.md), [external validation follow-ups](TODO.md), and [changelog](CHANGELOG.md).
