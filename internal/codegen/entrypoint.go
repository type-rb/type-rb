package codegen

import (
	"crypto/sha256"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// Script modules contain declarations and initializers, never an implicit call
// to application main. This driver loads the ordered runtime roots first.
func attachScriptEntrypoint(output *Generated, program *ir.Program, programs []*ir.Program) {
	if program.CompilationUnit == "" || !ir.HasEntrypoint(program) {
		return
	}
	extension := Extension(program.Mode)
	sum := sha256.Sum256([]byte(program.ModulePath))
	output.EntryPath = fmt.Sprintf("trb/entry/%x/main%s", sum[:16], extension)
	extensions := map[string]string{}
	for _, module := range programs {
		extensions[module.ModulePath] = extension
		if module.UsesJSX && program.Mode == "typescript" {
			extensions[module.ModulePath] = ".tsx"
		}
	}
	relative := func(module string) string {
		name, _ := filepath.Rel(filepath.FromSlash(path.Dir(output.EntryPath)), filepath.FromSlash(module+extensions[module]))
		name = filepath.ToSlash(name)
		if !strings.HasPrefix(name, ".") {
			name = "./" + name
		}
		return strconv.Quote(name)
	}
	var body strings.Builder
	for _, module := range program.InitializationOrder {
		if program.Mode == "ruby" {
			fmt.Fprintf(&body, "require_relative %s\n", relative(module))
		} else {
			fmt.Fprintf(&body, "await import(%s);\n", relative(module))
		}
	}
	if program.Mode == "ruby" {
		body.WriteString("__trb_start()\n")
	} else {
		fmt.Fprintf(&body, "await (await import(%s)).__trb_start();\n", relative(program.ModulePath))
		body.WriteString("export {};\n")
	}
	output.SupportFiles = append(output.SupportFiles, SupportFile{Path: output.EntryPath, Output: []byte(body.String())})
}
