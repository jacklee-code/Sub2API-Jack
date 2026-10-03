#!/bin/sh
set -eu
runtime=/app/data/jack-runtime
if [ "$(id -u)" = 0 ]; then
    mkdir -p "$runtime"
    chown -R sub2api:sub2api /app/data
    exec su-exec sub2api "$0" "$@"
fi
mkdir -p "$runtime"
if [ ! -f "$runtime/sub2api" ]; then
    cp /app/sub2api "$runtime/sub2api.initial"
    chmod 755 "$runtime/sub2api.initial"
    mv "$runtime/sub2api.initial" "$runtime/sub2api"
fi
exec "$runtime/sub2api" "$@"
