package main

import (
	"log"
	"os"

	"github.com/perfgo/perfgo/cli"
)

func main() {
	c := cli.New()
	err := c.Run(os.Args)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
