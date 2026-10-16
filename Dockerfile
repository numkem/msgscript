FROM golang:1.26-bookworm as builder

ENV CGO_LDFLAGS="-lassuan -lgpg-error"
RUN apt-get update && \
    apt-get install -y build-essential xz-utils libgpgme-dev libbtrfs-dev libassuan-dev libgpg-error0 && \
    curl https://wasmtime.dev/install.sh -sSf | bash

WORKDIR /build
COPY ./go.mod ./go.sum .
RUN go mod download

ENV CGO_ENABLED=1

COPY --parents ./nats.go ./embed.go ./logo.webp ./README.md ./cmd ./executor ./lua ./plugins ./script ./store ./templates .
RUN go build ./cmd/cli && \
    go build ./cmd/server && \
    go build -tags "wasmtime podman" ./cmd/worker && \
    go build -buildmode=plugin -o db.so ./plugins/db && \
    go build -buildmode=plugin -o gopher-lua-libs.so ./plugins/gopher-lua-libs && \
    go build -buildmode=plugin -o hello.so ./plugins/hello && \
    go build -buildmode=plugin -o httpscrape.so ./plugins/httpscrape && \
    go build -buildmode=plugin -o vault.so ./plugins/vault

FROM debian:bookworm-slim

RUN curl https://wasmtime.dev/install.sh -sSf | bash

WORKDIR /app

RUN mkdir -p /app/plugins
COPY ./docker/entrypoint.sh /entrypoint.sh
COPY --from=builder /build/cli /app/cli
COPY --from=builder /build/server /app/server
COPY --from=builder /build/worker /app/worker
COPY --from=builder /build/*.so /app/plugins/
RUN chmod +x /entrypoint.sh

EXPOSE 7643

ENTRYPOINT ["/entrypoint.sh"]
