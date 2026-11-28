# Features

The authoritative catalogue of what emergup promises its users. Each entry has a
stable id and is stated as an outcome the user can observe. Integration tests
name the id they verify. Ids are never reused; a dropped feature is marked
**withdrawn** rather than deleted.

**Status:** `planned` · `partial` · `done`

## Update workflow (UPD)

| Id | Feature | Status |
| --- | --- | --- |
| UPD-1 | One command runs the whole update: sync, news, `@world` update, live-package rebuild, preserved-library rebuild, config-file updates, depclean. | done |
| UPD-2 | Each step is announced, and emergup asks for confirmation before moving to the next; `--yes` runs without pausing, taking the usual action at every question and saying so. | done |
| UPD-3 | Every step can be enabled or disabled in the configuration, and the command and options it runs can be changed. | partial |
| UPD-4 | Optional steps whose tool is not installed (e.g. `smart-live-rebuild`, `haskell-updater`) are skipped with a notice instead of failing. | done |
| UPD-5 | The preserved-library rebuild runs only when Portage reports preserved libraries. | done |
| UPD-6 | `--pretend` shows the steps and commands that would run, without running them. | partial |
| UPD-7 | At the end, a summary lists each step as succeeded, skipped, failed, stopped by an interruption, disabled in the configuration, or not run because the run ended before it, and says whether the run finished, failed or was stopped. | done |
| UPD-8 | Only one emergup run of a user can be active at a time; a second one refuses to start and says which process holds the run. `--pretend` can run alongside. The lock is a file in the user's own state directory, which emergup refuses to use when another user could change it; runs of different users, such as a user's and a root crontab's, are not kept apart, and Portage's own locks keep the system consistent. | done |
| UPD-9 | The live-package rebuild has a mode: `smart` rebuilds the live packages whose upstream has changed, as smart-live-rebuild lists them (UPD-16); `interval` rebuilds each live package built longer ago than a configurable interval (24 hours by default); `always` rebuilds every live package; `auto`, the default, is `smart` when smart-live-rebuild is installed and `interval` otherwise. | done |
| UPD-10 | Before the `@world` update and the depclean, emergup shows what emerge would do (`--pretend`) and asks for confirmation itself; with `--yes` it goes ahead without asking. | done |
| UPD-11 | Interrupting a run (Ctrl-C) lets the running command stop by itself; it is killed only if it is still running 30 seconds later. | planned |
| UPD-12 | Each step shows how long it took when it ends, and the final summary lists the time of every step and of the whole run. | done |
| UPD-13 | On a terminal the output is coloured: step headings, prompts, successes, skips, failures and notices each have their colour (CFG-6), and emerge keeps its own colours although emergup copies its output to the log, which is written without colours. `NO_COLOR`, or `TERM=dumb`, turns every colour off. | done |
| UPD-14 | A question is answered with one key, without Enter: the keys are shown after the question, Enter takes the usual action (the key in capitals), each other action has its letter and `?` lists them all. Keys pressed before the question appears are ignored, and Ctrl-C or the end of the input aborts instead of answering. When standard input is not a terminal, each answer is a line: a letter, or nothing for the usual action. | done |
| UPD-15 | Each run keeps a log of what it showed, without colours: emergup's lines, the questions and their answers, and every command with its output. The log is in a directory of the run's own under the invoking user's state directory, `$XDG_STATE_HOME/emergup/runs/` (by default `~/.local/state/emergup/runs/`), and the end of the run says where. The last 10 run directories are kept; `[runs] keep` changes how many, 0 keeps them all. | done |
| UPD-16 | In every mode of the live-package rebuild, emergup shows the packages it would rebuild and asks before rebuilding them; with none, the step is skipped without a question. In the `smart` mode the list comes from `smart-live-rebuild --pretend`, run as root, which checks the upstreams; emerge then rebuilds the packages listed without checking them again. A live package is always built from its sources, never installed from a binary package. | done |
| UPD-17 | When a step fails, built-in or custom, emergup asks what to do: `r` runs the step again, `s` goes on with the next step, `a` ends the run; with AI on, the AI actions of a failure too (AI-4). The question has no usual action: Enter is not an answer, so a failure is never passed over by accident. Running the step again does not repeat its question or its hooks before it; a step that shows what it would do and asks (UPD-10, UPD-16, NEWS-1) shows it and asks again. The step's hooks after it run once it has ended, after the last answer, and its time in the summary is that of all its runs. A step that is passed over stays failed, in the summary and in the exit status. `a` ends the run as a failure, as `--yes` does without asking: the hooks after the step and after the run still run. Ctrl-C or the end of the input at the question stop the run instead. Each failure leaves its own failure report (DIAG-1): `failure` for the first, `failure-2`, `failure-3`… for the next. | done |

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
| PRIV-3 | Hooks, custom steps and AI backends run as the invoking user, unless a hook or a custom step is explicitly configured to be elevated (a backup tool, `haskell-updater`). | partial |
| PRIV-4 | Started as root (a root shell, a root crontab), emergup runs the commands directly, turns AI assistance off, and says so once at the start. | done |
| PRIV-5 | Environment variables listed in the configuration, none by default, such as `EDITOR` for `dispatch-conf`, are kept for the commands run as root: through `sudo --preserve-env`, which the sudoers rule must allow (a rule for `ALL` does), or `run0 --setenv`; doas has no such option, and emergup says at the start of the run that `doas.conf` must keep them. A hook or a remedy run as root gets its `EMERGUP_*` variables (HOOK-4) the same way. | done |

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
| HOOK-1 | User commands, *hooks*, can run before and after the whole run and before and after any step, a custom one (HOOK-3) included. A step's hooks run when the run reaches it, before its question, and once it has ended, however it ended, unless the step is disabled; the hooks after the run run before the summary, even after a failure. Hooks at the same place run in the order of their names, a configuration file can turn off a hook another one adds, and `--pretend` shows them. A hook that fails ends the run as a failure, unless it is configured to go on with a notice, or to ask (HOOK-2). No hook starts once the run is stopped, by an interruption or at a question. | done |
| HOOK-2 | When a *check* hook fails, emergup asks whether to run its configured remedy, if it has one, and check again (e.g. "the last backup is older than 20 days — run the backup now?"), to go on without it, or to abort the run. Enter, and `--yes`, run the remedy, `--yes` only once; without a remedy they abort. A check can list the exit statuses its remedy fixes: failing otherwise — with another status, without starting, or killed by a signal — it says so and asks only whether to go on or abort, and Enter and `--yes` abort (e.g. "the backup disk is not mounted", which a backup would not fix). A check that fails again after its remedy asks again. Aborting fails the run, the hooks after it still running; `--pretend` shows the remedy a check would offer, and for which exit statuses. | done |
| HOOK-3 | Custom steps can be defined in the configuration and placed after any built-in step; several after the same step run in the order of their names, and a configuration file can turn off a custom step another one adds. A custom step is announced, asked about and listed in the summary like a built-in step, can have hooks, and when its command fails emergup asks what to do, as for a built-in step (UPD-17). It runs as the invoking user unless it is configured to be elevated (PRIV-3). An optional one whose program is not installed is skipped with a notice, without asking (UPD-4). `--pretend` shows what each would run. | done |
| HOOK-4 | Hooks receive the run context in environment variables: the hook's name (`EMERGUP_HOOK`), the step (`EMERGUP_STEP`, `run` for the whole run), whether the hook runs `before` or `after` it (`EMERGUP_WHEN`), the run directory (`EMERGUP_RUN_DIR`) and its log (`EMERGUP_LOG`), and how the step or the run ended (`EMERGUP_STEP_STATUS`, `EMERGUP_RESULT`) for the hooks after them. A hook or a remedy run as root gets them too, kept through the elevation (PRIV-5). | done |
| HOOK-5 | A hook can run only when someone answers the questions: when the run answers them by itself, as under `--yes`, it is skipped, with a notice, and the run goes on. A check can instead run but offer its remedy only then: under `--yes` its failure aborts the run, as for a check without a remedy, rather than start a long remedy such as a backup. `--pretend` shows what each would do under `--yes`. | done |

