# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
pull requests or discussions.**

Report vulnerabilities privately through one of these channels:

- **GitHub private vulnerability reporting** — if enabled, use the "Report a
  vulnerability" button on the repository's **Security** tab.
- **Email** — Red Hat Product Security at
  [secalert@redhat.com](mailto:secalert@redhat.com). See
  <https://access.redhat.com/security/team/contact> for details and for the
  PGP key to encrypt sensitive reports.

Please include:

- a description of the issue and its impact,
- the affected version or commit,
- steps to reproduce or a proof of concept,
- any known mitigations.

We will acknowledge your report, keep you informed about the fix, and credit
you in the advisory unless you prefer to remain anonymous. Please give us a
reasonable amount of time to release a fix before disclosing the issue publicly.

## Supported versions

Security fixes are applied to the latest release and the `main` branch.

## Security model

Keep the following in mind when deploying `devlake-telemetry-sidecar`:

- **The management endpoints are unauthenticated.** `/metrics` and `/health`
  are served on `METRICS_PORT` without authentication or authorisation.
  Restrict the port with network policies, a reverse proxy or an authenticating
  gateway if the metrics are not meant to be public.
- **pprof is opt-in and unauthenticated.** `ENABLE_PPROF=true` exposes
  `/debug/pprof`, which includes runtime internals. Never expose the management
  port publicly while it is enabled.
- **The listener serves plain HTTP.** Terminate TLS in front of the service with
  an ingress or reverse proxy; the binary has no TLS mode.
- **`DEVLAKE_URL` can embed credentials.** The startup log redacts a URL
  password, but other URL components can appear in logs. Prefer network-level
  trust or a token-free endpoint; do not put secrets in URL paths or queries.
- The service does not log client IP addresses, request bodies or
  `X-Forwarded-For`.
- **Supply-chain artefacts are not yet produced.** Establish SBOM, provenance
  and image signing before publishing production images.
