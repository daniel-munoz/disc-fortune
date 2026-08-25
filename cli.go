package main

import (
	"flag"
	"io"
)

// newFlagSet builds a FlagSet that never prints or exits on its own, so the
// caller controls the message and the exit code.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parseInterspersed parses args allowing flags to appear before, after, or
// around positional arguments. Go's flag package stops at the first non-flag
// argument, which would silently drop trailing flags such as the --year in
// `favorite "miles" --year 1959`.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// filterFlags holds the four filter flags shared by pick, list, favorite, and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
type filterFlags struct {
	year   *string
	genre  *string
	label  *string
	format *string
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	return &filterFlags{
		year:   fs.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)"),
		genre:  fs.String("genre", "", "Filter by genre (case-insensitive substring match)"),
		label:  fs.String("label", "", "Filter by label (case-insensitive substring match)"),
		format: fs.String("format", "", "Filter by format (case-insensitive substring match)"),
	}
}

// Filter builds a Filter from the parsed flags, validating the year format.
func (ff *filterFlags) Filter() (Filter, error) {
	if err := ParseYearFilter(*ff.year); err != nil {
		return Filter{}, err
	}
	return Filter{
		Year:   *ff.year,
		Genre:  *ff.genre,
		Label:  *ff.label,
		Format: *ff.format,
	}, nil
}

// any reports whether any filter flag was set.
func (ff *filterFlags) any() bool {
	return *ff.year != "" || *ff.genre != "" || *ff.label != "" || *ff.format != ""
}
