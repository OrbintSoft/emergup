// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// What the sync says when it has nothing to sync.
const (
	noRepository  = "no repository to sync"
	nothingToSync = "nothing to sync"
)

// syncStep syncs the repositories whose last sync is older than the
// interval, those always synced, or with force all of them.
type syncStep struct {
	config config.Sync
	force  bool
}

// Name implements pipeline.Step.
func (syncStep) Name() string { return "sync" }

// Title implements pipeline.Step.
func (syncStep) Title() string { return "sync the repositories" }

// Enabled implements pipeline.Step.
func (s syncStep) Enabled() bool { return s.config.Enabled }

// Pretend implements pipeline.Step.
func (s syncStep) Pretend(ctx context.Context, env pipeline.Env) ([]string, error) {
	plan, err := s.plan(ctx, env)
	if err != nil {
		return nil, err
	}

	lines := slices.Clone(plan.notices)
	if len(plan.decisions) == 0 {
		return append(lines, noRepository), nil
	}

	for _, row := range plan.rows() {
		lines = append(lines, row.text)
	}

	due := plan.due()
	if len(due) == 0 {
		return append(lines, nothingToSync), nil
	}

	return append(lines, strings.Join(s.command(env, due), " ")), nil
}

// Run implements pipeline.Step. The repositories due are synced with one
// command, so that the hooks Portage runs after a sync run once.
func (s syncStep) Run(ctx context.Context, env pipeline.Env) pipeline.Outcome {
	plan, err := s.plan(ctx, env)
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	for _, notice := range plan.notices {
		env.Printer.Notice(notice)
	}

	if len(plan.decisions) == 0 {
		return pipeline.Skip(noRepository)
	}

	for _, row := range plan.rows() {
		env.Printer.Row(row.kind, row.text)
	}

	due := plan.due()
	if len(due) == 0 {
		interval, _ := s.config.MinInterval.MarshalText()

		return pipeline.Skip("every repository was synced less than " + string(interval) + " ago")
	}

	command := s.command(env, due)

	result, err := env.Runner.Run(ctx, runner.Command{Args: command, Mode: runner.Stream, Root: true})
	if err != nil {
		return pipeline.Fail(err.Error())
	}

	if result.ExitCode != 0 {
		return pipeline.Fail(fmt.Sprintf("%s exited with status %d", command[0], result.ExitCode))
	}

	return pipeline.Success()
}

// command returns the command line that syncs the repositories due. emerge
// writes in colour when emergup does: its output reaches the terminal
// through a pipe, where it would not by itself.
func (s syncStep) command(env pipeline.Env, due []string) []string {
	color := "--color=n"
	if env.Printer.Color {
		color = "--color=y"
	}

	command := append(slices.Clone(s.config.Command), color)

	return append(command, due...)
}

// syncPlan is what the sync does with each repository.
type syncPlan struct {
	decisions []decision
	// notices are the always-sync names the sync cannot follow.
	notices []string
}

// decision is what the sync does with one repository, and why.
type decision struct {
	name   string
	sync   bool
	reason string
}

// due returns the names of the repositories to sync.
func (p syncPlan) due() []string {
	var names []string

	for _, decision := range p.decisions {
		if decision.sync {
			names = append(names, decision.name)
		}
	}

	return names
}

// row is the line that says what is done with a repository.
type row struct {
	kind ui.Kind
	text string
}

// rows returns a row for each repository: its name and what is done with
// it, a kept one in the colour of a skip.
func (p syncPlan) rows() []row {
	width := 0
	for _, decision := range p.decisions {
		width = max(width, len(decision.name))
	}

	rows := make([]row, 0, len(p.decisions))

	for _, decision := range p.decisions {
		kind := ui.KindPlain
		if !decision.sync {
			kind = ui.KindSkip
		}

		rows = append(rows, row{kind: kind, text: fmt.Sprintf("%-*s  %s", width, decision.name, decision.reason)})
	}

	return rows
}

