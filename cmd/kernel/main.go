package main

import (
	"runtime"

	"github.com/kernel/cli/cmd"
)

var (
	version   = "dev"
	commit    = "none"
	date      = "unknown"
	goversion = runtime.Version()
)

func main() {
	cmd.Execute(cmd.Metadata{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: goversion,
	})
}
