# Dependencies

Every external dependency of emergup: what is needed to build and test it, and
what it runs on the target system. A new dependency is added here in the same
change that introduces it; licences are in [COPYRIGHT.md](COPYRIGHT.md).

## Build and test

| Dependency | Purpose | Version and updates |
|---|---|---|
| **Go** | Compiling and testing. | The `go` line in [go.mod](go.mod) is the minimum (`1.26.0`) and stays one Go release behind the latest; the `toolchain` line (`go1.27.4`) is the toolchain builds use and follows the latest. The [Makefile](Makefile) exports it as `GOTOOLCHAIN`, so the go command downloads it when it is not the installed one; `GOTOOLCHAIN=local` builds with the installed Go instead. |
| **GNU make** | Running the [Makefile](Makefile) targets (`make help` lists them). | Any recent version. |
| **golangci-lint** | Linting the Go code and formatting it with its bundled gofumpt (`make lint-go`, `make fmt`), configured by [.golangci.yml](.golangci.yml). | Pinned in the [Makefile](Makefile) and run with `go run`, so nothing has to be installed. |
| **checkmake** | Linting the Makefile (`make lint-make`). | Pinned in the [Makefile](Makefile) and run with `go run`. |
| **sed**, **awk**, **install** | Reading the toolchain from `go.mod`, `make help`, `make install`. | Any POSIX `sed`/`awk` and coreutils `install`. |

## Go modules

| Module | Purpose | Version and updates |
|---|---|---|
| `github.com/stretchr/testify` | Assertions in the unit tests; not compiled into the binary. | Pinned in [go.mod](go.mod). |
| `go.yaml.in/yaml/v3` | Required by testify (indirect). | Pinned in [go.mod](go.mod). |

Updates are not automated yet: until Dependabot and Renovate are configured,
the versions pinned in `go.mod` and in the Makefile are bumped by hand.

## Runtime

None yet.