## News (NEWS)

| Id | Feature | Status |
| --- | --- | --- |
| NEWS-1 | After the sync, whether or not it synced anything, a step of its own lists the unread news items Portage found relevant to the system: a row for each with its date and title, and its repository when it is not `gentoo`; with none, the step is skipped without a question. emergup asks whether to read them, to go on leaving them unread, or to abort the run. Reading runs the configured command, `eselect news read new` by default, which shows them and marks them read: as the invoking user when they can write Portage's record of the news read (a member of the `portage` group can), as root otherwise. Then emergup asks whether to go on with the run, so that the actions a news item asks for can be taken before the update. With `--yes` the items are listed and left unread, and the run goes on; `--pretend` lists them and says what would run. | done |
| NEWS-2 | `emergup news` lists the unread news items as the news step of a run does (NEWS-1), and asks whether to read them or to quit leaving them unread; reading runs the step's command, as the invoking user when they can write Portage's record of the news read and as root otherwise, and marks them read. With none unread it says so. `emergup news --all` lists every item, those already read marked `(read)`, and asks nothing. It keeps no log, and exits with status 0, 1 when the items cannot be listed or read, and 130 when it is stopped at the question. | done |

## Failure diagnostics (DIAG)

| Id | Feature | Status |
| --- | --- | --- |
| DIAG-1 | When a step fails, emergup collects a failure report in a directory of its own in the run directory, one for each failure (UPD-17): a summary of what failed and how, the last 1000 lines of the step's output without colours, and for each package emerge names as failed its build log, copied as root since only Portage can read it, with the output of `emerge --info` and `emerge -pqv` for it, the attachments a Gentoo bug report asks for. Every failed step gets a report, a custom one too; a failed hook does not, and `--pretend` writes none. What cannot be collected is said in the summary, and does not fail the run a second time. | done |
| DIAG-2 | The end of a run that failed says where its failure reports are, a line for each in the order of the failures, before where the log is, ready to be read or attached to a bug report. | done |

