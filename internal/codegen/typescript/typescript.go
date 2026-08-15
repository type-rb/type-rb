package typescript

import (
	"fmt"
	pathpkg "path"
	"strconv"
	"strings"

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
	b                strings.Builder
	indent           int
	inClass          int
	functionDepth    int
	methods          map[string]bool
	modulePath       string
	moduleExtensions map[string]string
	topFunctions     map[string]bool
	topTargets       map[string]string
	records          map[string]bool
	typeAliases      map[string]string
	typeMappings     map[string]string
	browserRuntime   string
	httpRuntime      bool
	webRuntime       bool
	jsonRuntime      bool
	reactStateHelper bool
	oidcRuntime      bool
	temporary        int
	suspension       *SuspensionPlan
	execution        *effectplan.Plan
	executionActive  bool
	jobs             *jobsintegration.Manifest
	jobsSQL          *jobssql.Manifest
	orm              *ormintegration.Manifest
	breakTarget      string
	enumReceiver     string
	sourceRecorder   *sourcemap.Recorder
	sourcePath       string
}

func Generate(program *ir.Program) string {
	generated, err := GenerateProjectMapped([]*ir.Program{program})
	if err != nil || len(generated) == 0 {
		return ""
	}
	return generated[0].Output
}

func GenerateProject(programs []*ir.Program) ([]string, error) {
	mapped, err := GenerateProjectMapped(programs)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(mapped))
	for index, generated := range mapped {
		result[index] = generated.Output
	}
	return result, nil
}

func GenerateMapped(program *ir.Program) (sourcemap.Generated, error) {
	generated, err := GenerateProjectMapped([]*ir.Program{program})
	if err != nil || len(generated) == 0 {
		return sourcemap.Generated{}, err
	}
	return generated[0], nil
}

func GenerateProjectMapped(programs []*ir.Program) ([]sourcemap.Generated, error) {
	interactive := false
	for _, program := range programs {
		// The REPL executes ORM operations through its shared host runtime; it does
		// not execute the generated TypeScript modules in the configured runtime.
		if program.ModulePath == "__trb_repl__" {
			interactive = true
			break
		}
	}
	for _, program := range programs {
		if !interactive && ormintegration.ManifestFrom(program.Extensions) != nil && program.TypeScriptRuntime != "bun" {
			return nil, fmt.Errorf(`trb/orm in mode: typescript currently requires typescript.runtime: "bun"`)
		}
		if !interactive && jobssql.ManifestFrom(program.Extensions) != nil && program.TypeScriptRuntime != "bun" {
			return nil, fmt.Errorf(`trb/jobs in mode: typescript currently requires typescript.runtime: "bun"`)
		}
	}
	plan, err := AnalyzeSuspension(programs)
	if err != nil {
		return nil, err
	}
	execution := effectplan.ExecutionScope(programs)
	moduleExtensions := map[string]string{}
	for _, program := range programs {
		moduleExtensions[program.ModulePath] = ".ts"
		if program.UsesJSX {
			moduleExtensions[program.ModulePath] = ".tsx"
		}
	}
	generated := make([]sourcemap.Generated, len(programs))
	for index, program := range programs {
		generated[index] = generate(program, plan, execution, moduleExtensions)
	}
	return generated, nil
}

func generate(program *ir.Program, suspension *SuspensionPlan, execution *effectplan.Plan, moduleExtensions map[string]string) sourcemap.Generated {
	g := &generator{modulePath: program.ModulePath, moduleExtensions: moduleExtensions, topFunctions: map[string]bool{}, topTargets: map[string]string{}, records: map[string]bool{}, typeAliases: map[string]string{}, typeMappings: map[string]string{}, suspension: suspension, execution: execution, jobs: jobsintegration.ManifestFrom(program.Extensions), jobsSQL: jobssql.ManifestFrom(program.Extensions), orm: ormintegration.ManifestFrom(program.Extensions), sourceRecorder: sourcemap.NewRecorder(program.SourcePath), sourcePath: program.SourcePath}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok {
			g.topFunctions[method.Name] = true
			if method.TargetName != "" {
				g.topTargets[method.Name] = method.TargetName
			}
		}
		if record, ok := statement.(*ir.Record); ok {
			g.records[record.Name] = true
		}
	}
	g.integrationImports(program.Extensions)
	for i, statement := range program.Statements {
		if i > 0 {
			g.b.WriteByte('\n')
		}
		g.statement(statement)
	}
	g.integrations(program.Extensions)
	if g.modulePath == "trb/std/test/index" {
		g.testRuntimeSupport()
	}
	if g.oidcRuntime {
		g.oidcBearerRuntimeSupport()
	}
	if g.topFunctions["main"] {
		if len(program.Statements) > 0 {
			g.b.WriteByte('\n')
		}
		method := topLevelMethod(program.Statements, "main")
		call := "main()"
		if g.methodUsesExecutionScope(method) {
			call = "main(undefined)"
		}
		if method != nil && suspension.Methods[method] {
			call = "await " + call
		}
		if g.jobs != nil && len(g.jobs.Jobs) > 0 {
			g.line("if (!(await trbJobsRunWorkerOrCommand())) { " + call + "; }")
		} else {
			g.line(call + ";")
		}
	}
	output := strings.TrimRight(g.b.String(), "\n") + "\n"
	return sourcemap.Generated{Output: output, Map: g.sourceRecorder.Build(output)}
}

func (g *generator) ensureHTTPRuntime() string {
	const alias = "__trb_http"
	if !g.httpRuntime {
		g.line("import * as " + alias + " from " + strconv.Quote(tsImportPath(g.modulePath, "trb/http/index", g.moduleExtensions["trb/http/index"])) + ";")
		g.httpRuntime = true
	}
	for _, symbol := range []string{"Body", "Header", "Headers", "HttpMethod"} {
		g.typeAliases[symbol] = alias
	}
	return alias
}

func (g *generator) ensureWebRuntime() string {
	const alias = "__trb_web"
	if !g.webRuntime {
		g.line("import * as " + alias + " from " + strconv.Quote(tsImportPath(g.modulePath, "trb/web/index", g.moduleExtensions["trb/web/index"])) + ";")
		g.webRuntime = true
	}
	return alias
}

func (g *generator) ensureJSONRuntime() string {
	const alias = "__trb_json"
	if !g.jsonRuntime {
		g.line("import * as " + alias + " from " + strconv.Quote(tsImportPath(g.modulePath, "trb/std/json/index", g.moduleExtensions["trb/std/json/index"])) + ";")
		g.jsonRuntime = true
	}
	return alias
}

func topLevelMethod(statements []ir.Statement, name string) *ir.Method {
	for _, statement := range statements {
		if method, ok := statement.(*ir.Method); ok && method.Name == name {
			return method
		}
	}
	return nil
}

func (g *generator) statements(statements []ir.Statement) {
	if g.executionActive && len(statements) > 0 {
		g.line(`{ const __trbDeadline = (__trbScope as (AbortSignal & { __trbDeadline?: number; __trbCancel?: () => void }) | undefined)?.__trbDeadline; if (__trbDeadline !== undefined && performance.now() >= __trbDeadline) (__trbScope as AbortSignal & { __trbCancel?: () => void }).__trbCancel?.(); if (__trbScope?.aborted) throw new DOMException("TypeRB execution was cancelled", "AbortError"); }`)
	}
	for _, statement := range statements {
		g.statement(statement)
	}
}

