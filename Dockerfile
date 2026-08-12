# ---- build ----
FROM golang:1.25-alpine AS builder
# CGO (needed below for the bundled SQLite driver) requires a C toolchain;
# Alpine doesn't ship one by default.
RUN apk add --no-cache build-base
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO is required for the bundled SQLite driver (crypto store).
# -tags goolm selects mautrix-go's pure-Go olm/megolm implementation instead
# of cgo bindings to system libolm.
RUN CGO_ENABLED=1 go build -tags goolm -trimpath -ldflags="-s -w" \
    -o /out/matrix-absence-bot .

# ---- runtime ----
FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 bot \
    && adduser -D -u 10001 -G bot -s /sbin/nologin bot \
    && mkdir -p /data \
    && chown bot:bot /data

COPY --from=builder /out/matrix-absence-bot /usr/local/bin/matrix-absence-bot

# config.yaml, crypto.db and state.json all live here. Owned by bot (uid
# 10001) so it's writable out of the box even if nothing is mounted here;
# mount a host directory here so state survives container
# restarts/rebuilds - if you do, make sure it's writable by uid 10001 too.
WORKDIR /data
VOLUME ["/data"]

USER bot
ENTRYPOINT ["matrix-absence-bot", "-config", "/data/config.yaml"]
