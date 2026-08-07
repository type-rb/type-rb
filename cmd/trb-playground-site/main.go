package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/type-rb/type-rb/internal/playground"
)

func main() {
	flags := flag.NewFlagSet("trb-playground-site", flag.ExitOnError)
	output := flags.String("out", "dist/playground", "static site output directory")
	version := flags.String("version", "dev", "compiler version shown by the site")
	_ = flags.Parse(os.Args[1:])
	if err := playground.ExportStatic(playground.StaticOptions{OutputDir: *output, Version: *version}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
