package golang

import (
	"encoding/hex"
	goast "go/ast"
	"go/format"
	goparser "go/parser"
	gotoken "go/token"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/sourcemap"
	"github.com/type-rb/type-rb/internal/types"
)

type generator struct {
	b                 strings.Builder
	indent            int
	functionDepth     int
	receiver          string
	returnType        types.Type
	inConstructor     bool
	methods           map[string]bool
	topMethods        map[string]bool
	staticMethods     map[string]map[string]bool
	records           map[string]bool
	classes           map[string]bool
	typeAliases       map[string]string
	typeKinds         map[string]string
	imports           map[string]string
	bindingNames      map[string]string
	bindingSources    map[string]bool
	lexicalNames      map[string]string
	modulePath        string
	goModule          string
	temporary         int
	breakTarget       string
	jobs              *jobsintegration.Manifest
	jobsSQL           *jobssql.Manifest
	orm               *ormintegration.Manifest
	ormCommonRuntime  bool
	ormPackageModels  []ormintegration.Model
	projectNames      *goProjectNames
	execution         *effectplan.Plan
	executionActive   bool
	oidcRuntime       bool
	arrayIndexRuntime bool
	recordSources     bool
	sourceMarker      int
	sourceLocations   map[int]sourcemap.Location
	sourcePath        string
	checkedInteger    bool
}

func Generate(program *ir.Program) string {
	return GenerateMapped(program).Output
}

func GenerateProject(programs []*ir.Program) []string {
	mapped := GenerateProjectMapped(programs)
	result := make([]string, len(mapped))
	for index, generated := range mapped {
		result[index] = generated.Output
	}
	return result
}

func GenerateMapped(program *ir.Program) sourcemap.Generated {
	return GenerateProjectMapped([]*ir.Program{program})[0]
}

func GenerateProjectMapped(programs []*ir.Program) []sourcemap.Generated {
	projectNames := analyzeGoProjectNames(programs)
	ormRuntime := analyzeGoORMRuntime(programs)
	execution := effectplan.ExecutionScope(programs)
	result := make([]sourcemap.Generated, len(programs))
	for index, program := range programs {
		result[index] = generate(program, projectNames, ormRuntime, execution)
	}
	return result
}

func generate(program *ir.Program, projectNames *goProjectNames, ormRuntime *goORMRuntimePlan, execution *effectplan.Plan) sourcemap.Generated {
	generated, imports, bindings := generatePass(program, projectNames, ormRuntime, execution, nil)
	bindingNames := analyzeGoBindingNames(bindings, imports)
	if len(bindingNames) == 0 {
		return generated
	}
	generated, _, _ = generatePass(program, projectNames, ormRuntime, execution, bindingNames)
	return generated
}

func generatePass(program *ir.Program, projectNames *goProjectNames, ormRuntime *goORMRuntimePlan, execution *effectplan.Plan, bindingNames map[string]string) (sourcemap.Generated, map[string]string, map[string]bool) {
	ormPackageKey := goORMPackageKey(program)
	g := &generator{
		topMethods:       map[string]bool{},
		staticMethods:    map[string]map[string]bool{},
		records:          map[string]bool{},
		classes:          map[string]bool{},
		typeAliases:      map[string]string{},
		typeKinds:        map[string]string{},
		imports:          map[string]string{},
		bindingNames:     bindingNames,
		bindingSources:   map[string]bool{},
		lexicalNames:     map[string]string{},
		modulePath:       program.ModulePath,
		goModule:         program.GoModule,
		jobs:             jobsintegration.ManifestFrom(program.Extensions),
		jobsSQL:          jobssql.ManifestFrom(program.Extensions),
		orm:              ormintegration.ManifestFrom(program.Extensions),
		ormCommonRuntime: ormRuntime.owners[ormPackageKey] == program.ModulePath,
		ormPackageModels: ormRuntime.models[ormPackageKey],
		projectNames:     projectNames,
		execution:        execution,
		recordSources:    true,
		sourceLocations:  map[int]sourcemap.Location{},
		sourcePath:       program.SourcePath,
	}
	for _, statement := range program.Statements {
		switch n := statement.(type) {
		case *ir.Method:
			g.topMethods[n.Name] = true
		case *ir.Class:
			g.classes[n.Name] = true
			for _, member := range n.Body {
				if method, ok := member.(*ir.Method); ok && method.Class {
					if g.staticMethods[n.Name] == nil {
						g.staticMethods[n.Name] = map[string]bool{}
					}
					g.staticMethods[n.Name][method.Name] = true
				}
			}
		case *ir.Record:
			g.records[n.Name] = true
		case *ir.Enum:
			g.typeKinds[n.Name] = "enum"
		case *ir.TypeAlias:
			g.typeKinds[n.Name] = "type_alias"
		case *ir.Newtype:
			g.typeKinds[n.Name] = "newtype"
		}
	}
	for _, statement := range program.Statements {
		if imp, ok := statement.(*ir.Import); ok {
			g.importStatement(imp)
		}
	}
	for _, statement := range program.Statements {
		if _, ok := statement.(*ir.Import); ok {
			continue
		}
		g.statement(statement)
	}
	if g.modulePath == "trb/std/time/index" {
		g.timeDatabaseInterop()
	}
	if g.modulePath == "trb/std/test/index" {
		g.testRuntimeSupport()
	}
	g.integrations(program.Extensions)
	if g.oidcRuntime {
		g.oidcBearerRuntimeSupport()
	}
	if g.arrayIndexRuntime {
		g.arrayIndexRuntimeSupport()
	}
	if g.checkedInteger || strings.Contains(g.b.String(), "trbInteger") {
		g.checkedIntegerRuntimeSupport()
	}
	g.imports = pruneUnusedImports(g.b.String(), g.imports)
	packageName := program.Package
	if packageName == "" {
		packageName = "main"
	}
	var output strings.Builder
	output.WriteString("package " + goIdentifier(packageName, false) + "\n\n")
	paths := make([]string, 0, len(g.imports))
	for importPath := range g.imports {
		paths = append(paths, importPath)
	}
	sort.Strings(paths)
	for _, importPath := range paths {
		alias := g.imports[importPath]
		if alias != "" && alias != pathpkg.Base(importPath) {
			output.WriteString("import " + goImportAlias(alias) + " " + strconv.Quote(importPath) + "\n")
		} else {
			output.WriteString("import " + strconv.Quote(importPath) + "\n")
		}
	}
	if len(paths) > 0 {
		output.WriteByte('\n')
	}
	output.WriteString(g.b.String())
	generated := strings.TrimRight(output.String(), "\n") + "\n"
	if formatted, err := format.Source([]byte(generated)); err == nil {
		output, mapping := sourcemap.ExtractMarkers(string(formatted), g.sourceLocations)
		return sourcemap.Generated{Output: output, Map: mapping}, g.imports, g.bindingSources
	}
	generated, mapping := sourcemap.ExtractMarkers(generated, g.sourceLocations)
	return sourcemap.Generated{Output: generated, Map: mapping}, g.imports, g.bindingSources
}

func pruneUnusedImports(body string, imports map[string]string) map[string]string {
	if len(imports) == 0 {
		return imports
	}
	file, err := goparser.ParseFile(gotoken.NewFileSet(), "generated.go", "package generated\n\n"+body, 0)
	if err != nil {
		// Preserve the original imports so a code-generation syntax error keeps
		// its most useful diagnostics instead of being obscured by this cleanup.
		return imports
	}
	referenced := map[string]bool{}
	goast.Inspect(file, func(node goast.Node) bool {
		selector, ok := node.(*goast.SelectorExpr)
		if !ok {
			return true
		}
		if qualifier, ok := selector.X.(*goast.Ident); ok {
			referenced[qualifier.Name] = true
		}
		return true
	})
	used := make(map[string]string, len(imports))
	for importPath, alias := range imports {
		name := alias
		if name == "" {
			name = pathpkg.Base(importPath)
		}
		name = goImportAlias(name)
		if name == "_" || name == "." || referenced[name] {
			used[importPath] = alias
		}
	}
	return used
}

func (g *generator) importStatement(imported *ir.Import) {
	if imported.Native && len(imported.RuntimeSymbols) > 0 {
		return
	}
	if (imported.Standard || imported.Official) && (!imported.Runtime || !imported.RuntimeRequired) {
		return
	}
	directory := pathpkg.Dir(imported.Path)
	if directory == "." {
		directory = ""
	}
	if directory == g.currentDirectory() {
		return
	}
	generatedDirectory := GeneratedSourceDirectory(directory)
	importPath := generatedDirectory
	if g.goModule != "" {
		importPath = pathpkg.Join(g.goModule, generatedDirectory)
	}
	if importPath == "" {
		return
	}
	alias := imported.Alias
	if alias == "" {
		alias = pathpkg.Base(directory)
	}
	// Compiler-owned portable HTTP types participate in generated dispatcher
	// code. Keep their Go import binding independent from user package names
	// such as presentation/http.
	if strings.TrimSuffix(imported.Path, "/index") == "trb/http" {
		alias = "__trb_http"
	}
	alias = g.requireSourceImport(importPath, alias)
	for _, symbol := range append(append([]string(nil), imported.Symbols...), imported.GeneratedTypeSymbols...) {
		g.typeAliases[symbol] = goImportAlias(alias)
		g.typeKinds[symbol] = imported.SymbolKinds[symbol]
	}
	if strings.TrimSuffix(imported.Path, "/index") == "trb/http" && g.typeAliases["Headers"] != "" && g.typeAliases["Header"] == "" {
		// Headers.new accepts Array<Header>, so an inferred empty array can
		// mention Header in generated Go without an explicit source import.
		g.typeAliases["Header"] = goImportAlias(alias)
		g.typeKinds["Header"] = "record"
	}
}

func (g *generator) currentDirectory() string {
	directory := pathpkg.Dir(g.modulePath)
	if directory == "." {
		return ""
	}
	return directory
}

func (g *generator) requireImport(importPath, alias string) {
	if importPath != "" {
		g.imports[importPath] = alias
	}
}

func (g *generator) requireSourceImport(importPath, alias string) string {
	if imported, exists := g.imports[importPath]; exists {
		return imported
	}
	candidate := goImportAlias(alias)
	occupied := map[string]bool{}
	for path, imported := range g.imports {
		if path == importPath || imported == "_" {
			continue
		}
		if imported == "" {
			imported = pathpkg.Base(path)
		}
		occupied[goImportAlias(imported)] = true
	}
	if occupied[candidate] {
		candidate = "__trb_import_" + hex.EncodeToString([]byte(importPath))
		for occupied[candidate] {
			candidate += "_"
		}
	}
	g.imports[importPath] = candidate
	return candidate
}

func (g *generator) statement(statement ir.Statement) {
	marker := -1
	if g.recordSources && statement.SourceSpan().Start.Line > 0 {
		marker = g.sourceMarker
		g.sourceMarker++
		g.sourceLocations[marker] = sourcemap.Location{Path: g.sourcePath, Span: statement.SourceSpan()}
		g.line(sourcemap.StartMarker(marker))
		defer g.line(sourcemap.EndMarker(marker))
	}
	switch n := statement.(type) {
	case *ir.Comment:
		text := "//" + strings.TrimPrefix(strings.TrimSpace(n.Text), "#")
		if sourcemap.IsMarkerLine(text) {
			text = strings.Replace(text, "// ", "//  ", 1)
		}
		g.line(text)
	case *ir.Class:
		if n.External {
			break
		}
		g.class(n)
	case *ir.Record:
		g.record(n)
	case *ir.Enum:
		g.enum(n)
	case *ir.TypeAlias:
		g.typeAlias(n)
	case *ir.Newtype:
		g.line("type " + goIdentifier(n.Name, true) + " = " + g.goType(n.Target) + goTrailingComment(n.TrailingComment))
		g.b.WriteByte('\n')
	case *ir.Module:
		g.line("// module " + n.Name)
		for _, member := range n.Body {
			g.statement(member)
		}
	case *ir.Interface:
		g.line("type " + goIdentifier(n.Name, true) + goTypeParameterDeclarations(n.TypeParameters) + " interface {")
		g.indent++
		for _, method := range n.Methods {
			g.line(goMethodName(method.Name) + "(" + g.methodParameters(method) + ")" + g.goReturn(method.ReturnType))
		}
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	case *ir.Method:
		g.topLevelMethod(n)
	case *ir.Variable:
		if g.functionDepth == 0 {
			name := g.bindingIdentifier(n.Name)
			if n.Constant {
				name = goConstantIdentifier(n.Owner, n.Name)
			}
			g.line("var " + name + " " + g.goType(n.Type) + " = " + g.expr(n.Value))
		} else {
			name := g.bindingIdentifier(n.Name)
			g.line(name + " := " + g.exprExpected(n.Value, n.Type))
			if namedUnusedBinding(n.Name) {
				g.line("_ = " + name)
			}
		}
	case *ir.Temporary:
		g.line("var " + n.Name + " " + g.goType(n.Type))
	case *ir.Assignment:
		target := g.assignmentTarget(n.Target)
		switch n.Operator {
		case "&&=":
			g.line(target + " = " + target + " && " + g.expr(n.Value))
		case "||=":
			g.line(target + " = " + target + " || " + g.expr(n.Value))
		default:
			if n.Target.ExprType().Kind == types.Int && isCheckedIntegerAssignment(n.Operator) {
				g.line(target + " = " + g.checkedIntegerBinary(strings.TrimSuffix(n.Operator, "="), target, g.expr(n.Value)))
			} else {
				g.line(target + " " + n.Operator + " " + g.expr(n.Value))
			}
		}
	case *ir.Return:
		if g.inConstructor && n.Value == nil {
			return
		}
		if n.Value == nil {
			g.line("return")
		} else {
			g.line("return " + g.returnExpr(n.Value))
		}
	case *ir.Break:
		if g.breakTarget != "" {
			g.line("break " + g.breakTarget)
		} else {
			g.line("break")
		}
	case *ir.Next:
		g.line("continue")
	case *ir.ExpressionStatement:
		if call, ok := n.Expression.(*ir.Call); ok && call.DeclarationOnly {
			break
		}
		if call, ok := n.Expression.(*ir.Call); ok && call.Block != nil && g.testCallBlock(call) {
			break
		}
		if identifier, ok := n.Expression.(*ir.Identifier); ok && identifier.Generated {
			g.line("_ = " + g.expr(identifier))
		} else {
			g.line(g.expr(n.Expression))
		}
	case *ir.If:
		g.line("if " + g.expr(n.Condition) + " {")
		g.indent++
		g.statements(n.Then)
		g.indent--
		for _, branch := range n.ElseIf {
			g.line("} else if " + g.expr(branch.Condition) + " {")
			g.indent++
			g.statements(branch.Body)
			g.indent--
		}
		if len(n.Else) > 0 {
			g.line("} else {")
			g.indent++
			g.statements(n.Else)
			g.indent--
		}
		g.line("}")
	case *ir.Case:
		if n.TypeUnion {
			g.typeUnionCase(n)
			break
		}
		g.statements(n.Leading)
		g.temporary++
		value := "__trbCase" + strconv.Itoa(g.temporary)
		g.line("{")
		g.indent++
		g.line(value + " := " + g.expr(n.Value) + goTrailingComment(n.TrailingComment))
		for index, branch := range n.Branches {
			header := "if "
			if index > 0 {
				header = "} else if "
			}
			condition := value + " == " + g.expr(branch.Value)
			for _, alternative := range branch.Alternatives {
				condition += " || " + value + " == " + g.expr(alternative)
			}
			if branch.PayloadEnum {
				condition = value + ".Kind == " + g.enumTag(branch)
			}
			g.line(header + condition + " {" + goTrailingComment(branch.TrailingComment))
			g.indent++
			g.caseNarrowings(branch.Narrowings)
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				field := goIdentifier(branch.Member, true) + goIdentifier(binding.Field, true)
				name := g.caseBindingIdentifier(binding)
				g.line(name + " := " + value + "." + field)
				if namedUnusedBinding(binding.Name) {
					g.line("_ = " + name)
				}
			}
			g.statements(branch.Body)
			g.indent--
		}
		if n.HasElse {
			g.line("} else {")
			g.indent++
			g.caseNarrowings(n.ElseNarrowings)
			g.statements(n.Else)
			g.indent--
		} else {
			g.line("} else {")
			g.indent++
			g.line("panic(\"unreachable exhaustive case\")")
			g.indent--
		}
		if len(n.Branches) > 0 {
			g.line("}")
		}
		g.indent--
		g.line("}")
	case *ir.While:
		g.line("for " + g.expr(n.Condition) + " {")
		g.indent++
		previousBreakTarget := g.breakTarget
		g.breakTarget = ""
		g.statements(n.Body)
		g.breakTarget = previousBreakTarget
		g.indent--
		g.line("}")
	case *ir.Iterate:
		g.iterate(n)
	case *ir.StructuredBlock:
		g.structuredBlock(n)
	}
}

