# Architecture

`cmd/ps4-server` is the executable entrypoint. `internal/config` owns validation, credentials and environment seeding; `internal/storage` owns atomic persistent writes and containment. Subsequent packages add parsing, scanning, metadata, HTTP and embedded assets. Tests live next to their implementations. `tools/source-map.go` generates the complete file inventory and checks for stale coverage.
