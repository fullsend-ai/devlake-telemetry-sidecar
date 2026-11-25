# devlake-telemetry-sidecar

A Prometheus exporter sidecar for Apache DevLake, part of the
**Org Pulse Leading Forward Metrics**. It runs alongside the Lake pod, probes the Lake API
with synthetic HTTP requests, polls pipeline run outcomes, and exposes metrics
on a dedicated management port for Prometheus scraping.

No changes to the upstream DevLake source are required. The Go module is
`github.com/fullsend-ai/devlake-telemetry-sidecar`.

## Quick start

```sh
make build   # compile to bin/devlake-telemetry-sidecar
make run     # build and run locally on :9090
make test    # run tests with the race detector
make image   # build the container image
```

## Endpoints

The management server (default `:9090`) exposes:

| Path | Method | Description |
|------|--------|-------------|
| `/metrics` | GET | Prometheus metrics |
| `/health` | GET | Liveness probe, always returns `{"status":"healthy"}` |
| `/debug/pprof/*` | GET | Go pprof, only when `ENABLE_PPROF=true` |

`/metrics` and `/health` are unauthenticated. Restrict the management port with
network controls; see [SECURITY.md](SECURITY.md).

## Configuration

All configuration is via environment variables. See
[`env.example`](env.example) for a copyable template.

| Variable | Default | Description |
|----------|---------|-------------|
| `DEVLAKE_URL` | — | DevLake API base URL, e.g. `http://devlake:8080/api`. Collectors are disabled when unset. |
| `PROBE_INTERVAL_SECONDS` | `30` | Probe loop interval |
| `PIPELINE_POLL_INTERVAL_SECONDS` | `60` | Pipeline poller interval |
| `METRICS_PORT` | `9090` | Management server listen port |
| `ENABLE_PPROF` | `false` | Expose `/debug/pprof` endpoints |

Both intervals must be greater than zero and the port must be 1–65535;
invalid values stop the process at startup.

## Metrics

The probe collector requests `/health` and `/ping` on `DEVLAKE_URL`:

| Metric | Labels | Description |
|--------|--------|-------------|
| `devlake_probe_requests_total` | `endpoint`, `status_code` | Probe requests by endpoint and status |
| `devlake_probe_duration_seconds` | `endpoint` | Probe request duration |
| `devlake_probe_up` | `endpoint` | `1` when the last probe was 2xx, otherwise `0` |

The pipeline collector reads `GET /pipelines?pageSize=100`:

| Metric | Labels | Description |
|--------|--------|-------------|
| `devlake_pipeline_runs_total` | `status` | Terminal pipeline runs by status (`success`, `failed`, `cancelled`) |
| `devlake_pipeline_last_run_timestamp` | `status` | Unix timestamp of the newest observed run by status |

## Container

The container uses pinned UBI9 builder and runtime images.

The image examples and `make image` retain the existing image repository for
compatibility with current deployments. To use a different repository, pass
`IMAGE=your-registry/your-image` to `make image` or `make push`.

```sh
podman build -t quay.io/dprod/devlake-sidecar:latest .
podman run --rm -p 127.0.0.1:9090:9090 \
  -e DEVLAKE_URL=http://devlake:8080/api \
  quay.io/dprod/devlake-sidecar:latest
```

## Security

See [SECURITY.md](SECURITY.md) for how to report vulnerabilities and for the
deployment security model. Contributions are welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE) for attribution.