func (g *generator) statement(statement ir.Statement) {
	start := g.b.Len()
	defer func() {
		if g.sourceRecorder != nil {
			g.sourceRecorder.Record(start, g.b.Len(), statement.SourceSpan())
		}
	}()
	switch n := statement.(type) {
	case *ir.Comment:
		g.line(comment(n.Text))
	case *ir.Import:
		if n.Path == "trb/platform/typescript/react" || n.Path == "trb/platform/typescript/react/index" {
			g.line(`import * as React from "react";`)
			for _, symbol := range n.Symbols {
				switch symbol {
				case "ReactNode":
					g.typeMappings[symbol] = "React.ReactNode"
				case "SyntheticEvent":
					g.typeMappings[symbol] = "React.SyntheticEvent<HTMLElement>"
				case "MouseEvent":
					g.typeMappings[symbol] = "React.MouseEvent<HTMLElement>"
				case "ChangeEvent":
					g.typeMappings[symbol] = "React.ChangeEvent<HTMLInputElement>"
				case "FormEvent":
					g.typeMappings[symbol] = "React.FormEvent<HTMLFormElement>"
				case "KeyboardEvent":
					g.typeMappings[symbol] = "React.KeyboardEvent<HTMLElement>"
				case "ReactState":
					g.typeMappings[symbol] = "__TrbReactState"
				}
			}
			if containsString(n.Symbols, "mount") {
				g.line(`import { createRoot } from "react-dom/client";`)
			}
			if containsString(n.Symbols, "use_state") && !g.reactStateHelper {
				g.typeMappings["ReactState"] = "__TrbReactState"
				g.line(`type __TrbReactState<T> = Readonly<{ value: T; set: (value: T) => void }>;`)
				g.line(`function useTrbState<T>(initial: T): __TrbReactState<T> {`)
				g.indent++
				g.line(`const [value, setValue] = React.useState<T>(initial);`)
				g.line(`return { value, set: setValue };`)
				g.indent--
				g.line(`}`)
				g.reactStateHelper = true
			}
			return
		}
		if (n.Standard || n.Official) && (!n.Runtime || !n.RuntimeRequired) {
			return
		}
		importPath := n.Path
		if !n.Native {
			importPath = tsImportPath(g.modulePath, n.Path, g.moduleExtensions[n.Path])
		}
		browserRuntime := n.Path == "trb/platform/typescript/browser/index" && n.RuntimeRequired
		if browserRuntime {
			browserAlias := "__trb_browser"
			if n.Namespace && n.Alias != "" {
				browserAlias = n.Alias
			}
			g.browserRuntime = browserAlias
			g.ensureHTTPRuntime()
			for symbol, kind := range n.SymbolKinds {
				switch kind {
				case "class", "record", "enum", "interface", "type_alias", "enum_alias":
					g.typeAliases[symbol] = browserAlias
				}
			}
			if !n.Namespace || n.Alias == "" {
				g.line("import * as " + browserAlias + " from " + strconv.Quote(importPath) + ";")
			}
		}
		webRuntime := n.Path == "trb/web/index" && n.RuntimeRequired
		if webRuntime {
			g.ensureHTTPRuntime()
			g.ensureWebRuntime()
			g.ensureJSONRuntime()
		}
		jsonRuntime := n.Path == "trb/std/json/index" && n.RuntimeRequired
		if n.Namespace && n.Alias != "" {
			for symbol, kind := range n.SymbolKinds {
				switch kind {
				case "class", "record", "enum", "interface", "type_alias", "enum_alias":
					g.typeAliases[symbol] = n.Alias
				}
			}
			g.line("import * as " + n.Alias + " from " + strconv.Quote(importPath) + ";")
		} else if len(n.Symbols) > 0 {
			values := make([]string, 0, len(n.Symbols))
			types := make([]string, 0, len(n.Symbols))
			intrinsicRuntime := false
			for _, symbol := range n.Symbols {
				if n.IntrinsicSymbols[symbol] {
					if !n.RuntimeIndependentSymbols[symbol] {
						intrinsicRuntime = true
					}
					continue
				}
				switch n.SymbolKinds[symbol] {
				case "record", "interface", "type_alias":
					types = append(types, symbol)
				case "function":
					values = append(values, tsCallableName(symbol))
				default:
					values = append(values, symbol)
				}
			}
			for _, symbol := range n.GeneratedTypeSymbols {
				target := &types
				switch n.SymbolKinds[symbol] {
				case "enum", "enum_alias":
					// Generated codecs construct enum values at runtime. Keep
					// transitive enum dependencies as value imports even when the
					// source only names their enclosing record.
					target = &values
				}
				if !containsString(*target, symbol) {
					*target = append(*target, symbol)
				}
			}
			if intrinsicRuntime && !browserRuntime && !webRuntime && !(jsonRuntime && g.jsonRuntime) {
				g.line("import * as __trb_" + pathpkg.Base(pathpkg.Dir(n.Path)) + " from " + strconv.Quote(importPath) + ";")
				if jsonRuntime {
					g.jsonRuntime = true
				}
			}
			if len(values) > 0 {
				g.line("import { " + strings.Join(values, ", ") + " } from " + strconv.Quote(importPath) + ";")
			}
			if len(types) > 0 {
				g.line("import type { " + strings.Join(types, ", ") + " } from " + strconv.Quote(importPath) + ";")
			}
		} else if n.Alias != "" {
			g.line("import * as " + n.Alias + " from " + strconv.Quote(importPath) + ";")
		} else {
			g.line("import " + strconv.Quote(importPath) + ";")
		}
	case *ir.Class:
		if n.External {
			return
		}
		header := "export class " + n.Name + tsTypeParameterDeclarations(n.TypeParameters)
		if n.Superclass != nil {
			header += " extends " + g.expr(n.Superclass)
		}
		if len(n.Implements) > 0 {
			implemented := make([]string, len(n.Implements))
			for index, item := range n.Implements {
				implemented[index] = g.tsType(item)
			}
			header += " implements " + strings.Join(implemented, ", ")
		}
		g.line(header + " {")
		g.indent++
		g.inClass++
		previousMethods := g.methods
		g.methods = classMethods(n.Body)
		for _, member := range n.Body {
			g.statement(member)
		}
		g.methods = previousMethods
		g.inClass--
		g.indent--
		g.line("}")
	case *ir.Record:
		g.line("export interface " + n.Name + tsTypeParameterDeclarations(n.TypeParameters) + " {")
		g.indent++
		for _, member := range n.Body {
			switch field := member.(type) {
			case *ir.Comment:
				g.statement(field)
			case *ir.RecordField:
				g.line(field.Name + ": " + g.tsType(field.Type) + ";")
			}
		}
		g.indent--
		g.line("}")
	case *ir.Enum:
		if enumHasPayload(n) {
			g.payloadEnum(n)
			break
		}
		brand := "__trb" + n.Name + "Brand"
		underlying := "string"
		if n.RawType.Kind != "" {
			underlying = g.tsType(n.RawType)
		}
		g.line("declare const " + brand + ": unique symbol;")
		g.line("export type " + n.Name + " = " + underlying + " & { readonly [" + brand + "]: true };")
		g.line("export const " + n.Name + " = Object.freeze({" + tsTrailingComment(n.TrailingComment))
		g.indent++
		for _, statement := range n.Body {
			switch member := statement.(type) {
			case *ir.Comment:
				g.statement(member)
			case *ir.EnumMember:
				value := strconv.Quote(member.Name)
				if member.RawValue != nil {
					value = g.expr(member.RawValue)
				}
				g.line(member.Name + ": " + value + " as " + n.Name + "," + tsTrailingComment(member.TrailingComment))
			}
		}
		g.enumMethodProperties(n)
		g.indent--
		g.line("});")
	case *ir.TypeAlias:
		parameters := tsTypeParameterDeclarations(n.TypeParameters)
		g.line("export type " + n.Name + parameters + " = " + g.tsType(n.Target) + ";" + tsTrailingComment(n.TrailingComment))
		if len(n.Variants) > 0 {
			g.line("export const " + n.Name + " = " + g.runtimeName(n.Target.Name) + ";")
		}
	case *ir.Module:
		g.line("export namespace " + n.Name + " {")
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("}")
	case *ir.Interface:
		g.line("export interface " + n.Name + tsTypeParameterDeclarations(n.TypeParameters) + " {")
		g.indent++
		for _, method := range n.Methods {
			returnType := g.tsType(method.ReturnType)
			if g.suspension.Methods[method] {
				returnType = "Promise<" + returnType + ">"
			}
			g.line(tsMethodName(method.Name) + "(" + g.methodParameters(method) + "): " + returnType + ";")
		}
		g.indent--
		g.line("}")
	case *ir.Field:
		name := strings.TrimPrefix(n.Name, "@")
		prefix := ""
		if strings.HasPrefix(name, "_") {
			prefix = "private "
			name = strings.TrimPrefix(name, "_")
		}
		name = "__trb_" + name
		if n.ReadOnly {
			prefix += "readonly "
		}
		separator := ": "
		if n.Value == nil {
			separator = "!: "
		}
		text := prefix + name + separator + g.tsType(n.Type)
		if n.Value != nil {
			text += " = " + g.expr(n.Value)
		}
		g.line(text + ";")
	case *ir.Method:
		if n.External {
			return
		}
		if g.inClass > 0 {
			g.method(n)
		} else {
			g.function(n)
		}
	case *ir.Variable:
		variableType := g.tsType(n.Type)
		if lambda, ok := n.Value.(*ir.Lambda); ok && g.suspension.Lambdas[lambda] {
			variableType = g.tsSuspendingFunctionType(n.Type)
		}
		if g.inClass > 0 && g.functionDepth == 0 && n.Constant {
			g.line("static readonly " + n.Name + ": " + variableType + " = " + g.expr(n.Value) + ";")
			break
		}
		keyword := "const"
		if n.Mutable {
			keyword = "let"
		}
		if n.Constant {
			keyword = "const"
		}
		prefix := ""
		if g.functionDepth == 0 && n.Constant {
			prefix = "export "
		}
		g.line(prefix + keyword + " " + n.Name + ": " + variableType + " = " + g.expr(n.Value) + ";")
		if g.functionDepth > 0 && !n.Constant && namedUnusedBinding(n.Name) {
			g.line("void " + n.Name + ";")
		}
	case *ir.Temporary:
		g.line("let " + n.Name + ": " + g.tsType(n.Type) + ";")
	case *ir.Assignment:
		target := g.assignmentTarget(n.Target)
		if n.Operator == "/=" && n.Target.ExprType().Kind == types.Int {
			g.line(target + " = Math.trunc(" + target + " / " + g.expr(n.Value) + ");")
		} else {
			g.line(target + " " + n.Operator + " " + g.expr(n.Value) + ";")
		}
	case *ir.Return:
		if n.Value == nil {
			g.line("return;")
		} else {
			g.line("return " + g.expr(n.Value) + ";")
		}
	case *ir.Break:
		if g.breakTarget != "" {
			g.line("break " + g.breakTarget + ";")
		} else {
			g.line("break;")
		}
	case *ir.Next:
		g.line("continue;")
	case *ir.ExpressionStatement:
		if call, ok := n.Expression.(*ir.Call); ok && call.Block != nil && g.testCallBlock(call) {
			return
		}
		if g.inClass > 0 {
			if call, ok := n.Expression.(*ir.Call); ok {
				if identifier, identifierOK := call.Callee.(*ir.Identifier); identifierOK && (identifier.Name == "belongs_to" || identifier.Name == "has_many" || identifier.Name == "has_one" || identifier.Name == "enum_column") || g.jobsDeclaration(call) {
					return
				}
			}
		}
		g.line(g.expr(n.Expression) + ";")
	case *ir.If:
		g.line("if (" + g.expr(n.Condition) + ") {")
		g.indent++
		g.statements(n.Then)
		g.indent--
		for _, branch := range n.ElseIf {
			g.line("} else if (" + g.expr(branch.Condition) + ") {")
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
		g.line("const " + value + " = " + g.expr(n.Value) + ";" + tsTrailingComment(n.TrailingComment))
		narrowingName, narrowingTemp := g.caseNarrowingCapture(n)
		for index, branch := range n.Branches {
			header := "if ("
			if index > 0 {
				header = "} else if ("
			}
			condition := value + " === " + g.expr(branch.Value)
			for _, alternative := range branch.Alternatives {
				condition += " || " + value + " === " + g.expr(alternative)
			}
			if branch.PayloadEnum {
				condition = value + ".kind === " + strconv.Quote(branch.Member)
			}
			g.line(header + condition + ") {" + tsTrailingComment(branch.TrailingComment))
			g.indent++
			g.caseNarrowings(branch.Narrowings, narrowingName, narrowingTemp)
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				g.line("const " + binding.Name + " = " + value + "." + binding.Field + ";")
				if namedUnusedBinding(binding.Name) {
					g.line("void " + binding.Name + ";")
				}
			}
			g.statements(branch.Body)
			g.indent--
		}
		if n.HasElse {
			g.line("} else {")
			g.indent++
			g.caseNarrowings(n.ElseNarrowings, narrowingName, narrowingTemp)
			g.statements(n.Else)
			g.indent--
		} else {
			g.line("} else {")
			g.indent++
			g.line("throw new Error(\"unreachable exhaustive case\");")
			g.indent--
		}
		if len(n.Branches) > 0 {
			g.line("}")
		}
		g.indent--
		g.line("}")
	case *ir.While:
		g.line("while (" + g.expr(n.Condition) + ") {")
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("}")
	case *ir.Iterate:
		g.iterate(n)
	case *ir.StructuredBlock:
		g.structuredBlock(n)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (g *generator) typeUnionCase(node *ir.Case) {
	g.statements(node.Leading)
	g.temporary++
	value := "__trbCase" + strconv.Itoa(g.temporary)
	g.line("{")
	g.indent++
	g.line("const " + value + " = " + g.expr(node.Value) + ";" + tsTrailingComment(node.TrailingComment))
	for index, branch := range node.Branches {
		header := "if ("
		if index > 0 {
			header = "} else if ("
		}
		g.line(header + tsTypePattern(value, branch.MatchType) + ") {" + tsTrailingComment(branch.TrailingComment))
		g.indent++
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			g.line("const " + binding.Name + " = " + value + ";")
			if namedUnusedBinding(binding.Name) {
				g.line("void " + binding.Name + ";")
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
		g.line("throw new Error(\"unreachable exhaustive case\");")
	}
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}

func (g *generator) caseNarrowingCapture(node *ir.Case) (string, string) {
	name := ""
	for _, branch := range node.Branches {
		if len(branch.Narrowings) > 0 {
			name = branch.Narrowings[0].Name
			break
		}
	}
	if name == "" && len(node.ElseNarrowings) > 0 {
		name = node.ElseNarrowings[0].Name
	}
	if name == "" {
		return "", ""
	}
	g.temporary++
	temporary := "__trbNarrow" + strconv.Itoa(g.temporary)
	g.line("const " + temporary + " = " + name + ";")
	return name, temporary
}

func (g *generator) caseNarrowings(narrowings []ir.CaseBinding, name, temporary string) {
	for _, narrowing := range narrowings {
		if narrowing.Name != name || temporary == "" {
			continue
		}
		g.line("const " + name + " = " + temporary + " as " + g.tsType(narrowing.Type) + ";")
		g.line("void " + name + ";")
	}
}

func tsTypePattern(value string, typ types.Type) string {
	switch typ.Kind {
	case types.Bool:
		return "typeof " + value + ` === "boolean"`
	case types.Int, types.Float:
		return "typeof " + value + ` === "number"`
	case types.String:
		return "typeof " + value + ` === "string"`
	default:
		return "false"
	}
}

func (g *generator) iterate(iteration *ir.Iterate) {
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
		keyBinding := binding(0)
		valueBinding := binding(1)
		key := keyBinding.Name
		loopKey := key
		if keyBinding.Type.Kind == types.Int && key != "_" {
			g.temporary++
			loopKey = "__trbKey" + strconv.Itoa(g.temporary)
		}
		g.line("for (let [" + loopKey + ", " + valueBinding.Name + "] of Object.entries(" + g.expr(iteration.Source) + ")) {")
		g.indent++
		if loopKey != key {
			g.line("let " + key + " = Number(" + loopKey + ");")
		}
		g.line("void " + key + ";")
		g.line("void " + valueBinding.Name + ";")
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		return
	}
	itemBinding := binding(0)
	if iteration.Operation == "each_slice" {
		g.temporary++
		suffix := strconv.Itoa(g.temporary)
		items := "__trbItems" + suffix
		size := "__trbSize" + suffix
		offset := "__trbOffset" + suffix
		g.line("{")
		g.indent++
		g.line("const " + items + " = " + g.iterableExpr(iteration.Source) + ";")
		g.line("const " + size + " = " + g.expr(iteration.SliceSize) + ";")
		g.line("if (" + size + " <= 0) throw new Error(\"each_slice size must be greater than zero\");")
		g.line("for (let " + offset + " = 0; " + offset + " < " + items + ".length; " + offset + " += " + size + ") {")
		g.indent++
		g.line("let " + itemBinding.Name + " = " + items + ".slice(" + offset + ", " + offset + " + " + size + ");")
		g.line("void " + itemBinding.Name + ";")
		if iteration.WithIndex {
			indexBinding := binding(1)
			g.line("let " + indexBinding.Name + " = Math.floor(" + offset + " / " + size + ");")
			g.line("void " + indexBinding.Name + ";")
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		g.indent--
		g.line("}")
		return
	}
	indexBinding := binding(1)
	if iteration.WithIndex {
		g.line("for (let [" + indexBinding.Name + ", " + itemBinding.Name + "] of " + g.iterableExpr(iteration.Source) + ".entries()) {")
	} else {
		g.line("for (let " + itemBinding.Name + " of " + g.iterableExpr(iteration.Source) + ") {")
	}
	g.indent++
	g.line("void " + itemBinding.Name + ";")
	if iteration.WithIndex {
		g.line("void " + indexBinding.Name + ";")
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
	return "((bounds: [number, number, boolean]): Array<number> => { const [start, end, exclusive] = bounds; return Array.from({ length: Math.max(0, end - start + (exclusive ? 0 : 1)) }, (_, index) => start + index); })(" + value + ")"
}

func enumHasPayload(enum *ir.Enum) bool {
	for _, statement := range enum.Body {
		if member, ok := statement.(*ir.EnumMember); ok && len(member.Fields) > 0 {
			return true
		}
	}
	return false
}

func (g *generator) payloadEnum(enum *ir.Enum) {
	typeParameters := tsTypeParameterDeclarations(enum.TypeParameters)
	typeArguments := tsTypeParameterArguments(enum.TypeParameters)
	variants := []string{}
	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		fields := []string{"readonly kind: " + strconv.Quote(member.Name)}
		for _, field := range member.Fields {
			fields = append(fields, "readonly "+field.Name+": "+g.tsType(field.Type))
		}
		variants = append(variants, "{ "+strings.Join(fields, "; ")+" }")
	}
	g.line("export type " + enum.Name + typeParameters + " = " + strings.Join(variants, " | ") + ";")
	g.line("export const " + enum.Name + " = Object.freeze({" + tsTrailingComment(enum.TrailingComment))
	g.indent++
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			if len(member.Fields) == 0 {
				g.line(member.Name + ": { kind: " + strconv.Quote(member.Name) + " } as " + enum.Name + "," + tsTrailingComment(member.TrailingComment))
				continue
			}
			parameters := g.parameters(member.Fields)
			fields := []string{"kind: " + strconv.Quote(member.Name)}
			for _, field := range member.Fields {
				fields = append(fields, field.Name)
			}
			g.line(member.Name + ": " + typeParameters + "(" + parameters + "): " + enum.Name + typeArguments + " => ({ " + strings.Join(fields, ", ") + " })," + tsTrailingComment(member.TrailingComment))
		}
	}
	g.enumMethodProperties(enum)
	g.indent--
	g.line("});")
}

func (g *generator) enumMethodProperties(enum *ir.Enum) {
	for _, statement := range enum.Body {
		method, ok := statement.(*ir.Method)
		if !ok || method.External {
			continue
		}
		parameters := g.methodParameters(method)
		if parameters != "" {
			parameters = ", " + parameters
		}
		generic := tsTypeParameterDeclarations(enum.TypeParameters)
		if generic != "" {
			generic += ""
		}
		prefix := ""
		returnType := g.tsType(method.ReturnType)
		if g.suspension != nil && g.suspension.Methods[method] {
			prefix = "async "
			returnType = "Promise<" + returnType + ">"
		}
		g.line("__trb_" + tsCallableName(method.Name) + ": " + prefix + generic + "(self: " + enum.Name + tsTypeParameterArguments(enum.TypeParameters) + parameters + "): " + returnType + " => {")
		g.indent++
		previous := g.enumReceiver
		g.enumReceiver = "self"
		g.functionDepth++
		previousExecution := g.executionActive
		g.executionActive = g.methodUsesExecutionScope(method)
		g.statements(method.Body)
		g.executionActive = previousExecution
		g.functionDepth--
		g.enumReceiver = previous
		g.indent--
		g.line("},")
	}
}

func tsTypeParameterDeclarations(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	return "<" + strings.Join(parameters, ", ") + ">"
}

func tsTypeParameterArguments(parameters []string) string {
	return tsTypeParameterDeclarations(parameters)
}

func (g *generator) method(method *ir.Method) {
	suspends := g.suspension.Methods[method]
	if method.Name == "initialize" {
		g.line("constructor(" + g.methodParameters(method) + ") {")
	} else {
		prefix := ""
		name := tsMethodName(method.Name)
		if method.Name == "component_did_mount" {
			name = "componentDidMount"
		}
		if method.Class {
			prefix = "static "
		}
		if strings.HasPrefix(method.Name, "_") {
			prefix += "private "
		}
		returnType := g.tsType(method.ReturnType)
		if suspends {
			prefix += "async "
			returnType = "Promise<" + returnType + ">"
		}
		g.line(prefix + name + tsTypeParameterDeclarations(method.TypeParameters) + "(" + g.methodParameters(method) + "): " + returnType + " {")
	}
	g.indent++
	g.functionDepth++
	previousExecution := g.executionActive
	g.executionActive = g.methodUsesExecutionScope(method)
	g.statements(method.Body)
	g.executionActive = previousExecution
	g.functionDepth--
	g.indent--
	g.line("}")
}

func (g *generator) function(method *ir.Method) {
	name := method.Name
	if method.TargetName != "" {
		name = method.TargetName
	}
	name = tsCallableName(name)
	prefix := "export function "
	if name == "main" {
		prefix = "function "
	}
	returnType := g.tsType(method.ReturnType)
	if g.suspension.Methods[method] {
		prefix = strings.Replace(prefix, "function ", "async function ", 1)
		returnType = "Promise<" + returnType + ">"
	}
	parameters := g.methodParameters(method)
	component := isReactComponent(method)
	if component {
		parameters = g.parameters(method.Parameters)
	}
	g.line(prefix + name + tsTypeParameterDeclarations(method.TypeParameters) + "(" + parameters + "): " + returnType + " {")
	g.indent++
	g.functionDepth++
	previousExecution := g.executionActive
	g.executionActive = g.methodUsesExecutionScope(method)
	if component && g.executionActive {
		g.line("const __trbScope: AbortSignal | undefined = undefined;")
	}
	g.statements(method.Body)
	g.executionActive = previousExecution
	g.functionDepth--
	g.indent--
	g.line("}")
}

func isReactComponent(method *ir.Method) bool {
	return method != nil && method.ReturnType.Name == "ReactNode"
}

func (g *generator) parameters(parameters []ir.Parameter) string {
	parts := make([]string, len(parameters))
	for i, parameter := range parameters {
		name := parameter.Name
		if parameter.Rest || parameter.KeywordRest {
			name = "..." + name
		}
		part := name + ": " + g.tsType(parameter.Type)
		if parameter.Default != nil {
			part += " = " + g.expr(parameter.Default)
		}
		parts[i] = part
	}
	return strings.Join(parts, ", ")
}

func (g *generator) methodUsesExecutionScope(method *ir.Method) bool {
	return method != nil && (strings.HasPrefix(method.TargetName, "trb_web_route_") ||
		strings.HasPrefix(method.TargetName, "trb_web_middleware_") ||
		g.execution != nil && g.execution.Methods[method])
}

func (g *generator) methodParameters(method *ir.Method) string {
	parameters := g.parameters(method.Parameters)
	if !g.methodUsesExecutionScope(method) {
		return parameters
	}
	if parameters == "" {
		return "__trbScope: AbortSignal | undefined"
	}
	return "__trbScope: AbortSignal | undefined, " + parameters
}

func (g *generator) executionArguments(call *ir.Call, arguments []string) []string {
	if g.execution == nil || !g.execution.Calls[call] {
		return arguments
	}
	// React invokes components with their props as the first argument. Keep the
	// compiler-owned execution scope inside that public boundary instead of
	// exposing it as part of the component's JavaScript calling convention.
	if g.isReactComponentCall(call) {
		return arguments
	}
	return append([]string{"__trbScope"}, arguments...)
}

func (g *generator) isReactComponentCall(call *ir.Call) bool {
	if call == nil || call.ExprType().Name != "ReactNode" {
		return false
	}
	identifier, ok := call.Callee.(*ir.Identifier)
	if !ok || identifier.Lexical {
		return false
	}
	if g.topFunctions[identifier.Name] {
		return true
	}
	return identifier.Reference != nil && (identifier.Reference.ExportKind == "function" || identifier.Reference.ExportKind == "component")
}

func (g *generator) awaitCall(call *ir.Call, value string) string {
	if g.suspension != nil && g.suspension.Calls[call] {
		return "(await " + value + ")"
	}
	return value
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
	case *ir.Attempt:
		return g.attemptExpression(n)
	case *ir.Lambda:
		parts := make([]string, len(n.Parameters))
		for index, parameter := range n.Parameters {
			parts[index] = parameter.Name + ": " + g.tsType(parameter.Type)
		}
		child := *g
		child.b = strings.Builder{}
		child.sourceRecorder = nil
		child.indent = g.indent + 1
		child.statements(n.Body)
		prefix := ""
		returnType := g.tsType(n.ReturnType)
		if g.suspension.Lambdas[n] {
			prefix = "async "
			returnType = "Promise<" + returnType + ">"
		}
		return prefix + "(" + strings.Join(parts, ", ") + "): " + returnType + " => {\n" + child.b.String() + strings.Repeat("  ", g.indent) + "}"
	case *ir.UnhandledEffect:
		return g.expr(n.Value)
	case *ir.Identifier:
		if strings.HasPrefix(n.Name, "@") {
			return "this.__trb_" + strings.TrimPrefix(strings.TrimPrefix(n.Name, "@"), "_")
		}
		if n.Name == "nil" {
			return "null"
		}
		if n.Name == "self" {
			if g.enumReceiver != "" {
				return g.enumReceiver
			}
			return "this"
		}
		if !n.Lexical && g.inClass > 0 && g.methods[n.Name] {
			return "this." + tsMethodName(n.Name) + ".bind(this)"
		}
		if n.Owner != "" {
			return strings.ReplaceAll(n.Owner, "::", ".") + "." + tsCallableName(n.Name)
		}
		if n.Reference != nil && n.Reference.ExportKind == "function" {
			return tsCallableName(n.Name)
		}
		return n.Name
	case *ir.Literal:
		if n.Kind == "nil" {
			return "null"
		}
		return n.Raw
	case *ir.InterpolatedString:
		var value strings.Builder
		value.WriteByte('`')
		for _, part := range n.Parts {
			if part.Expression != nil {
				value.WriteString("${")
				value.WriteString(g.expr(part.Expression))
				value.WriteByte('}')
			} else {
				value.WriteString(strings.ReplaceAll(part.Text, "`", "\\`"))
			}
		}
		value.WriteByte('`')
		return value.String()
	case *ir.Symbol:
		return strconv.Quote(n.Name)
	case *ir.Array:
		parts := make([]string, len(n.Elements))
		for i, element := range n.Elements {
			parts[i] = g.expr(element)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *ir.Hash:
		parts := make([]string, len(n.Entries))
		for i, entry := range n.Entries {
			parts[i] = "[" + g.expr(entry.Key) + "]: " + g.expr(entry.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *ir.JSXElement:
		return g.jsxElement(n)
	case *ir.Unary:
		op := n.Operator
		if op == "not" || op == "!" {
			return "!(" + g.expr(n.Operand) + ")"
		}
		return op + g.unaryOperand(n.Operand)
	case *ir.Conversion:
		switch n.Kind {
		case ir.RangeToIterableConversion:
			return g.iterableExpr(n.Value)
		case ir.ResultFunctionToPromiseRejectionConversion:
			return g.resultFunctionToPromiseRejection(n)
		case ir.PureFunctionToFallibleConversion:
			return g.pureFunctionToFallible(n)
		case ir.IntegerToFloatConversion:
			return "Number(" + g.expr(n.Value) + ")"
		case ir.UnionIntegerToFloatConversion:
			return "((value: " + g.tsType(n.Value.ExprType()) + "): " + g.tsType(n.ExprType()) + " => typeof value === \"number\" ? Number(value) : value)(" + g.expr(n.Value) + ")"
		case ir.NonNullableToNullableConversion:
			if n.Value.ExprType().Kind == types.Int && n.ExprType().Kind == types.Float {
				return "Number(" + g.expr(n.Value) + ")"
			}
			return g.expr(n.Value)
		case ir.NullableToNonNullableConversion:
			return g.expr(n.Value)
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
		if op == "**" && n.ExprType().Kind == types.Int {
			return "((base: number, exponent: number): number => { if (exponent < 0) { throw new RangeError(\"negative Integer exponent\"); } return Math.trunc(base ** exponent); })(" + left + ", " + right + ")"
		}
		if op == "/" && n.ExprType().Kind == types.Int {
			return "Math.trunc(" + left + " / " + right + ")"
		}
		return left + " " + op + " " + right
	case *ir.Range:
		exclusive := "false"
		if n.Exclusive {
			exclusive = "true"
		}
		return "([" + g.expr(n.Start) + ", " + g.expr(n.End) + ", " + exclusive + "] as [number, number, boolean])"
	case *ir.Transform:
		return g.transform(n)
	case *ir.Member:
		if n.Reference != nil && n.Reference.Intrinsic == "trb.orm.column" {
			return "__trbOrm.column(" + g.expr(n.Receiver) + ", " + strconv.Quote(n.Name) + ")"
		}
		receiver := g.expr(n.Receiver)
		op := "."
		if n.Safe {
			op = "?."
		}
		if n.ClassField {
			return receiver + op + "__trb_" + n.Name
		}
		return receiver + op + tsMethodName(n.Name)
	case *ir.Call:
		parts := make([]string, len(n.Arguments))
		for i, argument := range n.Arguments {
			value := g.expr(argument.Value)
			if argument.Splat != "" {
				value = "..." + value
			}
			parts[i] = value
		}
		args := strings.Join(parts, ", ")
		if reference := expressionReference(n.Callee); reference != nil && reference.Intrinsic != "" {
			if reference.ReceiverMethod {
				if member, ok := receiverMember(n.Callee); ok {
					parts = append([]string{g.expr(member.Receiver)}, parts...)
				}
			}
			generated := g.intrinsic(reference.Intrinsic, n, parts)
			if isSuspendingORM(reference.Intrinsic, n.Fails) {
				return "(await " + generated + ")"
			}
			return g.awaitCall(n, generated)
		}
		parts = g.executionArguments(n, parts)
		args = strings.Join(parts, ", ")
		if member, ok := n.Callee.(*ir.Member); ok && member.Name == "new" {
			if application, generic := member.Receiver.(*ir.TypeApply); generic && application.Kind == "record" {
				if identifier, named := application.Receiver.(*ir.Identifier); named {
					return g.awaitCall(n, g.recordLiteralApplied(identifier, application.Arguments, n.Arguments))
				}
			}
			if identifier, ok := member.Receiver.(*ir.Identifier); ok && (g.records[identifier.Name] || identifier.Reference != nil && identifier.Reference.ExportKind == "record") {
				return g.awaitCall(n, g.recordLiteral(identifier, n.Arguments))
			}
			return g.awaitCall(n, "new "+g.expr(member.Receiver)+"("+args+")")
		}
		if identifier, ok := n.Callee.(*ir.Identifier); ok {
			if !identifier.Lexical && g.inClass > 0 && g.methods[identifier.Name] {
				return g.awaitCall(n, "this."+tsMethodName(identifier.Name)+"("+args+")")
			}
			if g.topFunctions[identifier.Name] {
				name := identifier.Name
				if target := g.topTargets[identifier.Name]; target != "" {
					name = target
				}
				return g.awaitCall(n, tsCallableName(name)+"("+args+")")
			}
		}
		return g.awaitCall(n, g.expr(n.Callee)+"("+args+")")
	case *ir.EnumCall:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument.Value)
		}
		switch n.Method {
		case "raw_value":
			return "(" + g.expr(n.Receiver) + " as unknown as " + g.tsType(n.RawType) + ")"
		case "from_raw":
			return g.rawEnumFromValue(n, parts[0])
		default:
			owner := g.runtimeName(n.EnumName)
			parts = append([]string{g.expr(n.Receiver)}, parts...)
			if g.execution != nil && g.execution.EnumCalls[n] {
				parts = append(parts[:1], append([]string{"__trbScope"}, parts[1:]...)...)
			}
			result := owner + ".__trb_" + tsCallableName(n.Method) + "(" + strings.Join(parts, ", ") + ")"
			if g.suspension != nil && g.suspension.Expressions[n] {
				return "(await " + result + ")"
			}
			return result
		}
	case *ir.EnumConstruct:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument)
		}
		name := n.EnumName + "." + n.Member
		if len(n.TypeArguments) > 0 {
			arguments := make([]string, len(n.TypeArguments))
			for index, argument := range n.TypeArguments {
				arguments[index] = g.tsType(argument)
			}
			name += "<" + strings.Join(arguments, ", ") + ">"
		}
		return name + "(" + strings.Join(parts, ", ") + ")"
	case *ir.TypeApply:
		arguments := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			arguments[index] = g.tsType(argument)
		}
		name := g.expr(n.Receiver)
		if identifier, ok := n.Receiver.(*ir.Identifier); ok && g.topFunctions[identifier.Name] {
			name = identifier.Name
			if target := g.topTargets[identifier.Name]; target != "" {
				name = target
			}
			name = tsCallableName(name)
		}
		return name + "<" + strings.Join(arguments, ", ") + ">"
	case *ir.Index:
		if n.Receiver.ExprType().Kind == types.Hash && len(n.Receiver.ExprType().Args) == 2 {
			hashType := n.Receiver.ExprType()
			return "((values: " + g.tsType(hashType) + ", key: " + g.tsType(hashType.Args[0]) + "): " + g.tsType(hashType.Args[1]) + " => { if (!Object.prototype.hasOwnProperty.call(values, key)) { throw new Error(\"Hash key is missing\"); } return values[key]; })(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		if n.Receiver.ExprType().Kind == types.String {
			return "((value: string, index: number): string => { const characters = Array.from(value); if (index < 0 || index >= characters.length) throw new RangeError(\"String index is out of bounds\"); return characters[index]!; })(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		if n.Receiver.ExprType().Kind == types.Array {
			return "((values: " + g.tsType(n.Receiver.ExprType()) + ", index: number): " + g.tsType(n.ExprType()) + " => { if (index < 0 || index >= values.length) throw new RangeError(\"Array index is out of bounds\"); return values[index]!; })(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	default:
		return ""
	}
}