func (g *generator) typeUnionCase(node *ir.Case) {
	g.statements(node.Leading)
	g.temporary++
	value := "__trbCase" + strconv.Itoa(g.temporary)
	g.line("{")
	g.indent++
	g.line(value + " := " + g.expr(node.Value) + goTrailingComment(node.TrailingComment))
	for index, branch := range node.Branches {
		typed := value + "Value" + strconv.Itoa(index+1)
		header := "if "
		if index > 0 {
			header = "} else if "
		}
		g.line(header + typed + ", ok := " + value + ".(" + g.goType(branch.MatchType) + "); ok {" + goTrailingComment(branch.TrailingComment))
		g.indent++
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			name := g.caseBindingIdentifier(binding)
			g.line(name + " := " + typed)
			if namedUnusedBinding(binding.Name) {
				g.line("_ = " + name)
			}
		}
		g.statements(branch.Body)
		g.indent--
	}
	g.line("} else {")
	g.indent++
	if node.HasElse {
		g.statements(node.Else)
	} else {
		g.line("panic(\"unreachable exhaustive case\")")
	}
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}

func (g *generator) iterate(iteration *ir.Iterate) {
	previousBreakTarget := g.breakTarget
	g.breakTarget = ""
	defer func() { g.breakTarget = previousBreakTarget }()
	if iteration.Intrinsic == "trb.orm.query.find_each" || iteration.Intrinsic == "trb.orm.query.find_in_batches" {
		g.ormBatchIterate(iteration)
		return
	}
	binding := func(index int) ir.IterationBinding {
		if index < len(iteration.Bindings) {
			return iteration.Bindings[index]
		}
		return ir.IterationBinding{Name: "_", Type: types.Type{Kind: types.Any, Name: "Any"}}
	}
	if iteration.Source.ExprType().Kind == types.Hash {
		g.requireImport("maps", "")
		keyBinding := binding(0)
		valueBinding := binding(1)
		key := g.bindingIdentifier(keyBinding.Name)
		value := g.bindingIdentifier(valueBinding.Name)
		switch {
		case keyBinding.Name != "_" && valueBinding.Name != "_":
			g.line("for " + key + ", " + value + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		case keyBinding.Name != "_":
			g.line("for " + key + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		case valueBinding.Name != "_":
			g.line("for _, " + value + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		default:
			g.line("for range maps.Clone(" + g.expr(iteration.Source) + ") {")
		}
		g.indent++
		if keyBinding.Name != "_" {
			g.line("_ = " + key)
		}
		if valueBinding.Name != "_" {
			g.line("_ = " + value)
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		return
	}
	itemBinding := binding(0)
	item := g.bindingIdentifier(itemBinding.Name)
	if iteration.Operation == "each_slice" {
		g.temporary++
		suffix := strconv.Itoa(g.temporary)
		items := "__trbItems" + suffix
		size := "__trbSize" + suffix
		offset := "__trbOffset" + suffix
		end := "__trbEnd" + suffix
		g.line("{")
		g.indent++
		g.line(items + " := " + g.iterableExpr(iteration.Source))
		g.line(size + " := " + g.expr(iteration.SliceSize))
		g.line("if " + size + " <= 0 {")
		g.indent++
		g.line("panic(\"each_slice size must be greater than zero\")")
		g.indent--
		g.line("}")
		g.line("for " + offset + " := 0; " + offset + " < len(" + items + "); " + offset + " += " + size + " {")
		g.indent++
		if itemBinding.Name != "_" {
			g.line(end + " := min(" + offset + "+" + size + ", len(" + items + "))")
			g.line(item + " := " + items + "[" + offset + ":" + end + "]")
			g.line("_ = " + item)
		}
		if iteration.WithIndex {
			indexBinding := binding(1)
			if indexBinding.Name != "_" {
				index := g.bindingIdentifier(indexBinding.Name)
				g.line(index + " := " + offset + " / " + size)
				g.line("_ = " + index)
			}
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		g.indent--
		g.line("}")
		return
	}
	indexBinding := binding(1)
	if iteration.WithIndex && indexBinding.Name != "_" && itemBinding.Name != "_" {
		g.line("for " + g.bindingIdentifier(indexBinding.Name) + ", " + item + " := range " + g.iterableExpr(iteration.Source) + " {")
	} else if iteration.WithIndex && indexBinding.Name != "_" {
		g.line("for " + g.bindingIdentifier(indexBinding.Name) + " := range " + g.iterableExpr(iteration.Source) + " {")
	} else if itemBinding.Name != "_" {
		g.line("for _, " + item + " := range " + g.iterableExpr(iteration.Source) + " {")
	} else {
		g.line("for range " + g.iterableExpr(iteration.Source) + " {")
	}
	g.indent++
	if itemBinding.Name != "_" {
		g.line("_ = " + item)
	}
	if iteration.WithIndex && indexBinding.Name != "_" {
		g.line("_ = " + g.bindingIdentifier(indexBinding.Name))
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
}

func (g *generator) iterableExpr(expression ir.Expression) string {
	value := g.expr(expression)
	if expression.ExprType().Kind != types.Range {
		return value
	}
	return "func(bounds [3]int) []int { start, end, exclusive := bounds[0], bounds[1], bounds[2] == 1; values := []int{}; for current := start; current < end; current++ { values = append(values, current) }; if !exclusive && start <= end { values = append(values, end) }; return values }(" + value + ")"
}

func (g *generator) exprExpected(expression ir.Expression, expected types.Type) string {
	if array, ok := expression.(*ir.Array); ok && len(array.Elements) == 0 && expected.Kind == types.Array {
		return g.goType(expected) + "{}"
	}
	if literal, ok := expression.(*ir.Literal); ok && literal.Kind == "nil" && expected.Nullable {
		return "(" + g.goType(expected) + ")(nil)"
	}
	return g.expr(expression)
}

func (g *generator) returnExpr(expression ir.Expression) string {
	conversion, ok := expression.(*ir.Conversion)
	if !ok || conversion.Kind != ir.NonNullableToNullableConversion || !g.returnType.Nullable {
		return g.expr(expression)
	}
	if kind := g.typeKinds[g.returnType.Name]; kind != "type_alias" && kind != "enum_alias" {
		return g.expr(expression)
	}
	return g.nonNullableToNullableExpr(conversion, g.returnType)
}

func (g *generator) nonNullableToNullableExpr(conversion *ir.Conversion, target types.Type) string {
	value := g.expr(conversion.Value)
	base := target
	base.Nullable = false
	nullable := target
	valueBase := conversion.Value.ExprType()
	valueBase.Nullable = false
	if kind := g.typeKinds[valueBase.Name]; kind == "type_alias" || kind == "enum_alias" {
		base = valueBase
		nullable = valueBase
		nullable.Nullable = true
	}
	if conversion.Value.ExprType().Kind == types.Int && base.Kind == types.Float {
		value = "float64(" + value + ")"
	}
	baseType := g.goType(base)
	nullableType := g.goType(nullable)
	if baseType == nullableType {
		return value
	}
	return "func(value " + baseType + ") " + nullableType + " { return &value }(" + value + ")"
}

func (g *generator) record(record *ir.Record) {
	g.line("type " + goIdentifier(record.Name, true) + goTypeParameterDeclarations(record.TypeParameters) + " struct {")
	g.indent++
	for _, member := range record.Body {
		switch field := member.(type) {
		case *ir.Comment:
			g.statement(field)
		case *ir.RecordField:
			tags := []string{"json:" + strconv.Quote(field.Name)}
			for _, attribute := range field.Attributes {
				if attribute.Name != "gorm" && attribute.Name != "json" || len(attribute.Arguments) == 0 {
					continue
				}
				literal, ok := attribute.Arguments[0].Value.(*ir.Literal)
				if !ok || literal.Kind != "string" {
					continue
				}
				value, err := strconv.Unquote(literal.Raw)
				if err != nil {
					continue
				}
				key := attribute.Name
				if key == "json" {
					tags[0] = "json:" + strconv.Quote(value)
				} else {
					tags = append(tags, key+":"+strconv.Quote(value))
				}
			}
			g.line(goIdentifier(field.Name, true) + " " + g.goType(field.Type) + " `" + strings.Join(tags, " ") + "`")
		}
	}
	g.indent--
	g.line("}")
	fields := recordFields(record.Body)
	if recordFieldsHaveDefaults(fields) {
		g.b.WriteByte('\n')
		g.recordDefaultConstructor(record, fields)
	}
	g.b.WriteByte('\n')
}

func (g *generator) recordDefaultConstructor(record *ir.Record, fields []*ir.RecordField) {
	parameters := make([]string, 0, len(fields)*2)
	locals := make([]string, len(fields))
	execution := g.execution != nil && g.execution.RecordDefaultFor(record)
	if execution {
		g.requireImport("context", "trbcontext")
		parameters = append(parameters, "__trbScope trbcontext.Context")
	}
	for index, field := range fields {
		local := "__trbField" + strconv.Itoa(index)
		locals[index] = local
		parameters = append(parameters, local+" "+g.goType(field.Type))
		if field.Default != nil {
			parameters = append(parameters, local+"Provided bool")
		}
	}
	name := goRecordConstructorName(record.Name)
	result := goIdentifier(record.Name, true) + goTypeParameterArguments(record.TypeParameters)
	g.line("func " + name + goTypeParameterDeclarations(record.TypeParameters) + "(" + strings.Join(parameters, ", ") + ") " + result + " {")
	g.indent++
	previousExecution := g.executionActive
	previousLexicalNames := g.lexicalNames
	g.lexicalNames = make(map[string]string, len(previousLexicalNames)+len(fields))
	for name, target := range previousLexicalNames {
		g.lexicalNames[name] = target
	}
	g.executionActive = execution
	for index, field := range fields {
		local := locals[index]
		if field.Default != nil {
			g.line("if !" + local + "Provided {")
			g.indent++
			g.line(local + " = " + g.expr(field.Default))
			g.indent--
			g.line("}")
		}
		g.lexicalNames[field.Name] = local
	}
	g.executionActive = previousExecution
	g.lexicalNames = previousLexicalNames
	values := make([]string, len(fields))
	for index, field := range fields {
		values[index] = goIdentifier(field.Name, true) + ": " + locals[index]
	}
	g.line("return " + result + "{" + strings.Join(values, ", ") + "}")
	g.indent--
	g.line("}")
}

func recordFields(statements []ir.Statement) []*ir.RecordField {
	fields := make([]*ir.RecordField, 0, len(statements))
	for _, statement := range statements {
		if field, ok := statement.(*ir.RecordField); ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func recordFieldsHaveDefaults(fields []*ir.RecordField) bool {
	for _, field := range fields {
		if field.Default != nil {
			return true
		}
	}
	return false
}

func recordContractHasDefaults(fields []ir.RecordFieldContract) bool {
	for _, field := range fields {
		if field.HasDefault {
			return true
		}
	}
	return false
}

func goRecordConstructorName(name string) string {
	// Keep the exported helper reachable across generated Go packages while
	// retaining an identifier shape that source callable lowering cannot create.
	return "Trb__RecordNew__" + goIdentifier(name, true)
}

func (g *generator) enum(enum *ir.Enum) {
	name := goIdentifier(enum.Name, true)
	if enumHasPayload(enum) {
		g.payloadEnum(enum, name)
		g.enumMethods(enum, name)
		return
	}
	underlying := "int"
	if enum.RawType.Kind != "" {
		underlying = g.goType(enum.RawType)
	}
	g.line("type " + name + " " + underlying + goTrailingComment(enum.TrailingComment))
	g.b.WriteByte('\n')
	g.line("const (")
	g.indent++
	first := true
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			line := goConstantIdentifier(enum.Name, member.Name)
			if member.RawValue != nil {
				line += " " + name + " = " + g.expr(member.RawValue)
			} else if first {
				line += " " + name + " = iota"
			}
			first = false
			g.line(line + goTrailingComment(member.TrailingComment))
		}
	}
	g.indent--
	g.line(")")
	g.b.WriteByte('\n')
	g.enumMethods(enum, name)
}

func (g *generator) enumMethods(enum *ir.Enum, enumName string) {
	for _, statement := range enum.Body {
		method, ok := statement.(*ir.Method)
		if !ok || method.External {
			continue
		}
		parameters := g.methodParameters(method)
		if parameters != "" {
			parameters = ", " + parameters
		}
		typeParameters := append(append([]string(nil), enum.TypeParameters...), method.TypeParameters...)
		g.line("func " + enumMethodName(enum.Name, method.Name) + goTypeParameterDeclarations(typeParameters) + "(self " + enumName + goTypeParameterArguments(enum.TypeParameters) + parameters + ")" + g.goReturn(method.ReturnType) + " {")
		g.indent++
		previousExecution := g.executionActive
		g.executionActive = g.methodUsesExecutionScope(method)
		g.parameterDefaults(method.Parameters)
		g.functionDepth++
		g.statements(method.Body)
		g.executionActive = previousExecution
		g.functionDepth--
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
}

func enumMethodName(enumName, methodName string) string {
	return goIdentifier(enumName, true) + goMethodName(methodName)
}

func (g *generator) typeAlias(alias *ir.TypeAlias) {
	name := goIdentifier(alias.Name, true)
	g.line("type " + name + goTypeParameterDeclarations(alias.TypeParameters) + " = " + g.typeAliasTarget(alias) + goTrailingComment(alias.TrailingComment))
	if len(alias.Variants) == 0 {
		g.b.WriteByte('\n')
		return
	}
	target := alias.AuthoredTarget
	if target.Kind == "" {
		target = alias.Target
	}
	targetName := goIdentifier(target.Name, true)
	targetPrefix := ""
	if imported := g.typeAliases[target.Name]; alias.AuthoredTargetReference != nil && imported != "" {
		targetPrefix = imported + "."
	}
	for _, variant := range alias.Variants {
		aliasConstant := goConstantIdentifier(alias.Name, variant.Name)
		targetConstant := targetPrefix + goConstantIdentifier(target.Name, variant.Name)
		if len(variant.Fields) == 0 {
			g.line("var " + aliasConstant + " = " + targetConstant)
			continue
		}
		g.line("const " + aliasConstant + "Tag = " + targetConstant + "Tag")
		constructor := "New" + name + goIdentifier(variant.Name, true)
		returnType := name + goTypeParameterArguments(alias.TypeParameters)
		g.line("func " + constructor + goTypeParameterDeclarations(alias.TypeParameters) + "(" + g.parameters(variant.Fields) + ") " + returnType + " {")
		g.indent++
		targetConstructor := targetPrefix + "New" + targetName + goIdentifier(variant.Name, true)
		if len(target.Args) > 0 {
			arguments := make([]string, len(target.Args))
			for index, argument := range target.Args {
				arguments[index] = g.goType(argument)
			}
			targetConstructor += "[" + strings.Join(arguments, ", ") + "]"
		}
		values := make([]string, len(variant.Fields))
		for index, field := range variant.Fields {
			values[index] = g.bindingIdentifier(field.Name)
		}
		g.line("return " + targetConstructor + "(" + strings.Join(values, ", ") + ")")
		g.indent--
		g.line("}")
	}
	g.b.WriteByte('\n')
}

func (g *generator) payloadEnum(enum *ir.Enum, name string) {
	tagType := name + "Tag"
	g.line("type " + tagType + " int" + goTrailingComment(enum.TrailingComment))
	g.b.WriteByte('\n')
	g.line("const (")
	g.indent++
	first := true
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			line := goConstantIdentifier(enum.Name, member.Name) + "Tag"
			if first {
				line += " " + tagType + " = iota"
				first = false
			}
			g.line(line + goTrailingComment(member.TrailingComment))
		}
	}
	g.indent--
	g.line(")")
	g.b.WriteByte('\n')

	g.line("type " + name + goTypeParameterDeclarations(enum.TypeParameters) + " struct {")
	g.indent++
	g.line("Kind " + tagType)
	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		for _, field := range member.Fields {
			fieldName := goIdentifier(member.Name, true) + goIdentifier(field.Name, true)
			g.line(fieldName + " " + g.goType(field.Type))
		}
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		constant := goConstantIdentifier(enum.Name, member.Name)
		if len(member.Fields) == 0 {
			g.line("var " + constant + " = " + name + "{Kind: " + constant + "Tag}")
			continue
		}
		constructor := "New" + goIdentifier(enum.Name, true) + goIdentifier(member.Name, true)
		genericDeclarations := goTypeParameterDeclarations(enum.TypeParameters)
		genericArguments := goTypeParameterArguments(enum.TypeParameters)
		g.line("func " + constructor + genericDeclarations + "(" + g.parameters(member.Fields) + ") " + name + genericArguments + " {")
		g.indent++
		fields := []string{"Kind: " + constant + "Tag"}
		for _, field := range member.Fields {
			fieldName := goIdentifier(member.Name, true) + goIdentifier(field.Name, true)
			fields = append(fields, fieldName+": "+g.bindingIdentifier(field.Name))
		}
		g.line("return " + name + genericArguments + "{" + strings.Join(fields, ", ") + "}")
		g.indent--
		g.line("}")
	}
	g.b.WriteByte('\n')
}

func enumHasPayload(enum *ir.Enum) bool {
	for _, statement := range enum.Body {
		if member, ok := statement.(*ir.EnumMember); ok && len(member.Fields) > 0 {
			return true
		}
	}
	return false
}

func goTypeParameterDeclarations(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = goIdentifier(parameter, true) + " any"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func goTypeParameterArguments(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = goIdentifier(parameter, true)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (g *generator) enumTag(branch ir.CaseBranch) string {
	name := goConstantIdentifier(branch.EnumName, branch.Member) + "Tag"
	if member, ok := branch.Value.(*ir.Member); ok {
		if alias := g.referenceAlias(member.Reference); alias != "" {
			return alias + "." + name
		}
	}
	return name
}

func (g *generator) statements(statements []ir.Statement) {
	if g.executionActive && len(statements) > 0 {
		g.line("if err := __trbScope.Err(); err != nil { panic(err) }")
	}
	for _, statement := range statements {
		g.statement(statement)
	}
}

func (g *generator) typeAliasTarget(alias *ir.TypeAlias) string {
	target := alias.AuthoredTarget
	if target.Kind == "" {
		target = alias.Target
	}
	if alias.AuthoredTargetReference != nil || target.Kind != types.Named || target.Name == "" {
		return g.goType(target)
	}
	imported, exists := g.typeAliases[target.Name]
	delete(g.typeAliases, target.Name)
	result := g.goType(target)
	if exists {
		g.typeAliases[target.Name] = imported
	}
	return result
}

func (g *generator) class(class *ir.Class) {
	name := goIdentifier(class.Name, true)
	typeDeclarations := goTypeParameterDeclarations(class.TypeParameters)
	typeArguments := goTypeParameterArguments(class.TypeParameters)
	fields := []*ir.Field{}
	methods := []*ir.Method{}
	for _, member := range class.Body {
		switch n := member.(type) {
		case *ir.Field:
			fields = append(fields, n)
		case *ir.Method:
			methods = append(methods, n)
		case *ir.Variable:
			g.statement(n)
		}
	}
	previousMethods := g.methods
	g.methods = map[string]bool{}
	for _, method := range methods {
		if method.External {
			continue
		}
		g.methods[method.Name] = true
	}
	defer func() { g.methods = previousMethods }()
	g.line("type " + name + typeDeclarations + " struct {")
	g.indent++
	if class.Superclass != nil {
		superclass := g.expr(class.Superclass)
		if identifier, ok := class.Superclass.(*ir.Identifier); ok {
			superclass = goIdentifier(identifier.Name, true)
			if alias := g.typeAliases[identifier.Name]; alias != "" {
				superclass = alias + "." + goIdentifier(identifier.Name, true)
			}
		}
		g.line("*" + superclass)
	}
	for _, field := range fields {
		g.line(goFieldName(field.Name) + " " + g.goType(field.Type))
	}
	g.indent--
	g.line("}")
	if len(class.Implements) > 0 && len(class.TypeParameters) > 0 {
		g.line("func __trbAssert" + name + "Interfaces" + typeDeclarations + "() {")
		g.indent++
		for _, implemented := range class.Implements {
			g.line("var _ " + g.goType(implemented) + " = (*" + name + typeArguments + ")(nil)")
		}
		g.indent--
		g.line("}")
	} else {
		for _, implemented := range class.Implements {
			g.line("var _ " + g.goType(implemented) + " = (*" + name + typeArguments + ")(nil)")
		}
	}
	g.b.WriteByte('\n')

	initialize := findInitialize(methods)
	{
		parameters := g.classConstructorParameters(class, initialize)
		g.line("func New" + name + typeDeclarations + "(" + parameters + ") *" + name + typeArguments + " {")
		g.indent++
		previousExecution := g.executionActive
		g.executionActive = g.classConstructorUsesExecutionScope(class, initialize)
		if initialize != nil {
			g.parameterDefaults(initialize.Parameters)
		}
		g.line("self := &" + name + typeArguments + "{}")
		for _, field := range fields {
			if field.Value != nil {
				g.line("self." + goFieldName(field.Name) + " = " + g.expr(field.Value))
			}
		}
		previousReceiver, previousConstructor := g.receiver, g.inConstructor
		g.receiver, g.inConstructor = "self", true
		g.functionDepth++
		if initialize != nil {
			g.statements(initialize.Body)
		}
		g.functionDepth--
		g.executionActive = previousExecution
		g.receiver, g.inConstructor = previousReceiver, previousConstructor
		g.line("return self")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}

	for _, method := range methods {
		if method.Name == "initialize" || method.External {
			continue
		}
		g.classMethod(name, class.TypeParameters, method)
	}
}

func (g *generator) classMethod(className string, classTypeParameters []string, method *ir.Method) {
	name := goMethodName(method.Name)
	if method.Class {
		g.line("func " + className + name + "(" + g.methodParameters(method) + ")" + g.goReturn(method.ReturnType) + " {")
	} else {
		g.line("func (self *" + className + goTypeParameterArguments(classTypeParameters) + ") " + name + goTypeParameterDeclarations(method.TypeParameters) + "(" + g.methodParameters(method) + ")" + g.goReturn(method.ReturnType) + " {")
	}
	g.indent++
	previousExecution := g.executionActive
	g.executionActive = g.methodUsesExecutionScope(method)
	g.parameterDefaults(method.Parameters)
	previous := g.receiver
	g.receiver = "self"
	previousReturnType := g.returnType
	g.returnType = method.ReturnType
	g.functionDepth++
	g.statements(method.Body)
	g.executionActive = previousExecution
	g.functionDepth--
	g.returnType = previousReturnType
	g.receiver = previous
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) topLevelMethod(method *ir.Method) {
	name := g.projectFunctionName(g.modulePath, method.Name)
	parameters := g.methodParameters(method)
	if method.Name == "main" {
		parameters = g.parameters(method.Parameters)
	}
	g.line("func " + name + goTypeParameterDeclarations(method.TypeParameters) + "(" + parameters + ")" + g.goReturn(method.ReturnType) + " {")
	g.indent++
	if method.Name == "main" && g.methodUsesExecutionScope(method) {
		g.requireImport("context", "trbcontext")
		g.line("__trbScope := trbcontext.Background()")
	}
	previousExecution := g.executionActive
	g.executionActive = g.methodUsesExecutionScope(method)
	g.parameterDefaults(method.Parameters)
	previousReturnType := g.returnType
	g.returnType = method.ReturnType
	if method.Name == "main" && g.modulePath != "trb_test_main" && g.jobs != nil && len(g.jobs.Jobs) > 0 {
		g.line("if trbJobsRunWorkerIfRequested() { return }")
	}
	if method.Name == "main" && g.orm != nil && len(g.orm.Models) > 0 {
		g.line("defer " + g.ormLifecycleAlias() + ".TrbOrmCloseDatabase()")
	}
	g.functionDepth++
	g.statements(method.Body)
	g.executionActive = previousExecution
	g.functionDepth--
	g.returnType = previousReturnType
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) methodUsesExecutionScope(method *ir.Method) bool {
	return method != nil && (strings.HasPrefix(method.TargetName, "trb_web_route_") ||
		strings.HasPrefix(method.TargetName, "trb_web_middleware_") ||
		g.execution != nil && g.execution.Methods[method])
}

func (g *generator) classConstructorUsesExecutionScope(class *ir.Class, initialize *ir.Method) bool {
	return g.methodUsesExecutionScope(initialize) || g.execution != nil && g.execution.ClassConstructors[class]
}

func (g *generator) classConstructorParameters(class *ir.Class, initialize *ir.Method) string {
	parameters := ""
	if initialize != nil {
		parameters = g.parameters(initialize.Parameters)
	}
	if !g.classConstructorUsesExecutionScope(class, initialize) {
		return parameters
	}
	g.requireImport("context", "trbcontext")
	if parameters == "" {
		return "__trbScope trbcontext.Context"
	}
	return "__trbScope trbcontext.Context, " + parameters
}

func (g *generator) methodParameters(method *ir.Method) string {
	parameters := g.parameters(method.Parameters)
	if !g.methodUsesExecutionScope(method) {
		return parameters
	}
	g.requireImport("context", "trbcontext")
	if parameters == "" {
		return "__trbScope trbcontext.Context"
	}
	return "__trbScope trbcontext.Context, " + parameters
}

func (g *generator) executionArguments(call *ir.Call, arguments []string) []string {
	if g.execution == nil || !g.execution.Calls[call] {
		return arguments
	}
	return append([]string{g.executionScopeArgument()}, arguments...)
}

func (g *generator) executionScopeArgument() string {
	if g.executionActive {
		return "__trbScope"
	}
	g.requireImport("context", "trbcontext")
	return "trbcontext.Background()"
}

func (g *generator) nativeRuntimeCall(binding *ir.RuntimeBinding, arguments []string) string {
	alias := g.requireSourceImport(binding.Module, pathpkg.Base(binding.Dependency))
	return goImportAlias(alias) + "." + binding.Symbol + "(" + strings.Join(arguments, ", ") + ")"
}

func (g *generator) parameters(parameters []ir.Parameter) string {
	if hasNamedOnlyParameters(parameters) {
		parts := []string{}
		optionalPositional := false
		for _, parameter := range parameters {
			if parameter.NamedOnly {
				continue
			}
			if parameter.Default != nil {
				optionalPositional = true
				continue
			}
			parts = append(parts, g.bindingIdentifier(parameter.Name)+" "+g.goType(parameter.Type))
		}
		if optionalPositional {
			parts = append(parts, "__trbOptional []any")
		}
		parts = append(parts, "__trbNamed map[string]any")
		return strings.Join(parts, ", ")
	}
	optionalStart := optionalParameterStart(parameters)
	if optionalStart < 0 {
		optionalStart = len(parameters)
	}
	parts := make([]string, 0, optionalStart+1)
	for _, parameter := range parameters[:optionalStart] {
		name := g.bindingIdentifier(parameter.Name)
		typ := g.goType(parameter.Type)
		if parameter.Rest {
			typ = "..." + strings.TrimPrefix(typ, "[]")
		}
		parts = append(parts, name+" "+typ)
	}
	if optionalStart < len(parameters) {
		parts = append(parts, "__trbOptional ...any")
	}
	return strings.Join(parts, ", ")
}

func hasNamedOnlyParameters(parameters []ir.Parameter) bool {
	for _, parameter := range parameters {
		if parameter.NamedOnly {
			return true
		}
	}
	return false
}

func optionalParameterStart(parameters []ir.Parameter) int {
	start := -1
	for index, parameter := range parameters {
		if parameter.Default != nil {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			return -1
		}
	}
	return start
}

func (g *generator) parameterDefaults(parameters []ir.Parameter) {
	if hasNamedOnlyParameters(parameters) {
		g.namedParameterDefaults(parameters)
		return
	}
	start := optionalParameterStart(parameters)
	if start < 0 {
		return
	}
	for index, parameter := range parameters[start:] {
		name := g.bindingIdentifier(parameter.Name)
		typ := g.goType(parameter.Type)
		g.line("var " + name + " " + typ)
		g.line("if len(__trbOptional) > " + strconv.Itoa(index) + " {")
		g.indent++
		g.assignDynamicValue(name, "__trbOptional["+strconv.Itoa(index)+"]", parameter.Type)
		g.indent--
		g.line("} else {")
		g.indent++
		g.line(name + " = " + g.expr(parameter.Default))
		g.indent--
		g.line("}")
		g.line("_ = " + name)
	}
}

func (g *generator) namedParameterDefaults(parameters []ir.Parameter) {
	optionalIndex := 0
	for _, parameter := range parameters {
		name := g.bindingIdentifier(parameter.Name)
		typ := g.goType(parameter.Type)
		switch {
		case !parameter.NamedOnly && parameter.Default != nil:
			g.line("var " + name + " " + typ)
			g.line("if len(__trbOptional) > " + strconv.Itoa(optionalIndex) + " {")
			g.indent++
			g.assignDynamicValue(name, "__trbOptional["+strconv.Itoa(optionalIndex)+"]", parameter.Type)
			g.indent--
			g.line("} else {")
			g.indent++
			g.line(name + " = " + g.expr(parameter.Default))
			g.indent--
			g.line("}")
			g.line("_ = " + name)
			optionalIndex++
		case parameter.NamedOnly:
			g.line("var " + name + " " + typ)
			g.line("if __trbNamedValue, ok := __trbNamed[" + strconv.Quote(parameter.Name) + "]; ok {")
			g.indent++
			g.assignDynamicValue(name, "__trbNamedValue", parameter.Type)
			g.indent--
			if parameter.Default == nil {
				g.line("} else {")
				g.indent++
				g.line("panic(" + strconv.Quote("missing named argument "+parameter.Name) + ")")
				g.indent--
			} else {
				g.line("} else {")
				g.indent++
				g.line(name + " = " + g.expr(parameter.Default))
				g.indent--
			}
			g.line("}")
			g.line("_ = " + name)
		}
	}
}

func (g *generator) assignDynamicValue(target, value string, typ types.Type) {
	switch {
	case typ.Kind == types.Any:
		g.line(target + " = " + value)
	case typ.Nullable:
		g.line("if " + value + " == nil {")
		g.indent++
		g.line(target + " = nil")
		g.indent--
		g.line("} else {")
		g.indent++
		g.line(target + " = " + value + ".(" + g.goType(typ) + ")")
		g.indent--
		g.line("}")
	default:
		g.line(target + " = " + value + ".(" + g.goType(typ) + ")")
	}
}

func (g *generator) expr(expression ir.Expression) string {
	if expression == nil {
		return ""
	}
	switch n := expression.(type) {
	case *ir.If:
		return g.ifExpression(n)
	case *ir.Case:
		return g.caseExpression(n)
	case *ir.Lambda:
		parts := make([]string, len(n.Parameters))
		for index, parameter := range n.Parameters {
			parts[index] = g.bindingIdentifier(parameter.Name) + " " + g.goType(parameter.Type)
		}
		header := "func(" + strings.Join(parts, ", ") + ")"
		if returned := g.goType(n.ReturnType); returned != "" {
			header += " " + returned
		}
		child := *g
		child.b = strings.Builder{}
		child.recordSources = false
		child.indent = g.indent + 1
		child.returnType = n.ReturnType
		child.statements(n.Body)
		return header + " {\n" + child.b.String() + strings.Repeat("\t", g.indent) + "}"
	case *ir.Identifier:
		if n.Generated {
			return n.Name
		}
		if strings.HasPrefix(n.Name, "@") {
			return "self." + goFieldName(n.Name)
		}
		if n.Name == "self" {
			return "self"
		}
		if n.Lexical {
			return g.bindingIdentifier(n.Name)
		}
		if strings.HasPrefix(n.Name, "_") && g.receiver != "" {
			return g.receiver + "." + goMethodName(n.Name)
		}
		if n.Reference != nil && n.Reference.Intrinsic == "" && n.Reference.Package != "" {
			if alias := g.referenceAlias(n.Reference); alias != "" {
				return alias + "." + g.goImportedName(n.Name, n.Reference)
			}
			if n.Reference.ExportKind == "function" {
				return g.projectFunctionName(n.Reference.Package, n.Reference.Symbol)
			}
		}
		if n.Owner != "" {
			return goConstantIdentifier(n.Owner, n.Name)
		}
		if isUpper(n.Name) {
			return goConstantIdentifier("", n.Name)
		}
		return goIdentifier(n.Name, isUpper(n.Name))
	case *ir.Literal:
		if n.Kind == "nil" {
			return "nil"
		}
		return n.Raw
	case *ir.InterpolatedString:
		g.requireImport("fmt", "")
		var format strings.Builder
		var arguments []string
		for _, part := range n.Parts {
			if part.Expression != nil {
				format.WriteString("%v")
				arguments = append(arguments, g.expr(part.Expression))
				continue
			}
			text := part.Text
			if decoded, err := strconv.Unquote("\"" + text + "\""); err == nil {
				text = decoded
			}
			format.WriteString(strings.ReplaceAll(text, "%", "%%"))
		}
		args := ""
		if len(arguments) > 0 {
			args = ", " + strings.Join(arguments, ", ")
		}
		return "fmt.Sprintf(" + strconv.Quote(format.String()) + args + ")"
	case *ir.Symbol:
		return strconv.Quote(n.Name)
	case *ir.Array:
		parts := make([]string, len(n.Elements))
		for i, element := range n.Elements {
			parts[i] = g.expr(element)
		}
		return g.goType(n.ExprType()) + "{" + strings.Join(parts, ", ") + "}"
	case *ir.Hash:
		parts := make([]string, len(n.Entries))
		for i, entry := range n.Entries {
			parts[i] = g.expr(entry.Key) + ": " + g.expr(entry.Value)
		}
		return g.goType(n.ExprType()) + "{" + strings.Join(parts, ", ") + "}"
	case *ir.Unary:
		op := n.Operator
		if op == "not" || op == "!" {
			return "!(" + g.expr(n.Operand) + ")"
		}
		if op == "-" && n.ExprType().Kind == types.Int {
			if literal, ok := n.Operand.(*ir.Literal); ok && literal.Kind == "integer" {
				return "-" + literal.Raw
			}
			g.checkedInteger = true
			return g.checkedIntegerRuntimeName("Negate") + "(" + g.expr(n.Operand) + ")"
		}
		return op + g.unaryOperand(n.Operand)
	case *ir.Conversion:
		switch n.Kind {
		case ir.RangeToIterableConversion:
			return g.iterableExpr(n.Value)
		case ir.IntegerToFloatConversion:
			return "float64(" + g.expr(n.Value) + ")"
		case ir.UnionIntegerToFloatConversion:
			return "func(value any) any { if integer, ok := value.(int); ok { return float64(integer) }; return value }(" + g.expr(n.Value) + ")"
		case ir.NonNullableToNullableConversion:
			return g.nonNullableToNullableExpr(n, n.ExprType())
		case ir.NullableToNonNullableConversion:
			value := g.expr(n.Value)
			if g.goType(n.Value.ExprType()) == g.goType(n.ExprType()) {
				return value
			}
			return "*(" + value + ")"
		default:
			return g.expr(n.Value)
		}
	case *ir.Binary:
		op := n.Operator
		if op == "and" {
			op = "&&"
		} else if op == "or" {
			op = "||"
		}
		left := g.binaryOperand(n.Left)
		right := g.binaryOperand(n.Right)
		if op == "**" && n.ExprType().Kind != types.Int {
			g.requireImport("math", "math")
			return "math.Pow(" + left + ", " + right + ")"
		}
		if n.ExprType().Kind == types.Int && isCheckedIntegerOperator(op) {
			return g.checkedIntegerBinary(op, left, right)
		}
		if n.ExprType().Kind == types.Float && (op == "+" || op == "-" || op == "*" || op == "/") {
			return "func(left float64, right float64) float64 { return left " + op + " right }(" + left + ", " + right + ")"
		}
		return left + " " + op + " " + right
	case *ir.Range:
		exclusive := "0"
		if n.Exclusive {
			exclusive = "1"
		}
		return "[3]int{" + g.expr(n.Start) + ", " + g.expr(n.End) + ", " + exclusive + "}"
	case *ir.Transform:
		return g.transform(n)
	case *ir.Member:
		if len(n.UnionAlternatives) > 0 {
			return g.unionMemberExpression(n)
		}
		if n.Reference != nil && n.Reference.Intrinsic == "trb.orm.column" {
			return g.memberReceiver(n.Receiver) + "." + goORMColumnGetter(n.Name) + "()"
		}
		if n.Namespace && isUpper(n.Name) {
			owner := n.Receiver.ExprType().Name
			if owner == "" {
				owner = irExpressionName(n.Receiver)
			}
			name := goConstantIdentifier(owner, n.Name)
			if alias := g.referenceAlias(n.Reference); alias != "" {
				return alias + "." + name
			}
			return name
		}
		if n.ClassField {
			return g.memberReceiver(n.Receiver) + "." + goFieldName(n.Name)
		}
		return g.memberReceiver(n.Receiver) + "." + goMethodName(n.Name)
	case *ir.Call:
		parts := make([]string, len(n.Arguments))
		for i, argument := range n.Arguments {
			parts[i] = g.expr(argument.Value)
		}
		args := strings.Join(parts, ", ")
		if application, ok := n.Callee.(*ir.TypeApply); ok && application.Kind == "method" && g.typeKinds[application.Owner] == "enum" && referenceIntrinsic(n.Callee) == "" {
			if member, method := application.Receiver.(*ir.Member); method {
				name := goIdentifier(application.Owner, true) + goMethodName(member.Name)
				if alias := g.referenceAlias(member.Reference); alias != "" {
					name = alias + "." + name
				}
				typeArguments := append(append([]types.Type(nil), application.OwnerArguments...), application.Arguments...)
				if len(typeArguments) > 0 {
					items := make([]string, len(typeArguments))
					for index, argument := range typeArguments {
						items[index] = g.goType(argument)
					}
					name += "[" + strings.Join(items, ", ") + "]"
				}
				parts = g.sourceCallArguments(n.Arguments, n.CallSignature, parts)
				values := append([]string{g.expr(member.Receiver)}, g.executionArguments(n, parts)...)
				return name + "(" + strings.Join(values, ", ") + ")"
			}
		}
		if reference := expressionReference(n.Callee); reference != nil && reference.Runtime != nil {
			parts = g.executionArguments(n, parts)
			return g.nativeRuntimeCall(reference.Runtime, parts)
		}
		if reference := expressionReference(n.Callee); reference != nil && reference.Intrinsic != "" {
			if reference.ReceiverMethod {
				if member, ok := receiverMember(n.Callee); ok {
					parts = append([]string{g.expr(member.Receiver)}, parts...)
				}
			}
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		parts = g.sourceCallArguments(n.Arguments, n.CallSignature, parts)
		parts = g.executionArguments(n, parts)
		args = strings.Join(parts, ", ")
		if member, ok := n.Callee.(*ir.Member); ok && member.Name == "new" {
			if n.RecordTarget != nil {
				if identifier, typeArguments, record := goRecordTarget(n.RecordTarget); record {
					if recordContractHasDefaults(n.RecordFields) {
						return g.recordDefaultCall(n, identifier, typeArguments, n.Arguments, n.RecordFields)
					}
					if len(typeArguments) > 0 {
						return g.recordLiteralApplied(identifier, typeArguments, n.Arguments)
					}
					return g.recordLiteral(identifier, n.Arguments)
				}
			}
			if application, generic := member.Receiver.(*ir.TypeApply); generic && (application.Kind == "class" || application.Kind == "record") {
				identifier, named := application.Receiver.(*ir.Identifier)
				if named {
					if application.Kind == "record" {
						if recordContractHasDefaults(n.RecordFields) {
							return g.recordDefaultCall(n, identifier, application.Arguments, n.Arguments, n.RecordFields)
						}
						return g.recordLiteralApplied(identifier, application.Arguments, n.Arguments)
					}
					name := "New" + goIdentifier(identifier.Name, true)
					if alias := g.referenceAlias(identifier.Reference); alias != "" {
						name = alias + "." + name
					}
					arguments := make([]string, len(application.Arguments))
					for index, argument := range application.Arguments {
						arguments[index] = g.goType(argument)
					}
					return name + "[" + strings.Join(arguments, ", ") + "](" + args + ")"
				}
			}
			if identifier, ok := member.Receiver.(*ir.Identifier); ok {
				if g.records[identifier.Name] || identifier.Reference != nil && identifier.Reference.ExportKind == "record" {
					if recordContractHasDefaults(n.RecordFields) {
						return g.recordDefaultCall(n, identifier, nil, n.Arguments, n.RecordFields)
					}
					return g.recordLiteral(identifier, n.Arguments)
				}
				if alias := g.referenceAlias(identifier.Reference); alias != "" {
					return alias + ".New" + goIdentifier(identifier.Name, true) + "(" + args + ")"
				}
				return "New" + goIdentifier(identifier.Name, true) + "(" + args + ")"
			}
			return "New" + goIdentifier(g.expr(member.Receiver), true) + "(" + args + ")"
		}
		if member, ok := n.Callee.(*ir.Member); ok {
			if receiver, ok := member.Receiver.(*ir.Identifier); ok && receiver.Reference != nil && receiver.Reference.ExportKind == "class" {
				name := goIdentifier(receiver.Name, true) + goMethodName(member.Name)
				if alias := g.referenceAlias(receiver.Reference); alias != "" {
					name = alias + "." + name
				}
				return name + "(" + args + ")"
			}
		}
		if member, ok := n.Callee.(*ir.Member); ok {
			if receiver, ok := member.Receiver.(*ir.Identifier); ok && g.staticMethods[receiver.Name][member.Name] {
				return goIdentifier(receiver.Name, true) + goMethodName(member.Name) + "(" + args + ")"
			}
		}
		if identifier, ok := n.Callee.(*ir.Identifier); ok {
			if !identifier.Lexical && g.receiver != "" && g.methods[identifier.Name] {
				return g.receiver + "." + goMethodName(identifier.Name) + "(" + args + ")"
			}
			if g.topMethods[identifier.Name] {
				name := g.projectFunctionName(g.modulePath, identifier.Name)
				return name + "(" + args + ")"
			}
		}
		return g.expr(n.Callee) + "(" + args + ")"
	case *ir.EnumCall:
		parts := make([]string, 0, len(n.Arguments)+1)
		for _, argument := range n.Arguments {
			parts = append(parts, g.expr(argument.Value))
		}
		switch n.Method {
		case "raw_value":
			return g.goType(n.RawType) + "(" + g.expr(n.Receiver) + ")"
		case "from_raw":
			return g.rawEnumFromValue(n, parts[0])
		default:
			parts = g.sourceCallArguments(n.Arguments, n.CallSignature, parts)
			parts = append([]string{g.expr(n.Receiver)}, parts...)
			if g.execution != nil && g.execution.EnumCalls[n] {
				parts = append(parts[:1], append([]string{"__trbScope"}, parts[1:]...)...)
			}
			name := enumMethodName(n.EnumName, n.Method)
			if alias := g.referenceAlias(n.Reference); alias != "" {
				name = alias + "." + name
			}
			return name + "(" + strings.Join(parts, ", ") + ")"
		}
	case *ir.EnumConstruct:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument)
		}
		name := "New" + goIdentifier(n.EnumName, true) + goIdentifier(n.Member, true)
		if alias := g.referenceAlias(n.Reference); alias != "" {
			name = alias + "." + name
		}
		if len(n.TypeArguments) > 0 {
			types := make([]string, len(n.TypeArguments))
			for index, argument := range n.TypeArguments {
				types[index] = g.goType(argument)
			}
			name += "[" + strings.Join(types, ", ") + "]"
		}
		return name + "(" + strings.Join(parts, ", ") + ")"
	case *ir.TypeApply:
		arguments := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			arguments[index] = g.goType(argument)
		}
		name := g.expr(n.Receiver)
		if identifier, ok := n.Receiver.(*ir.Identifier); ok && g.topMethods[identifier.Name] {
			name = g.projectFunctionName(g.modulePath, identifier.Name)
		}
		return name + "[" + strings.Join(arguments, ", ") + "]"
	case *ir.Index:
		if n.Receiver.ExprType().Kind == types.Hash && len(n.Receiver.ExprType().Args) == 2 {
			hashType := n.Receiver.ExprType()
			keyType := g.goType(hashType.Args[0])
			valueType := g.goType(hashType.Args[1])
			return "func(values " + g.goType(hashType) + ", key " + keyType + ") " + valueType + " { value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; return value }(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		if n.Receiver.ExprType().Kind == types.String {
			return "func(value string, index int) string { characters := []rune(value); if index < 0 { index += len(characters) }; if index < 0 || index >= len(characters) { panic(\"String index is out of bounds\") }; return string(characters[index]) }(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		if n.Receiver.ExprType().Kind == types.Array {
			g.arrayIndexRuntime = true
			return g.arrayIndexName() + "(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	default:
		return ""
	}
}

func (g *generator) sourceCallArguments(arguments []ir.CallArgument, signature []callsignature.Parameter, authored []string) []string {
	if !callsignature.HasNamedOnly(signature) {
		return authored
	}
	positional := []string{}
	named := []string{}
	for index, argument := range arguments {
		if argument.Name == "" {
			positional = append(positional, authored[index])
		} else {
			named = append(named, strconv.Quote(argument.Name)+": "+authored[index])
		}
	}
	requiredPositional := 0
	hasOptionalPositional := false
	for _, parameter := range signature {
		if parameter.Kind != callsignature.Positional {
			continue
		}
		if parameter.Presence == callsignature.Required {
			requiredPositional++
		} else {
			hasOptionalPositional = true
		}
	}
	positionalEnd := requiredPositional
	if positionalEnd > len(positional) {
		positionalEnd = len(positional)
	}
	result := append([]string(nil), positional[:positionalEnd]...)
	if hasOptionalPositional {
		optional := []string{}
		if len(positional) > requiredPositional {
			optional = positional[requiredPositional:]
		}
		result = append(result, "[]any{"+strings.Join(optional, ", ")+"}")
	}
	namedExpression := "map[string]any{}"
	if len(named) > 0 {
		assignments := make([]string, len(named))
		for index, entry := range named {
			parts := strings.SplitN(entry, ": ", 2)
			assignments[index] = "__trbValues[" + parts[0] + "] = " + parts[1]
		}
		namedExpression = "func() map[string]any { __trbValues := map[string]any{}; " + strings.Join(assignments, "; ") + "; return __trbValues }()"
	}
	result = append(result, namedExpression)
	return result
}

func (g *generator) checkedIntegerBinary(operator, left, right string) string {
	g.checkedInteger = true
	name := map[string]string{
		"+": "Add", "-": "Subtract", "*": "Multiply", "/": "Divide", "%": "Remainder", "**": "Power",
	}[operator]
	return g.checkedIntegerRuntimeName(name) + "(" + left + ", " + right + ")"
}

func (g *generator) checkedIntegerRuntimeSupport() {
	check := g.checkedIntegerRuntimeName("Check")
	add := g.checkedIntegerRuntimeName("Add")
	subtract := g.checkedIntegerRuntimeName("Subtract")
	multiply := g.checkedIntegerRuntimeName("Multiply")
	divide := g.checkedIntegerRuntimeName("Divide")
	remainder := g.checkedIntegerRuntimeName("Remainder")
	power := g.checkedIntegerRuntimeName("Power")
	negate := g.checkedIntegerRuntimeName("Negate")
	g.line("func " + check + `(value int) int { if value < -9007199254740991 || value > 9007199254740991 { panic("Integer is outside the portable range") }; return value }`)
	g.line("func " + add + "(left int, right int) int { return " + check + "(left + right) }")
	g.line("func " + subtract + "(left int, right int) int { return " + check + "(left - right) }")
	g.line("func " + multiply + `(left int, right int) int { if left > 0 { if right > 0 && left > 9007199254740991/right { panic("Integer is outside the portable range") }; if right < 0 && right < -9007199254740991/left { panic("Integer is outside the portable range") } } else if left < 0 { if right > 0 && left < -9007199254740991/right { panic("Integer is outside the portable range") }; if right < 0 && left < 9007199254740991/right { panic("Integer is outside the portable range") } }; return ` + check + `(left * right) }`)
	g.line("func " + divide + `(left int, right int) int { if right == 0 { panic("division by zero") }; return ` + check + `(left / right) }`)
	g.line("func " + remainder + `(left int, right int) int { if right == 0 { panic("division by zero") }; return ` + check + `(left % right) }`)
	g.line("func " + power + `(base int, exponent int) int { if exponent < 0 { panic("negative Integer exponent") }; result, factor := 1, base; for exponent > 0 { if exponent%2 == 1 { result = ` + multiply + `(result, factor) }; exponent /= 2; if exponent > 0 { factor = ` + multiply + `(factor, factor) } }; return result }`)
	g.line("func " + negate + "(value int) int { return " + check + "(-value) }")
}

func (g *generator) checkedIntegerRuntimeName(operation string) string {
	return "trbInteger" + operation + "_" + naming.PrivateSuffix("integer:"+g.modulePath)
}

func isCheckedIntegerAssignment(operator string) bool {
	switch operator {
	case "+=", "-=", "*=", "/=":
		return true
	}
	return false
}

func isCheckedIntegerOperator(operator string) bool {
	switch operator {
	case "+", "-", "*", "/", "%", "**":
		return true
	}
	return false
}

func (g *generator) rawEnumFromValue(call *ir.EnumCall, argument string) string {
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	valueType := g.goType(types.FromName(call.EnumName))
	errorType := g.goType(types.FromName("EnumValueError"))
	resultType := g.goType(call.ExprType())
	prefix := ""
	if alias := g.referenceAlias(call.Reference); alias != "" {
		prefix = alias + "."
	}
	lines := []string{"func() " + resultType + " { value := " + argument + "; switch value {"}
	for _, item := range call.RawValues {
		constant := prefix + goConstantIdentifier(call.EnumName, item.Member)
		lines = append(lines, "case "+item.Raw+": return "+resultAlias+".NewResultOk["+valueType+", "+errorType+"]("+constant+");")
	}
	message := strconv.Quote("unknown raw value for " + call.EnumName)
	lines = append(lines, "}; return "+resultAlias+".NewResultErr["+valueType+", "+errorType+"]("+errorType+"{Value: value, Message: "+message+"}) }()")
	return strings.Join(lines, " ")
}

func (g *generator) ifExpression(node *ir.If) string {
	child := &generator{
		functionDepth:   g.functionDepth,
		receiver:        g.receiver,
		returnType:      node.ExprType(),
		inConstructor:   g.inConstructor,
		methods:         g.methods,
		topMethods:      g.topMethods,
		staticMethods:   g.staticMethods,
		records:         g.records,
		classes:         g.classes,
		typeAliases:     g.typeAliases,
		typeKinds:       g.typeKinds,
		imports:         g.imports,
		bindingNames:    g.bindingNames,
		bindingSources:  g.bindingSources,
		modulePath:      g.modulePath,
		goModule:        g.goModule,
		temporary:       g.temporary,
		breakTarget:     g.breakTarget,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		projectNames:    g.projectNames,
		execution:       g.execution,
		executionActive: g.executionActive,
	}
	child.line("func() " + child.goType(node.ExprType()) + " {")
	child.indent++
	child.line("if " + child.expr(node.Condition) + " {" + goTrailingComment(node.TrailingComment))
	child.indent++
	child.statements(node.Then)
	if !node.ThenDiverges {
		child.line("return " + child.expr(node.ThenResult))
	}
	child.indent--
	for _, branch := range node.ElseIf {
		child.line("} else if " + child.expr(branch.Condition) + " {")
		child.indent++
		child.statements(branch.Body)
		if !branch.Diverges {
			child.line("return " + child.expr(branch.Result))
		}
		child.indent--
	}
	child.line("} else {")
	child.indent++
	child.statements(node.Else)
	if !node.ElseDiverges {
		child.line("return " + child.expr(node.ElseResult))
	}
	child.indent--
	child.line("}")
	child.indent--
	child.line("}()")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) caseExpression(node *ir.Case) string {
	child := &generator{
		functionDepth:   g.functionDepth,
		receiver:        g.receiver,
		returnType:      node.ExprType(),
		inConstructor:   g.inConstructor,
		methods:         g.methods,
		topMethods:      g.topMethods,
		staticMethods:   g.staticMethods,
		records:         g.records,
		classes:         g.classes,
		typeAliases:     g.typeAliases,
		typeKinds:       g.typeKinds,
		imports:         g.imports,
		bindingNames:    g.bindingNames,
		bindingSources:  g.bindingSources,
		modulePath:      g.modulePath,
		goModule:        g.goModule,
		temporary:       g.temporary,
		breakTarget:     g.breakTarget,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		projectNames:    g.projectNames,
		execution:       g.execution,
		executionActive: g.executionActive,
	}
	child.line("func() " + child.goType(node.ExprType()) + " {")
	child.indent++
	child.statements(node.Leading)
	child.temporary++
	value := "__trbCase" + strconv.Itoa(child.temporary)
	child.line(value + " := " + child.expr(node.Value) + goTrailingComment(node.TrailingComment))
	if node.TypeUnion {
		typed := value + "Value"
		child.line("switch " + typed + " := " + value + ".(type) {")
		child.indent++
		for _, branch := range node.Branches {
			child.line("case " + child.goType(branch.MatchType) + ":" + goTrailingComment(branch.TrailingComment))
			child.indent++
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				name := child.caseBindingIdentifier(binding)
				child.line(name + " := " + typed)
				if namedUnusedBinding(binding.Name) {
					child.line("_ = " + name)
				}
			}
			child.statements(branch.Body)
			if !branch.Diverges {
				child.line("return " + child.expr(branch.Result))
			}
			child.indent--
		}
		child.line("default:")
		child.indent++
		if node.HasElse {
			child.line("_ = " + typed)
			child.statements(node.Else)
			if !node.ElseDiverges {
				child.line("return " + child.expr(node.ElseResult))
			}
		} else {
			child.line("panic(\"unreachable exhaustive case\")")
		}
		child.indent--
		child.indent--
		child.line("}")
	} else {
		for index, branch := range node.Branches {
			header := "if "
			if index > 0 {
				header = "} else if "
			}
			condition := value + " == " + child.expr(branch.Value)
			for _, alternative := range branch.Alternatives {
				condition += " || " + value + " == " + child.expr(alternative)
			}
			if branch.PayloadEnum {
				condition = value + ".Kind == " + child.enumTag(branch)
			}
			child.line(header + condition + " {" + goTrailingComment(branch.TrailingComment))
			child.indent++
			child.caseNarrowings(branch.Narrowings)
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				field := goIdentifier(branch.Member, true) + goIdentifier(binding.Field, true)
				name := child.caseBindingIdentifier(binding)
				child.line(name + " := " + value + "." + field)
				if namedUnusedBinding(binding.Name) {
					child.line("_ = " + name)
				}
			}
			child.statements(branch.Body)
			if !branch.Diverges {
				child.line("return " + child.expr(branch.Result))
			}
			child.indent--
		}
		child.line("} else {")
		child.indent++
		if node.HasElse {
			child.caseNarrowings(node.ElseNarrowings)
			child.statements(node.Else)
			if !node.ElseDiverges {
				child.line("return " + child.expr(node.ElseResult))
			}
		} else {
			child.line("panic(\"unreachable exhaustive case\")")
		}
		child.indent--
		child.line("}")
	}
	child.indent--
	child.line("}()")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) transform(transform *ir.Transform) string {
	if transform.Operation == "concurrent_map" {
		return g.concurrentMap(transform)
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items := "__trbItems" + suffix
	result := "__trbResult" + suffix
	item := g.bindingIdentifier(transform.Item)
	if item == "" || item == "_" {
		item = "__trbItem" + suffix
	}
	itemUse := ""
	if strings.HasPrefix(transform.Item, "_") {
		itemUse = "_ = " + item + "; "
	}
	source := g.iterableExpr(transform.Source)
	value := g.transformResult(transform)
	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		g.requireImport("slices", "")
		keyType := transform.Result.ExprType()
		decorated := "__trbDecorated" + suffix
		index := "__trbIndex" + suffix
		comparison := g.portableSortComparison("left.key", "right.key", keyType, transform.Operation == "sort_by_descending")
		return "func() " + g.goType(transform.ExprType()) + " { " + items + " := " + source + "; type " + decorated + " struct { value " + g.goType(transform.ItemType) + "; key " + g.goType(keyType) + " }; ordered := make([]" + decorated + ", 0, len(" + items + ")); for " + index + ", " + item + " := range " + items + " { _ = " + index + "; " + itemUse + "ordered = append(ordered, " + decorated + "{value: " + item + ", key: " + value + "}) }; slices.SortStableFunc(ordered, func(left, right " + decorated + ") int { return " + comparison + " }); " + result + " := make(" + g.goType(transform.ExprType()) + ", 0, len(ordered)); for _, entry := range ordered { " + result + " = append(" + result + ", entry.value) }; return " + result + " }()"
	case "map":
		index := "_"
		indexUse := ""
		if transform.WithIndex {
			index = g.bindingIdentifier(transform.Index)
			if index == "" {
				index = "_"
			}
			if namedUnusedBinding(transform.Index) {
				indexUse = "_ = " + index + "; "
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + items + " := " + source + "; " + result + " := make(" + g.goType(transform.ExprType()) + ", 0, len(" + items + ")); for " + index + ", " + item + " := range " + items + " { " + itemUse + indexUse + result + " = append(" + result + ", " + value + ") }; return " + result + " }()"
	case "select":
		index := "_"
		indexUse := ""
		if transform.WithIndex {
			index = g.bindingIdentifier(transform.Index)
			if index == "" {
				index = "_"
			}
			if namedUnusedBinding(transform.Index) {
				indexUse = "_ = " + index + "; "
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + items + " := " + source + "; " + result + " := make(" + g.goType(transform.ExprType()) + ", 0, len(" + items + ")); for " + index + ", " + item + " := range " + items + " { " + itemUse + indexUse + "if " + value + " { " + result + " = append(" + result + ", " + item + ") } }; return " + result + " }()"
	case "any?", "all?", "none?":
		initial := "false"
		match := value
		if transform.Operation == "all?" || transform.Operation == "none?" {
			initial = "true"
		}
		if transform.Operation == "all?" {
			match = "!(" + value + ")"
		}
		matched := "true"
		if transform.Operation == "all?" || transform.Operation == "none?" {
			matched = "false"
		}
		return "func() bool { for _, " + item + " := range " + source + " { " + itemUse + "if " + match + " { return " + matched + " } }; return " + initial + " }()"
	case "find":
		found := "&" + item
		if len(transform.Source.ExprType().Args) > 0 {
			elementType := transform.Source.ExprType().Args[0]
			if g.goType(elementType) == g.goType(transform.ExprType()) {
				found = item
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { for _, " + item + " := range " + source + " { " + itemUse + "if " + value + " { return " + found + " } }; return nil }()"
	case "find_index":
		index := "__trbIndex" + suffix
		return "func() " + g.goType(transform.ExprType()) + " { for " + index + ", " + item + " := range " + source + " { " + itemUse + "if " + value + " { " + result + " := " + index + "; return &" + result + " } }; return nil }()"
	case "reduce":
		accumulator := g.bindingIdentifier(transform.Accumulator)
		binding := ""
		if accumulator != "" && accumulator != "_" {
			binding = accumulator + " := " + result + "; "
			if namedUnusedBinding(transform.Accumulator) {
				binding += "_ = " + accumulator + "; "
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + result + " := " + g.expr(transform.Initial) + "; for _, " + item + " := range " + source + " { " + itemUse + binding + result + " = " + value + " }; return " + result + " }()"
	default:
		return "nil"
	}
}

func (g *generator) concurrentMap(transform *ir.Transform) string {
	g.requireImport("context", "trbcontext")
	g.requireImport("sync", "")
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items := "__trbItems" + suffix
	result := "__trbResult" + suffix
	requested := "__trbRequested" + suffix
	group := "__trbGroup" + suffix
	concurrentScope := "__trbConcurrentScope" + suffix
	semaphore := "__trbSemaphore" + suffix
	localLimit := "__trbLocalLimit" + suffix
	workerCount := "__trbWorkerCount" + suffix
	childScope := "__trbChildScope" + suffix
	cancel := "__trbCancel" + suffix
	jobs := "__trbJobs" + suffix
	waitGroup := "__trbWaitGroup" + suffix
	panicOnce := "__trbPanicOnce" + suffix
	panicValue := "__trbPanic" + suffix
	index := "__trbIndex" + suffix
	worker := "__trbWorker" + suffix
	item := g.bindingIdentifier(transform.Item)
	if item == "" || item == "_" {
		item = "__trbItem" + suffix
	}
	itemUse := ""
	if strings.HasPrefix(transform.Item, "_") {
		itemUse = "_ = " + item + "; "
	}
	limit := "8"
	explicit := transform.Limit != nil
	if explicit {
		limit = g.expr(transform.Limit)
	}
	value := g.transformResult(transform)
	typeName := g.goType(transform.ExprType())
	source := g.iterableExpr(transform.Source)
	held := "__trbHeld" + suffix
	return "func() " + typeName + " { " +
		items + " := " + source + "; " + requested + " := " + limit + "; if " + requested + " <= 0 { panic(\"concurrent_map limit must be greater than zero\") }; " +
		concurrentScope + " := __trbScope; " + group + ", __trbHasGroup := __trbScope.Value(\"type-rb/concurrency-group\").(map[string]any); if !__trbHasGroup { " + semaphore + " := make(chan struct{}, " + requested + "); " + group + " = map[string]any{\"semaphore\": " + semaphore + "}; " + concurrentScope + " = trbcontext.WithValue(__trbScope, \"type-rb/concurrency-group\", " + group + ") }; " +
		semaphore + " := " + group + "[\"semaphore\"].(chan struct{}); " + localLimit + " := cap(" + semaphore + "); " +
		"func() { if " + strconv.FormatBool(explicit) + " && " + requested + " < " + localLimit + " { " + localLimit + " = " + requested + " } }(); " +
		held + ", _ := __trbScope.Value(\"type-rb/concurrency-held\").(bool); if " + held + " { <-" + semaphore + "; defer func() { " + semaphore + " <- struct{}{} }() }; " +
		childScope + ", " + cancel + " := trbcontext.WithCancel(" + concurrentScope + "); defer " + cancel + "(); " + result + " := make(" + typeName + ", len(" + items + ")); " +
		workerCount + " := " + localLimit + "; if len(" + items + ") < " + workerCount + " { " + workerCount + " = len(" + items + ") }; " + jobs + " := make(chan int); var " + waitGroup + " sync.WaitGroup; var " + panicOnce + " sync.Once; var " + panicValue + " any; " +
		"for " + worker + " := 0; " + worker + " < " + workerCount + "; " + worker + "++ { " + waitGroup + ".Add(1); go func() { defer " + waitGroup + ".Done(); for " + index + " := range " + jobs + " { select { case " + semaphore + " <- struct{}{}: case <-" + childScope + ".Done(): return }; func() { defer func() { <-" + semaphore + " }(); defer func() { if recovered := recover(); recovered != nil { " + panicOnce + ".Do(func() { " + panicValue + " = recovered; " + cancel + "() }) } }(); __trbScope := trbcontext.WithValue(" + childScope + ", \"type-rb/concurrency-held\", true); _ = __trbScope; " + item + " := " + items + "[" + index + "]; " + itemUse + result + "[" + index + "] = " + value + " }() } }() }; " +
		"func() { defer close(" + jobs + "); for " + index + " := range " + items + " { select { case " + jobs + " <- " + index + ": case <-" + childScope + ".Done(): return } } }(); " + waitGroup + ".Wait(); if " + panicValue + " != nil { panic(" + panicValue + ") }; if err := __trbScope.Err(); err != nil { panic(err) }; return " + result + " }()"
}

func (g *generator) transformResult(transform *ir.Transform) string {
	if len(transform.Body) == 0 {
		return g.expr(transform.Result)
	}
	child := *g
	child.b = strings.Builder{}
	child.recordSources = false
	child.indent = 0
	child.line("func() " + child.goType(transform.Result.ExprType()) + " {")
	child.indent++
	child.statements(transform.Body)
	child.line("return " + child.expr(transform.Result))
	child.indent--
	child.line("}()")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) portableSortComparison(left, right string, typ types.Type, descending bool) string {
	typ = sortScalarType(typ)
	switch typ.Kind {
	case types.Float:
		g.requireImport("math", "")
		if descending {
			return "func() int { if math.IsNaN(" + left + ") { if math.IsNaN(" + right + ") { return 0 }; return 1 }; if math.IsNaN(" + right + ") { return -1 }; if " + left + " > " + right + " { return -1 }; if " + left + " < " + right + " { return 1 }; return 0 }()"
		}
		return "func() int { if math.IsNaN(" + left + ") { if math.IsNaN(" + right + ") { return 0 }; return 1 }; if math.IsNaN(" + right + ") { return -1 }; if " + left + " < " + right + " { return -1 }; if " + left + " > " + right + " { return 1 }; return 0 }()"
	case types.String:
		if descending {
			return "func() int { if " + left + " > " + right + " { return -1 }; if " + left + " < " + right + " { return 1 }; return 0 }()"
		}
		return "func() int { if " + left + " < " + right + " { return -1 }; if " + left + " > " + right + " { return 1 }; return 0 }()"
	default:
		if descending {
			return "func() int { if " + left + " > " + right + " { return -1 }; if " + left + " < " + right + " { return 1 }; return 0 }()"
		}
		return "func() int { if " + left + " < " + right + " { return -1 }; if " + left + " > " + right + " { return 1 }; return 0 }()"
	}
}

func sortScalarType(typ types.Type) types.Type {
	if base, literal := types.LiteralBase(typ); literal {
		return base
	}
	return typ
}

func namedUnusedBinding(name string) bool {
	return name != "_" && strings.HasPrefix(name, "_")
}

func (g *generator) caseBindingIdentifier(binding ir.CaseBinding) string {
	if binding.Generated {
		return binding.Name
	}
	return g.bindingIdentifier(binding.Name)
}

func (g *generator) bindingIdentifier(name string) string {
	if name != "" && name != "_" {
		g.bindingSources[name] = true
	}
	if target := g.lexicalNames[name]; target != "" {
		return target
	}
	if target := g.bindingNames[name]; target != "" {
		return target
	}
	return goBindingIdentifier(name)
}

func goBindingIdentifier(name string) string {
	if name == "_" {
		return "_"
	}
	if namedUnusedBinding(name) {
		return "__trb_unused_" + hex.EncodeToString([]byte(name))
	}
	return goIdentifier(name, false)
}

func (g *generator) assignmentTarget(expression ir.Expression) string {
	if index, ok := expression.(*ir.Index); ok {
		if index.Receiver.ExprType().Kind == types.Array {
			g.arrayIndexRuntime = true
			receiver := g.expr(index.Receiver)
			position := g.arrayIndexPositionName() + "(" + g.expr(index.Index) + ", len(" + receiver + "))"
			return receiver + "[" + position + "]"
		}
		return g.expr(index.Receiver) + "[" + g.expr(index.Index) + "]"
	}
	return g.expr(expression)
}

func (g *generator) arrayIndexRuntimeSupport() {
	g.line("func " + g.arrayIndexPositionName() + `(index int, size int) int { if index < 0 { index += size }; if index < 0 || index >= size { panic("Array index is out of bounds") }; return index }`)
	g.line("func " + g.arrayIndexName() + "[T any](values []T, index int) T { return values[" + g.arrayIndexPositionName() + "(index, len(values))] }")
}

func (g *generator) arrayIndexPositionName() string {
	return "trbArrayIndexPosition_" + naming.PrivateSuffix("array-index:"+g.modulePath)
}

func (g *generator) arrayIndexName() string {
	return "trbArrayIndex_" + naming.PrivateSuffix("array-index:"+g.modulePath)
}

func (g *generator) binaryOperand(expression ir.Expression) string {
	value := g.expr(expression)
	switch expression.(type) {
	case *ir.Binary, *ir.Range, *ir.Unary:
		return "(" + value + ")"
	default:
		return value
	}
}

func (g *generator) unaryOperand(expression ir.Expression) string {
	value := g.expr(expression)
	switch expression.(type) {
	case *ir.Binary, *ir.Range:
		return "(" + value + ")"
	default:
		return value
	}
}

func (g *generator) memberReceiver(expression ir.Expression) string {
	value := g.expr(expression)
	switch expression.(type) {
	case *ir.Conversion:
		return "(" + value + ")"
	default:
		return value
	}
}

func (g *generator) recordLiteral(record *ir.Identifier, arguments []ir.CallArgument) string {
	name := goIdentifier(record.Name, true)
	if alias := g.referenceAlias(record.Reference); alias != "" {
		name = alias + "." + name
	}
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, goIdentifier(argument.Name, true)+": "+g.expr(argument.Value))
	}
	return name + "{" + strings.Join(fields, ", ") + "}"
}

func goRecordTarget(expression ir.Expression) (*ir.Identifier, []types.Type, bool) {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node, nil, true
	case *ir.Member:
		if !node.Namespace {
			return nil, nil, false
		}
		return &ir.Identifier{ExprBase: node.ExprBase, Name: node.Name, Reference: node.Reference}, nil, true
	case *ir.TypeApply:
		identifier, _, ok := goRecordTarget(node.Receiver)
		if !ok {
			return nil, nil, false
		}
		return identifier, node.Arguments, true
	default:
		return nil, nil, false
	}
}

func (g *generator) recordLiteralApplied(record *ir.Identifier, typeArguments []types.Type, arguments []ir.CallArgument) string {
	name := goIdentifier(record.Name, true)
	if alias := g.referenceAlias(record.Reference); alias != "" {
		name = alias + "." + name
	}
	items := make([]string, len(typeArguments))
	for index, argument := range typeArguments {
		items[index] = g.goType(argument)
	}
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, goIdentifier(argument.Name, true)+": "+g.expr(argument.Value))
	}
	return name + "[" + strings.Join(items, ", ") + "]{" + strings.Join(fields, ", ") + "}"
}

func (g *generator) recordDefaultCall(call *ir.Call, record *ir.Identifier, typeArguments []types.Type, arguments []ir.CallArgument, fields []ir.RecordFieldContract) string {
	typeName := goIdentifier(record.Name, true)
	helper := goRecordConstructorName(record.Name)
	if alias := g.referenceAlias(record.Reference); alias != "" {
		typeName = alias + "." + typeName
		helper = alias + "." + helper
	}
	if len(typeArguments) > 0 {
		items := make([]string, len(typeArguments))
		for index, argument := range typeArguments {
			items[index] = g.goType(argument)
		}
		applied := "[" + strings.Join(items, ", ") + "]"
		typeName += applied
		helper += applied
	}
	explicit := map[string]string{}
	statements := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		g.temporary++
		name := "__trbRecordArg" + strconv.Itoa(g.temporary)
		statements = append(statements, name+" := "+g.expr(argument.Value))
		explicit[argument.Name] = name
	}
	values := make([]string, 0, len(fields)*2)
	if g.execution != nil && (g.execution.RecordCallDefaults[call] || g.execution.RecordCallSync[call]) {
		values = append(values, g.executionScopeArgument())
	}
	for _, field := range fields {
		value, provided := explicit[field.Name]
		if !provided {
			g.temporary++
			value = "__trbRecordZero" + strconv.Itoa(g.temporary)
			statements = append(statements, "var "+value+" "+g.goType(field.Type))
		}
		values = append(values, value)
		if field.HasDefault {
			values = append(values, strconv.FormatBool(provided))
		}
	}
	statements = append(statements, "return "+helper+"("+strings.Join(values, ", ")+")")
	return "func() " + typeName + " { " + strings.Join(statements, "; ") + " }()"
}

func (g *generator) unionMemberExpression(member *ir.Member) string {
	child := *g
	child.b = strings.Builder{}
	child.recordSources = false
	child.indent = 1
	resultType := g.goType(member.ExprType())
	field := goMethodName(member.Name)
	if member.ClassField {
		field = goFieldName(member.Name)
	}
	child.line("switch value := value.(type) {")
	child.indent++
	for _, alternative := range member.UnionAlternatives {
		child.line("case " + g.goType(alternative.Type) + ":")
		child.indent++
		value := "value." + field
		if member.ExprType().Kind == types.Float && (alternative.MemberType.Kind == types.Int || alternative.MemberType.Kind == types.IntLiteral) {
			value = "float64(" + value + ")"
		}
		child.line("return " + value)
		child.indent--
	}
	child.line("default:")
	child.indent++
	child.line("panic(\"unreachable discriminated union member access\")")
	child.indent--
	child.indent--
	child.line("}")
	return "func(value any) " + resultType + " {\n" + child.b.String() + strings.Repeat("\t", g.indent) + "}(" + g.expr(member.Receiver) + ")"
}

func (g *generator) caseNarrowings(narrowings []ir.CaseBinding) {
	for _, narrowing := range narrowings {
		name := g.bindingIdentifier(narrowing.Name)
		value := name
		if narrowing.Type.Kind != types.Union {
			value = name + ".(" + g.goType(narrowing.Type) + ")"
		}
		g.line(name + " := " + value)
		g.line("_ = " + name)
	}
}

func (g *generator) portableFloatInteger(value, operation string) string {
	g.requireImport("math", "")
	return "func() int { value := math." + operation + "(" + value + "); if math.IsNaN(value) || math.IsInf(value, 0) { panic(\"Float cannot be converted to Integer\") }; if value < -9007199254740991 || value > 9007199254740991 { panic(\"Integer is outside the portable range\") }; return int(value) }()"
}

func (g *generator) portableFloatString(value string) string {
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	return "func() string { value := " + value + "; if math.IsNaN(value) { return \"NaN\" }; if math.IsInf(value, 1) { return \"Infinity\" }; if math.IsInf(value, -1) { return \"-Infinity\" }; if value == 0 { return \"0.0\" }; text := strconv.FormatFloat(value, 'f', -1, 64); if !strings.Contains(text, \".\") { text += \".0\" }; return text }()"
}

func (g *generator) jsonParse(call *ir.Call, argument string, comments bool) string {
	g.requireImport("encoding/json", "stdjson")
	g.requireImport("errors", "")
	g.requireImport("io", "")
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	result := call.ExprType()
	resultType := g.goType(result)
	valueType := g.goType(types.FromName("JsonValue"))
	errorType := g.goType(types.FromName("JsonError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	jsonAlias := g.typeAliases["JsonValue"]
	prefix := ""
	if jsonAlias != "" {
		prefix = jsonAlias + "."
	}
	ok := func(value string) string {
		return resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "](" + value + ")"
	}
	errResult := func(kind, message, path, line, column string) string {
		value := errorType + "{Kind: " + prefix + "JsonErrorKind" + kind + ", Message: " + message + ", Path: " + path + ", Line: " + line + ", Column: " + column + "}"
		return resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "](" + value + ")"
	}
	strip := ""
	if comments {
		strip = `stripComments := func(input string) string { result := []byte(input); inString := false; escaped := false; for index := 0; index < len(result); index++ { if inString { if escaped { escaped = false; continue }; if result[index] == '\\' { escaped = true } else if result[index] == '"' { inString = false }; continue }; if result[index] == '"' { inString = true; continue }; if result[index] != '/' || index+1 >= len(result) { continue }; if result[index+1] == '/' { result[index], result[index+1] = ' ', ' '; index += 2; for index < len(result) && result[index] != '\n' { if result[index] != '\r' { result[index] = ' ' }; index++ }; index-- } else if result[index+1] == '*' { result[index], result[index+1] = ' ', ' '; index += 2; for index < len(result) { if index+1 < len(result) && result[index] == '*' && result[index+1] == '/' { result[index], result[index+1] = ' ', ' '; index++; break }; if result[index] != '\n' && result[index] != '\r' { result[index] = ' ' }; index++ } } }; return string(result) }; source = stripComments(source); `
	}
	conversionError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + prefix + "JsonErrorKindDecode, Message: message, Path: path}; return &value }"
	convert := "var convert func(any, string) (" + valueType + ", *" + errorType + "); convert = func(input any, path string) (" + valueType + ", *" + errorType + ") { switch value := input.(type) { case nil: return " + prefix + "JsonValueNull, nil; case bool: return " + prefix + "NewJsonValueBoolean(value), nil; case stdjson.Number: number, parseErr := strconv.ParseFloat(string(value), 64); if parseErr != nil || math.IsInf(number, 0) || math.IsNaN(number) { return " + valueType + "{}, conversionError(path, \"JSON number is not finite\") }; if math.Trunc(number) == number { if number < -9007199254740991 || number > 9007199254740991 { return " + valueType + "{}, conversionError(path, \"JSON integer is outside the portable range\") }; return " + prefix + "NewJsonValueInteger(int(number)), nil }; return " + prefix + "NewJsonValueFloat(number), nil; case string: return " + prefix + "NewJsonValueString(value), nil; case []any: items := make([]" + valueType + ", len(value)); for index, item := range value { converted, conversionErr := convert(item, path+\"/\"+strconv.Itoa(index)); if conversionErr != nil { return " + valueType + "{}, conversionErr }; items[index] = converted }; return " + prefix + "NewJsonValueArray(items), nil; case map[string]any: fields := make(map[string]" + valueType + ", len(value)); for key, item := range value { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); converted, conversionErr := convert(item, path+\"/\"+escaped); if conversionErr != nil { return " + valueType + "{}, conversionErr }; fields[key] = converted }; return " + prefix + "NewJsonValueObject(fields), nil; default: return " + valueType + "{}, conversionError(path, \"unsupported JSON value\") } }"
	location := "func(source string, parseErr error) (*int, *int) { syntax, ok := parseErr.(*stdjson.SyntaxError); if !ok { return nil, nil }; offset := int(syntax.Offset) - 1; if offset < 0 { offset = 0 }; if offset > len(source) { offset = len(source) }; line, column := 1, 1; for _, value := range source[:offset] { if value == '\\n' { line++; column = 1 } else { column++ } }; return &line, &column }"
	return "func() " + resultType + " { source := " + argument + "; " + strip + "sourceLocation := " + location + "; decoder := stdjson.NewDecoder(strings.NewReader(source)); decoder.UseNumber(); var raw any; if err := decoder.Decode(&raw); err != nil { lineValue, columnValue := sourceLocation(source, err); return " + errResult("Syntax", "err.Error()", `""`, "lineValue", "columnValue") + " }; if err := decoder.Decode(&struct{}{}); err != io.EOF { if err == nil { err = errors.New(\"JSON source contains multiple values\") }; lineValue, columnValue := sourceLocation(source, err); return " + errResult("Syntax", "err.Error()", `""`, "lineValue", "columnValue") + " }; conversionError := " + conversionError + "; " + convert + "; value, conversionErr := convert(raw, \"\"); if conversionErr != nil { return " + resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "](*conversionErr) }; return " + ok("value") + " }()"
}

func (g *generator) jsonStringify(call *ir.Call, argument string) string {
	g.requireImport("encoding/json", "stdjson")
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	result := call.ExprType()
	resultType := g.goType(result)
	valueType := g.goType(types.FromName("JsonValue"))
	errorType := g.goType(types.FromName("JsonError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	jsonAlias := g.typeAliases["JsonValue"]
	prefix := ""
	if jsonAlias != "" {
		prefix = jsonAlias + "."
	}
	ok := resultAlias + ".NewResultOk[string, " + errorType + "]"
	errResult := func(message, path string) string {
		value := errorType + "{Kind: " + prefix + "JsonErrorKindEncode, Message: " + message + ", Path: " + path + "}"
		return resultAlias + ".NewResultErr[string, " + errorType + "](" + value + ")"
	}
	conversionError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + prefix + "JsonErrorKindEncode, Message: message, Path: path}; return &value }"
	convert := "var convert func(" + valueType + ", string) (any, *" + errorType + "); convert = func(value " + valueType + ", path string) (any, *" + errorType + ") { switch value.Kind { case " + prefix + "JsonValueNullTag: return nil, nil; case " + prefix + "JsonValueBooleanTag: return value.BooleanValue, nil; case " + prefix + "JsonValueIntegerTag: if value.IntegerValue < -9007199254740991 || value.IntegerValue > 9007199254740991 { return nil, conversionError(path, \"JSON integer is outside the portable range\") }; return value.IntegerValue, nil; case " + prefix + "JsonValueFloatTag: if math.IsInf(value.FloatValue, 0) || math.IsNaN(value.FloatValue) { return nil, conversionError(path, \"JSON Float must be finite\") }; return value.FloatValue, nil; case " + prefix + "JsonValueStringTag: return value.StringValue, nil; case " + prefix + "JsonValueArrayTag: items := make([]any, len(value.ArrayValue)); for index, item := range value.ArrayValue { converted, conversionErr := convert(item, path+\"/\"+strconv.Itoa(index)); if conversionErr != nil { return nil, conversionErr }; items[index] = converted }; return items, nil; case " + prefix + "JsonValueObjectTag: fields := make(map[string]any, len(value.ObjectValue)); for key, item := range value.ObjectValue { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); converted, conversionErr := convert(item, path+\"/\"+escaped); if conversionErr != nil { return nil, conversionErr }; fields[key] = converted }; return fields, nil; default: return nil, conversionError(path, \"unsupported JSON value\") } }"
	return "func() " + resultType + " { conversionError := " + conversionError + "; " + convert + "; raw, conversionErr := convert(" + argument + ", \"\"); if conversionErr != nil { return " + resultAlias + ".NewResultErr[string, " + errorType + "](*conversionErr) }; encoded, err := stdjson.Marshal(raw); if err != nil { return " + errResult("err.Error()", `""`) + " }; return " + ok + "(string(encoded)) }()"
}

func (g *generator) jsonDecode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	jsonAlias := g.jsonRuntimeAlias(call)
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	valueType := g.goCodecType(call.Codec)
	errorType := jsonAlias + ".JsonError"
	resultType := g.goType(call.ExprType())
	builder := &goJSONCodecBuilder{generator: g, jsonAlias: jsonAlias, errorType: errorType}
	decoder := builder.decoder(call.Codec)
	decodeError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + jsonAlias + ".JsonErrorKindDecode, Message: message, Path: path}; return &value }"
	parse := jsonAlias + ".Parse(" + argument + ")"
	errResult := resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "]"
	okResult := resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "]"
	return "func() " + resultType + " { decodeError := " + decodeError + "; " + builder.source.String() + " parsed := " + parse + "; if parsed.Kind == " + resultAlias + ".ResultErrTag { return " + errResult + "(parsed.ErrError) }; decoded, codecErr := " + decoder + "(parsed.OkValue, \"\"); if codecErr != nil { return " + errResult + "(*codecErr) }; return " + okResult + "(decoded) }()"
}

func (g *generator) jsonEncode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	jsonAlias := g.jsonRuntimeAlias(call)
	builder := &goJSONCodecBuilder{generator: g, jsonAlias: jsonAlias, errorType: jsonAlias + ".JsonError"}
	encoder := builder.encoder(call.Codec)
	return "func() " + g.goType(call.ExprType()) + " { " + builder.source.String() + " return " + jsonAlias + ".Stringify(" + encoder + "(" + argument + ")) }()"
}

func (g *generator) jsonRuntimeAlias(call *ir.Call) string {
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Intrinsic == "trb.web.request_json" {
		if alias := g.typeAliases["JsonError"]; alias != "" {
			return alias
		}
		return "json"
	}
	if reference != nil && reference.Alias != "" {
		return goImportAlias(reference.Alias)
	}
	if reference != nil && reference.Package != "" {
		return goImportAlias(pathpkg.Base(pathpkg.Dir(reference.Package)))
	}
	return "json"
}

func (g *generator) goCodecType(schema *ir.CodecSchema) string {
	if schema == nil {
		return "any"
	}
	if isTimeCodec(schema.Kind) {
		return g.goType(schema.Type)
	}
	base := schema.Type
	nullable := base.Nullable
	base.Nullable = false
	var result string
	switch schema.Kind {
	case "array":
		result = "[]" + g.goCodecType(schema.Element)
	case "hash":
		result = "map[string]" + g.goCodecType(schema.Element)
	case "record", "raw_enum":
		result = goIdentifier(base.Name, true)
		if schema.Reference != nil && schema.Reference.Package != "" && schema.Reference.Package != g.modulePath {
			alias := schema.Reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(schema.Reference.Package))
			}
			result = goImportAlias(alias) + "." + result
		}
	default:
		result = g.goType(base)
	}
	if nullable {
		return "*" + result
	}
	return result
}

func isTimeCodec(kind string) bool {
	return kind == "time_date" || kind == "time_of_day" || kind == "time_datetime" || kind == "time_instant" || kind == "time_duration" || kind == "time_zone"
}

func goTimeCodecOwner(schema *ir.CodecSchema, generator *generator) string {
	if schema.Reference == nil || schema.Reference.Package == "" || schema.Reference.Package == generator.modulePath {
		return ""
	}
	alias := schema.Reference.Alias
	if alias == "" {
		alias = pathpkg.Base(pathpkg.Dir(schema.Reference.Package))
	}
	return goImportAlias(alias) + "."
}

func timeCodecParseMethod(kind string) string {
	switch kind {
	case "time_date":
		return "DateTryParse"
	case "time_of_day":
		return "TimeOfDayTryParse"
	case "time_datetime":
		return "DateTimeTryParse"
	case "time_instant":
		return "InstantTryParse"
	case "time_duration":
		return "DurationTryParse"
	case "time_zone":
		return "TimeZoneTryGet"
	default:
		return ""
	}
}

type goJSONCodecBuilder struct {
	generator *generator
	jsonAlias string
	errorType string
	source    strings.Builder
	next      int
}

func (b *goJSONCodecBuilder) name(prefix string) string {
	b.next++
	return "__trbJSON" + prefix + strconv.Itoa(b.next)
}

func (b *goJSONCodecBuilder) decoder(schema *ir.CodecSchema) string {
	name := b.name("Decode")
	valueType := b.generator.goCodecType(schema)
	jsonValue := b.jsonAlias + ".JsonValue"
	zero := "var zero " + valueType + "; return zero, decodeError(path, message)"
	expected := func(kind string) string {
		return "message := \"expected " + kind + "\"; " + zero
	}
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.decoder(&nonnull)
		body := "if value.Kind == " + b.jsonAlias + ".JsonValueNullTag { return nil, nil }; decoded, err := " + child + "(value, path); if err != nil { return nil, err }; return &decoded, nil"
		if isTimeCodec(schema.Kind) {
			body = "if value.Kind == " + b.jsonAlias + ".JsonValueNullTag { return nil, nil }; return " + child + "(value, path)"
		}
		b.source.WriteString(name + " := func(value " + jsonValue + ", path string) (" + valueType + ", *" + b.errorType + ") { " + body + " }; ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueBooleanTag { " + expected("Boolean") + " }; return value.BooleanValue, nil"
	case "integer":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueIntegerTag { " + expected("Integer") + " }; return value.IntegerValue, nil"
	case "float":
		body = "if value.Kind == " + b.jsonAlias + ".JsonValueIntegerTag { return float64(value.IntegerValue), nil }; if value.Kind != " + b.jsonAlias + ".JsonValueFloatTag { " + expected("Float") + " }; return value.FloatValue, nil"
	case "string":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueStringTag { " + expected("String") + " }; return value.StringValue, nil"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		method := goTimeCodecOwner(schema, b.generator) + timeCodecParseMethod(schema.Kind)
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueStringTag { " + expected("String") + " }; parsed := " + method + "(value.StringValue); if parsed.Kind != 0 { message := " + strconv.Quote("invalid "+schema.Type.Name) + "; " + zero + " }; return parsed.OkValue, nil"
	case "raw_enum":
		raw := "value.StringValue"
		expectedKind := "String"
		jsonTag := "JsonValueStringTag"
		if schema.RawType.Kind == types.Int {
			raw = "value.IntegerValue"
			expectedKind = "Integer"
			jsonTag = "JsonValueIntegerTag"
		}
		prefix := ""
		if schema.Reference != nil && schema.Reference.Package != "" && schema.Reference.Package != b.generator.modulePath {
			alias := schema.Reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(schema.Reference.Package))
			}
			prefix = goImportAlias(alias) + "."
		}
		branches := make([]string, 0, len(schema.RawValues))
		for _, item := range schema.RawValues {
			branches = append(branches, "case "+item.Raw+": return "+prefix+goConstantIdentifier(schema.Type.Name, item.Member)+", nil")
		}
		body = "if value.Kind != " + b.jsonAlias + "." + jsonTag + " { " + expected(expectedKind) + " }; switch " + raw + " { " + strings.Join(branches, "; ") + " }; message := " + strconv.Quote("unknown raw value for "+schema.Type.Name) + "; " + zero
	case "array":
		b.generator.requireImport("strconv", "")
		child := b.decoder(schema.Element)
		elementType := b.generator.goCodecType(schema.Element)
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueArrayTag { " + expected("Array") + " }; decoded := make([]" + elementType + ", len(value.ArrayValue)); for index, item := range value.ArrayValue { child, err := " + child + "(item, path+\"/\"+strconv.Itoa(index)); if err != nil { var zero " + valueType + "; return zero, err }; decoded[index] = child }; return decoded, nil"
	case "hash":
		b.generator.requireImport("strings", "")
		child := b.decoder(schema.Element)
		elementType := b.generator.goCodecType(schema.Element)
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueObjectTag { " + expected("Object") + " }; decoded := make(map[string]" + elementType + ", len(value.ObjectValue)); for key, item := range value.ObjectValue { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); child, err := " + child + "(item, path+\"/\"+escaped); if err != nil { var zero " + valueType + "; return zero, err }; decoded[key] = child }; return decoded, nil"
	case "record":
		var fields strings.Builder
		fields.WriteString("if value.Kind != " + b.jsonAlias + ".JsonValueObjectTag { " + expected(schema.Type.Name) + " }; ")
		constructor := make([]string, 0, len(schema.Fields))
		for index, field := range schema.Fields {
			child := b.decoder(field.Schema)
			variable := "field" + strconv.Itoa(index)
			fieldType := b.generator.goCodecType(field.Schema)
			path := strconv.Quote("/" + jsonPointerEscape(field.WireName))
			fields.WriteString("var " + variable + " " + fieldType + "; if raw, exists := value.ObjectValue[" + strconv.Quote(field.WireName) + "]; exists { decoded, err := " + child + "(raw, path+" + path + "); if err != nil { var zero " + valueType + "; return zero, err }; " + variable + " = decoded }")
			if !field.Schema.Type.Nullable {
				fields.WriteString(" else { message := " + strconv.Quote("missing field "+field.WireName) + "; var zero " + valueType + "; return zero, decodeError(path+" + path + ", message) }")
			}
			fields.WriteString("; ")
			constructor = append(constructor, goIdentifier(field.Name, true)+": "+variable)
		}
		fields.WriteString("return " + valueType + "{" + strings.Join(constructor, ", ") + "}, nil")
		body = fields.String()
	}
	b.source.WriteString(name + " := func(value " + jsonValue + ", path string) (" + valueType + ", *" + b.errorType + ") { " + body + " }; ")
	return name
}

func (b *goJSONCodecBuilder) encoder(schema *ir.CodecSchema) string {
	name := b.name("Encode")
	valueType := b.generator.goCodecType(schema)
	jsonValue := b.jsonAlias + ".JsonValue"
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.encoder(&nonnull)
		body := "if value == nil { return " + b.jsonAlias + ".JsonValueNull }; return " + child + "(*value)"
		if isTimeCodec(schema.Kind) {
			body = "if value == nil { return " + b.jsonAlias + ".JsonValueNull }; return " + child + "(value)"
		}
		b.source.WriteString(name + " := func(value " + valueType + ") " + jsonValue + " { " + body + " }; ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "return " + b.jsonAlias + ".NewJsonValueBoolean(value)"
	case "integer":
		body = "return " + b.jsonAlias + ".NewJsonValueInteger(value)"
	case "float":
		body = "return " + b.jsonAlias + ".NewJsonValueFloat(value)"
	case "string":
		body = "return " + b.jsonAlias + ".NewJsonValueString(value)"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration":
		body = "return " + b.jsonAlias + ".NewJsonValueString(value.ToS())"
	case "time_zone":
		body = "return " + b.jsonAlias + ".NewJsonValueString(value.Identifier())"
	case "raw_enum":
		if schema.RawType.Kind == types.Int {
			body = "return " + b.jsonAlias + ".NewJsonValueInteger(int(value))"
		} else {
			body = "return " + b.jsonAlias + ".NewJsonValueString(string(value))"
		}
	case "array":
		child := b.encoder(schema.Element)
		body = "items := make([]" + jsonValue + ", len(value)); for index, item := range value { items[index] = " + child + "(item) }; return " + b.jsonAlias + ".NewJsonValueArray(items)"
	case "hash":
		child := b.encoder(schema.Element)
		body = "fields := make(map[string]" + jsonValue + ", len(value)); for key, item := range value { fields[key] = " + child + "(item) }; return " + b.jsonAlias + ".NewJsonValueObject(fields)"
	case "record":
		parts := make([]string, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			child := b.encoder(field.Schema)
			parts = append(parts, strconv.Quote(field.WireName)+": "+child+"(value."+goIdentifier(field.Name, true)+")")
		}
		body = "return " + b.jsonAlias + ".NewJsonValueObject(map[string]" + jsonValue + "{" + strings.Join(parts, ", ") + "})"
	}
	b.source.WriteString(name + " := func(value " + valueType + ") " + jsonValue + " { " + body + " }; ")
	return name
}

func jsonPointerEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func (g *generator) gormRead(call *ir.Call, arguments []string, operation string) string {
	resultType := g.goType(call.ExprType())
	valueType := resultType
	if call.ExprType().Kind == types.Array && len(call.ExprType().Args) > 0 {
		valueType = g.goType(call.ExprType().Args[0])
	}
	args := ""
	if len(arguments) > 3 {
		args = ", " + strings.Join(arguments[3:], ", ")
	}
	switch operation {
	case "find_all":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Find(&result).Error; err != nil { panic(err) }; return result }()"
	case "where":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Where(" + arguments[2] + args + ").Find(&result).Error; err != nil { panic(err) }; return result }()"
	case "raw":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Raw(" + arguments[2] + args + ").Scan(&result).Error; err != nil { panic(err) }; return result }()"
	case "first":
		return "func() " + valueType + " { var result " + valueType + "; if err := " + arguments[0] + ".Where(" + arguments[2] + args + ").First(&result).Error; err != nil { panic(err) }; return result }()"
	default:
		return "nil"
	}
}

func (g *generator) gormWrite(call *ir.Call, arguments []string, operation string) string {
	resultType := g.goType(call.ExprType())
	return "func() " + resultType + " { value := " + arguments[1] + "; if err := " + arguments[0] + "." + operation + "(&value).Error; err != nil { panic(err) }; return value }()"
}

func expressionReference(expression ir.Expression) *ir.Reference {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Reference
	case *ir.Member:
		return node.Reference
	case *ir.TypeApply:
		return expressionReference(node.Receiver)
	default:
		return nil
	}
}

func receiverMember(expression ir.Expression) (*ir.Member, bool) {
	switch node := expression.(type) {
	case *ir.Member:
		return node, true
	case *ir.TypeApply:
		return receiverMember(node.Receiver)
	default:
		return nil, false
	}
}

func referenceIntrinsic(expression ir.Expression) string {
	reference := expressionReference(expression)
	if reference == nil {
		return ""
	}
	return reference.Intrinsic
}

func (g *generator) referenceAlias(reference *ir.Reference) string {
	if reference == nil || reference.Intrinsic != "" || reference.Package == "" {
		return ""
	}
	if strings.TrimSuffix(reference.Package, "/index") == "trb/http" {
		return "__trb_http"
	}
	directory := pathpkg.Dir(reference.Package)
	if directory == "." {
		directory = ""
	}
	if directory == g.currentDirectory() {
		return ""
	}
	generatedDirectory := GeneratedSourceDirectory(directory)
	importPath := generatedDirectory
	if g.goModule != "" {
		importPath = pathpkg.Join(g.goModule, generatedDirectory)
	}
	if alias, exists := g.imports[importPath]; exists {
		if alias == "" {
			alias = pathpkg.Base(importPath)
		}
		return goImportAlias(alias)
	}
	alias := reference.Alias
	if alias == "" {
		alias = pathpkg.Base(directory)
	}
	return goImportAlias(alias)
}

func goImportAlias(name string) string {
	if name == "_" {
		return "_"
	}
	if strings.HasPrefix(name, "__trb_") {
		return name
	}
	return goIdentifier(name, false)
}

func (g *generator) goImportedName(name string, reference *ir.Reference) string {
	if reference != nil && reference.ExportKind == "function" {
		return g.projectFunctionName(reference.Package, reference.Symbol)
	}
	if reference != nil && reference.ExportKind == "value" && isUpper(name) {
		return goConstantIdentifier("", name)
	}
	return goIdentifier(name, true)
}

func (g *generator) goType(t types.Type) string {
	var result string
	switch t.Kind {
	case types.Void:
		result = ""
	case types.Any, types.Invalid:
		result = "any"
	case types.Union:
		if base, ok := types.LiteralUnionBase(t); ok {
			result = g.goType(base)
		} else {
			result = "any"
		}
	case types.Bool:
		result = "bool"
	case types.Int, types.IntLiteral:
		result = "int"
	case types.Float:
		result = "float64"
	case types.String, types.StringLiteral:
		result = "string"
	case types.Bytes:
		result = "[]byte"
	case types.StringBuilder:
		g.requireImport("strings", "")
		result = "*strings.Builder"
	case types.Array, types.Iterable:
		element := "any"
		if len(t.Args) > 0 {
			element = g.goType(t.Args[0])
		}
		result = "[]" + element
	case types.Range:
		result = "[3]int"
	case types.Hash:
		key := "any"
		value := "any"
		if len(t.Args) == 2 {
			key = g.goType(t.Args[0])
			value = g.goType(t.Args[1])
		}
		result = "map[" + key + "]" + value
	case types.Function:
		parameters, returned, ok := types.FunctionSignature(t)
		if !ok {
			result = "func()"
			break
		}
		parts := make([]string, len(parameters))
		for index, parameter := range parameters {
			parts[index] = g.goType(parameter)
		}
		result = "func(" + strings.Join(parts, ", ") + ")"
		if returnType := g.goType(returned); returnType != "" {
			result += " " + returnType
		}
	default:
		if t.Name == "" {
			result = "any"
		} else if (t.Name == "EnqueueError" || t.Name == "JobReference") && g.jobs != nil && g.modulePath != "trb/jobs/index" && g.typeAliases[t.Name] == "" {
			result = g.jobsContractAlias() + "." + goIdentifier(t.Name, true)
		} else if t.Name == "DbError" && g.orm != nil && g.modulePath != "trb/orm/index" && g.typeAliases[t.Name] == "" {
			result = g.ormLifecycleAlias() + ".DbError"
		} else if t.Name == "DbErrorKind" && g.orm != nil && g.modulePath != "trb/orm/index" && g.typeAliases[t.Name] == "" {
			result = g.ormLifecycleAlias() + ".DbErrorKind"
		} else if t.Name == "DbResult" && g.orm != nil && g.modulePath != "trb/orm/index" && g.typeAliases[t.Name] == "" {
			result = g.ormLifecycleAlias() + ".DbResult"
		} else if t.Name == "Transaction" && g.orm != nil {
			result = "*" + g.ormLifecycleAlias() + ".TrbOrmTransaction"
		} else if t.Name == "Subquery" && g.orm != nil {
			result = "*" + g.ormLifecycleAlias() + ".TrbOrmSubquery"
		} else if model, ok := g.orm.Model(t.Name); ok {
			result = "*" + g.ormModelQualifier(model) + goIdentifier(model.Name, true)
		} else if model, ok := g.orm.QueryModel(t.Name); ok {
			result = g.ormModelQualifier(model) + goORMQueryType(model)
		} else if model, ok := g.orm.ScopeModel(t.Name); ok {
			result = g.ormModelQualifier(model) + goORMQueryType(model)
		} else if model, column, ok := g.orm.GroupModel(t.Name); ok {
			result = g.ormModelQualifier(model) + goORMGroupType(model, column)
		} else if model, ok := g.orm.DraftModel(t.Name); ok {
			result = "*" + g.ormModelQualifier(model) + goIdentifier(model.DraftType(), true)
		} else if model, ok := g.orm.ChangesModel(t.Name); ok {
			result = "*" + g.ormModelQualifier(model) + goIdentifier(model.ChangesType(), true)
		} else if t.Name == "GormDB" {
			g.requireImport("gorm.io/gorm", "gorm")
			result = "*gorm.DB"
		} else if t.Name == "HTTPRouter" {
			g.requireImport("net/http", "http")
			result = "*http.ServeMux"
		} else if t.Name == "HTTPRequest" {
			g.requireImport("net/http", "http")
			result = "*http.Request"
		} else if t.Name == "HTTPResponse" {
			g.requireImport("net/http", "http")
			result = "http.ResponseWriter"
		} else if t.Name == "HTTPHandler" {
			g.requireImport("net/http", "http")
			result = "http.Handler"
		} else if alias := g.typeAliases[t.Name]; alias != "" {
			result = alias + "." + goIdentifier(t.Name, true)
			if g.typeKinds[t.Name] == "class" {
				result = "*" + result
			}
		} else {
			result = goIdentifier(t.Name, true)
			if g.classes[t.Name] {
				result = "*" + result
			}
		}
	}
	if t.Kind == types.Named && len(t.Args) > 0 {
		arguments := make([]string, len(t.Args))
		for index, argument := range t.Args {
			arguments[index] = g.goType(argument)
		}
		result += "[" + strings.Join(arguments, ", ") + "]"
	}
	if t.Nullable && result != "" && result != "any" && !strings.HasPrefix(result, "*") {
		return "*" + result
	}
	return result
}

func (g *generator) projectFunctionName(modulePath, sourceName string) string {
	if g.projectNames != nil {
		if names := g.projectNames.functions[modulePath]; names != nil {
			if name := names[sourceName]; name != "" {
				return name
			}
		}
	}
	if sourceName == "main" {
		return "main"
	}
	return goMethodName(sourceName)
}

func (g *generator) goReturn(t types.Type) string {
	if t.Kind == types.Void {
		return ""
	}
	return " " + g.goType(t)
}

func goFieldName(name string) string {
	name = strings.TrimPrefix(name, "@")
	if strings.HasPrefix(name, "_trb_") {
		return "TrbInternal" + goIdentifier(strings.TrimPrefix(name, "_trb_"), true)
	}
	if strings.HasPrefix(name, "_") {
		return "trbField" + goIdentifier(strings.TrimPrefix(name, "_"), true)
	}
	return "TrbField" + goIdentifier(name, true)
}

func goMethodName(name string) string {
	private := strings.HasPrefix(name, "_")
	if kind, encoded, ok := naming.CallableSuffix(name); ok {
		prefix := "Trb" + upperFirst(kind) + "_"
		if private {
			prefix = "trb" + upperFirst(kind) + "_"
		}
		return prefix + encoded
	}
	name = strings.TrimPrefix(name, "_")
	return goIdentifier(name, !private && name != "main")
}

func goIdentifier(name string, exported bool) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == '/' || r == '.' })
	if len(parts) == 0 {
		return "value"
	}
	for i := range parts {
		if i > 0 || exported {
			parts[i] = upperFirst(parts[i])
		} else {
			parts[i] = lowerFirst(parts[i])
		}
	}
	return strings.Join(parts, "")
}

