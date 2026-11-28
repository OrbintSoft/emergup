// SPDX-License-Identifier: GPL-3.0-or-later

package ai_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/emergup/internal/ai"
	"github.com/OrbintSoft/emergup/internal/config"
	"github.com/OrbintSoft/emergup/internal/runner"
	"github.com/OrbintSoft/emergup/internal/runner/runnertest"
	"github.com/OrbintSoft/emergup/internal/ui"
)

// The program of the command backend of the tests, a package, and a
// synthetic line of emerge --version.
const (
	askAI   = "/usr/local/bin/ask-ai"
	foo     = "app-misc/foo-1.2"
	version = "Portage 3.0.83 (python 3.13.5-final-0, default/linux/amd64/23.0, gcc-14, glibc-2.41-r4, " +
		"6.12.30-gentoo x86_64)"
)

// errBroken is the error of a runner that cannot start a program.
var errBroken = errors.New("exec format error")

// failure is a request of the tests, with two parts.
func failure() ai.Request {
	return ai.ExplainFailure(
		ai.Part{Title: "The failure report's summary", Text: "Step:     update\nFailure:  emerge exited with status 1\n"},
		ai.Part{Title: "The system (emerge --version)", Text: version + "\n"},
	)
}

func TestARequestHasTheInstructionsThenEachPartUnderItsTitle(t *testing.T) {
	t.Parallel()

	request := ai.Request{Instructions: "Explain this.\n", Parts: []ai.Part{
		{Title: "First", Text: "one\ntwo\n"},
		{Title: "Second", Text: "three"},
	}}

	assert.Equal(t, "Explain this.\n\n## First\n\none\ntwo\n\n## Second\n\nthree\n", request.Text())
}

func TestExplainingAFailureHasItsInstructions(t *testing.T) {
	t.Parallel()

	request := failure()
	assert.Contains(t, request.Instructions, "A step of the update has just\nfailed")
	assert.Contains(t, request.Instructions, "nothing you write is run")
	assert.Len(t, request.Parts, 2)
}

func TestTheCommandBackendSendsTheRequestOnItsStandardInput(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	backend := ai.Command{Runner: &fake, Args: []string{askAI, "--short"}}
	assert.Equal(t, "ask-ai", backend.Name())

	require.NoError(t, backend.Explain(t.Context(), failure()))

	commands := fake.Commands()
	require.Len(t, commands, 1)
	assert.Equal(t, []string{askAI, "--short"}, commands[0].Args)
	assert.Equal(t, runner.Stream, commands[0].Mode, "the answer is shown and logged as it comes")
	assert.False(t, commands[0].Root, "a backend never runs as root")
	assert.Equal(t, []string{failure().Text()}, fake.Inputs())
}

func TestTheCommandBackendSaysWhenThereIsNoAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		answer runnertest.Answer
		want   string
	}{
		{
			"a program that fails",
			runnertest.Answer{Result: runner.Result{ExitCode: 3}},
			"no answer from ask-ai: it exited with status 3",
		},
		{
			"a program that does not start",
			runnertest.Answer{Result: runner.Result{ExitCode: -1}, Err: errBroken},
			"no answer from ask-ai: exec format error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var fake runnertest.Fake

			fake.Script(tt.answer)

			err := ai.Command{Runner: &fake, Args: []string{askAI}}.Explain(t.Context(), failure())
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestTheProfileIsTheLineOfEmergeVersion(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{Output: []byte(version + "\n")}})

	part, err := ai.Profile(t.Context(), &fake)
	require.NoError(t, err)
	assert.Equal(t, ai.Part{Title: "The system (emerge --version)", Text: version + "\n"}, part)

	commands := fake.Commands()
	require.Len(t, commands, 1)
	assert.Equal(t, []string{"emerge", "--version"}, commands[0].Args)
	assert.Equal(t, runner.Capture, commands[0].Mode)
	assert.False(t, commands[0].Root, "it changes nothing")
}

func TestTheInfoIsEmergeInfo(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{Output: []byte("Portage 3.0.83\nACCEPT_KEYWORDS=\"amd64\"\n")}})

	part, err := ai.Info(t.Context(), &fake)
	require.NoError(t, err)
	assert.Equal(t, ai.Part{
		Title: "The configuration of Portage (emerge --info)", Text: "Portage 3.0.83\nACCEPT_KEYWORDS=\"amd64\"\n",
	}, part)

	commands := fake.Commands()
	require.Len(t, commands, 1)
	assert.Equal(t, []string{"emerge", "--info"}, commands[0].Args)
	assert.False(t, commands[0].Root)
}

func TestTheSystemQueriesSayWhenTheyFail(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	fake.Script(runnertest.Answer{Result: runner.Result{ExitCode: 1}}, runnertest.Answer{Err: errBroken})

	_, err := ai.Profile(t.Context(), &fake)
	require.EqualError(t, err, "emerge --version exited with status 1")

	_, err = ai.Info(t.Context(), &fake)
	require.ErrorIs(t, err, errBroken)
}

func TestThePackagesAreAPart(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ai.Part{Title: "The installed packages", Text: "app-misc/foo-1.2\nsys-apps/bar-3\n"},
		ai.Packages([]string{foo, "sys-apps/bar-3"}))
}

// keys is a Keyboard that returns keys in order, then the end of the input.
type keys []rune

