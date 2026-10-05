package main

import (
	"os"

	"github.com/manojpisini/rivu/internal/cli"
)

// ldflags targets (see docs: -X main.version=...); unresolved defaults
// fall back to build info inside cli (X-05).
var version = "dev"
var commit = "none"
var date = "unknown"

func main() {
	os.Exit(cli.Run(version, commit, date, os.Args[1:]))
}
