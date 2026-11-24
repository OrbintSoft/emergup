// SPDX-License-Identifier: GPL-3.0-or-later

package steps

import (
	"strings"
)

// defaultSection holds the values every repository takes when its own
// section does not set them.
const defaultSection = "DEFAULT"

// repository is a repository of Portage, as repos.conf sets it up.
type repository struct {
	name     string
	location string
	syncType string
	// autoSync is whether `emerge --sync` syncs it without being asked.
	autoSync bool
}

// parseReposConfig reads what `portageq repos_config /` prints: an INI
// file with a section for each repository, whose keys take the value of
// the DEFAULT section when they do not set one. The repositories are in
// the order of their sections.
func parseReposConfig(text string) []repository {
	parsed := parseINI(text)
	repositories := make([]repository, 0, len(parsed.order))

	for _, name := range parsed.order {
		value := func(key string) string {
			if own, isSet := parsed.sections[name][key]; isSet {
				return own
			}

			return parsed.defaults[key]
		}

		repositories = append(repositories, repository{
			name:     name,
			location: value("location"),
			syncType: value("sync-type"),
			autoSync: isOn(value("auto-sync")),
		})
	}

	return repositories
}

// iniFile is the content of an INI file: the keys of each section, and
// those of DEFAULT.
type iniFile struct {
	defaults map[string]string
	sections map[string]map[string]string
	// order lists the sections other than DEFAULT as they come.
	order []string
}

// parseINI reads text as Python's configparser writes it, which Portage
// uses: [section] lines, key = value lines, comments after # or ;, and the
// next lines of a value indented.
func parseINI(text string) iniFile {
	parsed := iniFile{defaults: map[string]string{}, sections: map[string]map[string]string{}, order: nil}

	var current map[string]string

	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "", strings.HasPrefix(trimmed, "#"), strings.HasPrefix(trimmed, ";"):
		case line[0] == ' ' || line[0] == '\t':
			// The next line of a value written on more lines: none of the
			// keys read here has one.
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			current = parsed.section(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
		case current != nil:
			if key, value, isPair := strings.Cut(trimmed, "="); isPair {
				current[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
	}

	return parsed
}

// section returns the keys of the section name, made when it comes first.
func (f *iniFile) section(name string) map[string]string {
	if name == defaultSection {
		return f.defaults
	}

	if _, isKnown := f.sections[name]; !isKnown {
		f.sections[name] = map[string]string{}
		f.order = append(f.order, name)
	}

	return f.sections[name]
}

// isOn reads a yes-or-no value of repos.conf: only no and false, in any
// case, turn it off.
func isOn(value string) bool {
	switch strings.ToLower(value) {
	case "no", "false":
		return false
	}

	return true
}
