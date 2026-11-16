# Copyright 2026 Gentoo Authors
# Distributed under the terms of the GNU General Public License v2

EAPI=8

inherit go-module

DESCRIPTION="A scrollable-tiling Wayland compositor built in Go"
HOMEPAGE="https://github.com/Nomadcxx/sysc-compositor"
SRC_URI="
	https://github.com/Nomadcxx/${PN}/archive/refs/tags/v${PV}.tar.gz -> ${P}.tar.gz
	https://github.com/Nomadcxx/${PN}/releases/download/v${PV}/${P}-vendor.tar.xz
"

# The compositor, then the vendored Go modules.
LICENSE="GPL-3+ BSD BSD-2 MIT || ( Unlicense BSD )"
SLOT="0"
KEYWORDS="~amd64"

RDEPEND="
	dev-libs/libinput:=
	dev-libs/wayland
	gui-libs/scenefx
	gui-libs/wlroots:0.20
	media-libs/libglvnd
	media-video/pipewire:=
	sys-apps/dbus
	x11-libs/libdrm
	x11-libs/libxkbcommon
	x11-libs/pixman
	x11-misc/xkeyboard-config
"
DEPEND="
	${RDEPEND}
	dev-libs/wayland-protocols
"
BDEPEND="
	>=dev-lang/go-1.26.4
	dev-util/wayland-scanner
	virtual/pkgconfig
"

src_unpack() {
	default
	# The release's vendor tarball holds a bare vendor/ directory.
	mv "${WORKDIR}"/vendor "${S}"/ || die
}

src_compile() {
	emake build VERSION="${PV}"
}

src_install() {
	emake install PREFIX="${EPREFIX}"/usr DESTDIR="${D}"
	# Gentoo keeps docs in the versioned doc directory and licences in the tree.
	rm -r "${ED}"/usr/share/{licenses,doc} || die
	dodoc README.md NOTICE
}