// plan lists the repositories Portage knows and decides what to do with
// each one that can be synced: those `emerge --sync` syncs by itself, with
// auto-sync on, and those always synced.
func (s syncStep) plan(ctx context.Context, env pipeline.Env) (syncPlan, error) {
	repositories, err := listRepositories(ctx, env.Runner)
	if err != nil {
		return syncPlan{decisions: nil, notices: nil}, err
	}

	plan := syncPlan{decisions: nil, notices: alwaysSyncNotices(s.config.AlwaysSync, repositories)}
	now := env.Now()

	for _, repo := range repositories {
		always := slices.Contains(s.config.AlwaysSync, repo.name)
		if repo.syncType == "" || !repo.autoSync && !always {
			continue
		}

		last, known := lastSync(env.Root, repo)
		plan.decisions = append(plan.decisions, s.decide(repo.name, always, last, known, now))
	}

	return plan, nil
}

// decide says whether to sync the repository name, last synced at last
// when known.
func (s syncStep) decide(name string, always bool, last time.Time, known bool, now time.Time) decision {
	interval := time.Duration(s.config.MinInterval)

	switch {
	case always:
		return decision{name: name, sync: true, reason: "always synced"}
	case s.force:
		return decision{name: name, sync: true, reason: "--force-sync"}
	case interval == 0:
		return decision{name: name, sync: true, reason: "synced on every run"}
	case !known:
		return decision{name: name, sync: true, reason: "last sync not known"}
	}

	age := max(now.Sub(last), 0)
	ago := "synced " + ui.FormatDuration(age) + " ago"

	if age >= interval {
		return decision{name: name, sync: true, reason: ago}
	}

	return decision{name: name, sync: false, reason: ago + ", kept"}
}

// alwaysSyncNotices returns a notice for each name of always that is not a
// repository that can be synced.
func alwaysSyncNotices(always config.Entries, repositories []repository) []string {
	var notices []string

	for _, name := range always {
		index := slices.IndexFunc(repositories, func(repo repository) bool { return repo.name == name })

		switch {
		case index < 0:
			notices = append(notices, "always-sync names "+name+", which is not a repository")
		case repositories[index].syncType == "":
			notices = append(notices, "always-sync names "+name+", which Portage cannot sync: it has no sync-type")
		}
	}

	return notices
}

// listRepositories asks Portage for its repositories, as repos.conf sets
// them up; the query changes nothing, so it runs unprivileged.
func listRepositories(ctx context.Context, run runner.Runner) ([]repository, error) {
	command := []string{"portageq", "repos_config", "/"}

	result, err := run.Run(ctx, runner.Command{Args: command, Mode: runner.Capture, Root: false})
	if err != nil {
		return nil, fmt.Errorf("could not list the repositories: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("could not list the repositories: %w", exitError{command: command[0], code: result.ExitCode})
	}

	return parseReposConfig(string(result.Output)), nil
}

// exitError is a command that ended with a status other than 0.
type exitError struct {
	command string
	code    int
}

func (e exitError) Error() string {
	return fmt.Sprintf("%s exited with status %d", e.command, e.code)
}

// The timestamp of an rsync or webrsync tree, which tells when the tree
// was made: Sun, 22 Nov 2026 08:30:00 +0000.
const timestampFile = "metadata/timestamp.chk"

// lastSync returns when repo was last synced, read from its tree on root:
// git records the time of the last fetch in .git/FETCH_HEAD, rsync and
// webrsync the time of the tree in metadata/timestamp.chk. A sync done
// outside emergup counts as well. It reports false when the time cannot be
// told.
func lastSync(root fs.FS, repo repository) (time.Time, bool) {
	if !path.IsAbs(repo.location) {
		return time.Time{}, false
	}

	dir := strings.TrimPrefix(path.Clean(repo.location), "/")

	switch repo.syncType {
	case "git":
		return modTime(root, path.Join(dir, ".git", "FETCH_HEAD"))
	case "rsync", "webrsync":
		file := path.Join(dir, timestampFile)

		data, err := fs.ReadFile(root, file)
		if err != nil {
			return time.Time{}, false
		}

		if stamp, err := time.Parse(time.RFC1123Z, strings.TrimSpace(string(data))); err == nil {
			return stamp, true
		}

		return modTime(root, file)
	}

	return time.Time{}, false
}

// modTime returns when the file at name on root was last modified.
func modTime(root fs.FS, name string) (time.Time, bool) {
	info, err := fs.Stat(root, name)
	if err != nil {
		return time.Time{}, false
	}

	return info.ModTime(), true
}
