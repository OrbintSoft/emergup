// SPDX-License-Identifier: GPL-3.0-or-later

// Package rundir makes the directory of each run, which holds its log,
// under emergup's state directory, and removes the oldest ones.
package rundir

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Errors of the run directories.
var (
	// errNoStateDir is the error of a system where emergup has nowhere to
	// keep its runs.
	errNoStateDir = errors.New("no state directory: neither XDG_STATE_HOME nor HOME is an absolute path")
	// errSameSecond is the error of too many runs in one second.
	errSameSecond = errors.New("runs started in the same second")
)

// LogName is the name of the log in a run directory.
const LogName = "emergup.log"

// The names of a run directory: the local time the run started at, with
// "-2", "-3"… after it for a later run that started in the same second.
const (
	nameLayout      = "2006-01-02T15-04-05"
	suffixSeparator = "-"
)

// Only the user can open the run directories and read the logs, which may
// hold what the commands printed about the system.
const (
	dirMode = 0o700
	logMode = 0o600
)

// maxSameSecond is how many runs that start in the same second get a
// directory, which only a run that stops at once can reach.
const maxSameSecond = 100

// StateDir returns emergup's state directory: emergup under
// $XDG_STATE_HOME, or under ~/.local/state when XDG_STATE_HOME is not an
// absolute path. getenv is os.Getenv.
func StateDir(getenv func(string) string) (string, error) {
	if state := getenv("XDG_STATE_HOME"); filepath.IsAbs(state) {
		return filepath.Join(state, "emergup"), nil
	}

	if home := getenv("HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".local", "state", "emergup"), nil
	}

	return "", errNoStateDir
}

// Run is the directory of one run, with its log open.
type Run struct {
	Dir     string
	LogPath string
	Log     io.WriteCloser
}

// Store makes the run directories under the state directory, in runs/.
type Store struct {
	// Getenv is os.Getenv, for StateDir.
	Getenv func(string) string
}

// Create makes the directory of a run that starts at now, named after the
// time now says, which time.Now gives in local time, and opens its log;
// only the user can read them.
func (s Store) Create(now time.Time) (Run, error) {
	runs, err := s.runs()
	if err != nil {
		return Run{Dir: "", LogPath: "", Log: nil}, err
	}

	if err := os.MkdirAll(runs, dirMode); err != nil {
		return Run{Dir: "", LogPath: "", Log: nil}, fmt.Errorf("making the run directory: %w", err)
	}

	dir, err := makeRunDir(runs, now.Format(nameLayout))
	if err != nil {
		return Run{Dir: "", LogPath: "", Log: nil}, err
	}

	logPath := filepath.Join(dir, LogName)

	//nolint:gosec // The run directory emergup has just made, under the user's own state directory.
	log, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, logMode)
	if err != nil {
		return Run{Dir: "", LogPath: "", Log: nil}, fmt.Errorf("opening the log of the run: %w", err)
	}

	return Run{Dir: dir, LogPath: logPath, Log: log}, nil
}

// makeRunDir makes the directory name in runs, or name-2, name-3… after
// the runs of that second there are already: a number is never taken again,
// even once its directory is removed, so that the latest run is the last in
// order.
func makeRunDir(runs, name string) (string, error) {
	first := highestNumber(runs, name) + 1

	for number := first; number < first+maxSameSecond; number++ {
		dir := filepath.Join(runs, name)
		if number > 1 {
			dir += suffixSeparator + strconv.Itoa(number)
		}

		err := os.Mkdir(dir, dirMode)
		if err == nil {
			return dir, nil
		}

		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("making the run directory: %w", err)
		}
	}

	return "", fmt.Errorf("making the run directory: %d %w, at %s", maxSameSecond, errSameSecond, name)
}

// highestNumber returns the highest number of the run directories in runs
// named after stamp, 0 when there is none. A runs that cannot be listed has
// none: Mkdir still refuses a name that is taken.
func highestNumber(runs, stamp string) int {
	highest := 0
	names, _ := runNames(runs)

	for _, name := range names {
		if strings.HasPrefix(name.name, stamp) {
			highest = max(highest, name.number)
		}
	}

	return highest
}

// runNames returns the run directories in runs, none when runs does not
// exist; what emergup did not create is not among them.
func runNames(runs string) ([]runName, error) {
	found, err := os.ReadDir(runs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("listing the run directories: %w", err)
	}

	var names []runName

	for _, entry := range found {
		if name, isRun := parseRunName(entry.Name()); isRun && entry.IsDir() {
			names = append(names, name)
		}
	}

	return names, nil
}

// Prune removes the oldest run directories, so that keep of them are left,
// current among them whatever its name says; 0 keeps them all. What
// emergup did not create is left alone.
func (s Store) Prune(keep int, current string) error {
	if keep == 0 {
		return nil
	}

	runs, err := s.runs()
	if err != nil {
		return err
	}

	names, err := runNames(runs)
	if err != nil {
		return err
	}

	others := slices.DeleteFunc(names, func(name runName) bool {
		return name.name == filepath.Base(current)
	})
	slices.SortFunc(others, compareRunNames)

	var problems []error

	for _, old := range others[:max(len(others)-(keep-1), 0)] {
		if err := os.RemoveAll(filepath.Join(runs, old.name)); err != nil {
			problems = append(problems, err)
		}
	}

	return errors.Join(problems...)
}

// runs returns the directory that holds the run directories.
func (s Store) runs() (string, error) {
	state, err := StateDir(s.Getenv)
	if err != nil {
		return "", err
	}

	return filepath.Join(state, "runs"), nil
}

// runName is the name of a run directory: when its run started, and its
// number among the runs that started in that second.
type runName struct {
	name    string
	started time.Time
	number  int
}

// parseRunName reads name as the name of a run directory, and reports
// whether it is one.
func parseRunName(name string) (runName, bool) {
	stamp, suffix := name, ""
	if len(name) > len(nameLayout) {
		stamp, suffix = name[:len(nameLayout)], name[len(nameLayout):]
	}

	// The names are wall-clock times: read in UTC, which has no changes of
	// the clock, they sort as they were written.
	started, err := time.Parse(nameLayout, stamp)
	if err != nil {
		return runName{name: "", started: time.Time{}, number: 0}, false
	}

	number := 1

	if suffix != "" {
		digits, hasSeparator := strings.CutPrefix(suffix, suffixSeparator)

		number, err = strconv.Atoi(digits)
		if !hasSeparator || err != nil || number < 2 {
			return runName{name: "", started: time.Time{}, number: 0}, false
		}
	}

	return runName{name: name, started: started, number: number}, true
}

// compareRunNames orders run directories from the oldest run to the
// latest.
func compareRunNames(a, b runName) int {
	if order := a.started.Compare(b.started); order != 0 {
		return order
	}

	return a.number - b.number
}