func (g *generator) pureFunctionToFallible(conversion *ir.Conversion) string {
	parameters, success, ok := types.FunctionSignature(conversion.ExprType())
	if !ok {
		return g.expr(conversion.Value)
	}
	failure := types.FunctionFailure(conversion.ExprType())
	parts := make([]string, len(parameters))
	arguments := make([]string, len(parameters))
	for index, parameter := range parameters {
		name := "__trbArg" + strconv.Itoa(index)
		parts[index] = name + ": " + g.tsType(parameter)
		arguments[index] = name
	}
	internalSuccess := success
	call := "__trbValue(" + strings.Join(arguments, ", ") + ")"
	value := call
	prefix := ""
	if success.Kind == types.Void {
		internalSuccess = types.FromName("Unit")
		prefix = call + "; "
		value = "({} satisfies " + g.tsType(internalSuccess) + ")"
	}
	asyncPrefix := ""
	returnType := g.tsType(types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{internalSuccess, failure}})
	if lambda, literal := conversion.Value.(*ir.Lambda); literal && g.suspension != nil && g.suspension.Lambdas[lambda] {
		asyncPrefix = "async "
		returnType = "Promise<" + returnType + ">"
		if success.Kind != types.Void {
			value = "await " + value
		} else {
			prefix = "await " + call + "; "
		}
	}
	result := g.runtimeName("Result")
	wrapped := asyncPrefix + "(" + strings.Join(parts, ", ") + "): " + returnType + " => { " + prefix + "return " +
		result + ".Ok<" + g.tsType(internalSuccess) + ", " + g.tsType(failure) + ">(" + value + "); }"
	return "((__trbValue: " + g.tsType(conversion.Value.ExprType()) + "): " + g.tsType(conversion.ExprType()) +
		" => " + wrapped + ")(" + g.expr(conversion.Value) + ")"
}

