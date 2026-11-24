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
  packages or admins. No per-user configuration. Keys are kebab-case, as in
  Portage's `repos.conf`; commands are argument lists, never shell strings;
  durations use Go's syntax (`24h`). Lists of entries add up across files like
  `USE` (`-entry` removes, `-*` clears); other values are replaced (CFG-5).
  The files are read with go-toml/v2 (MIT, no dependencies), which gives the
  line of an unknown key or a bad value (CFG-3); BurntSushi/toml lists unknown
  keys without their position.
- **Privileges:** the process runs as the invoking user; only privileged
  commands go through an elevation wrapper — auto-detected
  (`sudo` → `doas` → `run0`) or set in the configuration. Hooks and AI run
  unprivileged. Started as root, emergup runs the commands directly and turns
  AI off (PRIV-4).
- **Run directory:** each run writes its log, and later its failure report,
  in a directory of its own under the invoking user's state directory,
  `$XDG_STATE_HOME/emergup/runs/` (by default `~/.local/state/emergup/runs/`;
  root's is under `/root`). Writing there needs no elevation, and the user,
  like the AI backends that run as the user, reads the logs and reports
  without `sudo`; `/var/log/emergup/` would need root for every write.
- **CLI without a library:** subcommand dispatch, option parsing and the usage
  text are hand-written in `internal/cli`, so all of it is unit-tested;
  `cmd/emergup` holds only `os.Exit(cli.Main(ctx, …))`.
- **Interface:** a step-by-step CLI with confirmations and colours (honouring
  `NO_COLOR`), `--yes` for unattended runs, and subcommands (`run`, `news`,
  `config`, `version`, …). emergup asks, not emerge: it runs `--pretend`
  first and confirms itself, and never passes `--ask` (UPD-10).
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
  resolved and the required checks passing (`make test` and `make lint` from
  GitHub Actions, `DCO` from the DCO app; the branch need not be up to date);
  no force push, no deletion.
- **Linters (CLAUDE.md rule 19):** `make lint` runs golangci-lint on Go,
  checkmake on the Makefile, markdownlint-cli2 on Markdown, yamllint on YAML,
  actionlint on the workflows, `go mod tidy -diff` on `go.mod`/`go.sum` and
  `renovate-config-validator --strict` on `renovate.json5` (the only JSON5
  file; no general JSON5 linter). TOML has no linter: the only TOML files are
  the example configuration, which `make test` loads with emergup's own strict
  loader (syntax, unknown keys, types, values) and compares with the defaults
  in the code, more than a generic TOML linter checks; they are formatted by
  hand. Shell (shellcheck) and Dockerfile (hadolint) linters are evaluated
  when those file types arrive.

## Open questions

1. ~~**CLI library**~~ — answered: hand-written, see Decisions taken.
2. ~~**List merging across `config.d`**~~ — answered: lists add up like `USE`,
   see Decisions taken (CFG-5).
3. ~~**Where run logs and failure reports live**~~ — answered: in the
   invoking user's state directory, see Decisions taken.
4. ~~**emerge `--ask`**~~ — answered: `--pretend` first and emergup asks, see
   Decisions taken (UPD-10).
5. ~~**Running as root**~~ — answered: allowed, see Decisions taken (PRIV-4).
6. **Applying AI fixes:** emergup applies a proposed change after confirmation,
   or hands off to an interactive Claude Code session? (Phase 9) Proposed in
   9.0: both, one per action.
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

## Phase 1 — Scaffolding and quality tooling  `[x]`

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
- [x] **1.5** CI workflow running the Makefile targets: build, vet, test
      (race) and `make lint` (golangci-lint, `go mod tidy`, checkmake,
      actionlint, markdownlint-cli2, yamllint pinned there); DCO sign-off check
      on pull requests by the DCO GitHub App, required by the master ruleset.
- [x] **1.6** Dependabot (`.github/dependabot.yml`) and Renovate
      (`renovate.json5`, with custom managers for the tool pins) on different
      days and hours; the Renovate configuration is validated by
      `make lint-renovate`, part of `make lint` and so of CI. Renovate keeps
      the `go` line one Go release behind the latest and moves `toolchain` to
      the latest. Checked with a local `renovate --platform=local` dry run.
- [x] **1.7** `DEPENDENCIES.md` completed with the tooling, and
      `docs/DEVELOPMENT.md` (requirements, targets, layout, conventions,
      dependencies, pull requests).

## Phase 2 — Configuration  `[x]`  (CFG-*)

- [x] **2.1** Configuration schema as Go types, with generic defaults (CFG-2),
      in `internal/config`: elevation and the built-in steps of Phase 4.
      Hooks, news and AI add their sections in their phases.
- [x] **2.2** Loader: `config.toml` + `config.d/*.toml` in lexical order, lists
      merged like `USE` (CFG-1, CFG-5), through an `fs.FS` so tests use an
      in-memory file system. TOML read with go-toml/v2; durations and the
      named values (elevation method, live-rebuild mode) decode through
      `UnmarshalText`. Errors name the file; their line and key come in 2.3.
- [x] **2.3** Validation with file and line, unknown keys rejected (CFG-3).
      Every problem is a `config.Error` (`file:line:column: key: reason`);
      `Load` reports the problems of all the files and applies none of a file
      that has one. Besides the decoder's (unknown keys, types, syntax,
      invalid names and durations), it rejects an integer where a name or a
      duration is expected, an empty command or target, and an entry that is
      empty, a lone `-` or holds a space. Key positions come from go-toml's
      `unstable` parser, used only for that in `internal/config/keys.go`.
