# Dependencies

Every external dependency of emergup: what is needed to build and test it, and
what it runs on the target system. A new dependency is added here in the same
change that introduces it; licences are in [COPYRIGHT.md](COPYRIGHT.md).

## Build and test

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| **Go** | Compiling and testing. | The `go` line in [go.mod](go.mod) is the minimum (`1.26.0`) and stays one Go release behind the latest; the `toolchain` line (`go1.27.4`) is the toolchain builds use and follows the latest. The [Makefile](Makefile) exports it as `GOTOOLCHAIN`, so the go command downloads it when it is not the installed one; `GOTOOLCHAIN=local` builds with the installed Go instead. |
| **GNU make** | Running the [Makefile](Makefile) targets (`make help` lists them). | Any recent version. |
| **golangci-lint** | Linting the Go code and formatting it with its bundled gofumpt (`make lint-go`, `make fmt`), configured by [.golangci.yml](.golangci.yml). | Pinned in the [Makefile](Makefile) and run with `go run`, so nothing has to be installed. |
| **checkmake** | Linting the Makefile (`make lint-make`). | Pinned in the [Makefile](Makefile) and run with `go run`. |
| **actionlint** | Linting the GitHub Actions workflows (`make lint-actions`). | Pinned in the [Makefile](Makefile) and run with `go run`. |
| **shellcheck** | Checking the `run:` scripts of the workflows, through actionlint. | Optional locally: actionlint uses it when it is on `PATH`. |
| **Node.js** ≥ 22 and **npm** | Running markdownlint-cli2 through `npx`. | Any version that markdownlint-cli2 supports. |
| **markdownlint-cli2** | Linting the Markdown files (`make lint-markdown`), configured by [.markdownlint-cli2.yaml](.markdownlint-cli2.yaml). | Pinned in the [Makefile](Makefile) and run with `npx`, so nothing has to be installed. |
| **yamllint** | Linting the YAML files (`make lint-yaml`), configured by [.yamllint.yaml](.yamllint.yaml). | Taken from `PATH` (for example `dev-python/yamllint` or `pip install yamllint`), so the local version is not pinned; CI pins it in [ci.yml](.github/workflows/ci.yml). |
| **sed**, **awk**, **install** | Reading the toolchain from `go.mod`, `make help`, `make install`. | Any POSIX `sed`/`awk` and coreutils `install`. |

## Continuous integration

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs the Makefile targets
on GitHub-hosted `ubuntu-latest` runners.

| Dependency | Purpose | Version and updates |
| --- | --- | --- |
| `actions/checkout` | Checking out the repository. | Pinned to a commit SHA in the workflow. |
| `actions/setup-go` | Installing the toolchain named in [go.mod](go.mod). | Pinned to a commit SHA in the workflow. |
| `actions/setup-node` | Installing Node.js for markdownlint-cli2. | Pinned to a commit SHA; the Node.js version is the latest LTS. |
| **pipx** | Installing yamllint at the pinned version. | Preinstalled on the runner. |
| **DCO** GitHub App | Checking that every commit of a pull request is signed off. | Installed on the repository; maintained by its authors. |

## Go modules

| Module | Purpose | Version and updates |
| --- | --- | --- |
| `github.com/stretchr/testify` | Assertions in the unit tests; not compiled into the binary. | Pinned in [go.mod](go.mod). |
| `go.yaml.in/yaml/v3` | Required by testify (indirect). | Pinned in [go.mod](go.mod). |

Updates are not automated yet: until Dependabot and Renovate are configured,
the versions pinned in `go.mod`, in the Makefile and in the workflow are
bumped by hand.

## Runtime

None yet.
