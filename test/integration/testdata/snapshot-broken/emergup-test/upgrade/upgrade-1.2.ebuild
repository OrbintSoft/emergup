# SPDX-License-Identifier: GPL-3.0-or-later

EAPI=8

DESCRIPTION="A version of the package the integration tests of emergup cannot build"
HOMEPAGE="https://github.com/OrbintSoft/emergup"
S="${WORKDIR}"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="amd64"

src_compile() {
	die "this version does not build"
}
