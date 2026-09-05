package projectintegration

import (
	"github.com/type-rb/type-rb/internal/ast"
	cliapp "github.com/type-rb/type-rb/internal/cliapp"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/resolver"
)

func init() {
	register(cliapp.ProjectProvider, analyzeCLI)
}

func analyzeCLI(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	input, err := packageextensionhost.ExportProjectDeclarationInput(
		cliapp.PackageName,
		programs,
		packageextensionhost.ProjectDeclarationInputOptions{PackageAliasesByModule: context.PackageAliasesByModule},
	)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	var requests []cliapp.InvocationRequest
	var issues []Issue
	for _, source := range context.Sources {
		resolution := context.Resolutions[source.ModulePath]
		walkCLIStatements(source.Program.Statements, func(call *ast.CallExpression) {
			generic, ok := call.Callee.(*ast.GenericExpression)
			if !ok || !cliRunBinding(generic.Receiver, resolution) {
				return
			}
			if len(generic.Arguments) != 1 {
				return
			}
			rootType := generic.Arguments[0]
			root, diagnostic := cliRootReference(source.ModulePath, rootType, resolution)
			if diagnostic != "" {
				issues = append(issues, Issue{Filename: source.Filename, Message: diagnostic, Span: rootType.Span()})
				return
			}
			requests = append(requests, cliapp.InvocationRequest{
				ModulePath: source.ModulePath, Offset: call.Span().Start.Offset, Root: root,
				Span: packageextensionhost.ExportSourceSpan(call.Span()),
			})
		})
	}
	manifest, schemaIssues := cliapp.Analyze(input, requests)
	filenames := map[string]string{}
	for _, source := range context.Sources {
		filenames[source.ModulePath] = source.Filename
	}
	for _, schemaIssue := range schemaIssues {
		issues = append(issues, Issue{Filename: filenames[schemaIssue.ModulePath], Message: schemaIssue.Message, Span: packageextensionhost.ImportSourceSpan(schemaIssue.Span)})
	}
	return Contribution{Extension: manifest, AllPrograms: true}, issues
}

func cliRunBinding(expression ast.Expression, resolution resolver.Result) bool {
	var binding resolver.Binding
	var ok bool
	switch node := expression.(type) {
	case *ast.Identifier:
		binding, ok = resolution.Symbols[node.Name]
	case *ast.MemberExpression:
		receiver, named := node.Receiver.(*ast.Identifier)
		if named {
			binding, ok = resolution.Member(receiver.Name, node.Name)
		}
	}
	return ok && binding.Import != nil && binding.Import.RuntimePath() == cliapp.ModulePath && binding.Name == "run"
}

func cliRootReference(modulePath string, ref ast.TypeRef, resolution resolver.Result) (cliapp.TypeReference, string) {
	if ref.Nullable {
		return cliapp.TypeReference{}, "trb/cli run root " + ref.String() + " must be non-nullable"
	}
	if len(ref.Arguments) > 0 {
		return cliapp.TypeReference{}, "trb/cli run root " + ref.String() + " must be non-generic"
	}
	if ref.Name == "" || ref.Array || len(ref.Union) > 0 || ref.FunctionReturn != nil {
		return cliapp.TypeReference{}, "trb/cli run root " + ref.String() + " must name a record type directly"
	}
	if binding, ok := resolution.ImportedType(ref.Name); ok && binding.Import != nil && binding.Export != nil {
		return cliapp.TypeReference{ModulePath: binding.Import.RuntimePath(), Name: binding.Export.Name}, ""
	}
	return cliapp.TypeReference{ModulePath: modulePath, Name: ref.Name}, ""
}

func walkCLIStatements(statements []ast.Statement, visit func(*ast.CallExpression)) {
	for _, statement := range statements {
		walkCLIStatement(statement, visit)
	}
}

