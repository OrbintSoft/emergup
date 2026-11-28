# Dependencies

Every external dependency of emergup: what is needed to build and test it, and
what it runs on the target system. A new dependency is added here in the same
change that introduces it; licences are in [COPYRIGHT.md](COPYRIGHT.md).

Dependabot ([.github/dependabot.yml](.github/dependabot.yml)) and Renovate
([renovate.json5](renovate.json5)) open the update pull requests; a row says
when a dependency is not updated by them.

## Build and test

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| **Go** | Compiling and testing. | The `go` line in [go.mod](go.mod) is the minimum (`1.26.0`) and stays one Go release behind the latest: Renovate moves it to the next `X.Y.0` once that release is 200 days old, so its successor is out. The `toolchain` line (`go1.27.4`) is the toolchain builds use; Renovate moves it to the latest release. The [Makefile](Makefile) exports it as `GOTOOLCHAIN`, so the go command downloads it when it is not the installed one; `GOTOOLCHAIN=local` builds with the installed Go instead. |
| **GNU make** | Running the [Makefile](Makefile) targets (`make help` lists them). | Any recent version. |
| **git** | Getting the source; in a git checkout the go command also runs it to record the revision and the version in the binary. | Any recent version. |
| **golangci-lint** | Linting the Go code and formatting it with its bundled gofumpt (`make lint-go`, `make fmt`), configured by [.golangci.yml](.golangci.yml). | Pinned in the [Makefile](Makefile) and run with `go run`, so nothing has to be installed; Renovate bumps the pin. |
| **checkmake** | Linting the Makefile (`make lint-make`). | Pinned in the [Makefile](Makefile) and run with `go run`; Renovate bumps the pin. |
| **actionlint** | Linting the GitHub Actions workflows (`make lint-actions`). | Pinned in the [Makefile](Makefile) and run with `go run`; Renovate bumps the pin. |
| **shellcheck** | Linting the shell scripts (`make lint-shell`); checking the `run:` scripts of the workflows, through actionlint. | For `make lint-shell`, run from its image `koalaman/shellcheck`, pinned by tag and digest in the [Makefile](Makefile); Renovate bumps the pin. actionlint uses the one on `PATH`, when there is one: optional locally, preinstalled on the CI runner. |
| **Node.js** and **npm** | Running markdownlint-cli2 and the Renovate configuration validator through `npx`. | A version both support: markdownlint-cli2 needs Node.js ≥ 22, renovate `^24.11.0` or `^26.13.0`. |
| **markdownlint-cli2** | Linting the Markdown files (`make lint-markdown`), configured by [.markdownlint-cli2.yaml](.markdownlint-cli2.yaml). | Pinned in the [Makefile](Makefile) and run with `npx`, so nothing has to be installed; Renovate bumps the pin. |
| **renovate** (npm) | Validating [renovate.json5](renovate.json5) with `renovate-config-validator --strict` (`make lint-renovate`). | Pinned in the [Makefile](Makefile) and run with `npx`; Renovate bumps the pin. |
| **yamllint** | Linting the YAML files (`make lint-yaml`), configured by [.yamllint.yaml](.yamllint.yaml). | Taken from `PATH` (for example `dev-util/yamllint` or `pip install yamllint`), so the local version is not pinned; CI pins it in [ci.yml](.github/workflows/ci.yml), and Renovate bumps that pin. |
| **sed**, **awk**, **install** | Reading the toolchain from `go.mod`, `make help`, `make install`. | Any POSIX `sed`/`awk` and coreutils `install`. |
| **Docker** | Building the integration image and running the integration tests in its containers (`make integration`); running hadolint and shellcheck (`make lint-docker`, `make lint-shell`). | Any recent version; the legacy builder is enough. Another command that takes the arguments of docker can replace it (`DOCKER=podman`). |
| **hadolint** | Linting the Dockerfile of the integration image (`make lint-docker`). | Run from its image `hadolint/hadolint`, pinned by tag and digest in the [Makefile](Makefile); Renovate bumps the pin. |
| **tar**, **gzip**, **xz**, **sha256sum** | The release archives and their checksums (`make dist`). | GNU tar, for `--sort` and the owner options that make the archives the same on every machine; coreutils `sha256sum`. |

## Integration image

The integration tests run emergup in containers of an image built from
[test/integration/image/Dockerfile](test/integration/image/Dockerfile).

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| `gentoo/stage3` (`amd64-openrc`) | The system the integration tests run emergup in. | Pinned by its dated tag and digest in the Dockerfile; Dependabot and Renovate bump it, Renovate together with the tree. |
| `gentoo/portage` | The Gentoo tree of the image. | Pinned by its dated tag and digest in the Dockerfile; Dependabot and Renovate bump it, Renovate together with the stage3. |
| `app-admin/sudo` | The elevation of the tests run as an ordinary user. | Installed when the image is built, from the Gentoo binary host when it has the version of the tree, built otherwise; it follows the tree. |

