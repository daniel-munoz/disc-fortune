package main

import (
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
)

var versionSpec = cli.Spec[app]{
	Name:    "version",
	Summary: "Print the version",
	Usage:   "Usage: disc-fortune version\n\nPrints the disc-fortune version and exits.",
	New:     func() cli.Command[app] { return &versionCmd{noArgsCmd{name: "version"}} },
}

type versionCmd struct{ noArgsCmd }

func (c *versionCmd) Run(a app) error {
	fmt.Fprintf(a.stdout, "disc-fortune %s\n", version)
	return nil
}
