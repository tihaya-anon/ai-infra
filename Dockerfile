FROM golang:1.24.0 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY api ./api
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/controller ./cmd/controller && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/scheduler ./cmd/scheduler && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/controller /controller
COPY --from=builder /out/scheduler /scheduler
COPY --from=builder /out/worker /worker
