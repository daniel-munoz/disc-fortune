package term

import (
	"fmt"
	"os"
)

// Mode is the resolved value of --color.
type Mode int

const (
	// Auto colorizes only when writing to a terminal and NO_COLOR is unset.
	Auto Mode = iota
	Always
	Never
)

// ParseMode converts the --color flag value to a Mode.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "auto":
		return Auto, nil
	case "always":
		return Always, nil
	case "never":
		return Never, nil
	default:
		return Auto, fmt.Errorf("invalid --color value %q (want auto, always, or never)", s)
	}
}

// Use decides whether to emit escape sequences, given the resolved --color
// mode and whether the destination is a terminal.
//
// An explicit --color=always or --color=never always wins: no-color.org asks
// that NO_COLOR be overridable by the user's own instruction, and someone who
// typed --color=always meant it. Only under auto does NO_COLOR apply, and
// then only when non-empty, again per no-color.org.
func Use(mode Mode, tty bool, getenv func(string) string) bool {
	switch mode {
	case Always:
		return true
	case Never:
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	return tty
}

const (
	Reset     = "\033[0m"
	BoldCyan  = "\033[1;36m"
	BoldWhite = "\033[1;37m"
	Dim       = "\033[2m"
)

// IsTTY returns true if the file is a terminal.
func IsTTY(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