func (g *generator) resultFunctionToPromiseRejection(conversion *ir.Conversion) string {
	parameters, success, ok := types.FunctionSignature(conversion.ExprType())
	if !ok {
		return g.expr(conversion.Value)
	}
	parts := make([]string, len(parameters))
	arguments := make([]string, len(parameters))
	for index, parameter := range parameters {
		name := "__trbArg" + strconv.Itoa(index)
		parts[index] = name + ": " + g.tsType(parameter)
		arguments[index] = name
	}
	returned := g.tsType(success)
	returnValue := "return __trbResult.value;"
	if success.Kind == types.Void {
		returnValue = "return;"
	}
	return "((__trbCallback: " + g.tsType(conversion.Value.ExprType()) + ") => async (" + strings.Join(parts, ", ") +
		"): Promise<" + returned + "> => { const __trbResult = await __trbCallback(" + strings.Join(arguments, ", ") +
		"); if (__trbResult.kind === \"Err\") { throw __trbResult.error; } " + returnValue + " })(" + g.expr(conversion.Value) + ")"
}

func (g *generator) rawEnumFromValue(call *ir.EnumCall, argument string) string {
	valueType := g.tsType(types.FromName(call.EnumName))
	errorType := g.tsType(types.FromName("EnumValueError"))
	resultType := g.tsType(call.ExprType())
	result := g.runtimeName("Result")
	owner := g.runtimeName(call.EnumName)
	parts := []string{"((): " + resultType + " => { const value = " + argument + "; switch (value) {"}
	for _, item := range call.RawValues {
		parts = append(parts, "case "+item.Raw+": return "+result+".Ok<"+valueType+", "+errorType+">("+owner+"."+item.Member+");")
	}
	message := strconv.Quote("unknown raw value for " + call.EnumName)
	parts = append(parts, "} return "+result+".Err<"+valueType+", "+errorType+">({ value, message: "+message+" }); })()")
	return strings.Join(parts, " ")
}

func (g *generator) ifExpression(node *ir.If) string {
	child := &generator{
		inClass:         g.inClass,
		functionDepth:   g.functionDepth,
		methods:         g.methods,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		records:         g.records,
		typeAliases:     g.typeAliases,
		temporary:       g.temporary,
		suspension:      g.suspension,
		execution:       g.execution,
		executionActive: g.executionActive,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		enumReceiver:    g.enumReceiver,
	}
	suspends := child.suspension != nil && child.suspension.Expressions[node]
	if suspends {
		child.line("(async (): Promise<" + child.tsType(node.ExprType()) + "> => {")
	} else {
		child.line("((): " + child.tsType(node.ExprType()) + " => {")
	}
	child.indent++
	child.line("if (" + child.expr(node.Condition) + ") {" + tsTrailingComment(node.TrailingComment))
	child.indent++
	child.statements(node.Then)
	if !node.ThenDiverges {
		child.line("return " + child.expr(node.ThenResult) + ";")
	}
	child.indent--
	for _, branch := range node.ElseIf {
		child.line("} else if (" + child.expr(branch.Condition) + ") {")
		child.indent++
		child.statements(branch.Body)
		if !branch.Diverges {
			child.line("return " + child.expr(branch.Result) + ";")
		}
		child.indent--
	}
	child.line("} else {")
	child.indent++
	child.statements(node.Else)
	if !node.ElseDiverges {
		child.line("return " + child.expr(node.ElseResult) + ";")
	}
	child.indent--
	child.line("}")
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	generated := strings.TrimSpace(child.b.String())
	if suspends {
		return "(await " + generated + ")"
	}
	return generated
}

func (g *generator) attemptExpression(node *ir.Attempt) string {
	child := &generator{
		inClass:         g.inClass,
		functionDepth:   g.functionDepth,
		methods:         g.methods,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		topTargets:      g.topTargets,
		records:         g.records,
		typeAliases:     g.typeAliases,
		temporary:       g.temporary,
		suspension:      g.suspension,
		execution:       g.execution,
		executionActive: g.executionActive,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		enumReceiver:    g.enumReceiver,
	}
	suspends := child.suspension != nil && child.suspension.Expressions[node]
	if suspends {
		child.line("(async (): Promise<" + child.tsType(node.ExprType()) + "> => {")
	} else {
		child.line("((): " + child.tsType(node.ExprType()) + " => {")
	}
	child.indent++
	child.statements(node.Body)
	result := node.Value
	if result == nil {
		result = node.BodyResult
	}
	child.line("return " + child.expr(result) + ";")
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	value := strings.TrimSpace(child.b.String())
	if suspends {
		return "(await " + value + ")"
	}
	return value
}