func goConstantIdentifier(owner, name string) string {
	var result strings.Builder
	if owner != "" {
		for _, part := range strings.Split(owner, "::") {
			result.WriteString(goIdentifier(part, true))
		}
	}
	result.WriteString(goIdentifier(strings.ToLower(name), true))
	return result.String()
}

func goTrailingComment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return " //" + strings.TrimPrefix(value, "#")
}

func irExpressionName(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Name
	case *ir.Member:
		prefix := irExpressionName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	default:
		return ""
	}
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}

func findInitialize(methods []*ir.Method) *ir.Method {
	for _, method := range methods {
		if method.Name == "initialize" {
			return method
		}
	}
	return nil
}

func usesInterpolation(statements []ir.Statement) bool {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ir.Class:
			if expressionUsesInterpolation(n.Superclass) || usesInterpolation(n.Body) {
				return true
			}
		case *ir.Module:
			if usesInterpolation(n.Body) {
				return true
			}
		case *ir.Interface:
			for _, method := range n.Methods {
				if usesInterpolation(method.Body) {
					return true
				}
			}
		case *ir.Field:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Method:
			for _, parameter := range n.Parameters {
				if expressionUsesInterpolation(parameter.Default) {
					return true
				}
			}
			if usesInterpolation(n.Body) {
				return true
			}
		case *ir.Variable:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Assignment:
			if expressionUsesInterpolation(n.Target) || expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Return:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.ExpressionStatement:
			if expressionUsesInterpolation(n.Expression) {
				return true
			}
		case *ir.If:
			if expressionUsesInterpolation(n.Condition) || usesInterpolation(n.Then) || usesInterpolation(n.Else) {
				return true
			}
			for _, branch := range n.ElseIf {
				if expressionUsesInterpolation(branch.Condition) || usesInterpolation(branch.Body) {
					return true
				}
			}
		case *ir.Case:
			if expressionUsesInterpolation(n.Value) || usesInterpolation(n.Leading) || usesInterpolation(n.Else) {
				return true
			}
			for _, branch := range n.Branches {
				if expressionUsesInterpolation(branch.Value) || expressionsUseInterpolation(branch.Alternatives) || usesInterpolation(branch.Body) {
					return true
				}
			}
		case *ir.While:
			if expressionUsesInterpolation(n.Condition) || usesInterpolation(n.Body) {
				return true
			}
		case *ir.Iterate:
			if expressionUsesInterpolation(n.Source) || expressionUsesInterpolation(n.SliceSize) || usesInterpolation(n.Body) {
				return true
			}
		}
	}
	return false
}

