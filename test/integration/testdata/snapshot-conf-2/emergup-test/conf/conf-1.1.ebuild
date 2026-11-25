# SPDX-License-Identifier: GPL-3.0-or-later

EAPI=8

DESCRIPTION="A package with a configuration file the integration tests of emergup update"
HOMEPAGE="https://github.com/OrbintSoft/emergup"
S="${WORKDIR}"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="amd64"

src_install() {
	insinto /etc
	newins - emergup-test.conf <<<"${PV}"
}
