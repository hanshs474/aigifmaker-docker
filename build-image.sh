#!/bin/bash
# Build san8888/aigifmaker for linux/amd64 + linux/arm64 without a Docker daemon.
#   ./build-image.sh --local      cross-compile the static binaries and layers into dist/ only (no push)
#   ./build-image.sh [version]    also push with crane; needs crane and Docker Hub credentials in DOCKER_CONFIG
set -euo pipefail
cd "$(dirname "$0")"
LOCAL=0
if [ "${1:-}" = "--local" ]; then LOCAL=1; shift; fi
R=index.docker.io/san8888/aigifmaker; V=${1:-0.1.0}
rm -rf dist
for a in amd64 arm64; do
  mkdir -p dist/$a/root/etc/ssl/certs
  CGO_ENABLED=0 GOOS=linux GOARCH=$a go build -trimpath -ldflags "-s -w -X main.version=$V" -o dist/$a/root/aigifmaker .
  cp /etc/ssl/cert.pem dist/$a/root/etc/ssl/certs/ca-certificates.crt
  (cd dist/$a/root && COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata --uid 0 --gid 0 --format ustar -cf ../layer.tar aigifmaker etc)
  [ "$LOCAL" = 1 ] && continue
  crane append --platform linux/$a -f dist/$a/layer.tar -t "${R}:$a"
  crane mutate --entrypoint=/aigifmaker --set-platform linux/$a -e SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt -t "${R}:$a" "${R}:$a"
done
if [ "$LOCAL" = 1 ]; then ls -l dist/*/root/aigifmaker; exit 0; fi
crane index append -m "${R}:amd64" -m "${R}:arm64" -t "${R}:$V"
crane copy "${R}:$V" "${R}:latest"
