# SPDX-License-Identifier: GPL-3.0-or-later
#
# The image the integration tests run emergup in: a Gentoo stage3 with the
# Gentoo tree, sudo, an ordinary user who may use it, and the test
# repository emergup-test. The repository syncs from an upstream copy in
# the image, so a test needs no network. The emergup binary is not in the
# image: each test mounts the one under test.
#
# Both images are pinned by digest; Renovate and Dependabot update them.
# What changes least comes first, so that a change to the test repository
# rebuilds only the last layers.

FROM docker.io/gentoo/portage:20261124@sha256:5435a4f500f52b43e91fe533ec31431b36f564f32b5757a8da3e6d96201c0c07 AS tree

FROM docker.io/gentoo/stage3:amd64-openrc-20261123@sha256:b49a69efcf22e141a2cc5e6a08fe75d651463d8ecec0ec055c2da4b97056ec89

COPY --from=tree /var/db/repos/gentoo /var/db/repos/gentoo
COPY etc/portage/make.conf /etc/portage/make.conf

# sudo comes from the Gentoo binary host when it has the version of the
# tree, and is built otherwise. Then every package installed goes into the
# world set, so that the depclean removes only what a test leaves behind.
RUN emerge --quiet --getbinpkg --noreplace app-admin/sudo \
    && rm -rf /var/cache/binpkgs/* /var/cache/distfiles/* \
    && qlist --installed --slots > /var/lib/portage/world

# tester is the ordinary user who starts emergup.
RUN useradd --create-home tester
COPY tester.sudoers /etc/sudoers.d/tester
RUN chmod 0440 /etc/sudoers.d/tester \
    && visudo --check --strict

COPY etc/ /etc/

# The test repository: its upstream, then the synced copy Portage uses,
# with the package installed that the update tests upgrade.
COPY repo/ /srv/emergup-test/
RUN emerge --sync emergup-test \
    && emerge --quiet emergup-test/upgrade
