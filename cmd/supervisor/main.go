// Command supervisor is the Thinre Supervisor: the open-source edge agent
// that runs next to a managed black-box application, receives desired state
// from Thinre Cloud over OpAMP, and reconciles it by executing locally
// defined lifecycle hooks.
//
// This is the process skeleton only; reconciliation logic lives in the
// supervisor/... packages and is wired in as milestones land.
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
		fmt.Println("thinre-supervisor", version)
		return
	}

	// Startup sequence (config load, identity, OpAMP connection) is
	// implemented in later milestones. Until then, starting without
	// -version is an explicit error rather than a silent no-op.
	fmt.Fprintln(os.Stderr, "thinre-supervisor: not yet operational; run with -version")
	os.Exit(1)
}
