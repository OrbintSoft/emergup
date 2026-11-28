// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"slices"

	"github.com/OrbintSoft/emergup/internal/ai"
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/elevation"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/steps"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// keptFiles are the files of a failure report that an explanation of the
// failure reads: small ones, the build logs never among them.
func keptFiles() []string {
	return []string{report.SummaryFile, report.OutputFile}
}

// keepingFiles is the directory of a failure report that keeps a copy of
// each file of keptFiles it writes, in kept.
type keepingFiles struct {
	report.Files

	kept map[string]*bytes.Buffer
}

// Create implements report.Files.
func (k keepingFiles) Create(name string) (io.WriteCloser, error) {
	file, err := k.Files.Create(name)
	if err != nil {
		return nil, err //nolint:wrapcheck // The directory's own error, as report.Write expects it.
	}

	if !slices.Contains(keptFiles(), name) {
		return file, nil
	}

	copied := &bytes.Buffer{}
	k.kept[name] = copied

	return keptFile{Writer: io.MultiWriter(file, copied), file: file}, nil
}

// keptFile is a file of a failure report written to a copy too.
type keptFile struct {
	io.Writer

	file io.Closer
}

// Close closes the file.
func (k keptFile) Close() error {
	return k.file.Close() //nolint:wrapcheck // The file's own error.
}

// material returns what an explanation of step, which failed as stepReport
// says, is about: the summary and the output of the failure report just
// written or, without one, the step and why it failed.
func (f *failureReport) material(step pipeline.Step, stepReport pipeline.StepReport) []ai.Part {
	summary, written := f.kept[report.SummaryFile]
	if !written {
		return []ai.Part{{
			Title: "The failed step",
			Text:  fmt.Sprintf("Step:     %s (%s)\nFailure:  %s\n", step.Name(), step.Title(), stepReport.Reason),
		}}
	}

	parts := []ai.Part{{Title: "The failure report's summary", Text: summary.String()}}

	if output := f.kept[report.OutputFile]; output != nil && output.Len() > 0 {
		parts = append(parts, ai.Part{Title: "The end of the step's output", Text: output.String()})
	}

	return parts
}

// failureHelp is how a run explains a step that failed, or chats about it.
type failureHelp = func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) error

// failureAssistant returns how a run with the AI configuration cfg explains
// a step that failed, with the material of failures, and chats about it:
// nil for what it cannot do, and for both when it has no AI, none being
// configured or emergup running as root. A run that answers its questions
// by itself asks none, so it does neither. The backend and the queries of
// a request run unprivileged, through the runner of failures; Claude Code
// runs in stateDir, the state directory.
func failureAssistant(cfg config.AI, env Env, stepsEnv pipeline.Env, failures *failureReport, stateDir string,
) (failureHelp, failureHelp) {
	if elevation.IsRoot(env.System) {
		return nil, nil
	}

	run := failures.recorder.Next

	backend := aiBackend(cfg, run, stateDir, stepsEnv.Printer)
	if backend == nil {
		return nil, nil
	}

	extras := aiExtras(run, env.Root)

	request := func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) ai.Request {
		parts := failures.material(step, stepReport)

		profile, err := ai.Profile(ctx, run)
		if err != nil {
			stepsEnv.Printer.Notice("the profile of the system is not sent: " + err.Error())
		} else {
			parts = append(parts, profile)
		}

		return ai.ExplainFailure(parts...)
	}

	explain := func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) error {
		return ai.Ask(ctx, stepsEnv.Menu, backend, failures.conversation(), request(ctx, step, stepReport), extras)
	}

	chatter, chats := backend.(ai.Chatter)
	if !chats {
		return explain, nil
	}

	chat := func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) error {
		conversation := failures.conversation()

		// A chat that goes on from the conversation sends nothing: no need
		// to gather the request.
		var sent ai.Request
		if !conversation.Started {
			sent = request(ctx, step, stepReport)
		}

		return ai.Chat(ctx, stepsEnv.Menu, chatter, conversation, sent, extras)
	}

	return explain, chat
}

// conversation returns the conversation of the question after the last
// failure: each failure has one of its own.
func (f *failureReport) conversation() *ai.Conversation {
	if f.talk == nil || f.talkOf != f.failures {
		f.talk, f.talkOf = ai.NewConversation(), f.failures
	}

	return f.talk
}

// aiBackend returns the backend that cfg chooses, which runs with run, or
// nil for none; Claude Code runs in stateDir and shows its answers with
// printer.
//
//nolint:ireturn // The backend that the configuration chooses.
func aiBackend(cfg config.AI, run runner.Runner, stateDir string, printer ui.Printer) ai.Backend {
	switch cfg.Backend {
	case config.BackendNone:
		return nil
	case config.BackendCommand:
		return ai.Command{Runner: run, Args: cfg.Command.Command}
	case config.BackendClaudeCode:
		return ai.ClaudeCode{Runner: run, Args: cfg.ClaudeCode.Command, Dir: stateDir, Printer: printer}
	}

	return nil
}

// aiExtras returns the parts the user can add to a request: the installed
// packages of the system on root, and emerge --info, both asked with run.
func aiExtras(run runner.Runner, root fs.FS) ai.Extras {
	return ai.Extras{
		Packages: func(ctx context.Context) (ai.Part, error) {
			installed, err := steps.InstalledPackages(ctx, run, root)

			return ai.Packages(installed), err
		},
		Info: func(ctx context.Context) (ai.Part, error) { return ai.Info(ctx, run) },
	}
}