- [x] **2.4** `emergup config check` and `emergup config show` (CFG-3, CFG-4).
      `cli.Main` takes an `Env` (standard streams and the root file system);
      `config.Inspect` also returns the origin of every value and entry, which
      `config show` writes as a comment after each value. Its output reads
      back to the same configuration.
- [x] **2.5** Example configuration under `examples/etc/emergup/`: the main
      file shows every setting with its default, commented out, and two
      drop-ins hold the maintainer's choices (excluded packages, an
      always-synced overlay). A test loads them and checks that the main
      file, uncommented, gives exactly the defaults and shows every setting.

## Phase 3 — Core engine  `[x]`  (PRIV-*, UPD-2, UPD-6–8, UPD-11–15)

- [x] **3.1** Command runner interface (streamed output, tee to a log) and a fake
      for tests: `internal/runner` with three modes (stream to the terminal
      and the log, capture for parsing, terminal for interactive tools),
      through pipes rather than a pty; on Ctrl-C a command gets SIGTERM and
      30 seconds before SIGKILL (UPD-11); each Result has its duration, the
      base of the step times (UPD-12). The root context ends on SIGINT and
      SIGTERM. `runner/runnertest.Fake` scripts results for other tests.
- [x] **3.2** Elevation: auto-detection, configuration override, no-op as root
      (PRIV-1, PRIV-2, PRIV-4). `runner.Command.Root` marks a command that
      needs root; `elevation.Resolve` picks the prefix (none as root; `auto`
      tries sudo, doas, run0 in that order; a named method must be
      installed) through a small `System` interface, and `elevation.Runner`
      applies it. opendoas becomes the command itself, so it cannot be sent
      SIGTERM by unprivileged emergup; the terminal's Ctrl-C still reaches it.
- [x] **3.3** Terminal output and prompts: step headings, colours, confirmations,
      `--yes` (UPD-2, UPD-13, UPD-14). Two PRs: colours first, then the menu.
      - Colours are on when standard output is a terminal, `NO_COLOR` is unset
        or empty and `TERM` is not `dumb`. Each kind of output (heading,
        prompt, success, skip, failure, notice) has a key in `[colors]`
        (CFG-6), written as git writes its colours; the defaults use only the
        16 ANSI colours, which the terminal's theme remaps, so they suit light
        and dark themes alike. The key is `colors`, as in `NO_COLOR`,
        `--color` and git.
      - emerge gets `--color=y` with colours on and `--color=n` otherwise,
        since its output reaches the terminal through a pipe; the emerge
        command lines built in phase 4 add it. The runner strips escape
        sequences from what it copies to the log.
      - A confirmation is an action menu: Enter for the usual action, one key
        per action, `?` for the list, so later steps and the AI actions of 9.0
        add keys without changing it. A key is read on its own, without Enter
        (`golang.org/x/term`, raw mode only while the key is read); when
        standard input is not a terminal, a letter and Enter. The input
        pending from before the question is discarded first: commands run
        without standard input, so a key pressed during a long build would
        otherwise answer the next question. Ctrl-C and the end of the input
        abort, never take the usual action. `--yes` takes the usual action
        and says so; the flag itself comes with `run` in 3.4.
