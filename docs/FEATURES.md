# Features

The authoritative catalogue of what emergup promises its users. Each entry has a
stable id and is stated as an outcome the user can observe. Integration tests
name the id they verify. Ids are never reused; a dropped feature is marked
**withdrawn** rather than deleted.

**Status:** `planned` · `partial` · `done`

## Update workflow (UPD)

| Id | Feature | Status |
| --- | --- | --- |
| UPD-1 | One command runs the whole update: sync, news, `@world` update, live-package rebuild, preserved-library rebuild, config-file updates, depclean. | partial |
| UPD-2 | Each step is announced, and emergup asks for confirmation before moving to the next; `--yes` runs without pausing, taking the usual action at every question and saying so. | done |
| UPD-3 | Every step can be enabled or disabled in the configuration, and the command and options it runs can be changed. | partial |
| UPD-4 | Optional steps whose tool is not installed (e.g. `smart-live-rebuild`, `haskell-updater`) are skipped with a notice instead of failing. | partial |
| UPD-5 | The preserved-library rebuild runs only when Portage reports preserved libraries. | done |
| UPD-6 | `--pretend` shows the steps and commands that would run, without running them. | partial |
| UPD-7 | At the end, a summary lists each step as succeeded, skipped, failed, stopped by an interruption, disabled in the configuration, or not run because the run ended before it, and says whether the run finished, failed or was stopped. | done |
| UPD-8 | Only one emergup run of a user can be active at a time; a second one refuses to start and says which process holds the run. `--pretend` can run alongside. The lock is a file in the user's own state directory, which emergup refuses to use when another user could change it; runs of different users, such as a user's and a root crontab's, are not kept apart, and Portage's own locks keep the system consistent. | done |
| UPD-9 | The live-package rebuild has a mode: `smart` runs smart-live-rebuild; `interval` rebuilds each live package built longer ago than a configurable interval (24 hours by default); `always` rebuilds every live package; `auto`, the default, is `smart` when smart-live-rebuild is installed and `interval` otherwise. | done |
| UPD-10 | Before the `@world` update and the depclean, emergup shows what emerge would do (`--pretend`) and asks for confirmation itself; with `--yes` it goes ahead without asking. | done |
| UPD-11 | Interrupting a run (Ctrl-C) lets the running command stop by itself; it is killed only if it is still running 30 seconds later. | planned |
| UPD-12 | Each step shows how long it took when it ends, and the final summary lists the time of every step and of the whole run. | done |
| UPD-13 | On a terminal the output is coloured: step headings, prompts, successes, skips, failures and notices each have their colour (CFG-6), and emerge keeps its own colours although emergup copies its output to the log, which is written without colours. `NO_COLOR`, or `TERM=dumb`, turns every colour off. | done |
| UPD-14 | A question is answered with one key, without Enter: the keys are shown after the question, Enter takes the usual action (the key in capitals), each other action has its letter and `?` lists them all. Keys pressed before the question appears are ignored, and Ctrl-C or the end of the input aborts instead of answering. When standard input is not a terminal, each answer is a line: a letter, or nothing for the usual action. | done |
| UPD-15 | Each run keeps a log of what it showed, without colours: emergup's lines, the questions and their answers, and every command with its output. The log is in a directory of the run's own under the invoking user's state directory, `$XDG_STATE_HOME/emergup/runs/` (by default `~/.local/state/emergup/runs/`), and the end of the run says where. The last 10 run directories are kept; `[runs] keep` changes how many, 0 keeps them all. | done |
| UPD-16 | In the `smart` mode of the live-package rebuild, emergup shows the packages smart-live-rebuild would rebuild and asks before rebuilding them; with none, the step is skipped. | planned |

## Repository sync (SYNC)

| Id | Feature | Status |
| --- | --- | --- |
| SYNC-1 | The sync step works on the repositories `emerge --sync` would sync, and skips each one synced more recently than a configurable interval (24 hours by default), saying when it was last synced; a sync done outside emergup counts. A repository whose last sync time cannot be determined is synced, and an interval of 0 syncs every time. The repositories due are synced with one command. | done |
| SYNC-2 | `emergup run --force-sync` syncs the repositories regardless of the interval. | done |
| SYNC-3 | Repositories listed in the configuration are synced on every run regardless of the interval (e.g. an overlay the user publishes to), even one that Portage does not sync by itself (`auto-sync = no`). | done |

## Privileges (PRIV)

| Id | Feature | Status |
| --- | --- | --- |
| PRIV-1 | emergup is started as an ordinary user and asks for elevation only for the commands that need root. | planned |
| PRIV-2 | The elevation method is detected automatically (`sudo`, `doas`, `run0`) and can be set in the configuration. When none is installed, emergup says so before a run that needs root starts, not halfway through it. | done |
| PRIV-3 | Hooks, custom steps and AI backends run as the invoking user, unless a hook or a custom step is explicitly configured to be elevated (a backup tool, `haskell-updater`). | planned |
| PRIV-4 | Started as root (a root shell, a root crontab), emergup runs the commands directly, turns AI assistance off, and says so once at the start. | done |
| PRIV-5 | Environment variables listed in the configuration, such as `EDITOR` for `dispatch-conf`, are kept for the commands run as root, through `sudo --preserve-env` or `run0 --setenv`; doas has no such option, and emergup says that `doas.conf` must keep them. | planned |

