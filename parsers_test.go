package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

// These were the production parse functions before each command became a
// type (#54). They survive as test helpers with the same signatures, so the
// parsing tests read as they always have, while driving the real path:
// cli.Parse on the command's own Parser.

func parseSelection(name string, args []string) (selection, error) {
	c := &selectionCmd{name: name}
	if err := cli.Parse(c, name, args); err != nil {
		return selection{}, err
	}
	return c.cfg, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	c := &queryCmd{name: name}
	if err := cli.Parse(c, name, args); err != nil {
		return favoriteConfig{}, err
	}
	return c.cfg, nil
}

func parseOpen(args []string) (openConfig, error) {
	c := &openCmd{}
	if err := cli.Parse(c, "open", args); err != nil {
		return openConfig{}, err
	}
	return c.cfg, nil
}

func parseHistory(args []string) (historyConfig, error) {
	c := &historyCmd{}
	if err := cli.Parse(c, "history", args); err != nil {
		return historyConfig{}, err
	}
	return c.cfg, nil
}

func parseStats(args []string) (statsConfig, error) {
	c := &statsCmd{}
	if err := cli.Parse(c, "stats", args); err != nil {
		return statsConfig{}, err
	}
	return c.cfg, nil
}

func parseSync(args []string) (syncConfig, error) {
	c := &syncCmd{}
	if err := cli.Parse(c, "sync", args); err != nil {
		return syncConfig{}, err
	}
	return c.cfg, nil
}

func parseNoArgs(name string, args []string) error {
	return cli.Parse(&noArgsCmd{name: name}, name, args)
}

func parseCompletion(args []string) (string, error) {
	c := &completionCmd{}
	if err := cli.Parse(c, "completion", args); err != nil {
		return "", err
	}
	return c.shell, nil
}
