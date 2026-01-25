package main

import (
	"log"
	"os"

	"github.com/perfgo/perfgo/cli"
)

// Version information, set by goreleaser via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	c := cli.New()
	c.SetVersion(version, commit, date)
	err := c.Run(os.Args)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