func (g *generator) caseExpression(node *ir.Case) string {
	child := &generator{
		inClass:         g.inClass,
		functionDepth:   g.functionDepth,
		methods:         g.methods,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		records:         g.records,
		typeAliases:     g.typeAliases,
		temporary:       g.temporary,
		suspension:      g.suspension,
		execution:       g.execution,
		executionActive: g.executionActive,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		enumReceiver:    g.enumReceiver,
	}
	suspends := child.suspension != nil && child.suspension.Expressions[node]
	if suspends {
		child.line("(async (): Promise<" + child.tsType(node.ExprType()) + "> => {")
	} else {
		child.line("((): " + child.tsType(node.ExprType()) + " => {")
	}
	child.indent++
	child.statements(node.Leading)
	child.temporary++
	value := "__trbCase" + strconv.Itoa(child.temporary)
	child.line("const " + value + " = " + child.expr(node.Value) + ";" + tsTrailingComment(node.TrailingComment))
	narrowingName, narrowingTemp := child.caseNarrowingCapture(node)
	for index, branch := range node.Branches {
		header := "if ("
		if index > 0 {
			header = "} else if ("
		}
		condition := value + " === " + child.expr(branch.Value)
		for _, alternative := range branch.Alternatives {
			condition += " || " + value + " === " + child.expr(alternative)
		}
		if branch.PayloadEnum {
			condition = value + ".kind === " + strconv.Quote(branch.Member)
		} else if branch.TypePattern {
			condition = tsTypePattern(value, branch.MatchType)
		}
		child.line(header + condition + ") {" + tsTrailingComment(branch.TrailingComment))
		child.indent++
		child.caseNarrowings(branch.Narrowings, narrowingName, narrowingTemp)
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			bindingValue := value
			if branch.PayloadEnum {
				bindingValue += "." + binding.Field
			}
			child.line("const " + binding.Name + " = " + bindingValue + ";")
			if namedUnusedBinding(binding.Name) {
				child.line("void " + binding.Name + ";")
			}
		}
		child.statements(branch.Body)
		if !branch.Diverges {
			child.line("return " + child.expr(branch.Result) + ";")
		}
		child.indent--
	}
	child.line("} else {")
	child.indent++
	if node.HasElse {
		child.caseNarrowings(node.ElseNarrowings, narrowingName, narrowingTemp)
		child.statements(node.Else)
		if !node.ElseDiverges {
			child.line("return " + child.expr(node.ElseResult) + ";")
		}
	} else {
		child.line("throw new Error(\"unreachable exhaustive case\");")
	}
	child.indent--
	child.line("}")
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	generated := strings.TrimSpace(child.b.String())
	if suspends {
		return "(await " + generated + ")"
	}
	return generated
}

func tsCallableName(name string) string {
	if kind, encoded, ok := naming.CallableSuffix(name); ok {
		return "$trb$" + kind + "$" + encoded
	}
	return name
}

func tsMethodName(name string) string {
	if _, _, ok := naming.CallableSuffix(name); ok {
		return tsCallableName(name)
	}
	return strings.TrimPrefix(name, "_")
}

func namedUnusedBinding(name string) bool {
	return name != "_" && strings.HasPrefix(name, "_")
}

func (g *generator) transform(transform *ir.Transform) string {
	if g.suspension != nil && g.suspension.Expressions[transform] {
		return g.suspendingTransform(transform)
	}
	source := g.iterableExpr(transform.Source)
	result := g.transformResult(transform)
	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		comparison := tsPortableSortComparison("left.key", "right.key", transform.Result.ExprType(), transform.Operation == "sort_by_descending")
		return source + ".map((" + transform.Item + ", index) => ({ value: " + transform.Item + ", key: " + result + ", index })).sort((left, right) => { const compared = " + comparison + "; return compared === 0 ? left.index - right.index : compared; }).map((entry) => entry.value)"
	case "map", "select", "any?", "all?", "none?", "find", "find_index":
		parameters := transform.Item
		if transform.WithIndex {
			parameters += ", " + transform.Index
		}
		operation := transform.Operation
		if operation == "select" {
			operation = "filter"
		} else if operation == "any?" || operation == "none?" {
			operation = "some"
		} else if operation == "all?" {
			operation = "every"
		} else if operation == "find_index" {
			operation = "findIndex"
		}
		value := source + "." + operation + "((" + parameters + ") => " + result + ")"
		if transform.Operation == "none?" {
			return "!(" + value + ")"
		}
		if transform.Operation == "find" {
			return "(" + value + " ?? null)"
		}
		if transform.Operation == "find_index" {
			return "((index: number): number | null => index < 0 ? null : index)(" + value + ")"
		}
		return value
	case "reduce":
		return source + ".reduce((" + transform.Accumulator + ", " + transform.Item + ") => " + result + ", " + g.expr(transform.Initial) + ")"
	default:
		return "undefined"
	}
}

func (g *generator) transformResult(transform *ir.Transform) string {
	if len(transform.Body) == 0 {
		return g.expr(transform.Result)
	}
	child := *g
	child.b = strings.Builder{}
	child.sourceRecorder = nil
	child.indent = 0
	child.line("((): " + child.tsType(transform.Result.ExprType()) + " => {")
	child.indent++
	child.statements(transform.Body)
	child.line("return " + child.expr(transform.Result) + ";")
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) suspendingTransform(transform *ir.Transform) string {
	child := *g
	child.b = strings.Builder{}
	child.sourceRecorder = nil
	child.indent = 0
	child.temporary++
	suffix := strconv.Itoa(child.temporary)
	items := "__trbItems" + suffix
	result := "__trbResult" + suffix
	index := "__trbIndex" + suffix
	item := transform.Item
	if item == "" {
		item = "__trbItem" + suffix
	}
	child.line("(async (): Promise<" + child.tsType(transform.ExprType()) + "> => {")
	child.indent++
	child.line("const " + items + " = " + child.iterableExpr(transform.Source) + ";")

	emitBindings := func(includeIndex bool) {
		child.line("let " + item + " = " + items + "[" + index + "]!;")
		if includeIndex {
			name := transform.Index
			if name == "" {
				name = "__trbSourceIndex" + suffix
			}
			child.line("let " + name + " = " + index + ";")
		}
	}
	emitValue := func() string {
		child.statements(transform.Body)
		return child.expr(transform.Result)
	}

	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		keyType := child.tsType(transform.Result.ExprType())
		itemType := child.tsType(transform.ItemType)
		decorated := "__trbDecorated" + suffix
		child.line("const " + decorated + ": Array<{ value: " + itemType + "; key: " + keyType + "; index: number }> = [];")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		value := emitValue()
		child.line(decorated + ".push({ value: " + item + ", key: " + value + ", index: " + index + " });")
		child.indent--
		child.line("}")
		comparison := tsPortableSortComparison("left.key", "right.key", transform.Result.ExprType(), transform.Operation == "sort_by_descending")
		child.line(decorated + ".sort((left, right) => { const compared = " + comparison + "; return compared === 0 ? left.index - right.index : compared; });")
		child.line("return " + decorated + ".map((entry) => entry.value);")
	case "map", "select":
		child.line("const " + result + ": " + child.tsType(transform.ExprType()) + " = [];")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(transform.WithIndex)
		value := emitValue()
		if transform.Operation == "map" {
			child.line(result + ".push(" + value + ");")
		} else {
			child.line("if (" + value + ") " + result + ".push(" + item + ");")
		}
		child.indent--
		child.line("}")
		child.line("return " + result + ";")
	case "any?", "all?", "none?", "find", "find_index":
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		value := emitValue()
		switch transform.Operation {
		case "any?":
			child.line("if (" + value + ") return true;")
		case "all?":
			child.line("if (!(" + value + ")) return false;")
		case "none?":
			child.line("if (" + value + ") return false;")
		case "find":
			child.line("if (" + value + ") return " + item + ";")
		case "find_index":
			child.line("if (" + value + ") return " + index + ";")
		}
		child.indent--
		child.line("}")
		switch transform.Operation {
		case "any?":
			child.line("return false;")
		case "all?", "none?":
			child.line("return true;")
		default:
			child.line("return null;")
		}
	case "reduce":
		child.line("let " + result + " = " + child.expr(transform.Initial) + ";")
		child.line("for (let " + index + " = 0; " + index + " < " + items + ".length; " + index + " += 1) {")
		child.indent++
		emitBindings(false)
		accumulator := transform.Accumulator
		if accumulator == "" {
			accumulator = "__trbAccumulator" + suffix
		}
		child.line("let " + accumulator + " = " + result + ";")
		value := emitValue()
		child.line(result + " = " + value + ";")
		child.indent--
		child.line("}")
		child.line("return " + result + ";")
	default:
		child.line("throw new Error(\"unsupported TypeRB collection transformation\");")
	}
	child.indent--
	child.line("})()")
	g.temporary = child.temporary
	return "(await " + strings.TrimSpace(child.b.String()) + ")"
}

func tsPortableSortComparison(left, right string, typ types.Type, descending bool) string {
	if base, literal := types.LiteralBase(typ); literal {
		typ = base
	}
	if typ.Kind == types.Float {
		less, greater := "<", ">"
		if descending {
			less, greater = ">", "<"
		}
		return "(Number.isNaN(" + left + ") ? (Number.isNaN(" + right + ") ? 0 : 1) : (Number.isNaN(" + right + ") ? -1 : (" + left + " " + less + " " + right + " ? -1 : (" + left + " " + greater + " " + right + " ? 1 : 0))))"
	}
	if typ.Kind == types.String {
		comparison := "((a: string, b: string): number => { const leftCodepoints = Array.from(a, (value) => value.codePointAt(0)!); const rightCodepoints = Array.from(b, (value) => value.codePointAt(0)!); const length = Math.min(leftCodepoints.length, rightCodepoints.length); for (let index = 0; index < length; index += 1) { if (leftCodepoints[index]! < rightCodepoints[index]!) return -1; if (leftCodepoints[index]! > rightCodepoints[index]!) return 1; } return leftCodepoints.length < rightCodepoints.length ? -1 : (leftCodepoints.length > rightCodepoints.length ? 1 : 0); })(" + left + ", " + right + ")"
		if descending {
			return "-(" + comparison + ")"
		}
		return comparison
	}
	less, greater := "<", ">"
	if descending {
		less, greater = ">", "<"
	}
	return "(" + left + " " + less + " " + right + " ? -1 : (" + left + " " + greater + " " + right + " ? 1 : 0))"
}

func (g *generator) assignmentTarget(expression ir.Expression) string {
	if index, ok := expression.(*ir.Index); ok {
		return g.expr(index.Receiver) + "[" + g.expr(index.Index) + "]"
	}
	return g.expr(expression)
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

func (g *generator) recordLiteral(record *ir.Identifier, arguments []ir.CallArgument) string {
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, argument.Name+": "+g.expr(argument.Value))
	}
	return "({" + strings.Join(fields, ", ") + "} satisfies " + record.Name + ")"
}

func (g *generator) recordLiteralApplied(record *ir.Identifier, typeArguments []types.Type, arguments []ir.CallArgument) string {
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, argument.Name+": "+g.expr(argument.Value))
	}
	items := make([]string, len(typeArguments))
	for index, argument := range typeArguments {
		items[index] = g.tsType(argument)
	}
	name := g.runtimeName(record.Name) + "<" + strings.Join(items, ", ") + ">"
	return "({" + strings.Join(fields, ", ") + "} satisfies " + name + ")"
}

func portableFloatInteger(value, operation string) string {
	return "((): number => { const value = " + value + "; if (!Number.isFinite(value)) { throw new RangeError(\"Float cannot be converted to Integer\"); } const integer = " + operation + "; if (!Number.isSafeInteger(integer)) { throw new RangeError(\"Integer is outside the portable range\"); } return integer; })()"
}

func portableFloatString(value string) string {
	return "((): string => { const value = " + value + "; if (Number.isNaN(value)) return \"NaN\"; if (value === Infinity) return \"Infinity\"; if (value === -Infinity) return \"-Infinity\"; if (value === 0) return \"0.0\"; const raw = String(value); if (!/[eE]/.test(raw)) return raw.includes(\".\") ? raw : raw + \".0\"; const [mantissa, exponentText] = raw.toLowerCase().split(\"e\"); const negative = mantissa!.startsWith(\"-\"); const unsigned = negative ? mantissa!.slice(1) : mantissa!; const [whole, fraction = \"\"] = unsigned.split(\".\"); const digits = whole! + fraction; const decimal = whole!.length + Number(exponentText); let text: string; if (decimal <= 0) text = \"0.\" + \"0\".repeat(-decimal) + digits; else if (decimal >= digits.length) text = digits + \"0\".repeat(decimal - digits.length) + \".0\"; else text = digits.slice(0, decimal) + \".\" + digits.slice(decimal); text = text.replace(/(\\.\\d*?)0+$/, \"$1\").replace(/\\.$/, \".0\"); return negative ? \"-\" + text : text; })()"
}

