package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var favoriteSpec = cli.Spec[app]{
	Name:        "favorite",
	NeedsConfig: true,
	Summary:     "Add an album to favorites",
	Usage: `Usage: disc-fortune favorite [QUERY] [flags]

With no QUERY, favorites the last pick. With a QUERY, favorites the one
album in your collection whose "Artist - Title" contains it, case-insensitively.
If the query matches several albums, they are listed with their release IDs
and nothing is added; narrow it with filters, or name one with --release-id.

Two pressings of a title can be identical in every other field -- two
store-exclusive colours, say -- so --release-id is the one that always works.

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
` + filterFlagHelp,
	New: func() cli.Command[app] { return &favoriteCmd{queryCmd{name: "favorite"}} },
}

var unfavoriteSpec = cli.Spec[app]{
	Name:        "unfavorite",
	NeedsConfig: true,
	Summary:     "Remove an album from favorites",
	Usage: `Usage: disc-fortune unfavorite [QUERY] [flags]

With no QUERY, unfavorites the last pick. With a QUERY, removes the one
favorite whose "Artist - Title" contains it, case-insensitively. Removing
something that is not favorited succeeds quietly.

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
` + filterFlagHelp,
	New: func() cli.Command[app] { return &unfavoriteCmd{queryCmd{name: "unfavorite"}} },
}

type favoriteCmd struct{ queryCmd }

func (c *favoriteCmd) Run(a app) error { return a.runFavorite(c.cfg) }

type unfavoriteCmd struct{ queryCmd }

func (c *unfavoriteCmd) Run(a app) error { return a.runUnfavorite(c.cfg) }
