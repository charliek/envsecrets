package main

import (
	"os"

	"github.com/charliek/envsecrets/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
