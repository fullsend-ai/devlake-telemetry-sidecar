FROM registry.access.redhat.com/ubi9/go-toolset:1.26.10@sha256:0e3894cea4fc077084245e8aa2befc8a7d91a4f60fbf850f0b5b555cbe62ee26 AS builder
# /opt/app-root/src is go-toolset's default WORKDIR, owned by its default
# non-root user (1001) — build there instead of creating a new top-level
# directory, which that user can't do.
WORKDIR /opt/app-root/src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -o out/devlake-telemetry-sidecar ./cmd/devlake-telemetry-sidecar/

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8@sha256:f26ef64172ea61f5ac5ba4be8c5c738bf8760b22b3e5148ce7b40f7308ab32db
RUN microdnf install -y ca-certificates && microdnf clean all
COPY --from=builder /opt/app-root/src/out/devlake-telemetry-sidecar /devlake-telemetry-sidecar
COPY LICENSE NOTICE /licenses/devlake-telemetry-sidecar/
EXPOSE 9090
USER 1001
ENTRYPOINT ["/devlake-telemetry-sidecar"]