func (k *keys) ReadKey(context.Context) (rune, error) {
	if len(*k) == 0 {
		return 0, io.EOF
	}

	key := (*k)[0]
	*k = (*k)[1:]

	return key, nil
}

// screen is a menu answering keys, and what it writes.
func screen(answers ...rune) (ui.Menu, *bytes.Buffer) {
	var out bytes.Buffer

	printer := ui.Printer{Out: &out, Colors: config.Default().Colors, Color: false}
	pressed := keys(answers)

	return ui.Menu{Printer: printer, Keyboard: &pressed, Yes: false}, &out
}

// The screen of the request of failure.
const (
	listed = "==> To send to ask-ai:\n" +
		"    emergup's instructions         14 lines\n" +
		"    The failure report's summary    2 lines\n" +
		"    The system (emerge --version)   1 line\n"
	sendPrompt = "==> Send them to ask-ai? [Y/v/p/i/n/?] "
)

// extras returns extras that give a part each, counting how often they are
// asked for.
func extras(asked *[]string) ai.Extras {
	return ai.Extras{
		Packages: func(context.Context) (ai.Part, error) {
			*asked = append(*asked, "packages")

			return ai.Packages([]string{foo}), nil
		},
		Info: func(context.Context) (ai.Part, error) {
			*asked = append(*asked, "info")

			return ai.Part{}, errBroken
		},
	}
}

func TestConfirmListsThePartsAndSends(t *testing.T) {
	t.Parallel()

	menu, out := screen(ui.KeyEnter)

	request, send, err := ai.Confirm(t.Context(), menu, "ask-ai", failure(), ai.Extras{})
	require.NoError(t, err)
	assert.True(t, send)
	assert.Equal(t, failure(), request)
	assert.Equal(t, listed+"==> Send them to ask-ai? [Y/v/n/?] send them\n", out.String(),
		"without extras, nothing to add, but always a way not to send")
}

func TestConfirmSendsNothingWhenAsked(t *testing.T) {
	t.Parallel()

	var asked []string

	menu, out := screen('n')

	_, send, err := ai.Confirm(t.Context(), menu, "ask-ai", failure(), extras(&asked))
	require.NoError(t, err)
	assert.False(t, send)
	assert.Equal(t, listed+sendPrompt+"send nothing\n", out.String())
	assert.Empty(t, asked, "an extra is asked for only when it is added")
}

func TestConfirmShowsAllOfIt(t *testing.T) {
	t.Parallel()

	menu, out := screen('v', 'y')

	_, send, err := ai.Confirm(t.Context(), menu, "ask-ai", failure(), ai.Extras{})
	require.NoError(t, err)
	assert.True(t, send)

	written := out.String()
	assert.Contains(t, written, "==> Send them to ask-ai? [Y/v/n/?] show all of it\n"+
		"    You are helping someone who updates a Gentoo Linux system with emergup, a\n")
	assert.Contains(t, written, "    ## The failure report's summary\n    \n    Step:     update\n")
	assert.True(t, strings.HasSuffix(written, "==> Send them to ask-ai? [Y/v/n/?] send them\n"), "then asks again")
}

func TestConfirmAddsTheExtras(t *testing.T) {
	t.Parallel()

	var asked []string

	menu, out := screen('p', 'i', 'y')

	request, send, err := ai.Confirm(t.Context(), menu, "ask-ai", failure(), extras(&asked))
	require.NoError(t, err)
	assert.True(t, send)
	assert.Equal(t, []string{"packages", "info"}, asked)

	require.Len(t, request.Parts, 3, "the packages added, emerge --info not")
	assert.Equal(t, ai.Packages([]string{foo}), request.Parts[2])

	assert.Equal(t, listed+sendPrompt+"add the installed packages\n"+
		"==> To send to ask-ai:\n"+
		"    emergup's instructions         14 lines\n"+
		"    The failure report's summary    2 lines\n"+
		"    The system (emerge --version)   1 line\n"+
		"    The installed packages          1 line\n"+
		"==> Send them to ask-ai? [Y/v/i/n/?] add emerge --info\n"+
		"==> could not add emerge --info: exec format error\n"+
		"==> Send them to ask-ai? [Y/v/n/?] send them\n", out.String(),
		"what is added is listed, and cannot be added again; what fails is said")
}

func TestConfirmAbortsOnCtrlC(t *testing.T) {
	t.Parallel()

	menu, _ := screen(ui.KeyInterrupt)

	_, send, err := ai.Confirm(t.Context(), menu, "ask-ai", failure(), ai.Extras{})
	require.ErrorIs(t, err, ui.ErrAborted)
	assert.False(t, send)
}

func TestAskSendsWhatIsConfirmed(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	menu, out := screen('y')

	require.NoError(t, ai.Ask(t.Context(), menu, ai.Command{Runner: &fake, Args: []string{askAI}}, failure(), ai.Extras{}))
	assert.Equal(t, []string{failure().Text()}, fake.Inputs())
	assert.True(t, strings.HasSuffix(out.String(), "send them\n==> The answer of ask-ai:\n"), "the answer is introduced")
}

func TestAskSendsNothingUnconfirmed(t *testing.T) {
	t.Parallel()

	var fake runnertest.Fake

	menu, out := screen('n')

	require.NoError(t, ai.Ask(t.Context(), menu, ai.Command{Runner: &fake, Args: []string{askAI}}, failure(), ai.Extras{}))
	assert.Empty(t, fake.Commands())
	assert.NotContains(t, out.String(), "The answer")
}
