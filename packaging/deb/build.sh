#!/bin/sh
# Build the Debian package in debian:forky from a source tree, then install
# it in a fresh debian:forky and smoke-check it. The release workflow and a
# local `podman run` both call this from the tree's root.
# Usage: packaging/deb/build.sh VERSION OUTDIR   (run as root inside debian:forky)
set -eu

version=${1:?usage: build.sh VERSION OUTDIR}
out=${2:?usage: build.sh VERSION OUTDIR}
nfpm_version=2.47.0

export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -y -q --no-install-recommends ca-certificates curl gcc make pkgconf \
	golang-go wayland-protocols libwayland-dev libwlroots-0.20-dev libscenefx-0.5-dev \
	libpipewire-0.3-dev libxkbcommon-dev libpixman-1-dev libdrm-dev libinput-dev

# A Go older than go.mod's line fails here instead of fetching a toolchain.
export GOTOOLCHAIN=local
rm -rf root
make build install PREFIX=/usr DESTDIR="$PWD/root" VERSION="$version"
packaging/check-install.sh root /usr

curl -fsSL "https://github.com/goreleaser/nfpm/releases/download/v$nfpm_version/nfpm_${nfpm_version}_Linux_x86_64.tar.gz" |
	tar -xz -C /usr/local/bin nfpm
mkdir -p "$out"
VERSION=$version nfpm package --config packaging/deb/nfpm.yaml --packager deb --target "$out/"
