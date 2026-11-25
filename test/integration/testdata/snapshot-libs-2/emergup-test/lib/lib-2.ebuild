# SPDX-License-Identifier: GPL-3.0-or-later

EAPI=8

inherit toolchain-funcs

DESCRIPTION="A library the integration tests of emergup replace by an incompatible version"
HOMEPAGE="https://github.com/OrbintSoft/emergup"
S="${WORKDIR}"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="amd64"

src_compile() {
	$(tc-getCC) ${CFLAGS} ${LDFLAGS} -shared -fPIC -Wl,-soname,libemergup-test.so.${PV} \
		-o libemergup-test.so.${PV} "${FILESDIR}"/lib.c || die
}

src_install() {
	dolib.so libemergup-test.so.${PV}
	dosym libemergup-test.so.${PV} /usr/$(get_libdir)/libemergup-test.so
}
