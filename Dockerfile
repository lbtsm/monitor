# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /out/bridge-monitor ./cmd

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app/runtime

RUN mkdir -p /app/runtime/keys

COPY --from=builder /out/bridge-monitor /usr/local/bin/bridge-monitor

ENV TZ=Asia/Shanghai

ENTRYPOINT ["bridge-monitor"]
CMD ["monitor", "--config", "/app/runtime/config.json"]
