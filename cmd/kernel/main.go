// kernel is the CLI for the Kernel platform — it deploys apps, manages
// sandboxed browser sessions, handles authentication, and invokes actions.
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
