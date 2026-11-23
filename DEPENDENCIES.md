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
| **shellcheck** | Checking the `run:` scripts of the workflows, through actionlint. | Optional locally: actionlint uses it when it is on `PATH`. |
| **Node.js** and **npm** | Running markdownlint-cli2 and the Renovate configuration validator through `npx`. | A version both support: markdownlint-cli2 needs Node.js ≥ 22, renovate `^24.11.0` or `^26.13.0`. |
| **markdownlint-cli2** | Linting the Markdown files (`make lint-markdown`), configured by [.markdownlint-cli2.yaml](.markdownlint-cli2.yaml). | Pinned in the [Makefile](Makefile) and run with `npx`, so nothing has to be installed; Renovate bumps the pin. |
| **renovate** (npm) | Validating [renovate.json5](renovate.json5) with `renovate-config-validator --strict` (`make lint-renovate`). | Pinned in the [Makefile](Makefile) and run with `npx`; Renovate bumps the pin. |
| **yamllint** | Linting the YAML files (`make lint-yaml`), configured by [.yamllint.yaml](.yamllint.yaml). | Taken from `PATH` (for example `dev-util/yamllint` or `pip install yamllint`), so the local version is not pinned; CI pins it in [ci.yml](.github/workflows/ci.yml), and Renovate bumps that pin. |
| **sed**, **awk**, **install** | Reading the toolchain from `go.mod`, `make help`, `make install`. | Any POSIX `sed`/`awk` and coreutils `install`. |

## Continuous integration

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs the Makefile targets
on GitHub-hosted `ubuntu-latest` runners. The image moves with GitHub and is
not pinned: the tools that decide a result are pinned in the workflow or in the
Makefile, and only helpers such as pipx and shellcheck come from the image.

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| `actions/checkout` | Checking out the repository. | Pinned to a commit SHA in the workflow; Dependabot and Renovate bump it. |
| `actions/setup-go` | Installing the toolchain named in [go.mod](go.mod). | Pinned to a commit SHA in the workflow; Dependabot and Renovate bump it. |
| `actions/setup-node` | Installing Node.js for markdownlint-cli2 and the Renovate configuration validator. | Pinned to a commit SHA; Dependabot and Renovate bump it. The Node.js version is the latest LTS. |
| **pipx** | Installing yamllint at the pinned version. | Preinstalled on the runner. |
| **DCO** GitHub App | Checking that every commit of a pull request is signed off; commits by bots are exempt. | Installed on the repository; maintained by its authors. |
| **Dependabot** | Opening update pull requests for the Go modules and the actions, one per ecosystem, on Saturday mornings. | Configured by [.github/dependabot.yml](.github/dependabot.yml); part of GitHub. |
| **Renovate** GitHub App (Mend-hosted) | Opening update pull requests for go.mod, the actions and the tool pins in the Makefile and the workflow (actions and tool pins grouped in one), on Thursday afternoons. | Configured by [renovate.json5](renovate.json5); installed on the repository, so it needs no token or workflow. |

## Go modules

| Module | Purpose | Version and updates |
| --- | --- | --- |
| `github.com/pelletier/go-toml/v2` | Reading the TOML configuration files; compiled into the binary. It has no dependencies of its own. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `github.com/stretchr/testify` | Assertions in the unit tests; not compiled into the binary. | Pinned in [go.mod](go.mod); Dependabot and Renovate bump it. |
| `go.yaml.in/yaml/v3` | Required by testify (indirect). | Pinned in [go.mod](go.mod); follows testify. |

## Runtime

None yet.
