# SPDX-License-Identifier: GPL-3.0-or-later
#
# Build, test, lint and install emergup; `make help` lists the targets.
#
# The installation paths follow the usual conventions and can be overridden, so
# the same Makefile serves a local install and a distribution package. A Gentoo
# ebuild builds offline with the system Go and stamps the release version:
#
#     emake GOTOOLCHAIN=local VERSION="${PV}"
#     emake GOTOOLCHAIN=local PREFIX="${EPREFIX}/usr" DESTDIR="${D}" install

# Installation paths.
DESTDIR ?=
PREFIX  ?= /usr/local
bindir  ?= $(PREFIX)/bin

GO      ?= go
INSTALL ?= install

# The Go toolchain that builds, tests and lints: the toolchain line of go.mod,
# which the go command downloads when it is not the installed one. "local" uses
# the installed Go as it is.
GOTOOLCHAIN ?= $(shell sed -n 's/^toolchain //p' go.mod)
export GOTOOLCHAIN

# The version `emergup version` reports. When empty, the Go toolchain records
# the module version itself (a pseudo-version in a git checkout).
VERSION ?=
STAMP   := -X github.com/OrbintSoft/emergup/internal/version.stamped=
LDFLAGS := $(if $(VERSION),$(STAMP)$(VERSION))

# The Go linters run at pinned versions, built with the toolchain above, and
# markdownlint-cli2 and the Renovate configuration validator at pinned versions
# through npx, so the result does not depend on what happens to be installed.
# yamllint is taken from PATH: Python has no standard runner that fetches a
# pinned version. hadolint and shellcheck run from their images, pinned by
# digest, as Haskell has no such runner either. Renovate updates every pin. Override any of
# them to use another binary.
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
CHECKMAKE     ?= $(GO) run github.com/checkmake/checkmake/cmd/checkmake@v0.3.2
ACTIONLINT    ?= $(GO) run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
MARKDOWNLINT  ?= npx --yes markdownlint-cli2@0.24.2
RENOVATE_VALIDATE ?= npx --yes --package renovate@44.257.9 -- renovate-config-validator --strict
YAMLLINT      ?= yamllint
HADOLINT      ?= $(DOCKER) run --rm --interactive docker.io/hadolint/hadolint:v2.15.1@sha256:32dac94127fd60b7b7e3fbfc65e1383b9b5e25c9bfd7b8536de7a539fe68a12d hadolint
SHELLCHECK    ?= $(DOCKER) run --rm --volume '$(CURDIR):/mnt:ro' --workdir /mnt docker.io/koalaman/shellcheck:v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d

