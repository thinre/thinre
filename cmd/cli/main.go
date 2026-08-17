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

const usage = `usage: thinre <command> [flags]

commands:
  publish   publish a local integration manifest to Thinre Cloud

run "thinre <command> -h" for command flags, "thinre -version" for the
version.
`

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if *showVersion {
		fmt.Println("thinre", version)
		return
	}

	var err error
	switch flag.Arg(0) {
	case "publish":
		err = runPublish(flag.Args()[1:])
	case "":
		flag.Usage()
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "thinre: unknown command %q\n\n%s", flag.Arg(0), usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "thinre:", err)
		os.Exit(1)
	}
}