## Continuous integration

[.github/workflows/ci.yml](.github/workflows/ci.yml) and
[.github/workflows/integration.yml](.github/workflows/integration.yml) run the
Makefile targets on GitHub-hosted `ubuntu-latest` runners. The image moves with
GitHub and is not pinned: the tools that decide a result are pinned in the
workflows or in the Makefile, and only helpers such as Docker, pipx and
shellcheck come from the image.

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| `actions/checkout` | Checking out the repository. | Pinned to a commit SHA in the workflows; Dependabot and Renovate bump it. |
| `actions/setup-go` | Installing the toolchain named in [go.mod](go.mod). | Pinned to a commit SHA in the workflows; Dependabot and Renovate bump it. |
| `mathieudutour/github-tag-action` | Choosing the next version from the commits since the last tag, read as conventional commits, and creating its tag, in the release workflow. | Pinned to a commit SHA; Dependabot and Renovate bump it. |
| **GitHub CLI** (`gh`) | Publishing a release with its archives and notes, in the release workflow. | Preinstalled on the runner. |
| `actions/setup-node` | Installing Node.js for markdownlint-cli2 and the Renovate configuration validator. | Pinned to a commit SHA; Dependabot and Renovate bump it. The Node.js version is the latest LTS. |
| **pipx** | Installing yamllint at the pinned version. | Preinstalled on the runner. |
| **Docker** | Building the integration image and running its containers (`make integration`), and running hadolint and shellcheck (`make lint`). | Preinstalled on the runner. |
| **DCO** GitHub App | Checking that every commit of a pull request is signed off; commits by bots are exempt. | Installed on the repository; maintained by its authors. |
| **Dependabot** | Opening update pull requests for the Go modules, the actions and the images of the integration Dockerfile, one per ecosystem, on Saturday mornings. | Configured by [.github/dependabot.yml](.github/dependabot.yml); part of GitHub. |
| **Renovate** GitHub App (Mend-hosted) | Opening update pull requests for go.mod, the actions, the images of the integration Dockerfile (grouped in one) and the tool pins in the Makefile and the workflow (actions and tool pins grouped in one), on Thursday afternoons. | Configured by [renovate.json5](renovate.json5); installed on the repository, so it needs no token or workflow. |

## Go modules

| Module | Purpose | Version and updates |
| --- | --- | --- |
| `github.com/pelletier/go-toml/v2` | Reading the TOML configuration files; compiled into the binary. It has no dependencies of its own. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `golang.org/x/term` | Reading the answer to a question one key at a time, with the terminal in raw mode only while the key is read; compiled into the binary. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `golang.org/x/sys` | Discarding the keys typed before a question, and required by `golang.org/x/term`; compiled into the binary. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `github.com/stretchr/testify` | Assertions in the unit tests; not compiled into the binary. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `go.yaml.in/yaml/v3` | Required by testify (indirect). | Pinned in [go.mod](go.mod); follows testify. |

## Runtime

The commands emergup runs on the system it updates. They are the system's
own: emergup does not pin or update them.

| Command | Purpose | Package |
| --- | --- | --- |
| `emerge` | Running the steps of the update; the command line of each step can be changed in the configuration. For the failure report, `emerge --info` and `emerge -pqv` for each package that failed, unprivileged. | `sys-apps/portage` |
| `portageq` | Asking Portage about the system, read-only: unprivileged for the repositories (`repos_config`), the package database (`vdb_path`) and the protected configuration paths (`envvar`); elevated for the preserved libraries (`list_preserved_libs`), since only root and the portage group can read their registry. | `sys-apps/portage` |
| `smart-live-rebuild` | Listing the live packages whose upstream changed (`--pretend`), elevated, in the smart mode of the live-package rebuild; optional: without it the rebuild goes by the time of the last build. | `app-portage/smart-live-rebuild` |
| `dispatch-conf` | Merging the pending configuration-file updates, the default tool of the config-files step; `etc-update` can be configured instead. | `sys-apps/portage` |
| `find` | Looking for the pending configuration-file updates, elevated since some protected directories can be read by root alone; read-only. | `sys-apps/findutils` |
| `eselect` | Showing the unread news items and marking them read (`eselect news read new`), the default command of the news step and of `emergup news`: as the invoking user when they can write Portage's record of the news read, elevated otherwise. | `app-admin/eselect` |
| `cat` | Copying the build log of a package that failed into the failure report, elevated and read-only, since only Portage's own user can read it. | `sys-apps/coreutils` |
| `sudo`, `doas` or `run0` | Running the commands that need root, when emergup is not root; the first one installed, or the one the configuration names. | `app-admin/sudo`, `app-admin/doas`, `sys-apps/systemd` |
