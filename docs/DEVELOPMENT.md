# Development

How to build, test and lint emergup, and how a change gets into `master`. The
licence and sign-off terms are in [CONTRIBUTING.md](../CONTRIBUTING.md); every
tool mentioned here, with its version, is in
[DEPENDENCIES.md](../DEPENDENCIES.md).

## Requirements

- **Go**, any release that can download toolchains (1.21 or later). The
  Makefile builds with the toolchain named by the `toolchain` line of
  [go.mod](../go.mod) and the go command downloads it on first use; with
  `GOTOOLCHAIN=local` the installed Go is used instead, and it must be at least
  the `go` line of go.mod.
- **GNU make** and **git**.
- **Node.js** with **npm**, for markdownlint-cli2 and the Renovate
  configuration validator.
- **yamllint** on `PATH`.
- Optionally **shellcheck**, which actionlint uses to check the workflow
  scripts.

On Gentoo: `dev-lang/go`, `dev-build/make`, `dev-vcs/git`,
`net-libs/nodejs[npm]`, `dev-util/yamllint` and, optionally,
`dev-util/shellcheck` or `dev-util/shellcheck-bin`.

The first `make lint` needs the network: the Go linters are fetched and built
by `go run`, and the npm tools by `npx`, at the versions pinned in the
[Makefile](../Makefile). Both are cached afterwards.

## Building and testing

```sh
git clone https://github.com/OrbintSoft/emergup.git
cd emergup
make            # builds bin/emergup
make test       # unit tests
make check      # the whole quality gate
```

| Target | What it does |
| --- | --- |
| `build` | Builds `bin/emergup` (the default target). |
| `test` | Runs the unit tests. |
| `race` | Runs the unit tests with the race detector. |
| `vet` | Runs `go vet`. |
| `lint` | Runs every linter below. |
| `lint-go` | golangci-lint on the Go code, gofumpt formatting included, configured by [.golangci.yml](../.golangci.yml). |
| `lint-mod` | Checks that `go.mod` and `go.sum` are tidy. |
| `lint-make` | checkmake on the Makefile. |
| `lint-actions` | actionlint on the GitHub Actions workflows. |
| `lint-markdown` | markdownlint-cli2 on the Markdown files. |
| `lint-yaml` | yamllint on the YAML files. |
| `lint-renovate` | Validates [renovate.json5](../renovate.json5). |
| `fmt` | Formats the Go code with gofumpt. |
| `check` | `build`, `vet`, `lint` and `race`: run it before opening a pull request. |
| `install`, `uninstall` | Installs or removes the binary under `$(DESTDIR)$(PREFIX)/bin`. |
| `clean` | Removes the build output. |
| `help` | Lists the targets. |

Variables that can be set on the command line:

- `VERSION=x.y.z` stamps the version `emergup version` reports; without it the
  go command records the module version itself (a pseudo-version in a git
  checkout).
- `PREFIX` (default `/usr/local`) and `DESTDIR` place the installed binary.
- `GOTOOLCHAIN` chooses the Go toolchain, as above.
- `GOLANGCI_LINT`, `CHECKMAKE`, `ACTIONLINT`, `MARKDOWNLINT`,
  `RENOVATE_VALIDATE` and `YAMLLINT` replace a linter with another command.

The linters run through `go run` with the build toolchain on purpose: a
golangci-lint built with an older Go cannot read the export data of a newer
one, so an older golangci-lint installed on the system may fail where
`make lint-go` works.

## Code layout

| Path | Contents |
| --- | --- |
| `cmd/emergup` | `main`: creates the root context and calls `cli.Main`; nothing else. |
| `internal/cli` | Subcommand dispatch, option parsing and the usage text, hand-written and unit-tested. |
| `internal/config` | The configuration: its types, the generic defaults, and the loader that reads `config.toml` and `config.d`. |
| `internal/runner` | Running the external commands: output shown, collected or left to the terminal, copied to a log, stopped cleanly on Ctrl-C. `runnertest` holds the fake the other packages' tests use. |
| `internal/version` | The version `emergup version` reports. |
| `examples/etc/emergup` | The example configuration; `make test` checks that it loads and that the main file shows the generic defaults. |
| `docs/FEATURES.md` | What emergup promises its users, one entry per feature. |

## Writing a change

The binding list of working and design rules is in
[CLAUDE.md](../CLAUDE.md): it is written for AI assistants and recommended for
everyone. In practice:

- **Features first.** A user-visible behaviour is an entry of
  [docs/FEATURES.md](FEATURES.md) with a stable id; add or change the entry in
  the same change as the code. Integration tests name the id they verify.
- **Side effects behind interfaces.** Running commands, elevation, reading
  the Portage state and touching files go through small interfaces, so the
  logic is unit-tested with fakes. A unit test never runs a real system
  command (`emerge`, `sudo`, `eselect`, …).
- **Contexts.** `main` creates the only root context; every function below it
  takes a `context.Context` first and uses it. Tests use `t.Context()`.
- **Tests** use [testify](https://github.com/stretchr/testify): `require` when
  continuing makes no sense, `assert` otherwise. A new test is seen failing
  before it is committed passing.
- **Generic defaults.** emergup must work on any Gentoo system: a default never
  encodes one machine's packages, paths or tools; personal choices belong in
  example configuration.
- **Licence header.** A new source file starts with
  `SPDX-License-Identifier: GPL-3.0-or-later`.
- **No personal data.** Committed files never contain hostnames, user names,
  installed-package lists, `emerge --info` output or real logs; test fixtures
  use synthetic values.

## Adding a dependency

1. Check that its licence is compatible with `GPL-3.0-or-later` (MIT, BSD,
   Apache-2.0, MPL-2.0 and LGPL are; GPL-2.0-only is not).
2. List a Go module in [COPYRIGHT.md](../COPYRIGHT.md), and any dependency —
   module, tool, action, image or command run by emergup — in
   [DEPENDENCIES.md](../DEPENDENCIES.md).
3. Make sure Dependabot or Renovate updates it. A tool version pinned in a new
   place may need a custom manager in [renovate.json5](../renovate.json5); if
   neither bot can update it, say so in DEPENDENCIES.md.
4. A new kind of file in the repository deserves a look for a linter to wire
   into `make lint`.

## Dependency updates

Renovate (Thursday afternoon) and Dependabot (Saturday morning) open the update
pull requests; they run on different days and group their updates, so a week
brings only a few of them. Renovate's Dependency Dashboard issue lists what is
pending and can force an update out of schedule. The `go` line of go.mod is
moved by Renovate only once a Go release has a successor, so it stays one
release behind the latest; the `toolchain` line follows the latest release.

## Commits and pull requests

- Work on a branch; `master` changes only through pull requests. A pull request
  can be merged when the `make test` and `make lint` checks pass and every
  review thread is resolved; force pushes to `master` are refused.
- Sign off every commit (`git commit -s`), as
  [CONTRIBUTING.md](../CONTRIBUTING.md) explains.
- Write [Conventional Commits](https://www.conventionalcommits.org/): `feat`,
  `fix`, `docs`, `test`, `refactor`, `build` (Makefile, tooling, Go modules),
  `ci` (workflows, bots).
- Keep a pull request to one logical change.

## Troubleshooting

- `make lint-renovate` prints a warning and a stack trace about the `re2`
  module: it is an optional native module, and the validator falls back to the
  JavaScript regular expressions; the result is still valid.
