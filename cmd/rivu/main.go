package main

import (
	"os"

	"github.com/manojpisini/rivu/internal/cli"
)

// ldflags targets (see docs: -X main.version=...).
var version = "1.0.1"
var commit = "dev"
var date = "unknown"

func main() {
	os.Exit(cli.Run(version, commit, date, os.Args[1:]))
}
