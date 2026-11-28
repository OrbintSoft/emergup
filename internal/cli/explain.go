// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/OrbintSoft/emergup/internal/ai"
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/elevation"
	"github.com/OrbintSoft/emergup/internal/pipeline"
	"github.com/OrbintSoft/emergup/internal/report"
	"github.com/OrbintSoft/emergup/internal/steps"
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

// failureExplainer returns how a run with the AI configuration cfg
// explains a step that failed, with the material of failures, or nil when
// it has no AI: none is configured, or emergup runs as root. A run that
// answers its questions by itself asks none, so it explains nothing. The
// backend and the queries of the request run unprivileged, through the
// runner of failures.
func failureExplainer(cfg config.AI, env Env, stepsEnv pipeline.Env, failures *failureReport,
) func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) error {
	if elevation.IsRoot(env.System) {
		return nil
	}

	run := failures.recorder.Next

	var backend ai.Backend

	switch cfg.Backend {
	case config.BackendNone:
		return nil
	case config.BackendCommand:
		backend = ai.Command{Runner: run, Args: cfg.Command.Command}
	}

	extras := ai.Extras{
		Packages: func(ctx context.Context) (ai.Part, error) {
			installed, err := steps.InstalledPackages(ctx, run, env.Root)

			return ai.Packages(installed), err
		},
		Info: func(ctx context.Context) (ai.Part, error) { return ai.Info(ctx, run) },
	}

	return func(ctx context.Context, step pipeline.Step, stepReport pipeline.StepReport) error {
		parts := failures.material(step, stepReport)

		profile, err := ai.Profile(ctx, run)
		if err != nil {
			stepsEnv.Printer.Notice("the profile of the system is not sent: " + err.Error())
		} else {
			parts = append(parts, profile)
		}

		return ai.Ask(ctx, stepsEnv.Menu, backend, ai.ExplainFailure(parts...), extras)
	}
}