func tsJSONParse(call *ir.Call, argument string, comments bool) string {
	resultType := tsType(call.ExprType())
	strip := ""
	if comments {
		strip = `const stripComments = (input: string): string => { const result = input.split(""); let inString = false; let escaped = false; for (let index = 0; index < result.length; index += 1) { const character = result[index]!; if (inString) { if (escaped) { escaped = false; continue; } if (character === "\\") { escaped = true; } else if (character === "\"") { inString = false; } continue; } if (character === "\"") { inString = true; continue; } if (character !== "/" || index + 1 >= result.length) { continue; } if (result[index + 1] === "/") { result[index] = " "; result[index + 1] = " "; index += 2; while (index < result.length && result[index] !== "\n") { if (result[index] !== "\r") { result[index] = " "; } index += 1; } index -= 1; } else if (result[index + 1] === "*") { result[index] = " "; result[index + 1] = " "; index += 2; while (index < result.length) { if (index + 1 < result.length && result[index] === "*" && result[index + 1] === "/") { result[index] = " "; result[index + 1] = " "; index += 1; break; } if (result[index] !== "\n" && result[index] !== "\r") { result[index] = " "; } index += 1; } } } return result.join(""); }; __trbSource = stripComments(__trbSource); `
	}
	return "((): " + resultType + " => { let __trbSource = " + argument + "; " + strip + "const syntaxError = (error: unknown): JsonError => { const message = error instanceof Error ? error.message : String(error); const lineMatch = message.match(/line (\\d+)/i); const columnMatch = message.match(/column (\\d+)/i); let line: number | null = lineMatch === null ? null : Number.parseInt(lineMatch[1]!, 10); let column: number | null = columnMatch === null ? null : Number.parseInt(columnMatch[1]!, 10); if (line === null || column === null) { const positionMatch = message.match(/position (\\d+)/i); if (positionMatch !== null) { const position = Number.parseInt(positionMatch[1]!, 10); const prefix = __trbSource.slice(0, position); const lines = prefix.split(\"\\n\"); line = lines.length; column = Array.from(lines[lines.length - 1]!).length + 1; } } return { kind: JsonErrorKind.Syntax, message, path: \"\", line, column }; }; let raw: unknown; try { raw = JSON.parse(__trbSource); } catch (error) { return Result.Err<JsonValue, JsonError>(syntaxError(error)); } const decodeError = (path: string, message: string): JsonError => ({ kind: JsonErrorKind.Decode, message, path, line: null, column: null }); const failure = (error: JsonError): never => { throw { __trbJSONError: true, error }; }; const convert = (value: unknown, path: string): JsonValue => { if (value === null) { return JsonValue.Null; } if (typeof value === \"boolean\") { return JsonValue.Boolean(value); } if (typeof value === \"number\") { if (!Number.isFinite(value)) { return failure(decodeError(path, \"JSON number is not finite\")); } if (Number.isInteger(value)) { if (!Number.isSafeInteger(value)) { return failure(decodeError(path, \"JSON integer is outside the portable range\")); } return JsonValue.Integer(value); } return JsonValue.Float(value); } if (typeof value === \"string\") { return JsonValue.String(value); } if (Array.isArray(value)) { return JsonValue.Array(value.map((item, index) => convert(item, path + \"/\" + String(index)))); } if (typeof value === \"object\") { const fields: Record<string, JsonValue> = {}; for (const [key, item] of Object.entries(value)) { const escaped = key.replaceAll(\"~\", \"~0\").replaceAll(\"/\", \"~1\"); fields[key] = convert(item, path + \"/\" + escaped); } return JsonValue.Object(fields); } return failure(decodeError(path, \"unsupported JSON value\")); }; try { return Result.Ok<JsonValue, JsonError>(convert(raw, \"\")); } catch (error) { if (typeof error === \"object\" && error !== null && (error as any).__trbJSONError === true) { return Result.Err<JsonValue, JsonError>((error as any).error as JsonError); } return Result.Err<JsonValue, JsonError>(syntaxError(error)); } })()"
}

func tsJSONStringify(call *ir.Call, argument string) string {
	resultType := tsType(call.ExprType())
	return "((): " + resultType + " => { const encodeError = (path: string, message: string): JsonError => ({ kind: JsonErrorKind.Encode, message, path, line: null, column: null }); const failure = (error: JsonError): never => { throw { __trbJSONError: true, error }; }; const convert = (value: JsonValue, path: string): unknown => { switch (value.kind) { case \"Null\": return null; case \"Boolean\": return value.value; case \"Integer\": if (!Number.isSafeInteger(value.value)) { return failure(encodeError(path, \"JSON integer is outside the portable range\")); } return value.value; case \"Float\": if (!Number.isFinite(value.value)) { return failure(encodeError(path, \"JSON Float must be finite\")); } return value.value; case \"String\": return value.value; case \"Array\": return value.value.map((item, index) => convert(item, path + \"/\" + String(index))); case \"Object\": { const fields: Record<string, unknown> = {}; for (const [key, item] of Object.entries(value.value)) { const escaped = key.replaceAll(\"~\", \"~0\").replaceAll(\"/\", \"~1\"); fields[key] = convert(item, path + \"/\" + escaped); } return fields; } } }; try { return Result.Ok<string, JsonError>(JSON.stringify(convert(" + argument + ", \"\"))); } catch (error) { if (typeof error === \"object\" && error !== null && (error as any).__trbJSONError === true) { return Result.Err<string, JsonError>((error as any).error as JsonError); } const message = error instanceof Error ? error.message : String(error); return Result.Err<string, JsonError>(encodeError(\"\", message)); } })()"
}

func (g *generator) tsJSONDecode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "undefined"
	}
	jsonAlias := tsJSONRuntimeAlias(call)
	builder := &tsJSONCodecBuilder{jsonAlias: jsonAlias}
	decoder := builder.decoder(call.Codec)
	resultType := tsType(call.ExprType())
	valueType := tsCodecType(call.Codec)
	errorType := tsJSONQualified(jsonAlias, "JsonError")
	jsonErrorKind := tsJSONQualified(jsonAlias, "JsonErrorKind.Decode")
	parse := tsJSONQualified(jsonAlias, "parse")
	return "((): " + resultType + " => { const codecError = (path: string, message: string): " + errorType + " => ({ kind: " + jsonErrorKind + ", message, path, line: null, column: null }); const fail = (path: string, message: string): never => { throw { __trbJSONCodecError: true, error: codecError(path, message) }; }; " + builder.source.String() + " const parsed = " + parse + "(" + argument + "); if (parsed.kind === \"Err\") { return Result.Err<" + valueType + ", " + errorType + ">(parsed.error); } try { return Result.Ok<" + valueType + ", " + errorType + ">(" + decoder + "(parsed.value, \"\")); } catch (error) { if (typeof error === \"object\" && error !== null && (error as any).__trbJSONCodecError === true) { return Result.Err<" + valueType + ", " + errorType + ">((error as any).error as " + errorType + "); } throw error; } })()"
}

func (g *generator) tsJSONEncode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "undefined"
	}
	jsonAlias := tsJSONRuntimeAlias(call)
	builder := &tsJSONCodecBuilder{jsonAlias: jsonAlias}
	encoder := builder.encoder(call.Codec)
	return "((): " + tsType(call.ExprType()) + " => { " + builder.source.String() + " return " + tsJSONQualified(jsonAlias, "stringify") + "(" + encoder + "(" + argument + ")); })()"
}

func tsJSONRuntimeAlias(call *ir.Call) string {
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Alias == "$named" {
		return ""
	}
	if reference != nil && reference.Alias != "" {
		return reference.Alias
	}
	return "__trb_json"
}

func tsCodecType(schema *ir.CodecSchema) string {
	if schema == nil {
		return "unknown"
	}
	return tsType(schema.Type)
}

type tsJSONCodecBuilder struct {
	jsonAlias string
	source    strings.Builder
	next      int
}

func (b *tsJSONCodecBuilder) name(prefix string) string {
	b.next++
	return "__trbJSON" + prefix + strconv.Itoa(b.next)
}

func (b *tsJSONCodecBuilder) decoder(schema *ir.CodecSchema) string {
	name := b.name("Decode")
	valueType := tsCodecType(schema)
	jsonValue := tsJSONQualified(b.jsonAlias, "JsonValue")
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.decoder(&nonnull)
		b.source.WriteString("const " + name + " = (value: " + jsonValue + ", path: string): " + valueType + " => { if (value.kind === \"Null\") { return null; } return " + child + "(value, path); }; ")
		return name
	}
	expected := func(kind string) string { return "return fail(path, " + strconv.Quote("expected "+kind) + ")" }
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "if (value.kind !== \"Boolean\") { " + expected("Boolean") + "; } return value.value;"
	case "integer":
		body = "if (value.kind !== \"Integer\") { " + expected("Integer") + "; } return value.value;"
	case "float":
		body = "if (value.kind === \"Integer\") { return value.value; } if (value.kind !== \"Float\") { " + expected("Float") + "; } return value.value;"
	case "string":
		body = "if (value.kind !== \"String\") { " + expected("String") + "; } return value.value;"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		owner := schema.Type.Name
		if schema.Reference != nil && schema.Reference.Alias != "" {
			owner = schema.Reference.Alias + "." + owner
		}
		method := "try_parse"
		if schema.Kind == "time_zone" {
			method = "try_get"
		}
		body = "if (value.kind !== \"String\") { " + expected("String") + "; } const parsed = " + owner + "." + method + "(value.value); if (parsed.kind === \"Err\") { return fail(path, " + strconv.Quote("invalid "+schema.Type.Name) + "); } return parsed.value;"
	case "raw_enum":
		kind := "String"
		if schema.RawType.Kind == types.Int {
			kind = "Integer"
		}
		owner := schema.Type.Name
		if schema.Reference != nil && schema.Reference.Alias != "" {
			owner = schema.Reference.Alias + "." + owner
		}
		branches := make([]string, 0, len(schema.RawValues))
		for _, item := range schema.RawValues {
			branches = append(branches, "case "+item.Raw+": return "+owner+"."+item.Member+";")
		}
		body = "if (value.kind !== " + strconv.Quote(kind) + ") { " + expected(kind) + "; } switch (value.value) { " + strings.Join(branches, " ") + " } return fail(path, " + strconv.Quote("unknown raw value for "+schema.Type.Name) + ");"
	case "array":
		child := b.decoder(schema.Element)
		body = "if (value.kind !== \"Array\") { " + expected("Array") + "; } return value.value.map((item, index) => " + child + "(item, path + \"/\" + String(index)));"
	case "hash":
		child := b.decoder(schema.Element)
		body = "if (value.kind !== \"Object\") { " + expected("Object") + "; } const decoded: Record<string, " + tsCodecType(schema.Element) + "> = {}; for (const [key, item] of Object.entries(value.value)) { const escaped = key.replaceAll(\"~\", \"~0\").replaceAll(\"/\", \"~1\"); decoded[key] = " + child + "(item, path + \"/\" + escaped); } return decoded;"
	case "record":
		var fields strings.Builder
		fields.WriteString("if (value.kind !== \"Object\") { " + expected(schema.Type.Name) + "; } ")
		parts := make([]string, 0, len(schema.Fields))
		for index, field := range schema.Fields {
			child := b.decoder(field.Schema)
			variable := "field" + strconv.Itoa(index)
			path := "path + " + strconv.Quote("/"+tsJSONPointerEscape(field.WireName))
			fields.WriteString("let " + variable + ": " + tsCodecType(field.Schema) + "; if (Object.prototype.hasOwnProperty.call(value.value, " + strconv.Quote(field.WireName) + ")) { " + variable + " = " + child + "(value.value[" + strconv.Quote(field.WireName) + "]!, " + path + "); }")
			if field.Schema.Type.Nullable {
				fields.WriteString(" else { " + variable + " = null; }")
			} else {
				fields.WriteString(" else { " + variable + " = fail(" + path + ", " + strconv.Quote("missing field "+field.WireName) + "); }")
			}
			fields.WriteString(" ")
			parts = append(parts, field.Name+": "+variable)
		}
		fields.WriteString("return { " + strings.Join(parts, ", ") + " };")
		body = fields.String()
	}
	b.source.WriteString("const " + name + " = (value: " + jsonValue + ", path: string): " + valueType + " => { " + body + " }; ")
	return name
}

func tsJSONPointerEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func tsJSONQualified(alias, name string) string {
	if alias == "" {
		return name
	}
	return alias + "." + name
}

