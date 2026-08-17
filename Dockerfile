ARG RUNTIME_IMAGE=m.daocloud.io/gcr.io/distroless/static-debian12:nonroot
ARG GO_BUILDER_IMAGE=m.daocloud.io/docker.io/library/golang:1.25.0

FROM ${GO_BUILDER_IMAGE} AS builder
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download
COPY cmd ./cmd
COPY api ./api
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" \
      -o /out/ ./cmd/controller ./cmd/scheduler ./cmd/worker ./cmd/deviceplugin && \
    mv /out/deviceplugin /out/simulated-device-plugin

FROM ${RUNTIME_IMAGE}
COPY --from=builder /out/ /
USER 65532:65532
