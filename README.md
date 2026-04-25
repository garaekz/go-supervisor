# go-supervisor

Native JSONL task supervisor for `deadcode-laravel`.

The supervisor reads one `task.run` frame from stdin, emits a `task.started` frame, runs the PHP worker, and relays the worker's JSONL result back to stdout. It is intentionally small: Laravel runtime behavior stays in `deadcode-laravel`, and dead-code analysis stays in `deadcore`.

## Build

```bash
go test ./...
go build -o bin/deadcode-supervisor ./cmd/deadcode-supervisor
```

On Windows:

```powershell
go test ./...
go build -o bin/deadcode-supervisor.exe ./cmd/deadcode-supervisor
```

## Runtime Contract

Inputs:

- stdin: one JSONL `task.run` frame emitted by `deadcode-laravel`
- `DEADCODE_WORKER_SCRIPT`: optional path to `bin/ox-runtime-worker.php`
- `DEADCODE_WORKER_BOOTSTRAP`: optional path to the Laravel bootstrap file
- `DEADCODE_PHP_BINARY`: optional PHP executable, defaults to `php`

Defaults:

- worker script is resolved from the current Laravel app's `vendor/deadcode/deadcode-laravel/bin/ox-runtime-worker.php`
- bootstrap file is resolved from `bootstrap/app.php` under the current working directory

Outputs:

- `task.started`
- worker-emitted `task.completed`, or `task.failed` if the worker exits unsuccessfully

## Relationship To The Other Cores

- `deadcode-laravel` owns Laravel bootstrapping, task handlers, reports, staging, and rollback
- `deadcore` owns static analysis and `deadcode.analysis.v1`
- `go-supervisor` owns process supervision and JSONL relay only