func (b *tsJSONCodecBuilder) encoder(schema *ir.CodecSchema) string {
	name := b.name("Encode")
	valueType := tsCodecType(schema)
	jsonValue := tsJSONQualified(b.jsonAlias, "JsonValue")
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.encoder(&nonnull)
		b.source.WriteString("const " + name + " = (value: " + valueType + "): " + jsonValue + " => value === null ? " + tsJSONQualified(b.jsonAlias, "JsonValue.Null") + " : " + child + "(value); ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.Boolean") + "(value);"
	case "integer":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.Integer") + "(value);"
	case "float":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.Float") + "(value);"
	case "string":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.String") + "(value);"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.String") + "(value.to_s());"
	case "time_zone":
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.String") + "(value.identifier());"
	case "raw_enum":
		kind := "String"
		if schema.RawType.Kind == types.Int {
			kind = "Integer"
		}
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue."+kind) + "(value as " + tsType(schema.RawType) + ");"
	case "array":
		child := b.encoder(schema.Element)
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.Array") + "(value.map((item) => " + child + "(item)));"
	case "hash":
		child := b.encoder(schema.Element)
		body = "const fields: Record<string, " + jsonValue + "> = {}; for (const [key, item] of Object.entries(value)) { fields[key] = " + child + "(item); } return " + tsJSONQualified(b.jsonAlias, "JsonValue.Object") + "(fields);"
	case "record":
		parts := make([]string, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			child := b.encoder(field.Schema)
			parts = append(parts, strconv.Quote(field.WireName)+": "+child+"(value."+field.Name+")")
		}
		body = "return " + tsJSONQualified(b.jsonAlias, "JsonValue.Object") + "({ " + strings.Join(parts, ", ") + " });"
	}
	b.source.WriteString("const " + name + " = (value: " + valueType + "): " + jsonValue + " => { " + body + " }; ")
	return name
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

func (g *generator) importedJSONCall(call *ir.Call, packagePath string, arguments []string) (string, bool) {
	reference := expressionReference(call.Callee)
	if reference == nil || reference.Package != packagePath || g.modulePath == packagePath {
		return "", false
	}
	name := reference.Symbol
	switch callee := call.Callee.(type) {
	case *ir.Identifier:
		name = callee.Name
	case *ir.Member:
		name = g.expr(callee.Receiver) + "." + callee.Name
	}
	return name + "(" + strings.Join(arguments, ", ") + ")", true
}

func (g *generator) jsxElement(element *ir.JSXElement) string {
	name := element.Name
	if element.Fragment {
		name = ""
	} else if element.Component != nil {
		name = g.expr(element.Component)
	}
	var result strings.Builder
	result.WriteByte('<')
	result.WriteString(name)
	for _, attribute := range element.Attributes {
		result.WriteByte(' ')
		result.WriteString(attribute.Name)
		if attribute.Boolean {
			continue
		}
		result.WriteString("={")
		result.WriteString(g.expr(attribute.Value))
		result.WriteByte('}')
	}
	if len(element.Children) == 0 && !element.Fragment {
		result.WriteString(" />")
		return result.String()
	}
	result.WriteByte('>')
	for _, child := range element.Children {
		switch node := child.(type) {
		case *ir.JSXElement:
			result.WriteString(g.jsxElement(node))
		case *ir.JSXText:
			result.WriteString(node.Text)
		case *ir.JSXExpression:
			result.WriteByte('{')
			result.WriteString(g.expr(node.Value))
			result.WriteByte('}')
		}
	}
	result.WriteString("</")
	result.WriteString(name)
	result.WriteByte('>')
	return result.String()
}

func tsImportPath(modulePath, imported string, extensions ...string) string {
	currentDirectory := pathpkg.Dir(modulePath)
	if currentDirectory == "." || currentDirectory == "" {
		currentDirectory = "."
	}
	relative, err := filepathRel(currentDirectory, imported)
	if err != nil {
		relative = imported
	}
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	extension := ""
	if len(extensions) > 0 {
		extension = extensions[0]
	}
	if extension == "" {
		extension = ".ts"
	}
	return relative + extension
}

func filepathRel(base, target string) (string, error) {
	// path.Rel is unavailable; clean slash paths and compute the small
	// module-relative form without involving host path separators.
	baseParts := strings.Split(pathpkg.Clean(base), "/")
	targetParts := strings.Split(pathpkg.Clean(target), "/")
	if len(baseParts) == 1 && baseParts[0] == "." {
		baseParts = nil
	}
	for len(baseParts) > 0 && len(targetParts) > 0 && baseParts[0] == targetParts[0] {
		baseParts = baseParts[1:]
		targetParts = targetParts[1:]
	}
	parts := make([]string, len(baseParts))
	for i := range parts {
		parts[i] = ".."
	}
	parts = append(parts, targetParts...)
	if len(parts) == 0 {
		return ".", nil
	}
	return strings.Join(parts, "/"), nil
}

func tsType(t types.Type) string {
	return tsTypeWithMappings(t, nil, nil)
}

func (g *generator) tsType(t types.Type) string {
	if g.orm == nil || g.modulePath == "trb/orm/index" {
		return tsTypeWithMappings(t, g.typeAliases, g.typeMappings)
	}
	aliases := make(map[string]string, len(g.typeAliases)+3)
	for name, alias := range g.typeAliases {
		aliases[name] = alias
	}
	for _, name := range []string{"DbError", "DbErrorKind", "DbResult"} {
		if aliases[name] == "" {
			aliases[name] = "__trbOrm"
		}
	}
	return tsTypeWithMappings(t, aliases, g.typeMappings)
}

func (g *generator) tsSuspendingFunctionType(t types.Type) string {
	parameters, returned, ok := types.FunctionSignature(t)
	if !ok {
		return g.tsType(t)
	}
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = "arg" + strconv.Itoa(index) + ": " + g.tsType(parameter)
	}
	if failure := types.FunctionFailure(t); failure.Kind != types.Never {
		if returned.Kind == types.Void {
			returned = types.FromName("Unit")
		}
		returned = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{returned, failure}}
	}
	return "(" + strings.Join(parts, ", ") + ") => Promise<" + g.tsType(returned) + ">"
}

func (g *generator) runtimeName(name string) string {
	if alias := g.typeAliases[name]; alias != "" {
		return alias + "." + name
	}
	return name
}

func tsTypeWithMappings(t types.Type, aliases map[string]string, mappings map[string]string) string {
	var result string
	switch t.Kind {
	case types.Void:
		result = "void"
	case types.Any, types.Invalid:
		result = "unknown"
	case types.Union:
		alternatives := make([]string, len(t.Args))
		for index, alternative := range t.Args {
			alternatives[index] = tsTypeWithMappings(alternative, aliases, mappings)
		}
		result = strings.Join(alternatives, " | ")
	case types.Bool:
		result = "boolean"
	case types.Int, types.Float:
		result = "number"
	case types.IntLiteral, types.StringLiteral:
		result = t.Name
	case types.String:
		result = "string"
	case types.Bytes:
		result = "Uint8Array"
	case types.StringBuilder:
		result = "Array<string>"
	case types.Array, types.Iterable:
		element := "unknown"
		if len(t.Args) > 0 {
			element = tsTypeWithMappings(t.Args[0], aliases, mappings)
		}
		result = "Array<" + element + ">"
	case types.Range:
		result = "[number, number, boolean]"
	case types.Hash:
		key := "string"
		value := "unknown"
		if len(t.Args) == 2 {
			key = tsTypeWithMappings(t.Args[0], aliases, mappings)
			if t.Args[0].Kind == types.Bool {
				key = "string"
			}
			value = tsTypeWithMappings(t.Args[1], aliases, mappings)
		}
		result = "Record<" + key + ", " + value + ">"
	case types.Function:
		parameters, returned, ok := types.FunctionSignature(t)
		if !ok {
			result = "(...args: Array<unknown>) => unknown"
			break
		}
		parts := make([]string, len(parameters))
		for index, parameter := range parameters {
			parts[index] = "arg" + strconv.Itoa(index) + ": " + tsTypeWithMappings(parameter, aliases, mappings)
		}
		fallible := false
		if failure := types.FunctionFailure(t); failure.Kind != types.Never {
			fallible = true
			if returned.Kind == types.Void {
				returned = types.FromName("Unit")
			}
			returned = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{returned, failure}}
		}
		resultType := tsTypeWithMappings(returned, aliases, mappings)
		if fallible {
			resultType += " | Promise<" + resultType + ">"
		}
		result = "(" + strings.Join(parts, ", ") + ") => " + resultType
	case types.Nil:
		result = "null"
	default:
		switch t.Name {
		case "Callback":
			argument := "unknown"
			if len(t.Args) > 0 {
				argument = tsTypeWithMappings(t.Args[0], aliases, mappings)
			}
			result = "(value: " + argument + ") => void"
		default:
			result = mappings[t.Name]
			if result == "" {
				result = t.Name
				if alias := aliases[t.Name]; alias != "" {
					result = alias + "." + result
				}
			}
		}
	}
	if t.Kind == types.Named && len(t.Args) > 0 {
		arguments := make([]string, len(t.Args))
		for index, argument := range t.Args {
			arguments[index] = tsTypeWithMappings(argument, aliases, mappings)
		}
		result += "<" + strings.Join(arguments, ", ") + ">"
	}
	if t.Nullable && result != "null" {
		result += " | null"
	}
	return result
}

func md5Expression(argument string) string {
	return "(" + md5Function() + ")(" + argument + ")"
}

func md5Function() string {
	return `(input: Uint8Array): Uint8Array => {
  const shifts = [
    7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
    5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
    4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
    6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
  ];
  const constants = new Uint32Array([
    0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee, 0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
    0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be, 0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
    0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa, 0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
    0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed, 0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
    0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c, 0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
    0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05, 0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
    0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039, 0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
    0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1, 0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
  ]);
  const paddedLength = Math.ceil((input.byteLength + 9) / 64) * 64;
  const message = new Uint8Array(paddedLength);
  message.set(input);
  message[input.byteLength] = 0x80;
  const view = new DataView(message.buffer);
  view.setBigUint64(paddedLength - 8, BigInt(input.byteLength) * 8n, true);
  const rotateLeft = (value: number, bits: number): number => ((value << bits) | (value >>> (32 - bits))) >>> 0;
  let h0 = 0x67452301;
  let h1 = 0xefcdab89;
  let h2 = 0x98badcfe;
  let h3 = 0x10325476;
  for (let offset = 0; offset < paddedLength; offset += 64) {
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    for (let index = 0; index < 64; index += 1) {
      let mixed: number;
      let wordIndex: number;
      if (index < 16) {
        mixed = ((b & c) | (~b & d)) >>> 0;
        wordIndex = index;
      } else if (index < 32) {
        mixed = ((d & b) | (~d & c)) >>> 0;
        wordIndex = (5 * index + 1) % 16;
      } else if (index < 48) {
        mixed = (b ^ c ^ d) >>> 0;
        wordIndex = (3 * index + 5) % 16;
      } else {
        mixed = (c ^ (b | ~d)) >>> 0;
        wordIndex = (7 * index) % 16;
      }
      const sum = (a + mixed + constants[index]! + view.getUint32(offset + wordIndex * 4, true)) >>> 0;
      a = d;
      d = c;
      c = b;
      b = (b + rotateLeft(sum, shifts[index]!)) >>> 0;
    }
    h0 = (h0 + a) >>> 0;
    h1 = (h1 + b) >>> 0;
    h2 = (h2 + c) >>> 0;
    h3 = (h3 + d) >>> 0;
  }
  const digest = new Uint8Array(16);
  const digestView = new DataView(digest.buffer);
  [h0, h1, h2, h3].forEach((value, index) => digestView.setUint32(index * 4, value, true));
  return digest;
}`
}

func sha1Expression(argument string) string {
	return "(" + sha1Function() + ")(" + argument + ")"
}

func sha1Function() string {
	return `(input: Uint8Array): Uint8Array => {
  const paddedLength = Math.ceil((input.byteLength + 9) / 64) * 64;
  const message = new Uint8Array(paddedLength);
  message.set(input);
  message[input.byteLength] = 0x80;
  const view = new DataView(message.buffer);
  view.setBigUint64(paddedLength - 8, BigInt(input.byteLength) * 8n, false);
  const rotateLeft = (value: number, bits: number): number => ((value << bits) | (value >>> (32 - bits))) >>> 0;
  const words = new Uint32Array(80);
  let h0 = 0x67452301;
  let h1 = 0xefcdab89;
  let h2 = 0x98badcfe;
  let h3 = 0x10325476;
  let h4 = 0xc3d2e1f0;
  for (let offset = 0; offset < paddedLength; offset += 64) {
    for (let index = 0; index < 16; index += 1) words[index] = view.getUint32(offset + index * 4, false);
    for (let index = 16; index < 80; index += 1) {
      words[index] = rotateLeft(words[index - 3]! ^ words[index - 8]! ^ words[index - 14]! ^ words[index - 16]!, 1);
    }
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    let e = h4;
    for (let index = 0; index < 80; index += 1) {
      let mixed: number;
      let constant: number;
      if (index < 20) {
        mixed = ((b & c) | (~b & d)) >>> 0;
        constant = 0x5a827999;
      } else if (index < 40) {
        mixed = (b ^ c ^ d) >>> 0;
        constant = 0x6ed9eba1;
      } else if (index < 60) {
        mixed = ((b & c) | (b & d) | (c & d)) >>> 0;
        constant = 0x8f1bbcdc;
      } else {
        mixed = (b ^ c ^ d) >>> 0;
        constant = 0xca62c1d6;
      }
      const temporary = (rotateLeft(a, 5) + mixed + e + constant + words[index]!) >>> 0;
      e = d;
      d = c;
      c = rotateLeft(b, 30);
      b = a;
      a = temporary;
    }
    h0 = (h0 + a) >>> 0;
    h1 = (h1 + b) >>> 0;
    h2 = (h2 + c) >>> 0;
    h3 = (h3 + d) >>> 0;
    h4 = (h4 + e) >>> 0;
  }
  const digest = new Uint8Array(20);
  const digestView = new DataView(digest.buffer);
  [h0, h1, h2, h3, h4].forEach((value, index) => digestView.setUint32(index * 4, value, false));
  return digest;
}`
}

