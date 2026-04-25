# Release

`go-supervisor` publishes the `deadcode-supervisor` binary used by `deadcode-laravel`.

Pre-release checklist:

1. `go test ./...`
2. `go build -o bin/deadcode-supervisor ./cmd/deadcode-supervisor`
3. `./bin/deadcode-supervisor --version`
4. confirm `deadcode-laravel` can run `deadcode:doctor` with the built binary
5. confirm `deadcode-laravel` can run `deadcode:analyze` with the built binary in a real Laravel app or labelled local fixture

Release asset contract:

- binaries are published as `deadcode-supervisor_<tag>_<os>_<arch>[.exe]`
- `checksums.txt` is published in the same release bundle

Local proof does not verify hosted GitHub release assets, checksums, Packagist state, or external consumer-app behavior.