## Config-file updates (ETC)

| Id | Feature | Status |
| --- | --- | --- |
| ETC-1 | After the update, pending config-file updates (`._cfg*` files) are listed. | done |
| ETC-2 | For each pending file the user can view the difference with a configurable diff tool, keep the old file, take the new one, merge them with a configurable merge tool, or postpone. | planned |
| ETC-3 | The step can instead delegate to an external tool (`dispatch-conf`, `etc-update`). | done |

## AI assistance (AI)

| Id | Feature | Status |
| --- | --- | --- |
| AI-1 | AI assistance is off by default and is enabled in the configuration by choosing a backend, `backend` in `[ai]`: `none`, the default; `claude-code`, Claude Code, its program given in `[ai.claude-code]` (by default `claude --safe-mode`, without the user's own CLAUDE.md, skills, plugins and MCP servers; a model or other options among its arguments); or `command`, any program that reads the request on its standard input and writes the answer on its standard output, given in `[ai.command]`. The Anthropic API follows, with a table of its own. `emergup config check` reports a `command` backend without a command. With AI off, the questions offer only their usual actions and the run is the same. | partial |
| AI-2 | Before emergup sends anything to an AI backend, it says what will be sent, a line for each part with its size, emergup's own instructions among them, and asks whether to send them: `y` sends them, `v` shows all of it first, `p` adds the list of installed packages, `i` adds the full `emerge --info`, `n` sends nothing. It asks before every request. By default the parts are the question's own material (the news items, the failure report, the config-file diff, emerge's plan) and a short profile of the system: the line of `emerge --version`, which names Portage, Python, the profile, the compiler, the C library, the kernel and the architecture. | done |
| AI-3 | At the questions about unread news (NEWS-1, NEWS-2), `e` explains the news items, against the installed packages when the user adds them (AI-2). | planned |
| AI-4 | At a failed step (UPD-17), `e` explains the failure from its report (DIAG-1), and `i` investigates it: an agent that reads the system without changing it and ends with a diagnosis and a proposed fix. Its confirmation (AI-2) also says what the agent may do: read the files the invoking user can read and run the read-only commands of the configuration, which it lists; one confirmation covers the investigation, and anything else the agent tries is refused, not asked. The fix is shown as the exact command or diff and applied by emergup, through elevation when it needs root, only after the user confirms it; then the failed step's question comes back, to run it again. | partial |
| AI-5 | For a pending config-file update, the AI can explain the differences or propose a merged file, shown as a diff for approval. | planned |
| AI-6 | AI backends always run as the invoking user, never with root privileges; started as root, emergup turns AI off (PRIV-4). Nothing a backend produces runs with privileges unless the user has confirmed that exact command or diff. | done |
| AI-7 | At the question after reading the news (NEWS-1), `d` asks the AI for the commands the news items need on this system before the update. emergup shows them one at a time and runs each, through elevation when it needs root, only after the user confirms it; then it asks again whether to go on with the run. | planned |
| AI-8 | The AI actions are keys of the question they belong to, offered only where they apply and when the backend supports them: `e` explains, with one request whose answer is shown, and kept in the log, as it arrives, then asks the same question again (every backend); `i` investigates (AI-4; Claude Code and the API); `c` chats (AI-10; Claude Code). Under `--yes` there are no AI actions. | partial |
| AI-9 | The AI actions of one question share one conversation: a chat after an explanation or an investigation goes on from it, and the next question starts a new one. A backend that keeps no conversation is sent the material again. With Claude Code the conversation is a session of its own, kept by Claude Code for the directory it runs in, emergup's state directory, under which the run directories and their reports are. | done |
| AI-10 | `c` opens an interactive Claude Code session in the terminal that goes on from the question's conversation: when nothing was sent yet, emergup first sends the material, asking first (AI-2), and shows the explanation, as `e` does. When the session ends, the same question comes back. The material is never on a command line, which every local user can read. In the chat Claude Code may change the system: its own permission prompts, and the elevation's, confirm each command; emergup applies nothing of it. | done |
| AI-11 | `emergup config ai` sets up AI assistance: it looks for Claude Code for the invoking user (`claude` on `PATH`, logged in according to `claude auth status`), shows the drop-in it would write in `/etc/emergup/config.d/`, and writes it through elevation once the user confirms. `emergup config check` mentions it when Claude Code is found and AI is off. The configuration is system-wide: a user without Claude Code gets no AI actions. | planned |
| AI-12 | At the questions before the `@world` update and the depclean (UPD-10), `e` explains what emerge would do: the changes, the blocks and conflicts, and what would be removed and why. | planned |

## Command line (CLI)

| Id | Feature | Status |
| --- | --- | --- |
| CLI-1 | `emergup version` (or `--version`) prints the emergup version. | done |
| CLI-2 | `emergup help` (or `-h`, `--help`) prints the usage on standard output; a missing or unknown command, or an unexpected argument, prints the usage on standard error and exits with status 2. | done |
| CLI-3 | `emergup run` runs the update step by step; `-y` (`--yes`) takes the usual answer to every question and `-p` (`--pretend`) shows the steps without running them. It exits with status 0 when every step succeeded or was skipped, 1 when a step failed, even one passed over at the question after it (UPD-17), or the run could not start, and 130 when the run was stopped: Ctrl-C, `SIGTERM`, the abort key or the end of the input at a question. Ending the run at the question after a failed step does not stop it: the status is 1. | done |
