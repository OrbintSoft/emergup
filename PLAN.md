# Plan — emergup

Roadmap from an empty repository to a released tool that replaces a hand-written
Gentoo update script. Rules for assistants live in [CLAUDE.md](CLAUDE.md); what
the tool promises is in [docs/FEATURES.md](docs/FEATURES.md).

**Status legend:** `[ ]` todo · `[~]` in progress · `[x]` done

**Invariant:** every item leaves the repo committable when done. Large items are
split into sub-steps in a gitignored `<activity>-steps.md`.

---

## Decisions taken

- **Language Go**, module `github.com/OrbintSoft/emergup`, binary `emergup`.
  Authorized languages: Go; Bash for example hooks, test harness and CI glue.
  Anything else needs the maintainer's approval first.
- **Go versions:** the `go` line in `go.mod` is the minimum supported Go
  (`1.26.0`) and is kept one Go release behind the latest; the `toolchain` line
  is the toolchain builds use (`go1.27.4`) and follows the latest release.
  Renovate moves the `go` line to the next `X.Y.0` once that release is 200
  days old (Go ships every six months, so its successor is out) and the
  `toolchain` line to the latest release.
- **Licence `GPL-3.0-or-later`**; contributions under the **DCO only** (no CLA).
- **Configuration:** TOML, **system-wide only**: `/etc/emergup/config.toml` plus
  `/etc/emergup/config.d/*.toml`, always supported, read in lexical order.
  Drop-ins avoid editing a shared file, so there are no merge conflicts between
  packages or admins. No per-user configuration.
- **Privileges:** the process runs as the invoking user; only privileged
  commands go through an elevation wrapper — auto-detected
  (`sudo` → `doas` → `run0`) or set in the configuration. Hooks and AI run
  unprivileged.
- **CLI without a library:** subcommand dispatch, option parsing and the usage
  text are hand-written in `internal/cli`, so all of it is unit-tested;
  `cmd/emergup` holds only `os.Exit(cli.Main(ctx, …))`.
- **Interface:** a step-by-step CLI with confirmations and colours (honouring
  `NO_COLOR`), `--yes` for unattended runs, and subcommands (`run`, `news`,
  `config`, `version`, …).
- **AI:** pluggable backends behind one interface, off by default. Order:
  Claude Code CLI (`claude`) first, then the Anthropic API (official Go SDK,
  default model `claude-opus-5-5`), then a generic "prompt on stdin" command for
  any other LLM tool. Never run as root; the user sees what is sent first.
- **Tests:** unit tests with testify and fakes for every side effect;
  integration tests in Docker (`gentoo/stage3`), against a local test repository
  with synthetic ebuilds and news items so they are offline and deterministic.
- **Dependency bots:** Dependabot and Renovate both, on different days and hours
  (Europe/Rome): Renovate on Thursday from 12:00 to midnight, a half-day window
  so the hosted app's run lands inside it, for go.mod, the actions and the
  tool pins in the Makefile and the workflow; Dependabot on Saturday at 06:00
  for the Go modules and the actions. Updates are grouped (Dependabot one pull
  request per ecosystem, Renovate one for the actions and the tool pins), and
  the days are chosen so that the maintainer's repositories do not all open
  pull requests on the same day. Bot commits carry no sign-off: the DCO app
  exempts bots.
- **Repository:** public on GitHub, `OrbintSoft/emergup`, branch `master`;
  sponsors as for the other OrbintSoft repos (GitHub Sponsors, PayPal, Gentoo
  donations).
- **Branch protection:** the "Protect master" ruleset on the default branch,
  with no bypass: changes only through pull requests with every review thread
  resolved and the `make test` and `make lint` checks passing (from GitHub
  Actions; the branch need not be up to date); no force push, no deletion.
  The DCO check is added to it once the DCO app is installed.
- **Linters (CLAUDE.md rule 19):** `make lint` runs golangci-lint on Go,
  checkmake on the Makefile, markdownlint-cli2 on Markdown, yamllint on YAML,
  actionlint on the workflows, `go mod tidy -diff` on `go.mod`/`go.sum` and
  `renovate-config-validator --strict` on `renovate.json5` (the only JSON5
  file; no general JSON5 linter).
  TOML (taplo), shell (shellcheck) and Dockerfile (hadolint) linters are
  evaluated when those file types arrive.

