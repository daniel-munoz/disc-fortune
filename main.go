package main

import (
	"fmt"
	"os"
)

const version = "2.6.0"

// discogsUserAgent is the single place the version reaches the API client.
func discogsUserAgent() string { return "disc-fortune/" + version }

// fatal prints an error message to stderr and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	dispatch(os.Args[1:])
}
