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
- **Docker**, for the integration tests and for hadolint and shellcheck
  (`make lint-docker`, `make lint-shell`).
- Optionally **shellcheck**, which actionlint uses to check the workflow
  scripts.

On Gentoo: `dev-lang/go`, `dev-build/make`, `dev-vcs/git`,
`net-libs/nodejs[npm]`, `dev-util/yamllint`, `app-containers/docker` with
`app-containers/docker-cli` and, optionally, `dev-util/shellcheck` or
`dev-util/shellcheck-bin`.

The first `make lint` needs the network: the Go linters are fetched and built
by `go run`, the npm tools by `npx` and the hadolint and shellcheck images by
Docker, at the versions pinned in the [Makefile](../Makefile). All are cached
afterwards.

## Building and testing

```sh
git clone https://github.com/OrbintSoft/emergup.git
cd emergup
make            # builds bin/emergup
make test       # unit tests
make check      # the whole quality gate
make integration  # the integration tests, in Docker
```

| Target | What it does |
| --- | --- |
| `build` | Builds `bin/emergup` (the default target). |
| `test` | Runs the unit tests. |
| `race` | Runs the unit tests with the race detector. |
| `vet` | Runs `go vet`, on the integration tests too. |
| `integration` | Runs the integration tests in Docker containers (see below). |
| `lint` | Runs every linter below. |
| `lint-go` | golangci-lint on the Go code, gofumpt formatting included, configured by [.golangci.yml](../.golangci.yml). |
| `lint-mod` | Checks that `go.mod` and `go.sum` are tidy. |
| `lint-make` | checkmake on the Makefile. |
| `lint-actions` | actionlint on the GitHub Actions workflows. |
| `lint-markdown` | markdownlint-cli2 on the Markdown files. |
| `lint-yaml` | yamllint on the YAML files. |
| `lint-renovate` | Validates [renovate.json5](../renovate.json5). |
| `lint-docker` | hadolint on the Dockerfile of the integration image. |
| `lint-shell` | shellcheck on the shell scripts. |
| `fmt` | Formats the Go code with gofumpt. |
| `check` | `build`, `vet`, `lint` and `race`: run it before opening a pull request. |
| `install`, `uninstall` | Installs or removes the binary under `$(DESTDIR)$(PREFIX)/bin`. |
| `dist` | Builds the release archives in `dist/` for `VERSION=x.y.z`: a static binary for linux amd64 and arm64 with the documents, the vendored Go modules, and `SHA256SUMS`. |
| `clean` | Removes the build output. |
| `help` | Lists the targets. |

Variables that can be set on the command line:

- `VERSION=x.y.z` stamps the version `emergup version` reports; without it the
  go command records the module version itself (a pseudo-version in a git
  checkout).
- `PREFIX` (default `/usr/local`) and `DESTDIR` place the installed binary.
- `GOTOOLCHAIN` chooses the Go toolchain, as above.
- `GOLANGCI_LINT`, `CHECKMAKE`, `ACTIONLINT`, `MARKDOWNLINT`,
  `RENOVATE_VALIDATE`, `YAMLLINT`, `HADOLINT` and `SHELLCHECK` replace a linter
  with another command.
- `DOCKER` (default `docker`) is the command that builds and runs the
  containers; `INTEGRATION_IMAGE` (default `emergup-integration`) names the
  image; `INTEGRATION_RUN` picks the integration tests to run, as a pattern of
  `go test -run`.

The linters run through `go run` with the build toolchain on purpose: a
golangci-lint built with an older Go cannot read the export data of a newer
one, so an older golangci-lint installed on the system may fail where
`make lint-go` works.

## Integration tests

The unit tests never run a real command. The integration tests, in
[test/integration](../test/integration) behind the `integration` build tag,
run the real emergup with the real `emerge`, `portageq` and `sudo`.
`make integration` builds a static binary, builds the image of
[test/integration/image](../test/integration/image), and runs the tests; the
first build downloads the stage3 and the Gentoo tree, over 1 GB, and later
ones reuse Docker's cache. The
[integration workflow](../.github/workflows/integration.yml) runs
`make integration` on every push to `master`, on demand, and on the pull
requests that change the Go code, `go.mod`, the Makefile or
test/integration.

Each test starts a container of its own, without a network, with the binary
mounted on `PATH`, runs emergup as `tester`, an ordinary user who may use
sudo, or as root, and checks the output, the exit status and the system. The
image holds:

