# Contributing to devlake-telemetry-sidecar

Thanks for your interest in contributing! This document explains how to get
started.

## Reporting bugs and requesting features

- Search existing issues before opening a new one.
- For bugs, include the version or commit, steps to reproduce, the expected and
  actual behavior, and relevant log output (with credentials removed).
- **Security vulnerabilities must not be reported in public issues.** Follow
  [SECURITY.md](SECURITY.md) instead.

## Development setup

Requirements:

- Go (version from the `go` directive in [go.mod](go.mod))
- `make`
- Optional: [golangci-lint](https://golangci-lint.run/) v2, built with a Go
  version at least as new as the version in `go.mod`, for `make lint`
- Optional: `podman` (or Docker) for container builds

Common tasks:

```sh
make build      # compile to bin/devlake-telemetry-sidecar
make test       # run all tests with the race detector
make vet        # go vet
make lint       # golangci-lint (config in .golangci.yml)
make fmt-check  # verify gofmt formatting
make image      # build the container image
```

Configuration is via environment variables; see [README.md](README.md) and
[env.example](env.example).

## Submitting changes

1. Fork the repository and create a topic branch from `main`.
2. Make your change. Keep pull requests focused on a single concern.
3. Add or update tests. HTTP behaviour is tested with `net/http/httptest` and
   metrics with `prometheus/testutil`; tests never need a real DevLake instance.
4. Make sure `make fmt-check vet lint test` passes.
5. Update documentation (`README.md`, `env.example`) when behavior or
   configuration changes.
6. Open a pull request describing what changed and why.

### Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `build`, `ci`.

### Code conventions

- Standard library first; add third-party dependencies only when clearly
  justified.
- Format with `gofmt`.
- Only add comments when the "why" is not obvious from the code.

## License

By contributing, you agree that your contributions will be licensed under the
project's [license](LICENSE). Review [NOTICE](NOTICE) when changing dependencies;
binary distributions must include the applicable third-party notices and licenses.