func expressionUsesInterpolation(expression ir.Expression) bool {
	switch n := expression.(type) {
	case nil:
		return false
	case *ir.InterpolatedString:
		return true
	case *ir.Array:
		for _, element := range n.Elements {
			if expressionUsesInterpolation(element) {
				return true
			}
		}
	case *ir.Hash:
		for _, entry := range n.Entries {
			if expressionUsesInterpolation(entry.Key) || expressionUsesInterpolation(entry.Value) {
				return true
			}
		}
	case *ir.Unary:
		return expressionUsesInterpolation(n.Operand)
	case *ir.Conversion:
		return expressionUsesInterpolation(n.Value)
	case *ir.Binary:
		return expressionUsesInterpolation(n.Left) || expressionUsesInterpolation(n.Right)
	case *ir.Range:
		return expressionUsesInterpolation(n.Start) || expressionUsesInterpolation(n.End)
	case *ir.Call:
		if expressionUsesInterpolation(n.Callee) {
			return true
		}
		for _, argument := range n.Arguments {
			if expressionUsesInterpolation(argument.Value) {
				return true
			}
		}
	case *ir.Member:
		return expressionUsesInterpolation(n.Receiver)
	case *ir.Index:
		return expressionUsesInterpolation(n.Receiver) || expressionUsesInterpolation(n.Index)
	case *ir.Block:
		return usesInterpolation(n.Body)
	case *ir.If:
		if expressionUsesInterpolation(n.Condition) || expressionUsesInterpolation(n.ThenResult) || expressionUsesInterpolation(n.ElseResult) || usesInterpolation(n.Then) || usesInterpolation(n.Else) {
			return true
		}
		for _, branch := range n.ElseIf {
			if expressionUsesInterpolation(branch.Condition) || expressionUsesInterpolation(branch.Result) || usesInterpolation(branch.Body) {
				return true
			}
		}
	case *ir.Case:
		if expressionUsesInterpolation(n.Value) || expressionUsesInterpolation(n.ElseResult) || usesInterpolation(n.Leading) || usesInterpolation(n.Else) {
			return true
		}
		for _, branch := range n.Branches {
			if expressionUsesInterpolation(branch.Value) || expressionsUseInterpolation(branch.Alternatives) || expressionUsesInterpolation(branch.Result) || usesInterpolation(branch.Body) {
				return true
			}
		}
	}
	return false
}

func expressionsUseInterpolation(expressions []ir.Expression) bool {
	for _, expression := range expressions {
		if expressionUsesInterpolation(expression) {
			return true
		}
	}
	return false
}

func isUpper(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

func (g *generator) line(text string) {
	g.b.WriteString(strings.Repeat("\t", g.indent))
	g.b.WriteString(text)
	g.b.WriteByte('\n')
}
