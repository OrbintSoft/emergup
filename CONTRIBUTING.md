# Contributing to emergup

Thanks for your interest in contributing! This page explains the licensing terms
every contribution must follow and how to send changes.

## Licensing and sign-off

emergup is licensed under the **GNU General Public License, version 3 or (at
your option) any later version (`GPL-3.0-or-later`)**. Your contributions go in
under the same licence, and you keep the copyright in them.

**Every commit must carry a `Signed-off-by` trailer.** By adding it you certify
the [Developer Certificate of Origin](DCO.txt): that you wrote the change, or
otherwise have the right to submit it under the project's licence.

Sign off automatically with `-s`:

```sh
git commit -s -m "Your message"
```

This appends a line in the form:

```text
Signed-off-by: Your Name <your.email@example.com>
```

Use your real name and an email you can be reached at; it must match the author
identity of the commit.

### If the sign-off check fails

The check verifies **every commit** in the pull request, so it can fail on an
earlier commit even when your latest one is signed. Add the sign-off to all the
commits on your branch and update the pull request:

```sh
git rebase --signoff origin/master
git push --force-with-lease
```

## How to send changes

- Open a pull request against the `master` branch.
- Keep the repository language **English** (code, comments, docs, commit messages).
- Keep changes small and focused; one logical change per pull request.
- Prefer [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat(news): …`, `fix(config): …`) for commit messages.
- Run the linters and the tests before opening the pull request (`make lint`,
  `make test`, once the Makefile exists).
- Add or update tests when your change affects behaviour.

## Reporting issues

Open an issue describing what you expected, what happened, and how to reproduce
it. When attaching `emerge --info` output or build logs, check them first for
hostnames, user names, or other details you do not want to publish.
