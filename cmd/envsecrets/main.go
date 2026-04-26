package main

import (
	"os"

	"github.com/charliek/envsecrets/internal/cli"
	"github.com/charliek/envsecrets/internal/domain"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Surface the typed exit code so CI/scripts can distinguish
		// classes of failure (reconcile vs network vs auth vs etc.) —
		// otherwise the whole domain.errorToExitCode table is dead code.
		os.Exit(domain.GetExitCode(err))
	}
}