- [x] **3.4** Step model and pipeline: enable/disable, `--pretend`, final summary
      with the time of each step and of the run (UPD-3, UPD-6, UPD-7, UPD-12).
      Two PRs: the engine (`internal/pipeline`), then `emergup run`.
      - Each enabled step has a heading (`[2/6] update: update @world`), the
        question "Run this step?" (run it, skip it, abort the run), and an end
        line with its outcome and time. A failure stops the run; the
        diagnostics and the failure menu come in phases 8 and 9. A disabled
        step is neither announced nor asked about. The summary has a row per
        step: succeeded, skipped, failed, stopped (interrupted while it ran),
        disabled or not run, with its time and reason.
      - A step's time runs from its start to its end, without the question;
        the run's is the whole wall-clock time.
      - `--pretend` shows each enabled step and what it would run, asks
        nothing and runs nothing.
      - `run` exits with 0 when every step succeeded or was skipped, 1 when a
        step failed or the run could not start, 130 when it was stopped
        (Ctrl-C, the abort key, the end of the input, SIGTERM). The
        elevation is resolved before the first step (PRIV-2), and a run as
        root says so once (PRIV-4).
      - Until phase 4 the built-in steps are placeholders that run nothing
        and are skipped as "not available yet"; each phase-4 item replaces
        one. In phase 4 a step that confirms itself after its `--pretend`
        (update, depclean) replaces the pipeline's question with its own.
- [x] **3.5** Single-instance lock and run directory (UPD-8; open questions 3, 5).
      Two PRs: the lock, then the run directory.
      - The lock is per user: the file `run.lock` in the user's state
        directory, locked with `flock`. A system-wide lock (first an
        abstract Unix socket) needs a place every user can write, where any
        local user can take the lock and block the others' runs, root's
        crontab included, and where races, symlinks and the sticky bit have
        to be defended against. The kernel frees the lock when the process
        ends; the file is opened without following a symlink, must be a
        regular file of the user's, is never removed, and holds the pid of
        the run for the message of a second one. Only `run` takes it, not
        `--pretend`. Runs of different users are not kept apart; Portage's
        own locks keep the system consistent.
      - The state directory, and every directory above it, must belong to
        the user or root and be writable by nobody else, as OpenSSH's
        StrictModes asks of `~/.ssh`: then no other user can change anything
        along the path, and there is no window between a check and a use.
      - The run directory is `runs/<local time>/` under the state directory
        (0700), with `emergup.log` (0600): the whole transcript without
        colours. `[runs] keep` (10 by default, 0 keeps all) removes the
        oldest. `--pretend` creates none; without a usable state directory
        the run does not start.

## Phase 4 — Built-in steps: parity with the old script (MVP)  `[ ]`

- [x] **4.1** Sync (`emerge --sync <repo>...`, configurable), per repository: a
      repository synced within the minimum interval is skipped, unless it is
      listed as always synced or `--force-sync` is given (SYNC-1–3). The last
      sync time is read from the repository on disk (git: `.git/FETCH_HEAD`,
      rsync: `metadata/timestamp.chk`), so syncs done outside emergup count
      and no state file is needed; other sync types are always synced.
      - The repositories come from `portageq repos_config /`, Portage's own
        view of `repos.conf`, run unprivileged: those with a `sync-type` and
        `auto-sync` on, which `emerge --sync` would sync, and every
        `always-sync` one, `auto-sync = no` or not; an `always-sync` name that
        is no repository gets a notice.
      - rsync and webrsync give their snapshot time in
        `metadata/timestamp.chk` (or its modification time); git the
        modification time of `.git/FETCH_HEAD`.
      - The repositories due are synced with one `emerge --sync`, elevated,
        so the post-sync hooks run once; it gets `--color=y` or `--color=n`
        as emergup's own output. A row says what is done with each
        repository; with none due the step is skipped.
- [x] **4.2** `@world` update with configurable options and exclusions, each
      passed as `--exclude`; `--pretend` first, then emergup's confirmation
      (EXC-1, UPD-10).
      - The pretend runs unprivileged, as it changes nothing; its output is
        shown as it comes and read for "Total: 0 packages", which skips the
        step. A failed pretend (conflicts, blockers) fails the step, its
        output on the terminal and in the log.
      - The step asks itself, after the pretend: update, skip the update or
        abort the run; `--yes` goes ahead. The pipeline does not ask its
        own question of a step that confirms itself, and a step can stop
        the run. The real command is elevated.
      - `emergup run --pretend` shows both command lines without running
        emerge, so it stays fast.
      - A `make.conf` that only root can read (binhost credentials) would
        show the unprivileged pretend another configuration than the real
        run's; no option for it yet.
