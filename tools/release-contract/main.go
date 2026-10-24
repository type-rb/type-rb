// release-contract maintains the versioned shared release inventory.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/type-rb/type-rb/internal/releasecontract"
)

func main() {
	root := flag.String("root", ".", "repository root")
	discover := flag.Bool("discover", false, "print the discovered public surface")
	check := flag.Bool("check", false, "check reviewed inventory, source drift and generated view")
	write := flag.Bool("write-view", false, "write the generated review view")
	template := flag.Bool("results-template", false, "print a complete unqualified result template")
	results := flag.String("results", "", "validate an evidence result bundle")
	qualify := flag.Bool("qualify", false, "require all result cells to pass (with --results)")
	flag.Parse()
	actions := 0
	for _, active := range []bool{*discover, *check, *write, *template, *results != ""} {
		if active {
			actions++
		}
	}
	if actions != 1 || flag.NArg() != 0 || (*qualify && *results == "") {
		fmt.Fprintln(os.Stderr, "usage: release-contract [--root DIRECTORY] --discover|--check|--write-view|--results-template|--results FILE [--qualify]")
		os.Exit(2)
	}
	fail := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	output := json.NewEncoder(os.Stdout)
	output.SetEscapeHTML(false)
	output.SetIndent("", "  ")
	if *discover {
		surfaces, err := releasecontract.Discover(*root)
		fail(err)
		fail(output.Encode(surfaces))
		return
	}
	manifest, catalog, digest, err := releasecontract.Load(*root)
	fail(err)
	if *template {
		fail(output.Encode(releasecontract.ResultsTemplate(manifest, digest)))
		return
	}
	if *results != "" {
		result, err := releasecontract.LoadResults(*results)
		fail(err)
		fail(releasecontract.ValidateResults(manifest, digest, result, filepath.Dir(*results), *qualify))
		if *qualify {
			fmt.Println("All release contract cells pass; independent release, migration and authorization gates still apply.")
		} else {
			fmt.Println("Result schema is valid. This does not qualify a release.")
		}
		return
	}
	fail(releasecontract.CheckDiscovery(*root, catalog))
	view := releasecontract.Render(manifest, digest)
	if *write {
		fail(os.WriteFile(filepath.Join(*root, releasecontract.ViewPath), view, 0644))
		return
	}
	stored, err := os.ReadFile(filepath.Join(*root, releasecontract.ViewPath))
	fail(err)
	if !bytes.Equal(view, stored) {
		fail(fmt.Errorf("generated view differs; run --write-view after review"))
	}
	fmt.Printf("Reviewed inventory is consistent: %d rows, %d surfaces. No release candidate is qualified by this check.\n", len(manifest.Rows), len(catalog))
}
