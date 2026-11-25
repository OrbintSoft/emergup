# SPDX-License-Identifier: GPL-3.0-or-later

EAPI=8

inherit toolchain-funcs

DESCRIPTION="A program the integration tests of emergup rebuild against a new library"
HOMEPAGE="https://github.com/OrbintSoft/emergup"
S="${WORKDIR}"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="amd64"

# No slot operator: an incompatible version of the library does not
# rebuild the program, so Portage preserves the old one for it.
DEPEND="emergup-test/lib"
RDEPEND="${DEPEND}"

src_compile() {
	$(tc-getCC) ${CFLAGS} ${LDFLAGS} -o emergup-test-app "${FILESDIR}"/app.c -lemergup-test || die
}

src_install() {
	dobin emergup-test-app
}