- [x] **4.3** Live-package rebuild by mode (UPD-4, UPD-9): `smart`
      (smart-live-rebuild), `interval` (live packages of `@live-rebuild` whose
      `BUILD_TIME` in the Portage database is older than the interval, rebuilt
      `--oneshot`; no state file), `always`, `auto`.
      - The live packages are those of `@live-rebuild`: the installed
        packages with `live` in `PROPERTIES`, read from the database
        `portageq vdb_path` names, with their `BUILD_TIME`.
      - `interval` and `always` rebuild them with one elevated `emerge
        --oneshot =cat/pkg-version …`, the exact versions so that no other
        version is picked; a row says what is done with each. `smart` runs
        smart-live-rebuild, elevated, which checks the upstreams itself;
        not installed, the step is skipped with a notice. `auto` is `smart`
        when it is installed and `interval` otherwise, and says so.
      - The pipeline asks "Run this step?" as for any step: only the
        `@world` update and the depclean show a pretend first.
- [ ] **4.4** Preserved-library rebuild, only when needed; `haskell-updater`
      (UPD-5; open question 7).
- [ ] **4.5** Config-file updates delegated to `dispatch-conf` (ETC-3).
- [ ] **4.6** Depclean, `--pretend` first (UPD-10).
- [ ] **4.7** Held-back report for excluded packages (EXC-2).
- [ ] **4.8** First tagged pre-release `v0.1.0`.
      - The README gets its badges, as the other OrbintSoft repositories have
        them: CI, release, coverage, licence and sponsor. The release and
        coverage badges need their workflows (coverage published to a branch
        of its own, as in redo-backups).
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

- [ ] **9.0** Interaction design, written into docs/FEATURES.md (AI-*) before
      9.1. Starting point, from the maintainer's ideas:
      - Every decision point uses the action menu of 3.3: Enter does the usual
        action, `e` explains (one request, the answer streamed, then the same
        menu again), `c` chats (an interactive Claude Code session with the
        context; the menu comes back on exit), `a` aborts, `?` lists the keys.
      - An AI action is offered only when AI is enabled and the configured
        backend supports it; otherwise the menu shows only the usual actions
        and the flow is unchanged. Explain works with every backend,
        investigate with Claude Code and the API, chat with Claude Code only.
      - Setup: AI stays off until the user enables it (AI-1), but emergup
        helps. A setup command (name to decide, e.g. `emergup config ai`)
        detects Claude Code for the invoking user (`claude` on `PATH`, logged
        in according to `claude auth status`), shows the drop-in it would
        write in `/etc/emergup/config.d/` and writes it through elevation
        after confirmation; `emergup config check` mentions the command when
        Claude Code is found and AI is off. The configuration is system-wide
        while Claude Code is per user, so a user without it simply gets no AI
        actions. Any other AI tool is configured by hand with the generic
        command backend; further backends behind the interface are welcome as
        contributions, Claude Code being the reference one. The ebuild names
        `dev-util/claude-code` with `optfeature`.
      - Decision points: unread news (explained also against the installed
        packages, if the user agrees to send them); the `@world` plan before
        the update (open question 4); a failed step (retry, skip, abort,
        explain, investigate, chat; no default action), where investigate is
        a read-only agent that ends with a diagnosis and a proposed fix; each
        pending config file (the configured tool, diff, keep, take, postpone,
        explain, an AI merge shown as a diff); the depclean list.
      - A proposed fix is shown as the exact command or diff and applied by
        emergup, elevated, only after confirmation; chat hands over to Claude
        Code, whose own permission prompts and `sudo` confirm each command
        (open question 6). After a chat the failed step can be retried.
      - With `--yes` there are no menus and no AI: a failure aborts the run
        and writes the report.
      - The keys are read one at a time, without Enter, as 3.3 decided.
        To decide then: how much system context is sent by default;
        investigate as its own key or a deeper explain; explain and chat
        sharing one session.
- [ ] **9.1** Backend interface, configuration, "what will be sent" confirmation,
      refusal to run as root (AI-1, AI-2, AI-6).
- [ ] **9.2** Claude Code backend, with its detection and the setup command
      (9.0).
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
