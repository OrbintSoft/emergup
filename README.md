# emergup

Keeps a Gentoo system up to date with one command: it syncs, shows the news,
updates `@world`, rebuilds what needs rebuilding, walks you through the
config-file updates and cleans up — pausing between steps, asking for root only
when a step needs it, and helping you when something breaks.

> **Status: early development — not usable yet.** The configuration can be
> written and checked, but the update itself does not run yet. See
> [PLAN.md](PLAN.md) for the roadmap and [docs/FEATURES.md](docs/FEATURES.md)
> for what emergup will do.

## Why

Most Gentoo users end up with a personal script that chains `emaint sync`,
`emerge -uDN @world`, `@preserved-rebuild`, `dispatch-conf` and `--depclean`.
Such a script works for one machine, until something fails halfway, a news item
goes unread, or a package must be held back. emergup turns that routine into a
configurable, tested tool that works on any Gentoo system.

## What it will do

- **Run the whole update** step by step, with a confirmation between steps and a
  summary at the end; steps can be disabled or changed, and optional ones are
  skipped when their tool is not installed.
- **Run as an ordinary user**: only the commands that need root are elevated,
  through `sudo`, `doas` or `run0`.
- **Exclude packages** from the `@world` update (kernel sources, headers, GPU
  drivers, …) and remind you of what was held back.
- **Hooks and custom steps**: run your own scripts before or after any step — for
  example, check that a backup is recent enough and offer to start it if not.
- **Show the Gentoo news** before updating.
- **Collect a failure report** when a build fails: build log, `emerge --info`, the
  failing command.
- **Help with config-file updates** using the diff and merge tools you choose.
- **Optional AI assistance** (Claude Code, the Anthropic API, or any command-line
  LLM tool): explain news items, analyse a failed build and propose a fix,
  explain or merge config-file updates. Off by default; you see what is sent, and
  nothing is applied without your confirmation.

## Configuration

System-wide TOML in `/etc/emergup/config.toml`, plus drop-in files in
`/etc/emergup/config.d/*.toml`, read in lexical order; without any file,
emergup uses generic defaults. A drop-in that keeps packages out of the
update:

```toml
[steps.update]
exclude = ["sys-kernel/gentoo-sources", "sys-kernel/linux-headers"]
```

Lists such as `exclude` add up across files, like `USE` in `make.conf`.
`emergup config check` validates the files and `emergup config show` prints
the configuration in effect and where each value comes from.
[examples/etc/emergup/](examples/etc/emergup/) shows every setting with its
default.

## Contributing

Contributions are welcome. Every commit must be signed off (DCO) — see
[CONTRIBUTING.md](CONTRIBUTING.md); building, testing and linting are described
in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Support the project

If emergup is useful to you, you can sponsor its development through
[GitHub Sponsors](https://github.com/sponsors/OrbintSoft) or
[PayPal](https://paypal.me/orbintsoft) — and consider
[donating to Gentoo](https://www.gentoo.org/donate/), which makes all of this
possible.

## Licence

Copyright © 2026 Stefano Balzarotti (OrbintSoft) and contributors.

emergup is free software, licensed under the
[GNU General Public License v3.0 or later](LICENSE). See
[COPYRIGHT.md](COPYRIGHT.md).
