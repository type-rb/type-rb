package main

import (
	"os"

	"github.com/type-rb/type-rb/internal/cli"
)

func main() { os.Exit(cli.New().Run(os.Args[1:])) }
