package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

const version = "2.6.0"

// discogsUserAgent is the single place the version reaches the API client.
func discogsUserAgent() string { return "disc-fortune/" + version }

// program is disc-fortune's whole command surface, in help-listing order.
var program = cli.NewProgram(cli.Program[app]{
	Name:    "disc-fortune",
	Tagline: "randomly picks a record from your Discogs collection",
	Default: "pick",
	Footer:  "With no command, disc-fortune picks a random album.",
	Commands: []cli.Spec[app]{
		pickSpec,
		rerollSpec,
		listSpec,
		syncSpec,
		foldersSpec,
		historySpec,
		statsSpec,
		favoriteSpec,
		unfavoriteSpec,
		openSpec,
		migrateSpec,
		cli.Completion[app](),
		versionSpec,
		cli.Help[app](),
	},
	// FlagValues are the flags whose accepted values are compiled in, so
	// completion can offer them. TestCompletionEnumValuesAreAccepted pins
	// each value to what the parser actually takes.
	FlagValues: map[string][]string{
		"draw":  {"fresh", "any", "stale"},
		"color": {"auto", "always", "never"},
	},
})

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole process: argv in, exit code out. Every exit code comes
// from here, which is what lets a test call it without losing the test
// binary to an os.Exit.
func run(args []string, stdout, stderr io.Writer) int {
	spec, rest, err := resolve(args)
	if err != nil {
		fmt.Fprintf(stderr, "disc-fortune: %v\n", err)
		return 1
	}
	// Resolved once, here, so every path helper on app can be infallible.
	// A failure is only fatal for the commands that actually need it.
	a, cfgErr := newApp(os.Getenv, os.UserHomeDir, stdout, stderr)
	if cfgErr != nil {
		if spec.NeedsConfig {
			fmt.Fprintf(stderr, "disc-fortune: %v\n", cfgErr)
			return 1
		}
	} else if spec.NeedsConfig {
		fmt.Fprint(a.stderr, disc.MigrationNotice(a.loc, a.metaPath(), term.IsTTY(os.Stderr)))
	}
	return program.Execute(spec, rest, a, stdout, stderr)
}

// v1Signposts maps a v1 flag (leading dashes trimmed) to the v2 command that
// replaced it, for the migration signpost in resolve. Filter flags that still
// work verbatim under implicit pick (--favorites, --year, --genre, --label,
// --format) must NOT appear here: adding them would break working v2
// invocations by redirecting them to a command the user never asked for.
var v1Signposts = map[string]string{
	"sync":            "disc-fortune sync",
	"list":            "disc-fortune list",
	"list-folders":    "disc-fortune folders",
	"history":         "disc-fortune history",
	"favorite-last":   "disc-fortune favorite",
	"unfavorite-last": "disc-fortune unfavorite",
	"favorite":        "disc-fortune favorite QUERY",
}

// resolve maps raw argv to a command. The -v and v1-flag signposts are
// disc-fortune's own history, so they are checked here before the generic
// resolution in cli.
func resolve(args []string) (*cli.Spec[app], []string, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-v", "--version", "-version":
			return nil, nil, fmt.Errorf(
				"there is no %s flag; use `disc-fortune version`", args[0])
		}
		if target, ok := v1Signposts[strings.TrimLeft(args[0], "-")]; ok {
			return nil, nil, fmt.Errorf(
				"%s is now `%s` (see RELEASE_NOTES_v2.0.0.md)", args[0], target)
		}
	}
	return program.Resolve(args)
}
