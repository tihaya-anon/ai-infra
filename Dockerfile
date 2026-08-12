FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ai-infra-lab ./cmd/ai-infra-lab

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/ai-infra-lab /ai-infra-lab
ENTRYPOINT ["/ai-infra-lab"]