# The shell scripts shellcheck lints.
SHELL_SCRIPTS := $(wildcard test/integration/testdata/*/*.sh)

BUILDDIR := bin
BINARY   := emergup

# The integration tests run a static emergup binary, stamped with a version
# of its own, in containers of the image built from test/integration/image.
# DOCKER is any command that takes the arguments of docker.
DOCKER              ?= docker
INTEGRATION_IMAGE   ?= emergup-integration
INTEGRATION_BINARY  := $(BUILDDIR)/integration/$(BINARY)
INTEGRATION_VERSION := 0.0.0-integration
# A pattern of `go test -run` that picks the tests to run; all of them when
# empty.
INTEGRATION_RUN     ?=

# The release archives `make dist` builds: a binary for each architecture
# with the documents and the example configuration, and the vendored Go
# modules, from which a distribution package builds offline.
DISTDIR     := dist
DIST_ARCHES := amd64 arm64
DIST_FILES  := LICENSE COPYRIGHT.md README.md DEPENDENCIES.md docs examples
DIST_BINS   := $(foreach arch,$(DIST_ARCHES),$(DISTDIR)/$(BINARY)_$(VERSION)_linux_$(arch).tar.gz)
DIST_VENDOR := $(DISTDIR)/$(BINARY)-$(VERSION)-vendor.tar.xz
TAR         := tar --sort=name --owner=0 --group=0 --numeric-owner

.DEFAULT_GOAL := build

.PHONY: all clean test build race vet integration integration-docker lint lint-go lint-mod lint-make lint-actions lint-markdown lint-yaml lint-renovate lint-docker lint-shell fmt check install uninstall dist dist-version help

all: build ## Build the binary (the default)

build: ## Build bin/emergup
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BUILDDIR)/$(BINARY) ./cmd/emergup

test: ## Run the unit tests
	$(GO) test ./...

race: ## Run the unit tests with the race detector
	$(GO) test -race ./...

vet: ## Run go vet, on the integration tests too
	$(GO) vet -tags integration ./...

integration: integration-docker ## Run the integration tests in Docker containers
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(STAMP)$(INTEGRATION_VERSION)' -o $(INTEGRATION_BINARY) ./cmd/emergup
	$(DOCKER) build --tag $(INTEGRATION_IMAGE) test/integration/image
	EMERGUP_TEST_DOCKER='$(DOCKER)' EMERGUP_TEST_IMAGE='$(INTEGRATION_IMAGE)' EMERGUP_TEST_BINARY='$(abspath $(INTEGRATION_BINARY))' EMERGUP_TEST_VERSION='$(INTEGRATION_VERSION)' $(GO) test -tags integration -count=1 -timeout 30m -run '$(INTEGRATION_RUN)' ./test/integration/...

integration-docker:
	@command -v $(firstword $(DOCKER)) > /dev/null || { echo 'integration: the integration tests need $(firstword $(DOCKER)), which is not installed' >&2; exit 1; }

lint: lint-go lint-mod lint-make lint-actions lint-markdown lint-yaml lint-renovate lint-docker lint-shell ## Run every linter

lint-go: ## Lint the Go code, formatting included
	$(GOLANGCI_LINT) run ./...

lint-mod: ## Check that go.mod and go.sum are tidy
	$(GO) mod tidy -diff

lint-make: ## Lint this Makefile
	$(CHECKMAKE) Makefile

lint-actions: ## Lint the GitHub Actions workflows (with shellcheck, if installed)
	$(ACTIONLINT)

lint-markdown: ## Lint the Markdown files
	$(MARKDOWNLINT)

lint-yaml: ## Lint the YAML files
	$(YAMLLINT) --strict .

lint-renovate: ## Validate the Renovate configuration
	$(RENOVATE_VALIDATE)

lint-docker: ## Lint the Dockerfile of the integration tests
	$(HADOLINT) - < test/integration/image/Dockerfile

lint-shell: ## Lint the shell scripts
	$(SHELLCHECK) $(SHELL_SCRIPTS)

fmt: ## Format the Go code with gofumpt
	$(GOLANGCI_LINT) fmt

check: build vet lint race ## Run the whole quality gate

install: build ## Install the binary into $(DESTDIR)$(bindir)
	$(INSTALL) -d $(DESTDIR)$(bindir)
	$(INSTALL) -m 0755 $(BUILDDIR)/$(BINARY) $(DESTDIR)$(bindir)/$(BINARY)

uninstall: ## Remove the installed binary
	rm -f $(DESTDIR)$(bindir)/$(BINARY)

dist: $(DIST_BINS) $(DIST_VENDOR) ## Build the release archives in dist/ (VERSION=x.y.z)
	cd $(DISTDIR) && sha256sum $(notdir $^) > SHA256SUMS

dist-version:
	@test -n '$(VERSION)' || { echo 'dist: give the version, as in make dist VERSION=0.1.0' >&2; exit 1; }

# A static binary for linux on one architecture, with the documents.
$(DISTDIR)/$(BINARY)_$(VERSION)_linux_%.tar.gz: dist-version
	mkdir -p $(DISTDIR)/$(BINARY)_$(VERSION)_linux_$*
	CGO_ENABLED=0 GOOS=linux GOARCH=$* $(GO) build -trimpath -ldflags '-s -w $(LDFLAGS)' -o $(DISTDIR)/$(BINARY)_$(VERSION)_linux_$*/$(BINARY) ./cmd/emergup
	cp -R $(DIST_FILES) $(DISTDIR)/$(BINARY)_$(VERSION)_linux_$*/
	$(TAR) -C $(DISTDIR) -czf $@ $(BINARY)_$(VERSION)_linux_$*
	rm -rf $(DISTDIR)/$(BINARY)_$(VERSION)_linux_$*

# The vendored modules, unpacked as $(BINARY)-$(VERSION)/vendor, where the
# source archive of the tag unpacks.
$(DIST_VENDOR): dist-version
	$(GO) mod vendor -o $(DISTDIR)/$(BINARY)-$(VERSION)/vendor
	$(TAR) -C $(DISTDIR) -cJf $@ $(BINARY)-$(VERSION)
	rm -rf $(DISTDIR)/$(BINARY)-$(VERSION)

clean: ## Remove the build output
	rm -rf $(BUILDDIR) $(DISTDIR)

help: ## List the targets
	@awk -F ':.*## ' '/^[a-z-]+:.*## / { printf "  %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