- the test repository `emergup-test`, with synthetic ebuilds that install a
  file each. It syncs with rsync from its upstream, `/srv/emergup-test`, where
  a test publishes what the next sync should bring; the Gentoo tree never
  syncs. Portage syncs such a tree only when the upstream's
  `metadata/timestamp.chk` has changed, so a test publishes a snapshot: its
  files and a newer timestamp.
- `/etc/emergup/config.toml`, which points the update at `@emergup-test`, so
  that no stage3 package is ever updated. Every stage3 package is in the world
  set, so the depclean removes only what a test leaves behind.

A test is named after the feature it verifies, as in
`TestUPD6_PretendShowsTheStepsWithoutRunningThem` for UPD-6, and its comment
quotes the feature. The configuration files a test installs are files of
their own in [test/integration/testdata](../test/integration/testdata), a
directory for each set, copied into the container owned by root as a real
configuration is; a file with a problem on purpose says so in its comment.
The ebuilds a test publishes are there too, in a `snapshot-*` directory, at
their path in the repository.

## Code layout

| Path | Contents |
| --- | --- |
| `cmd/emergup` | `main`: creates the root context, gives `cli.Main` the real system (standard streams, files, commands, clock) and exits with its status; nothing else. |
| `internal/cli` | Subcommand dispatch, option parsing and the usage text, hand-written and unit-tested. |
| `internal/config` | The configuration: its types, the generic defaults, and the loader that reads `config.toml` and `config.d`. |
| `internal/runner` | Running the external commands: output shown, collected, both, or left to the terminal, copied to a log without colours, stopped cleanly on Ctrl-C. `runnertest` holds the fake the other packages' tests use. |
| `internal/elevation` | Running the commands that need root through sudo, doas or run0, or directly as root. |
| `internal/ui` | emergup's own output on the terminal: a line for each kind of message in its colour, the durations, and the questions, each answered with one key. |
| `internal/lock` | The lock file that keeps two runs of one user from overlapping. |
| `internal/rundir` | The user's state directory, refused when another user could change it; there, the lock and the directory of each run with its log, and the removal of the oldest. |
| `internal/pipeline` | Running the steps of an update in order: each announced and confirmed, how it ended and how long it took, and the summary of the run. |
| `internal/steps` | The built-in steps, in the order of the `[steps]` section, as the configuration sets them up. |
| `internal/version` | The version `emergup version` reports. |
| `test/integration` | The integration tests and the image they run in. |
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

## Releases

Every change of the Go code that reaches `master` is released. The
[release workflow](../.github/workflows/release.yml) runs on each push to
`master` that changes a `.go` file, and on demand for a change without one
that should ship, such as a dependency update. It runs the CI workflow and
the integration suite first, then tags the next version, which the commit
messages since the last tag give as conventional commits do: `feat` bumps
the minor version, anything else the patch. No commit is marked as
breaking (`!`, `BREAKING CHANGE`) before 1.0.0, which that would cut. Then
`make dist` and a GitHub release with the archives and notes from the
commits; a `0.x` version is a pre-release. When the publishing fails after
the tag, re-running its failed job publishes that tag. The vendored modules
let a distribution package build offline. The ebuild in the orbintsoft
overlay is bumped by hand, for the releases worth packaging. The
[coverage workflow](../.github/workflows/coverage.yml) publishes the coverage
of `master` for the README's badge on the orphan `badges` branch.

## Commits and pull requests

- Work on a branch; `master` changes only through pull requests. A pull request
  can be merged when the `make test` and `make lint` checks pass and every
  review thread is resolved; force pushes to `master` are refused. The
  `make integration` check runs only on the pull requests that change the
  code or the tests, so it is not a required one: look at it before merging
  such a pull request.
- Sign off every commit (`git commit -s`), as
  [CONTRIBUTING.md](../CONTRIBUTING.md) explains.
- Write [Conventional Commits](https://www.conventionalcommits.org/): `feat`,
  `fix`, `docs`, `test`, `refactor`, `build` (Makefile, tooling, Go modules),
  `ci` (workflows, bots). They choose the next version: see Releases.
- Keep a pull request to one logical change.

## Troubleshooting

- `make lint-renovate` prints a warning and a stack trace about the `re2`
  module: it is an optional native module, and the validator falls back to the
  JavaScript regular expressions; the result is still valid.