## Open questions

1. ~~**CLI library**~~ — answered: hand-written, see Decisions taken.
2. **List merging across `config.d`:** does a drop-in's `exclude = [...]` append
   to the earlier lists or replace them, and how is an entry removed? (Phase 2)
3. **Where run logs and failure reports live:** the user's state directory
   (`~/.local/state/emergup/`) or `/var/log/emergup/` (needs root)? (Phase 3)
4. **emerge `--ask`:** keep emerge's own prompt, or run `--pretend` first and ask
   in emergup? (Phase 4)
5. **Running as root** (root shell, cron): allowed? If so, AI is disabled.
   (Phase 3)
6. **Applying AI fixes:** emergup applies a proposed change after confirmation,
   or hands off to an interactive Claude Code session? (Phase 9)
7. **`haskell-updater`:** built-in optional step skipped when not installed
   (proposed), or only an example custom step? (Phase 4)

---

## Phase 0 — Project definition  `[x]`

- [x] **0.1** Repository skeleton: README draft, LICENSE, COPYRIGHT, AUTHORS,
      CONTRIBUTING + DCO, CLAUDE.md, PLAN.md, docs/FEATURES.md, FUNDING.yml,
      `.gitignore` / `.gitattributes` / `.editorconfig`. No code. Landed as an
      initial commit plus one small PR per topic.
- [x] **0.2** Create the public GitHub repository, push the first commit; set the
      description and topics; enable the Sponsor button (repo settings).

## Phase 1 — Scaffolding and quality tooling  `[~]`

- [x] **1.1** `go.mod` (minimum Go and build toolchain), `cmd/emergup/main.go`
      and `internal/cli` with `emergup version` and `help` (CLI-1, CLI-2); SPDX
      headers; testify; a first `DEPENDENCIES.md`.
- [x] **1.2** Makefile (`build`, `test`, `race`, `vet`, `lint`, `fmt`, `check`,
      `install`/`uninstall` with `PREFIX`/`DESTDIR` for an ebuild, `help`), built
      with the `go.mod` toolchain; linters pinned and run with `go run`. checkmake
      passes with its defaults, so there is no `checkmake.ini`.
