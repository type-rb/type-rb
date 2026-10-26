package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/type-rb/type-rb/internal/playground"
	"github.com/type-rb/type-rb/internal/site"
)

func main() {
	flags := flag.NewFlagSet("trb-site", flag.ExitOnError)
	output := flags.String("out", "dist/site", "static site output directory")
	docs := flags.String("docs", "docs", "documentation source directory")
	docsOnly := flags.Bool("docs-only", false, "publish documentation and browser-unavailable pages without executable browser assets")
	version := flags.String("version", "dev", "compiler version shown by the site")
	_ = flags.Parse(os.Args[1:])

	if !*docsOnly {
		if err := playground.ExportStatic(playground.StaticOptions{OutputDir: *output, Version: *version}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := site.Export(site.Options{OutputDir: *output, DocsDir: *docs, Version: *version, DocsOnly: *docsOnly}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := site.ValidateInternalLinks(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