func sha256Expression(argument string) string {
	return "(" + sha256Function() + ")(" + argument + ")"
}

func sha256Function() string {
	return `(input: Uint8Array): Uint8Array => {
  const constants = new Uint32Array([
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
  ]);
  const paddedLength = Math.ceil((input.byteLength + 9) / 64) * 64;
  const message = new Uint8Array(paddedLength);
  message.set(input);
  message[input.byteLength] = 0x80;
  const view = new DataView(message.buffer);
  view.setBigUint64(paddedLength - 8, BigInt(input.byteLength) * 8n, false);
  const rotateRight = (value: number, bits: number): number => ((value >>> bits) | (value << (32 - bits))) >>> 0;
  const words = new Uint32Array(64);
  let h0 = 0x6a09e667;
  let h1 = 0xbb67ae85;
  let h2 = 0x3c6ef372;
  let h3 = 0xa54ff53a;
  let h4 = 0x510e527f;
  let h5 = 0x9b05688c;
  let h6 = 0x1f83d9ab;
  let h7 = 0x5be0cd19;
  for (let offset = 0; offset < paddedLength; offset += 64) {
    for (let index = 0; index < 16; index += 1) words[index] = view.getUint32(offset + index * 4, false);
    for (let index = 16; index < 64; index += 1) {
      const s0 = (rotateRight(words[index - 15]!, 7) ^ rotateRight(words[index - 15]!, 18) ^ (words[index - 15]! >>> 3)) >>> 0;
      const s1 = (rotateRight(words[index - 2]!, 17) ^ rotateRight(words[index - 2]!, 19) ^ (words[index - 2]! >>> 10)) >>> 0;
      words[index] = (words[index - 16]! + s0 + words[index - 7]! + s1) >>> 0;
    }
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    let e = h4;
    let f = h5;
    let g = h6;
    let h = h7;
    for (let index = 0; index < 64; index += 1) {
      const sigma1 = (rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25)) >>> 0;
      const choice = ((e & f) ^ (~e & g)) >>> 0;
      const temporary1 = (h + sigma1 + choice + constants[index]! + words[index]!) >>> 0;
      const sigma0 = (rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22)) >>> 0;
      const majority = ((a & b) ^ (a & c) ^ (b & c)) >>> 0;
      const temporary2 = (sigma0 + majority) >>> 0;
      h = g;
      g = f;
      f = e;
      e = (d + temporary1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (temporary1 + temporary2) >>> 0;
    }
    h0 = (h0 + a) >>> 0;
    h1 = (h1 + b) >>> 0;
    h2 = (h2 + c) >>> 0;
    h3 = (h3 + d) >>> 0;
    h4 = (h4 + e) >>> 0;
    h5 = (h5 + f) >>> 0;
    h6 = (h6 + g) >>> 0;
    h7 = (h7 + h) >>> 0;
  }
  const digest = new Uint8Array(32);
  const digestView = new DataView(digest.buffer);
  [h0, h1, h2, h3, h4, h5, h6, h7].forEach((value, index) => digestView.setUint32(index * 4, value, false));
  return digest;
}`
}

func sha512Expression(argument string) string {
	return "(" + sha512Function() + ")(" + argument + ")"
}

func sha512Function() string {
	return `(input: Uint8Array): Uint8Array => {
  const mask = 0xffffffffffffffffn;
  const constants = [
    0x428a2f98d728ae22n, 0x7137449123ef65cdn, 0xb5c0fbcfec4d3b2fn, 0xe9b5dba58189dbbcn,
    0x3956c25bf348b538n, 0x59f111f1b605d019n, 0x923f82a4af194f9bn, 0xab1c5ed5da6d8118n,
    0xd807aa98a3030242n, 0x12835b0145706fben, 0x243185be4ee4b28cn, 0x550c7dc3d5ffb4e2n,
    0x72be5d74f27b896fn, 0x80deb1fe3b1696b1n, 0x9bdc06a725c71235n, 0xc19bf174cf692694n,
    0xe49b69c19ef14ad2n, 0xefbe4786384f25e3n, 0x0fc19dc68b8cd5b5n, 0x240ca1cc77ac9c65n,
    0x2de92c6f592b0275n, 0x4a7484aa6ea6e483n, 0x5cb0a9dcbd41fbd4n, 0x76f988da831153b5n,
    0x983e5152ee66dfabn, 0xa831c66d2db43210n, 0xb00327c898fb213fn, 0xbf597fc7beef0ee4n,
    0xc6e00bf33da88fc2n, 0xd5a79147930aa725n, 0x06ca6351e003826fn, 0x142929670a0e6e70n,
    0x27b70a8546d22ffcn, 0x2e1b21385c26c926n, 0x4d2c6dfc5ac42aedn, 0x53380d139d95b3dfn,
    0x650a73548baf63den, 0x766a0abb3c77b2a8n, 0x81c2c92e47edaee6n, 0x92722c851482353bn,
    0xa2bfe8a14cf10364n, 0xa81a664bbc423001n, 0xc24b8b70d0f89791n, 0xc76c51a30654be30n,
    0xd192e819d6ef5218n, 0xd69906245565a910n, 0xf40e35855771202an, 0x106aa07032bbd1b8n,
    0x19a4c116b8d2d0c8n, 0x1e376c085141ab53n, 0x2748774cdf8eeb99n, 0x34b0bcb5e19b48a8n,
    0x391c0cb3c5c95a63n, 0x4ed8aa4ae3418acbn, 0x5b9cca4f7763e373n, 0x682e6ff3d6b2b8a3n,
    0x748f82ee5defb2fcn, 0x78a5636f43172f60n, 0x84c87814a1f0ab72n, 0x8cc702081a6439ecn,
    0x90befffa23631e28n, 0xa4506cebde82bde9n, 0xbef9a3f7b2c67915n, 0xc67178f2e372532bn,
    0xca273eceea26619cn, 0xd186b8c721c0c207n, 0xeada7dd6cde0eb1en, 0xf57d4f7fee6ed178n,
    0x06f067aa72176fban, 0x0a637dc5a2c898a6n, 0x113f9804bef90daen, 0x1b710b35131c471bn,
    0x28db77f523047d84n, 0x32caab7b40c72493n, 0x3c9ebe0a15c9bebcn, 0x431d67c49c100d4cn,
    0x4cc5d4becb3e42b6n, 0x597f299cfc657e2an, 0x5fcb6fab3ad6faecn, 0x6c44198c4a475817n,
  ];
  const paddedLength = Math.ceil((input.byteLength + 17) / 128) * 128;
  const message = new Uint8Array(paddedLength);
  message.set(input);
  message[input.byteLength] = 0x80;
  const view = new DataView(message.buffer);
  view.setBigUint64(paddedLength - 8, BigInt(input.byteLength) * 8n, false);
  const rotateRight = (value: bigint, bits: bigint): bigint => ((value >> bits) | (value << (64n - bits))) & mask;
  const words = new Array<bigint>(80).fill(0n);
  let h0 = 0x6a09e667f3bcc908n;
  let h1 = 0xbb67ae8584caa73bn;
  let h2 = 0x3c6ef372fe94f82bn;
  let h3 = 0xa54ff53a5f1d36f1n;
  let h4 = 0x510e527fade682d1n;
  let h5 = 0x9b05688c2b3e6c1fn;
  let h6 = 0x1f83d9abfb41bd6bn;
  let h7 = 0x5be0cd19137e2179n;
  for (let offset = 0; offset < paddedLength; offset += 128) {
    for (let index = 0; index < 16; index += 1) words[index] = view.getBigUint64(offset + index * 8, false);
    for (let index = 16; index < 80; index += 1) {
      const s0 = rotateRight(words[index - 15]!, 1n) ^ rotateRight(words[index - 15]!, 8n) ^ (words[index - 15]! >> 7n);
      const s1 = rotateRight(words[index - 2]!, 19n) ^ rotateRight(words[index - 2]!, 61n) ^ (words[index - 2]! >> 6n);
      words[index] = (words[index - 16]! + s0 + words[index - 7]! + s1) & mask;
    }
    let a = h0;
    let b = h1;
    let c = h2;
    let d = h3;
    let e = h4;
    let f = h5;
    let g = h6;
    let h = h7;
    for (let index = 0; index < 80; index += 1) {
      const sigma1 = rotateRight(e, 14n) ^ rotateRight(e, 18n) ^ rotateRight(e, 41n);
      const choice = ((e & f) ^ (~e & g)) & mask;
      const temporary1 = (h + sigma1 + choice + constants[index]! + words[index]!) & mask;
      const sigma0 = rotateRight(a, 28n) ^ rotateRight(a, 34n) ^ rotateRight(a, 39n);
      const majority = ((a & b) ^ (a & c) ^ (b & c)) & mask;
      const temporary2 = (sigma0 + majority) & mask;
      h = g;
      g = f;
      f = e;
      e = (d + temporary1) & mask;
      d = c;
      c = b;
      b = a;
      a = (temporary1 + temporary2) & mask;
    }
    h0 = (h0 + a) & mask;
    h1 = (h1 + b) & mask;
    h2 = (h2 + c) & mask;
    h3 = (h3 + d) & mask;
    h4 = (h4 + e) & mask;
    h5 = (h5 + f) & mask;
    h6 = (h6 + g) & mask;
    h7 = (h7 + h) & mask;
  }
  const digest = new Uint8Array(64);
  const digestView = new DataView(digest.buffer);
  [h0, h1, h2, h3, h4, h5, h6, h7].forEach((value, index) => digestView.setBigUint64(index * 8, value, false));
  return digest;
}`
}

func hmacExpression(key, message string, blockSize int, hashFunction string) string {
	return `((key: Uint8Array, message: Uint8Array): Uint8Array => {
  const hash = ` + hashFunction + `;
  const material = key.byteLength > ` + strconv.Itoa(blockSize) + ` ? hash(key) : key;
  const blockKey = new Uint8Array(` + strconv.Itoa(blockSize) + `);
  blockKey.set(material);
  const innerPad = new Uint8Array(` + strconv.Itoa(blockSize) + `);
  const outerPad = new Uint8Array(` + strconv.Itoa(blockSize) + `);
  for (let index = 0; index < blockKey.byteLength; index += 1) {
    innerPad[index] = blockKey[index]! ^ 0x36;
    outerPad[index] = blockKey[index]! ^ 0x5c;
  }
  const innerInput = new Uint8Array(innerPad.byteLength + message.byteLength);
  innerInput.set(innerPad);
  innerInput.set(message, innerPad.byteLength);
  const innerDigest = hash(innerInput);
  const outerInput = new Uint8Array(outerPad.byteLength + innerDigest.byteLength);
  outerInput.set(outerPad);
  outerInput.set(innerDigest, outerPad.byteLength);
  return hash(outerInput);
})(` + key + `, ` + message + `)`
}

func comment(text string) string {
	return "//" + strings.TrimPrefix(strings.TrimSpace(text), "#")
}

func tsTrailingComment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return " //" + strings.TrimPrefix(value, "#")
}

func classMethods(statements []ir.Statement) map[string]bool {
	methods := map[string]bool{}
	for _, statement := range statements {
		if method, ok := statement.(*ir.Method); ok {
			methods[method.Name] = true
		}
	}
	return methods
}

func (g *generator) line(text string) {
	g.b.WriteString(strings.Repeat("  ", g.indent))
	g.b.WriteString(text)
	g.b.WriteByte('\n')
}
