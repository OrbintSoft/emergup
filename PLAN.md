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
  file; no general JSON5 linter). TOML has no linter: the TOML files are
  emergup configuration files, which emergup's own strict loader checks
  (syntax, unknown keys, types, values), more than a generic TOML linter
  does. `make test` loads the example configuration and compares it with the
  defaults in the code; `make integration` loads the integration image's
  configuration and the fixtures of the integration tests, some of them
  broken on purpose, which a linter would reject. They are formatted by
  hand. hadolint lints the Dockerfile of the integration image (`make
  lint-docker`), run from its pinned image. The other files of that image
  have no linter: `visudo --check` checks the sudoers file when the image is
  built, and Portage reads the Portage files (make.conf, repos.conf, the set,
  the test repository's layout.conf and ebuilds) in every integration test.
  pkgcheck is not used on the synthetic ebuilds: it needs the Gentoo tree and
  judges ebuilds meant for a real repository. The two C sources of the test
  library and program have no linter either: a few lines each, compiled by
  their ebuilds in the tests that need them. The shell scripts, the first
  being the hook of the integration tests, are linted by shellcheck (`make
  lint-shell`), run from its image pinned by digest like hadolint.

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
7. ~~**`haskell-updater`**~~ — answered: no step for one ecosystem. It is
   run by a custom step (6.3), which can be optional: skipped with a notice
   when its program is not installed (UPD-4). The example configuration
   shows it.

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
        one, and 4.6 replaced the last. In phase 4 a step that confirms
        itself after its `--pretend` (update, depclean) replaces the
        pipeline's question with its own.
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

## Phase 4 — Built-in steps: parity with the old script (MVP)  `[x]`

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
- [x] **4.4** Preserved-library rebuild, only when needed; `haskell-updater`
      (UPD-5; open question 7).
      - The preserved libraries come from `portageq list_preserved_libs /`,
        unprivileged; with none, the step is skipped without a question.
        Otherwise a row names each one, the step asks itself (rebuild,
        skip, abort the run) and runs `emerge @preserved-rebuild`, elevated.
      - With nothing to do the step skips without asking, so it confirms
        itself; `haskell-updater` is not a built-in step (open question 7).
- [x] **4.5** Config-file updates delegated to `dispatch-conf` (ETC-3).
      - The pending files (`._cfg????_*`) are looked for under
        `CONFIG_PROTECT`, as `portageq envvar` gives it, by one elevated and
        read-only `find`: several directories of `/etc` can be read by root
        alone, and an unprivileged scan would miss the files in them. The
        results are filtered as Portage does (next to a protected file with
        its name, not under `CONFIG_PROTECT_MASK`, not `~` or `.bak`).
      - With none the step is skipped without a question. Otherwise a row
        names each file it would replace (ETC-1), the step asks, and runs
        the tool elevated and attached to the terminal, since it reads the
        user's keys. With `--yes` the interactive tool cannot run, so the
        step is skipped and says to run it; `--pretend` elevates nothing.
- [x] **4.6** Depclean, `--pretend` first (UPD-10).
      - As the update: the pretend runs unprivileged, shown as it comes and
        read for "Number to remove: 0", which skips the step without a
        question; then the step asks (remove them, skip the depclean, abort
        the run) and runs the depclean elevated. `--yes` goes ahead, as
        UPD-10 says; emerge itself refuses a depclean whose dependencies it
        cannot resolve, which fails the step at the pretend.
- [x] **4.7** Held-back report for excluded packages (EXC-2).
      - Once the update has updated, or had nothing to update, the excluded
        entries are checked with one unprivileged `emerge --pretend --update
        --nodeps`, not shown: a line with `U` (not a downgrade) or a new
        slot (`NS`) is a package held back; a new install (`N`) is an
        excluded package that is not installed. Entries with a wildcard,
        which emerge refuses there, are named as not checked.
      - A step can leave notes, which the pipeline writes after the rows of
        the summary: "2 excluded packages have an update:" and a row for
        each. A failed check is a note, not a failure of the update.
- [x] **4.8** First tagged pre-release `v0.1.0`.
      - The README gets its badges, as the other OrbintSoft repositories have
        them: CI, release, coverage, licence and sponsor. The release and
        coverage badges need their workflows (coverage published to a branch
        of its own, as in redo-backups).
      - Releases are cut by hand, by pushing a tag `vX.Y.Z`: unlike
        redo-backups, which tags every change, each emergup release bumps the
        ebuild. The release workflow reuses the CI workflow as its gate, then
        `make dist` and `gh release create`; `0.x` is a pre-release. No
        GoReleaser: no deb, rpm or snap for a Gentoo tool.
      - `make dist` builds static binaries for linux amd64 and arm64 (`-s -w`,
        the version stamped) with the documents, the vendored modules for an
        offline package build (`<name>-<version>/vendor`, where the tag's
        source archive unpacks), and `SHA256SUMS`, with GNU tar options that
        make the archives the same on every machine.
      - The coverage workflow publishes a shields.io endpoint file on the
        orphan `badges` branch, as redo-backups does.
      - The tag itself is pushed with the maintainer's approval. `v0.1.0` was
        published on 2026-11-24: a pre-release with the two archives, the
        vendored modules and `SHA256SUMS`.
- [x] **4.9** Ebuild `app-portage/emergup` in the orbintsoft overlay
      (`orbintsoft-ebuild` repository), from `v0.1.0` on: the tool is
      packaged as soon as it can be used, and each pre-release bumps it.
      - `app-portage/emergup-0.1.0` (EAPI 9, `go-module`): the tag's source
        and the release's vendored modules, so the build is offline; built,
        tested and installed through the Makefile with `GOTOOLCHAIN=local`;
        the commented example as `/etc/emergup/config.toml`; `~amd64`. Its
        `src_test` passes in the Portage sandbox.
      - `BDEPEND` must name the go line of go.mod as it is written
        (`>=dev-lang/go-1.26.0:=`): go-module's EAPI 9 check dies otherwise,
        so a release that moves the go line moves it in the ebuild too.

## Phase 5 — Docker integration tests  `[x]`

- [x] **5.1** Harness: `gentoo/stage3` image (digest pinned, tracked by Renovate),
      local test repository with synthetic ebuilds and news, `make integration`.
      - The image (test/integration/image) is a `gentoo/stage3:amd64-openrc`
        with the tree of `gentoo/portage`, both pinned by dated tag and
        digest; Renovate bumps them together, Dependabot too. sudo comes
        from the binary host; `tester` may use it without a password.
      - The test repository `emergup-test` syncs with rsync from
        `file:///srv/emergup-test`, inside the image: the containers run
        with no network, and the Gentoo tree has `auto-sync = no`. News
        items join it with the tests that need them.
      - The image's emergup configuration updates `@emergup-test`, and every
        stage3 package is in the world set, so no test updates or removes a
        stage3 package. `emerge --depclean <atoms>` cannot restrict the
        depclean instead: it removes a named package even from the world
        set.
      - Go tests behind the `integration` build tag, one container each,
        named after the feature id (`TestUPD6_…`); `make integration`
        builds a static binary stamped `0.0.0-integration`, the image and
        runs them, `INTEGRATION_RUN` picks some. The first ones cover CLI-1
        and UPD-6; UPD-6 found the preserved-library query failing for a
        user outside the portage group, fixed in its own change.
- [x] **5.2** Integration tests for the Phase 2–4 features, each naming its id.
      - Configuration and command line: CLI-2, CLI-3 (status 130 at a
        question), CFG-1–6. The configuration files of a test are fixtures
        in test/integration/testdata, installed owned by root; the colours
        are checked on a terminal (`docker exec --tty`).
      - Sync: SYNC-1–3, each run with the sync alone. Portage cancels an
        rsync sync whose upstream timestamp has not changed ("already
        current"), so a test publishes a snapshot with a newer timestamp,
        and a file a sync brings shows whether one ran; a sync done with
        emerge before the run counts as a recent one.
      - Update, exclusions, live packages: UPD-4, UPD-9 (interval and
        always), UPD-10 (the update), EXC-1, EXC-2. Snapshots bring
        synthetic ebuilds (an upgrade, an excluded package, a live one with
        `PROPERTIES="live"`); an old build is an old `BUILD_TIME` in the
        Portage database. The smart mode itself needs smart-live-rebuild,
        which the image does not have.
      - Preserved libraries, configuration files, depclean: UPD-5, ETC-1,
        ETC-3, UPD-10 (the depclean). A library whose soname changes
        between versions, and a program linked against it without a slot
        operator, give Portage a library to preserve. A configuration file
        edited before its package is updated leaves a `._cfg0000_` file;
        dispatch-conf waits for keys, so the delegation is tested with
        `etc-update --automode -5`.
      - Privileges, lock, summary, colours, log: PRIV-2, PRIV-4, UPD-7,
        UPD-8, UPD-12, UPD-13, UPD-15. sudo is the image's only elevation
        tool: a test takes it off the PATH for a run without one, and for a
        run as root, which needs none; doas set by name stands for a method
        that is not installed. The lock is held by a run that waits at its
        first question, its input left open. A version of a test package
        that fails to build gives a failed step, and emerge's own colours
        are checked on the packages of the test repository.
      - The run itself: UPD-1, UPD-2, UPD-3, UPD-14. A key read without
        Enter needs a terminal whose keys the test presses: the test opens a
        pseudo-terminal for `docker exec --tty`. A key pressed while the
        update's pretend runs shows that the keys typed before a question
        are ignored.
- [x] **5.3** CI job for the integration suite.
      - The integration workflow runs `make integration` on every push to
        `master`, on demand, and on the pull requests that change the Go
        code, `go.mod`, the Makefile or test/integration. A check that runs
        only on some pull requests cannot be required by the ruleset (it
        would wait forever on the others), so it is not.

From here on, every feature lands with its integration test.

## Phase 6 — Hooks and custom steps  `[~]`  (HOOK-*, PRIV-3, PRIV-5, UPD-16)

This phase also brings what the old script still does and phase 4 left out:
`haskell-updater` after the preserved-library rebuild (6.3), the user's
editor in `dispatch-conf`, which the script kept with `sudo -E` (6.6), and the
live packages to rebuild shown before they are (6.7); and, new, a backup
check before the run (6.2, 6.4).

- [x] **6.1** Hooks before/after the run and each step, with context in the
      environment (HOOK-1, HOOK-4).
      - A hook is a named table, `[hooks.<name>]`, with `at`
        (`before-run`, `after-run`, `before-<step>`, `after-<step>`), its
        `command` and `on-failure` (`stop`, the default, or `continue`).
        Named tables let a drop-in add a hook, or turn one off with
        `enabled = false`, without repeating the others: an array of tables
        would be replaced as a whole. Hooks at the same place run in the
        order of their names, as the drop-ins are read.
      - A step's hooks run when the run reaches it, before its question,
        even if the step is then skipped: the same place for every step,
        update and depclean too, which ask after their own pretend.
      - A hook runs as the invoking user (PRIV-3), without a question; its
        output is shown and logged like a step's.
      - The hooks before a place stop at the first that fails; every hook
        after a place runs, whatever an earlier one did. A step whose hooks
        before it fail is not run and runs no hook after it; a hook after a
        step that fails keeps the step's status and stops the next steps.
        The summary lists the hooks that failed.
      - A hook's place is checked as a value of its own rather than decoded
        by a text unmarshaler: go-toml hands the raw text of an integer to
        one and returns its error without a position, which would hide the
        other problems of the file.
- [x] **6.2** Check hooks with an optional remedy and re-check (HOOK-2).
      - A check is a hook with `on-failure = "ask"`, and `remedy` its
        optional remedy. A failed check asks: run the remedy and check
        again, go on without it, or abort the run; Enter, and `--yes`, run
        the remedy, `--yes` only once, so that a remedy that does not help
        cannot loop. Without a remedy the question offers only to go on or
        abort, and abort is the usual action: an unattended run does not go
        on past a failed check.
      - Aborting at a check fails the run rather than stopping it: the
        check failed and nobody overrode it, which is a failure of the run,
        and the hooks after the run, a notification say, still run. Ctrl-C
        at the question stops the run, as anywhere.
      - The check runs as the invoking user; the remedy can be configured to
        be elevated (`remedy-elevated`), as a backup tool must, and so can a
        hook's own command (`elevated`) (PRIV-3).
- [x] **6.3** Custom steps (HOOK-3).
      - A custom step is a named table, `[custom-steps.<name>]`, with
        `after` (a built-in step), its `command`, an optional `title`, and
        `elevated` and `optional`; as for hooks, a drop-in adds one or turns
        one off by name. Its name is not that of a built-in step nor `run`,
        so that a hook can name it (`before-<name>`).
      - Custom steps go after a built-in step only, those after the same
        one in the order of their names: an order that needs no check for
        cycles, as custom steps after custom steps would.
      - A custom step whose command fails fails the run, as a built-in step
        does; it has no `on-failure`.
      - A custom step can be optional: when its program is not installed it
        is skipped with a notice instead of failing (UPD-4). The example
        configuration has one that runs `haskell-updater` after the
        preserved-library rebuild; nothing in emergup is specific to one
        language's tools. A program is installed when it is found as the
        invoking user would start it, on their `PATH`. An optional step
        whose program is missing is skipped without its question, as there
        is nothing to run; a custom step that is not optional fails then,
        so that a mistyped program does not go unnoticed.
      - A custom step runs as the invoking user unless it is configured to be
        elevated (PRIV-3), as `haskell-updater` must be. The example runs
        `haskell-updater --quiet -- --verbose`: emergup asks before the step,
        so the options of emerge have no `--ask`.
- [x] **6.4** The exit statuses for which a check offers its remedy (HOOK-2).
      - `remedy-status` lists them, from 1 to 255; without it any failure
        offers the remedy, as before. A check that fails otherwise, with
        another status, without starting or killed by a signal, says that
        its remedy is not for that failure and asks only whether to go on
        or abort; Enter and `--yes` abort, as for a check without a remedy.
        After the remedy the check runs again under the same rule. A backup
        check thus tells "the backup is too old", which the backup fixes,
        from "the backup disk is missing", which the backup would make
        worse by writing onto the root filesystem. The check alone cannot
        prevent that: its failure is what offers the remedy.
      - The list belongs to a check with a remedy. Empty, the default, any
        failure offers the remedy: `config show` writes the empty list and
        reads it back, and a drop-in sets it to bring the default back. A
        later file that sets it replaces it, as it does `command` and
        `remedy`: it is a value of the hook, not a list of entries added up
        like `USE`.
      - `--pretend` names the statuses: "if it fails with exit status 1,
        offers to run …; otherwise asks whether to go on".
      - What a check checks is the business of its command: emergup knows
        no backup format nor where a backup is, and the maximum age of a
        backup is an argument of the check. No `[backup]` section either:
        without a check of its own, it would be a check hook under another
        name. A backup tool can have the check itself (redo-backup is to get
        `redo-backup check`, with an exit status for "too old" and another
        for "destination missing"); otherwise the user writes a script.
      - The example configuration shows a backup check before the run, with
        the backup tool as its remedy, elevated, and `remedy-status`;
        emergup ships no script for it.
- [x] **6.5** Hooks that run only when someone answers the questions
      (HOOK-5).
      - `interactive-only = true` skips the hook when the run answers the
        questions by itself, under `--yes` and in the unattended runs of
        phase 13: a notice at its place, on the terminal and so in the log,
        and the run goes on as if the hook had passed. A hook skipped so has
        not failed, and the summary does not list it.
      - `remedy-interactive-only = true` keeps a check running but offers
        its remedy only when someone answers: under `--yes` a failure says
        so and aborts the run, as for a check without a remedy. It needs a
        remedy, as `remedy-elevated` does.
      - A backup is long: an unattended run may rather skip its check, or
        fail and leave the backup to the user, than start one. Both are
        the user's choice; by default a hook runs, and `--yes` runs a
        check's remedy once, as before.
      - `--pretend` says what each hook would do under `--yes`.
- [x] **6.6** The environment of the commands run as root (PRIV-5): an
      `[elevation]` list of variables to keep, such as `EDITOR`, so that
      `dispatch-conf` edits with the user's editor. sudo keeps them with
      `--preserve-env=…`, which the sudoers rule must allow (a rule for
      `ALL` does), run0 with `--setenv=…`; doas has no such option, and
      emergup says that `doas.conf` must keep them.
      - `keep-env` is a list of entries, added up across files like `USE`
        (CFG-5), and empty by default: a sudo rule for some commands only,
        without `SETENV`, refuses `--preserve-env`, and every command run
        as root would fail. The example configuration keeps `EDITOR` and
        `VISUAL`. A name that is not a variable name is rejected, since a
        comma would split sudo's list.
      - run0 gets `--setenv=NAME=value` for each variable that is set; as
        root, emergup needs nothing, the environment being its own.
      - With doas, the notice comes once, at the start of the run, when
        there are variables to keep.
      - The `EMERGUP_*` variables of a hook or a remedy run as root (HOOK-4)
        are kept the same way, always: HOOK-4 promises them, and a sudo
        rule for a hook's command alone needs `SETENV` for them.
- [ ] **6.7** The live packages listed before their rebuild in the smart mode
      (UPD-16): `smart-live-rebuild --pretend` first, then the question,
      then the rebuild. How the rebuild avoids checking the upstreams a
      second time (rebuilding the listed packages with `emerge --oneshot`,
      say) is decided when the item comes.

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
- [ ] **11.2** The first stable release, `v1.0.0`, through the release workflow
      of 4.8 (no GoReleaser, see 4.8).
- [ ] **11.3** Ebuild bumped to the first stable release.
- [ ] **11.4** Point the old script to emergup and retire it.

## Phase 12 — Ideas from gentup  `[ ]`  (to discuss)

Evaluated on 2026-11-24 from [gentup](https://github.com/jhelliwe/gentup), a
Gentoo updater in Rust (GPL-3.0). Nothing here is decided: each item is
discussed when the phase comes, and may be dropped.

- [ ] **12.1** Update first: a configurable list of packages updated before
      `@world` when they have an update, `sys-apps/portage` by default, as
      Portage itself advises; gentup also updates `sys-devel/gcc` first.
- [ ] **12.2** The elog messages of the packages merged in a run, shown at its
      end and kept in its log (gentup runs elogv).
- [ ] **12.3** Notifications at the end of a run: a mail through `sendmail`,
      or any configured command, with the summary and the news; it goes with
      phase 13 and the hooks of 6.1.
- [ ] **12.4** A clear error when the system is not Gentoo.
- [ ] **12.5** As optional custom steps (6.3), shown in the example
      configuration: `revdep-rebuild`, `eix-update`, `eix-test-obsolete`,
      `eclean-dist` and `eclean-pkg`, `eclean-kernel`. To decide: whether
      cleaning the distfiles and the old kernels deserves a built-in step,
      with a pretend and a question as the depclean.
- Not proposed: installing emergup's own dependencies (the ebuild's job),
  baseline packages for a new install (provisioning, not updating),
  pre-fetching the sources (Portage's `parallel-fetch` does it; at most a
  `--fetchonly` option), an interactive `--setup` (the configuration files
  and `config check`/`show` serve; perhaps a `config init` later).

## Phase 13 — Unattended runs  `[ ]`  (to discuss)

Asked on 2026-11-24: a silent mode with the logs, and a cron job for
automatic updates.

- [ ] **13.1** A quiet mode: the whole output in the run's log, and on the
      terminal only the failures or the summary, so that cron mails only what
      matters.
- [ ] **13.2** What the interactive or destructive steps do with nobody
      there: config-files is skipped already; the depclean perhaps only
      shown, or as the configuration says.
- [ ] **13.3** `emergup cron install` and its removal: shows the entry, then
      writes it through elevation, as the AI setup of 9.0; finds the
      scheduler (cronie and `/etc/cron.d`, fcron, dcron, a systemd timer).
- [ ] **13.4** Elevation with nobody there: a sudo rule for those commands
      only, or root's crontab (PRIV-4). A root run and a user's run are not
      kept apart by the per-user lock.
- [ ] **13.5** The notifications of 12.3 as the report of an unattended run.
