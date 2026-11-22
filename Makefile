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
LDFLAGS := $(if $(VERSION),-X github.com/OrbintSoft/emergup/internal/version.stamped=$(VERSION))

# The Go linters run at pinned versions, built with the toolchain above, and
# markdownlint-cli2 at a pinned version through npx, so the result does not
# depend on what happens to be installed. yamllint is taken from PATH: Python
# has no standard runner that fetches a pinned version. Override any of them
# to use another binary.
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
CHECKMAKE     ?= $(GO) run github.com/checkmake/checkmake/cmd/checkmake@v0.3.2
ACTIONLINT    ?= $(GO) run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
MARKDOWNLINT  ?= npx --yes markdownlint-cli2@0.24.2
YAMLLINT      ?= yamllint

BUILDDIR := bin
BINARY   := emergup

.DEFAULT_GOAL := build

.PHONY: all clean test build race vet lint lint-go lint-mod lint-make lint-actions lint-markdown lint-yaml fmt check install uninstall help

all: build ## Build the binary (the default)

build: ## Build bin/emergup
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BUILDDIR)/$(BINARY) ./cmd/emergup

test: ## Run the unit tests
	$(GO) test ./...

race: ## Run the unit tests with the race detector
	$(GO) test -race ./...

vet: ## Run go vet
	$(GO) vet ./...

lint: lint-go lint-mod lint-make lint-actions lint-markdown lint-yaml ## Run every linter

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

fmt: ## Format the Go code with gofumpt
	$(GOLANGCI_LINT) fmt

check: build vet lint race ## Run the whole quality gate

install: build ## Install the binary into $(DESTDIR)$(bindir)
	$(INSTALL) -d $(DESTDIR)$(bindir)
	$(INSTALL) -m 0755 $(BUILDDIR)/$(BINARY) $(DESTDIR)$(bindir)/$(BINARY)

uninstall: ## Remove the installed binary
	rm -f $(DESTDIR)$(bindir)/$(BINARY)

clean: ## Remove the build output
	rm -rf $(BUILDDIR)

help: ## List the targets
	@awk -F ':.*## ' '/^[a-z-]+:.*## / { printf "  %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
