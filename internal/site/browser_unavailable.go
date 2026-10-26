package site

import (
	"fmt"
	"os"
	"path/filepath"
)

// A previous interactive export must not leak executable assets into a
// documentation-only deployment. Refuse it rather than delete user files.
func exportBrowserUnavailable(options Options) error {
	for _, name := range []string{"assets/trb.wasm", "assets/wasm_exec.js", "assets/playground-worker.js", "assets/app.js", "assets/app.css", "runtime.json", "tour.json"} {
		if _, err := os.Lstat(filepath.Join(options.OutputDir, name)); err == nil {
			return fmt.Errorf("docs-only site requires a clean output directory: found %s", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, name := range []string{"play/index.html", "tour/index.html", "type-rb/play/index.html", "type-rb/tour/index.html"} {
		if err := renderTemplate(options.OutputDir, name, "browser-unavailable.html", landingData{Version: options.Version, DocsOnly: true}); err != nil {
			return err
		}
	}
	return writeFile(options.OutputDir, "type-rb/index.html", []byte(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=/"><title>TypeRB documentation</title></head><body><a href="/">Continue to TypeRB</a></body></html>
`))
}
