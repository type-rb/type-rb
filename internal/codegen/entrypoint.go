package codegen

import (
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
	output.EntryPath = "trb/entry/application/main" + extension
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
	load := func(module string) {
		if program.Mode == "ruby" {
			fmt.Fprintf(&body, "require_relative %s\n", relative(module))
		} else {
			fmt.Fprintf(&body, "await import(%s);\n", relative(module))
		}
	}
	for _, component := range ir.InitializationComponents(programs, []string{program.ModulePath}) {
		members := map[string]bool{}
		for _, module := range component.Modules {
			load(module)
			members[module] = true
		}
		if component.Cyclic {
			for _, step := range program.InitializationSteps {
				if !members[step.Module] || step.Action == "" {
					continue
				}
				if program.Mode == "ruby" {
					fmt.Fprintf(&body, "$__trb_initializers[%s].call\n", strconv.Quote(step.Module+"#"+step.Action))
				} else {
					fmt.Fprintf(&body, "await (await import(%s)).__trb_%s();\n", relative(step.Module), step.Action)
				}
			}
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
