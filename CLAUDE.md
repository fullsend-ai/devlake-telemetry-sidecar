# CLAUDE.md

Project conventions, structure and commit style live in [AGENTS.md](./AGENTS.md).

## Build & test commands

```sh
make build       # compile binary
make test        # run tests with -race
make vet         # go vet ./...
make fmt-check   # verify gofmt formatting
```

Run all checks before committing: `make fmt-check && make vet && make test`

## Project structure

- `cmd/devlake-telemetry-sidecar/` — entry point
- `internal/collector/` — prober and pipeline poller (with tests)
- `internal/server/` — HTTP server exposing /metrics and /health (with tests)

## Code style

- Go 1.26, standard library + prometheus/client_golang only
- No comments unless the "why" is non-obvious
- Collectors take `prometheus.Registerer` — pass `prometheus.DefaultRegisterer` in production, `prometheus.NewRegistry()` in tests
- Tests use `httptest.NewServer` for mocks, `testutil.ToFloat64()` for assertions