- [x] **1.3** `.golangci.yml` (starting from redo-backups'): every linter on, a
      few disabled with the reason; gofumpt as the formatter.
- [x] **1.4** markdownlint-cli2 (pinned, through `npx`) and yamllint (from
      `PATH`) wired into `make lint`, each with its configuration file.
- [~] **1.5** CI workflow running the Makefile targets: build, vet, test
      (race) and `make lint` (golangci-lint, `go mod tidy`, checkmake,
      actionlint, markdownlint-cli2, yamllint pinned there); DCO sign-off check
      on pull requests by the DCO GitHub App. Pending: the maintainer installs
      the app; its check then joins the master ruleset.
- [x] **1.6** Dependabot (`.github/dependabot.yml`) and Renovate
      (`renovate.json5`, with custom managers for the tool pins) on different
      days and hours; the Renovate configuration is validated by
      `make lint-renovate`, part of `make lint` and so of CI. Renovate keeps
      the `go` line one Go release behind the latest and moves `toolchain` to
      the latest. Checked with a local `renovate --platform=local` dry run.
- [x] **1.7** `DEPENDENCIES.md` completed with the tooling, and
      `docs/DEVELOPMENT.md` (requirements, targets, layout, conventions,
      dependencies, pull requests).

## Phase 2 — Configuration  `[ ]`  (CFG-*)

- [ ] **2.1** Configuration schema as Go types, with generic defaults (CFG-2).
- [ ] **2.2** Loader: `config.toml` + `config.d/*.toml` in lexical order, merge
      rules (open question 2) (CFG-1).
- [ ] **2.3** Validation with file and line, unknown keys rejected (CFG-3).
- [ ] **2.4** `emergup config check` and `emergup config show` (CFG-3, CFG-4).
- [ ] **2.5** Example configuration under `examples/etc/emergup/`.

## Phase 3 — Core engine  `[ ]`  (PRIV-*, UPD-2, UPD-6–8)

- [ ] **3.1** Command runner interface (streamed output, tee to a log) and a fake
      for tests.
- [ ] **3.2** Elevation: auto-detection, configuration override, no-op as root
      (PRIV-1, PRIV-2).
- [ ] **3.3** Terminal output and prompts: step headings, colours, confirmations,
      `--yes` (UPD-2).
- [ ] **3.4** Step model and pipeline: enable/disable, `--pretend`, final summary
      (UPD-3, UPD-6, UPD-7).
- [ ] **3.5** Single-instance lock and run directory (UPD-8; open questions 3, 5).

## Phase 4 — Built-in steps: parity with the old script (MVP)  `[ ]`

- [ ] **4.1** Sync (`emaint --auto sync`, configurable), per repository: a
      repository synced within the minimum interval is skipped, unless it is
      listed as always synced or `--force-sync` is given (SYNC-1–3). The last
      sync time is read from the repository on disk (git: `.git/FETCH_HEAD`,
      rsync: `metadata/timestamp.chk`), so syncs done outside emergup count
      and no state file is needed; other sync types are always synced.
- [ ] **4.2** `@world` update with configurable options and exclusions (EXC-1).
- [ ] **4.3** Live-package rebuild (`smart-live-rebuild`, skipped if absent) (UPD-4).
- [ ] **4.4** Preserved-library rebuild, only when needed; `haskell-updater`
      (UPD-5; open question 7).
- [ ] **4.5** Config-file updates delegated to `dispatch-conf` (ETC-3).
- [ ] **4.6** Depclean.
- [ ] **4.7** Held-back report for excluded packages (EXC-2).
- [ ] **4.8** First tagged pre-release `v0.1.0`.
- [ ] **4.9** Ebuild `app-portage/emergup` in the orbintsoft overlay
      (`orbintsoft-ebuild` repository), from `v0.1.0` on: the tool is
      packaged as soon as it can be used, and each pre-release bumps it.

## Phase 5 — Docker integration tests  `[ ]`

- [ ] **5.1** Harness: `gentoo/stage3` image (digest pinned, tracked by Renovate),
      local test repository with synthetic ebuilds and news, `make integration`.
- [ ] **5.2** Integration tests for the Phase 2–4 features, each naming its id.
- [ ] **5.3** CI job for the integration suite.

From here on, every feature lands with its integration test.

## Phase 6 — Hooks and custom steps  `[ ]`  (HOOK-*)

- [ ] **6.1** Hooks before/after the run and each step, with context in the
      environment (HOOK-1, HOOK-4).
- [ ] **6.2** Check hooks with an optional remedy and re-check (HOOK-2).
- [ ] **6.3** Custom steps (HOOK-3).
- [ ] **6.4** Example hook: backup freshness check (generic: age of a file or of
      a command's output), with a remedy that starts the configured backup tool.

## Phase 7 — News  `[ ]`  (NEWS-*)

- [ ] **7.1** Show unread news after the sync and confirm before updating (NEWS-1).
- [ ] **7.2** `emergup news` (NEWS-2).

## Phase 8 — Failure diagnostics  `[ ]`  (DIAG-*)

- [ ] **8.1** Detect the failed package and collect the failure report (DIAG-1).
- [ ] **8.2** Print where the report is (DIAG-2).

## Phase 9 — AI assistance  `[ ]`  (AI-*)

- [ ] **9.1** Backend interface, configuration, "what will be sent" confirmation,
      refusal to run as root (AI-1, AI-2, AI-6).
- [ ] **9.2** Claude Code backend.
- [ ] **9.3** Explain news (AI-3).
- [ ] **9.4** Analyse a failure and propose a fix (AI-4; open question 6).
- [ ] **9.5** Anthropic API backend (official Go SDK).
- [ ] **9.6** Generic command backend.

## Phase 10 — Config-file updates without dispatch-conf  `[ ]`  (ETC-*)

- [ ] **10.1** Find pending `._cfg*` files under `CONFIG_PROTECT` (ETC-1).
- [ ] **10.2** Per-file actions with configurable diff and merge tools, applied
      through elevation, with a backup of the replaced file (ETC-2).
- [ ] **10.3** AI explanation and proposed merge (AI-5).

## Phase 11 — Documentation, packaging, release  `[ ]`

- [ ] **11.1** User docs: configuration, hooks, AI, installation; README overhaul.
- [ ] **11.2** GoReleaser and release workflow.
- [ ] **11.3** Ebuild bumped to the first stable release.
- [ ] **11.4** Point the old script to emergup and retire it.