func walkCLIStatement(statement ast.Statement, visit func(*ast.CallExpression)) {
	switch node := statement.(type) {
	case *ast.ClassStatement:
		walkCLIExpression(node.Superclass, visit)
		walkCLIStatements(node.Body, visit)
	case *ast.RecordStatement:
		walkCLIStatements(node.Body, visit)
	case *ast.RecordFieldStatement:
		walkCLIExpression(node.Default, visit)
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				walkCLIExpression(argument.Value, visit)
			}
		}
	case *ast.EnumStatement:
		walkCLIStatements(node.Body, visit)
	case *ast.NewtypeStatement:
		walkCLIStatements(node.Body, visit)
	case *ast.EnumMemberStatement:
		walkCLIExpression(node.RawValue, visit)
		for _, parameter := range node.Parameters {
			walkCLIExpression(parameter.Default, visit)
		}
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				walkCLIExpression(argument.Value, visit)
			}
		}
	case *ast.ModuleStatement:
		walkCLIStatements(node.Body, visit)
	case *ast.InterfaceStatement:
		for _, method := range node.Methods {
			walkCLIStatement(method, visit)
		}
	case *ast.FieldStatement:
		walkCLIExpression(node.Value, visit)
	case *ast.MethodStatement:
		for _, parameter := range node.Parameters {
			walkCLIExpression(parameter.Default, visit)
		}
		walkCLIStatements(node.Body, visit)
	case *ast.VariableStatement:
		walkCLIExpression(node.Value, visit)
	case *ast.AssignmentStatement:
		walkCLIExpression(node.Target, visit)
		walkCLIExpression(node.Value, visit)
	case *ast.ReturnStatement:
		walkCLIExpression(node.Value, visit)
	case *ast.ExpressionStatement:
		walkCLIExpression(node.Expression, visit)
	case *ast.IfStatement:
		walkCLIIf(node, visit)
	case *ast.CaseStatement:
		walkCLICase(node, visit)
	case *ast.WhileStatement:
		walkCLIExpression(node.Condition, visit)
		walkCLIStatements(node.Body, visit)
	case *ast.NativeBlock:
		walkCLIStatements(node.Body, visit)
	}
}

func walkCLIExpression(expression ast.Expression, visit func(*ast.CallExpression)) {
	if expression == nil {
		return
	}
	switch node := expression.(type) {
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			walkCLIExpression(part.Expression, visit)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			walkCLIExpression(element, visit)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			walkCLIExpression(entry.Key, visit)
			walkCLIExpression(entry.Value, visit)
		}
	case *ast.JSXElement:
		walkCLIExpression(node.Component, visit)
		for _, attribute := range node.Attributes {
			walkCLIExpression(attribute.Value, visit)
		}
		for _, child := range node.Children {
			switch child := child.(type) {
			case *ast.JSXElement:
				walkCLIExpression(child, visit)
			case *ast.JSXExpression:
				walkCLIExpression(child.Value, visit)
			}
		}
	case *ast.UnaryExpression:
		walkCLIExpression(node.Operand, visit)
	case *ast.BinaryExpression:
		walkCLIExpression(node.Left, visit)
		walkCLIExpression(node.Right, visit)
	case *ast.RangeExpression:
		walkCLIExpression(node.Start, visit)
		walkCLIExpression(node.End, visit)
	case *ast.CallExpression:
		visit(node)
		walkCLIExpression(node.Callee, visit)
		for _, argument := range node.Arguments {
			walkCLIExpression(argument.Value, visit)
		}
		if node.Block != nil {
			walkCLIStatements(node.Block.Body, visit)
		}
	case *ast.GenericExpression:
		walkCLIExpression(node.Receiver, visit)
	case *ast.MemberExpression:
		walkCLIExpression(node.Receiver, visit)
	case *ast.IndexExpression:
		walkCLIExpression(node.Receiver, visit)
		walkCLIExpression(node.Index, visit)
	case *ast.BlockExpression:
		walkCLIStatements(node.Body, visit)
	case *ast.AttemptExpression:
		walkCLIExpression(node.Value, visit)
		walkCLIStatements(node.Body, visit)
	case *ast.TryExpression:
		walkCLIExpression(node.Value, visit)
	case *ast.CatchExpression:
		walkCLIExpression(node.Value, visit)
		walkCLIStatements(node.Body, visit)
	case *ast.LambdaExpression:
		for _, parameter := range node.Parameters {
			walkCLIExpression(parameter.Default, visit)
		}
		walkCLIStatements(node.Body, visit)
	case *ast.IterationExpression:
		walkCLIExpression(node.Source, visit)
		walkCLIExpression(node.SliceSize, visit)
		walkCLIExpression(node.Initial, visit)
		walkCLIExpression(node.Limit, visit)
		if node.Block != nil {
			walkCLIStatements(node.Block.Body, visit)
		}
	case *ast.IfStatement:
		walkCLIIf(node, visit)
	case *ast.CaseStatement:
		walkCLICase(node, visit)
	}
}

func walkCLIIf(node *ast.IfStatement, visit func(*ast.CallExpression)) {
	walkCLIExpression(node.Condition, visit)
	walkCLIStatements(node.Then, visit)
	for _, branch := range node.ElseIf {
		walkCLIExpression(branch.Condition, visit)
		walkCLIStatements(branch.Body, visit)
	}
	walkCLIStatements(node.Else, visit)
}

func walkCLICase(node *ast.CaseStatement, visit func(*ast.CallExpression)) {
	walkCLIExpression(node.Value, visit)
	walkCLIStatements(node.Leading, visit)
	for _, branch := range node.Branches {
		walkCLIExpression(branch.Value, visit)
		for _, alternative := range branch.Alternatives {
			walkCLIExpression(alternative, visit)
		}
		walkCLIStatements(branch.Body, visit)
	}
	walkCLIStatements(node.Else, visit)
}