## Configuration (CFG)

| Id | Feature | Status |
| --- | --- | --- |
| CFG-1 | Configuration is read from `/etc/emergup/config.toml` and then from `/etc/emergup/config.d/*.toml` in lexical order, so packages and admins can add drop-in files without editing a shared one. | done |
| CFG-2 | emergup works with no configuration file at all, using generic defaults. | done |
| CFG-3 | `emergup config check` validates the configuration and reports each problem with its file and line, exiting with status 1 when there is one; unknown keys are errors. Only a file that is not TOML, or that defines a table twice, is checked no further than that problem. | done |
| CFG-4 | `emergup config show` prints the effective configuration as TOML, each value with the file it came from or `default`. | done |
| CFG-5 | Lists of entries (excluded packages, always-synced repositories) add up across the configuration files, like `USE` in `make.conf`: a later file adds entries, `-entry` removes one added earlier and `-*` clears the list; any other value set by a later file replaces the earlier one. | done |
| CFG-6 | The colour of each kind of output (step headings, prompts, successes, skips, failures, notices) can be changed in the configuration to suit the terminal's theme, or turned off for that kind alone. A colour is written as words separated by spaces: attributes (`bold`, `dim`, `italic`, `underline`, `reverse`) and up to two colours, the text's then the background's, each a name (`red`, `bright-red`, `default`), a number from 0 to 255 or `#rrggbb`; `""` leaves that kind plain. | done |

## Package exclusions (EXC)

| Id | Feature | Status |
| --- | --- | --- |
| EXC-1 | Packages listed in the configuration are excluded from the `@world` update (e.g. kernel sources, `linux-headers`, `nvidia-drivers`). | done |
| EXC-2 | Excluded packages that have an update available are listed at the end of the run, so nothing is held back silently. | done |

## Hooks and custom steps (HOOK)

| Id | Feature | Status |
| --- | --- | --- |
| HOOK-1 | User scripts can run before and after the whole run and before and after any step. | planned |
| HOOK-2 | When a *check* hook fails, emergup asks whether to run its configured remedy, if it has one, and check again (e.g. "the last backup is older than 20 days — run the backup now?"), to go on without it, or to abort the run. | planned |
| HOOK-3 | Custom steps can be defined in the configuration and placed after any built-in step. | planned |
| HOOK-4 | Hooks receive the run context in environment variables (step name, phase, run directory). | planned |

## News (NEWS)

| Id | Feature | Status |
| --- | --- | --- |
| NEWS-1 | After the sync, unread Gentoo news items are shown, and emergup asks for confirmation before updating. | planned |
| NEWS-2 | `emergup news` lists news items and lets the user read them and mark them as read. | planned |

## Failure diagnostics (DIAG)

| Id | Feature | Status |
| --- | --- | --- |
| DIAG-1 | When a step fails, emergup collects a failure report in the run directory: the command, its output, the failed package's build log and `emerge --info`. | planned |
| DIAG-2 | The location of the failure report is printed, ready to be read or attached to a bug report. | planned |

## Config-file updates (ETC)

| Id | Feature | Status |
| --- | --- | --- |
| ETC-1 | After the update, pending config-file updates (`._cfg*` files) are listed. | done |
| ETC-2 | For each pending file the user can view the difference with a configurable diff tool, keep the old file, take the new one, merge them with a configurable merge tool, or postpone. | planned |
| ETC-3 | The step can instead delegate to an external tool (`dispatch-conf`, `etc-update`). | done |

## AI assistance (AI)

| Id | Feature | Status |
| --- | --- | --- |
| AI-1 | AI assistance is off by default and is enabled in the configuration by choosing a backend: Claude Code, the Anthropic API, or any command that reads a prompt on standard input. | planned |
| AI-2 | Before anything is sent to an AI backend, emergup says what will be sent and asks for confirmation. | planned |
| AI-3 | Unread news items can be explained by the AI in relation to the packages installed. | planned |
| AI-4 | After a failure, the AI can analyse the failure report and propose a fix; a proposed change is shown and applied only after the user confirms it. | planned |
| AI-5 | For a pending config-file update, the AI can explain the differences or propose a merged file, shown as a diff for approval. | planned |
| AI-6 | AI backends are never run with root privileges. | planned |

## Command line (CLI)

| Id | Feature | Status |
| --- | --- | --- |
| CLI-1 | `emergup version` (or `--version`) prints the emergup version. | done |
| CLI-2 | `emergup help` (or `-h`, `--help`) prints the usage on standard output; a missing or unknown command, or an unexpected argument, prints the usage on standard error and exits with status 2. | done |
| CLI-3 | `emergup run` runs the update step by step; `-y` (`--yes`) takes the usual answer to every question and `-p` (`--pretend`) shows the steps without running them. It exits with status 0 when every step succeeded or was skipped, 1 when a step failed or the run could not start, and 130 when the run was stopped: Ctrl-C, `SIGTERM`, the abort key or the end of the input at a question. | done |
