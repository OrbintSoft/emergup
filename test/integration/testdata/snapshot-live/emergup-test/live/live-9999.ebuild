# SPDX-License-Identifier: GPL-3.0-or-later

EAPI=8

DESCRIPTION="A live package the integration tests of emergup rebuild"
HOMEPAGE="https://github.com/OrbintSoft/emergup"
S="${WORKDIR}"

LICENSE="GPL-3+"
SLOT="0"
KEYWORDS="amd64"
# A live package fetches its sources from a version control system; this one
# fetches nothing, and is live only for Portage.
PROPERTIES="live"

src_install() {
	insinto /usr/share/emergup-test
	newins - "${PN}" <<<"${PV}"
}
