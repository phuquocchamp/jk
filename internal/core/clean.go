package core

import "regexp"

var (
	// consoleNoteRE matches a serialized console note (hudson.console.ConsoleNote
	// PREAMBLE ... POSTAMBLE). Raw progressiveText output carries them.
	consoleNoteRE = regexp.MustCompile("\x1b\\[8mha:[^\x1b]*\x1b\\[0m")
	// ansiRE matches CSI escape sequences such as colors (AnsiColor plugin).
	ansiRE = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	// timestampRE matches the prefix the Timestamper plugin adds to plain-text logs.
	timestampRE = regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\] `)
)

// cleaner normalizes log lines. Console notes are always removed; ANSI codes
// and timestamp prefixes unless raw is set.
type cleaner struct{ raw bool }

func newCleaner(raw bool) cleaner { return cleaner{raw: raw} }

func (c cleaner) line(l string) string {
	if len(l) == 0 {
		return l
	}
	l = consoleNoteRE.ReplaceAllString(l, "")
	if c.raw {
		return l
	}
	return timestampRE.ReplaceAllString(ansiRE.ReplaceAllString(l, ""), "")
}

// stripped names what line removes, for the output's "stripped" field.
func (c cleaner) stripped() []string {
	if c.raw {
		return nil
	}
	return []string{"ansi", "timestamps"}
}
