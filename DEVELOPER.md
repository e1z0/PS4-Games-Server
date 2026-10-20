# Developer guide

PS4 Games Server is an independent Go rewrite of FPKGi-node-server, using commit `88a969c` as its feature baseline. It has fresh configuration with no importer. Source and distributions use GPL-2.0-only; see [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_NOTICES.md).

For installation and everyday use, start with [README](README.md). Operational settings belong in [EXPERT.md](EXPERT.md).

## Build and run

Go 1.24+ is sufficient for native development; CI and Docker currently use Go 1.26. Normal builds use `CGO_ENABLED=0`. No Node.js, Python, Orbis tools, database or frontend build step is required. HTML/CSS/JavaScript are embedded in the executable. Windows services use the compiled-in `golang.org/x/sys` dependency.

```sh
git clone https://github.com/e1z0/PS4-Games-Server.git
cd PS4-Games-Server
make
./bin/ps4-server
```

Without Make:

```sh
CGO_ENABLED=0 go build -o bin/ps4-server ./cmd/ps4-server
./bin/ps4-server
```

On Windows, use `go build -o bin/ps4-server.exe ./cmd/ps4-server`. Running the executable with no subcommand starts HTTP and the setup wizard. Use a private working directory or `-config /absolute/path/config.json` for local configuration. Runtime files and builds are ignored by Git.

## Checks

```sh
make check
```

This verifies formatting, runs `go vet`, all Go tests, race tests and the source-map check. The race detector requires a supported host and its C compiler; ordinary native builds/tests need only Go. Synthetic CNT/SFO fixtures avoid shipping package content. IGDB tests use mocked HTTP, not real credentials.

CI runs native Linux/macOS/Windows tests, race checks, Windows service smoke tests, release cross-compilation and a container build. See [.github/workflows/ci.yml](.github/workflows/ci.yml), the [validation record](docs/validation.md), [acceptance audit](docs/acceptance.md) and [feature parity checklist](docs/parity.md). Do not describe synthetic/browser tests as live PS4 installation tests.

## Make targets

| Target | Purpose |
| --- | --- |
| `make` / `make build` | Build `bin/ps4-server` |
| `make run CONFIG=...` | Run from source with a selected config |
| `make test` | Run Go tests |
| `make check` | Format check, vet, tests, race tests, source-map check |
| `make fmt` | Format Go source |
| `make source-map` | Regenerate the file inventory |
| `make release VERSION=...` | Build six native release archives |
| `make docker` | Build the local Linux image |
| `make deploy` | Build and start repository Docker Compose |
| `make logs` / `make down` | Follow Compose logs / stop services |
| `make upload VERSION=... TAG=...` | Publish Linux amd64/arm64 images |

Docker is needed only for container targets. The root [compose.yaml](compose.yaml) builds locally and uses named volumes. Set `PACKAGES_PATH` to an existing package directory before `make deploy`. For consumers using the published image and bind mounts, see [examples/compose.nas.yml](examples/compose.nas.yml).

## Releases and Docker Hub

```sh
make release VERSION=0.1.0
```

Archives appear in `dist/` for Linux, macOS and Windows on amd64/arm64. Each contains one executable, example JSON, license, setup instructions and notices; Windows archives also contain WINDOWS.md. macOS executables are unsigned.

Authenticate with Docker Hub outside the build, then publish a chosen version:

```sh
docker login
make upload VERSION=0.1.0 TAG=0.1.0
```

The default repository is `nulldevil/ps4-games-server`; override `IMAGE` if needed. Upload creates/reuses the named Docker-container Buildx builder `ps4-games-server-release` (`BUILDER` overrides it). Both Linux image architectures cross-compile on the host CPU without QEMU. Credentials come from Docker's external login/configuration, not the source tree.

Ordinary uploads never move `latest`. Only an explicit stable release should use:

```sh
make upload VERSION=0.1.0 TAG=0.1.0 STABLE=1
```

CI also exports a multi-architecture OCI archive. The runtime image is non-root and has a binary-based health check. Persistent configuration/data remain outside the image.

## Source and documentation

- [Architecture](docs/architecture.md): entrypoint, package responsibilities, routes and important data flows.
- [SOURCE_MAP.md](SOURCE_MAP.md): generated inventory; run `make source-map` when files are added/removed.
- [Configuration reference](docs/configuration.md): JSON fields, every supported environment seed and CLI behavior.
- [CHANGELOG.md](CHANGELOG.md), [PROGRESS.md](PROGRESS.md) and [TODO.md](TODO.md): delivered behavior, verification and remaining external checks.

Keep user-facing installation steps in README/WINDOWS, advanced operations in EXPERT, and development details here. Screenshots in `docs/screenshots/` show the real populated production library/details and a local authenticated example admin panel. Capture the rendered application, keep secret controls empty/closed, and inspect images before committing. No game packages, credentials, setup tokens or browser sessions belong in the repository.

Update the source map and progress records with coherent tested changes. Use targeted checks for documentation and visual changes; use the full Go checks for implementation changes.
