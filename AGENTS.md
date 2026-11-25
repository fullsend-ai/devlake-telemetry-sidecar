# Agents

Base rules for AI assistants working in this repository. This file is the
source of truth for project conventions and structure.

## Golden rules

1. **One message = all related operations** — batch reads, writes, shell
   commands and agent spawns together.
2. **No working files in repo root** — the root holds only project files such
   as `README.md`, `AGENTS.md`, `CLAUDE.md`, `LICENSE`, `NOTICE`,
   `CONTRIBUTING.md`, `SECURITY.md`, `env.example`, `go.mod`/`go.sum`,
   `Makefile`, `Containerfile`, `.gitignore`, `.golangci.yml` and
   `.dockerignore`. Code goes in `cmd/` and `internal/`; docs go in
   `docs/`.
3. **Prefer editing over creating** — edit existing files rather than creating
   new ones.
4. **Prefer Grep/Glob over shell find/grep** when searching.

## Project overview

devlake-telemetry-sidecar is the Prometheus exporter sidecar for Apache DevLake
in the **Org Pulse Leading Forward Metrics**. It probes the DevLake API
(`/health`, `/ping`), polls pipeline outcomes, and serves metrics on `:9090`
from a single Go binary.

## Repository structure

| Directory | Purpose |
|---|---|
| `cmd/devlake-telemetry-sidecar/` | Entry point — config from env vars, wires collectors and server |
| `internal/collector/` | DevLake prober and pipeline poller |
| `internal/server/` | Management HTTP server (`/metrics`, `/health`, opt-in pprof) |

## Build and test

```sh
make build      # compile to bin/devlake-telemetry-sidecar
make test       # run tests with the race detector
make vet        # go vet
make lint       # golangci-lint (config in .golangci.yml)
make fmt-check  # verify gofmt formatting
make image      # build the container image
```

## Configuration

All config via environment variables: `DEVLAKE_URL`,
`PROBE_INTERVAL_SECONDS`, `PIPELINE_POLL_INTERVAL_SECONDS`, `METRICS_PORT`,
`ENABLE_PPROF`. See README.md for the full table.

## Key conventions

- Go 1.26, standard library plus `prometheus/client_golang` only
- Collectors take a `prometheus.Registerer` — pass
  `prometheus.DefaultRegisterer` in production and `prometheus.NewRegistry()`
  in tests
- Tests use `net/http/httptest` for mocks and `testutil.ToFloat64()` for
  assertions
- No comments unless the "why" is non-obvious

## Commit style

[Conventional Commits](https://www.conventionalcommits.org/):
`<type>(<scope>): <description>`. Types: `feat`, `fix`, `docs`, `style`,
`refactor`, `test`, `chore`, `build`, `ci`.

## Git workflow

- Work happens on a feature branch, never directly on `main`.
- Branch naming: `<type>/<short-description>`.
- **Never push to a remote or open an MR/PR without explicit user approval.**
