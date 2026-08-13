// Command cli is the Thinre command-line client: a thin client for the
// Thinre Cloud REST API. It contains no orchestration logic.
//
// The module is named "cli"; the user-facing command name ("thinre") is
// assigned at packaging time.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("thinre", version)
		return
	}

	// Subcommands (fleet, runtime, release, rollout, ...) arrive with the
	// API they call; the full CLI is deliberately deferred until after v1.
	fmt.Fprintln(os.Stderr, "thinre: no subcommands implemented yet; run with -version")
	os.Exit(1)
}
