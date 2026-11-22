# CLAUDE.md

Guidance for Claude (and any AI assistant) working in this repository. Rules are
binding for AI-assisted changes and recommended for all contributors.

- Roadmap and status: [PLAN.md](PLAN.md).
- What the tool promises its users: [docs/FEATURES.md](docs/FEATURES.md).

## What this repo is

`emergup` is a Go command-line tool that drives the routine Gentoo update
workflow — sync, news, `@world` update, rebuilds, config-file updates, depclean —
with configurable steps, package exclusions, hooks, failure diagnostics and
optional AI assistance. It is started as an ordinary user and elevates only the
commands that need root.

## Starting a session

1. Read PLAN.md (status, open questions) and any `*-steps.md` in the repo root:
   those gitignored files hold the sub-steps of the activity in progress.
2. Run `git branch --show-current` and `git status`.
3. Ask the maintainer which step to work on.

## Working rules

1. **Small, authorized steps.** Keep changes small. Ask for authorization before
   each step. If a change touches many files or lines, split it into sub-steps
   tracked in a gitignored `<activity>-steps.md`, so work can resume from a clean
   session even if the tree is incomplete in between.
2. **Every PLAN.md item is committable when done.** Intermediate sub-steps may
   leave the tree broken; a completed plan item may not.
3. **Leave the session resumable.** Before stopping, update PLAN.md (status) and
   the steps file (what is done, what is next, anything learned), so a new
   session can continue without the old context.
4. **Propose new rules** when one would help — don't add them silently.
5. **Ask before committing or pushing**, and check the branch first: work on a
   feature branch, never directly on `master`. The branch can change between
   turns (e.g. after a PR is merged) — verify, don't assume.
6. **Every commit is signed off** (`git commit -s`, DCO — see CONTRIBUTING.md).
7. **Repo language is English** (code, comments, docs, commit messages). Chat with
   the maintainer may be in Italian.
8. **No sensitive data in committable files.** Never commit personal data or
   details of a real machine: hostnames, user names, installed-package lists,
   `emerge --info` output, real logs. Fixtures use synthetic values; real material
   goes only in gitignored scratch files.
9. **Quality gate before declaring done or opening a PR.** Run build, `go vet`,
   `golangci-lint` and the tests, and report the actual results — including what
   failed or was skipped.

## Design rules

10. **Generic, not personal.** emergup must work on any Gentoo system. No default
    may encode the maintainer's own setup (excluded packages, backup tool, diff
    tool, paths); personal choices belong in example configuration only.
11. **Least privilege at runtime.** The process runs unprivileged; only the single
    command that needs root is elevated, through the configured method. Hooks and
    AI backends run as the invoking user. Nothing an AI backend produces is
    executed or applied with privileges unless the user has confirmed the exact
    command or diff.
12. **Side effects behind interfaces.** Running commands, elevation, reading the
    Portage state and touching files go behind small interfaces, so logic is
    unit-tested with fakes. No unit test runs a real system command (`emerge`,
    `sudo`, `eselect`, …); real commands run only in the Docker integration suite.
13. **Every user-visible behaviour is a documented feature.** Before implementing
    or changing behaviour, find its entry in docs/FEATURES.md — or add it there
    first, in the same change. Integration tests are derived from those entries
    and name the feature id they verify.
14. **Tests use testify** (`require` when continuing makes no sense, `assert`
    otherwise). A new test must be seen failing before it is committed passing.
15. **One root context**, created in `main`; everything below takes a
    `context.Context` as first argument and uses it (`exec.CommandContext`, …).
    Tests use `t.Context()`.
16. **Configuration over hardcoding.** Commands, options and paths are driven by
    configuration with sane, generic defaults.

## Repository rules

17. **Licence: GPL-3.0-or-later.** New source files carry
    `SPDX-License-Identifier: GPL-3.0-or-later`. Before adding a Go dependency or
    copying code, check its licence is GPL-3-compatible (MIT, BSD, Apache-2.0,
    MPL-2.0, LGPL are fine; GPL-2.0-only is not) and list it in COPYRIGHT.md.
18. **Document and automate every dependency.** A new Go module, build tool, CI
    action, container image or runtime-invoked command is recorded in
    DEPENDENCIES.md in the same change, and wired into Dependabot or Renovate — or
    PLAN.md says why it can't be and how it is updated instead.
19. **New file type → consider a linter.** When a new kind of file enters the repo,
    evaluate a linter for it and wire it into `make lint` and CI if reasonable;
    record the decision (including "no linter") in PLAN.md.
20. **Don't embed one language inside another.** A config body, a script or other
    structured content is authored as its own file and referenced, never inlined in
    a heredoc or string literal of the host language.
21. **Least-privilege `GITHUB_TOKEN`.** Every workflow declares an explicit
    top-level `permissions:` block (default `contents: read`); wider scopes only
    per job, where a step needs them. GitHub Actions are pinned to a full commit
    SHA with a `# vX.Y.Z` comment.
22. **Comments serve readers, not history.** Comments and help text explain what
    the code does and any non-obvious *why*. They never narrate how the code came
    to be and never reference PLAN.md, phases or rule numbers; describe only what
    the file itself controls.
