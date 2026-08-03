// Package checker resolves names, infers local declaration types, validates
// assignments/returns, and records a type for every portable expression.
package checker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type Result struct {
	Program     *ast.Program
	Expressions map[ast.Expression]types.Type
	Variables   map[*ast.VariableStatement]types.Type
	Resolution  resolver.Result
	References  map[ast.Expression]resolver.Binding
}

type symbol struct {
	typ      types.Type
	mutable  bool
	span     token.Span
	variable *ast.VariableStatement
}

type scope struct {
	parent *scope
	values map[string]symbol
}

func (s *scope) lookup(name string) (symbol, bool) {
	for current := s; current != nil; current = current.parent {
		if value, ok := current.values[name]; ok {
			return value, true
		}
	}
	return symbol{}, false
}

type classInfo struct {
	name       string
	superclass string
	mixins     []string
	fields     map[string]*ast.FieldStatement
	methods    map[string]*ast.MethodStatement
}

type recordInfo struct {
	name   string
	fields []*ast.RecordFieldStatement
	byName map[string]*ast.RecordFieldStatement
}

type Checker struct {
	mode       string
	result     Result
	diags      []diagnostic.Diagnostic
	classes    map[string]*classInfo
	records    map[string]*recordInfo
	interfaces map[string]*ast.InterfaceStatement
	functions  map[string]*ast.MethodStatement
	current    *classInfo
	returns    []types.Type
	resolution resolver.Result
	external   map[ast.Expression]declaration.Member
}

func Check(program *ast.Program, resolution resolver.Result) (Result, []diagnostic.Diagnostic) {
	c := &Checker{
		mode: program.Mode,
		result: Result{
			Program:     program,
			Expressions: map[ast.Expression]types.Type{},
			Variables:   map[*ast.VariableStatement]types.Type{},
			Resolution:  resolution,
			References:  map[ast.Expression]resolver.Binding{},
		},
		classes:    map[string]*classInfo{},
		records:    map[string]*recordInfo{},
		interfaces: map[string]*ast.InterfaceStatement{},
		functions:  map[string]*ast.MethodStatement{},
		resolution: resolution,
		external:   map[ast.Expression]declaration.Member{},
	}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok {
			c.functions[method.Name] = method
		}
	}
	c.collect(program.Statements)
	c.checkStatements(program.Statements, &scope{values: map[string]symbol{}})
	return c.result, c.diags
}

func (c *Checker) collect(statements []ast.Statement) {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ast.ClassStatement:
			if _, exists := c.classes[n.Name]; exists {
				c.error(n.Span(), fmt.Sprintf("class %s is already declared", n.Name))
				continue
			}
			info := &classInfo{name: n.Name, superclass: expressionTypeName(n.Superclass), fields: map[string]*ast.FieldStatement{}, methods: map[string]*ast.MethodStatement{}}
			for _, member := range n.Body {
				switch m := member.(type) {
				case *ast.FieldStatement:
					if previous, exists := info.fields[m.Name]; exists {
						c.error(m.Span(), fmt.Sprintf("field %s was already declared at %s", m.Name, previous.Span().Start))
					} else {
						info.fields[m.Name] = m
					}
				case *ast.MethodStatement:
					if previous, exists := info.methods[m.Name]; exists {
						c.error(m.Span(), fmt.Sprintf("method %s was already declared at %s", m.Name, previous.Span().Start))
					} else {
						info.methods[m.Name] = m
					}
				case *ast.NativeStatement:
					if mixin := includedModule(m.Text); mixin != "" {
						info.mixins = append(info.mixins, mixin)
					}
				}
			}
			c.classes[n.Name] = info
			c.collect(n.Body)
		case *ast.RecordStatement:
			if _, exists := c.records[n.Name]; exists {
				c.error(n.Span(), fmt.Sprintf("record %s is already declared", n.Name))
				continue
			}
			info := &recordInfo{name: n.Name, byName: map[string]*ast.RecordFieldStatement{}}
			for _, member := range n.Body {
				field, ok := member.(*ast.RecordFieldStatement)
				if !ok {
					continue
				}
				if previous := info.byName[field.Name]; previous != nil {
					c.error(field.Span(), fmt.Sprintf("record field %s was already declared at %s", field.Name, previous.Span().Start))
					continue
				}
				info.fields = append(info.fields, field)
				info.byName[field.Name] = field
			}
			c.records[n.Name] = info
		case *ast.InterfaceStatement:
			if previous := c.interfaces[n.Name]; previous != nil {
				c.error(n.Span(), fmt.Sprintf("interface %s was already declared at %s", n.Name, previous.Span().Start))
			} else {
				c.interfaces[n.Name] = n
			}
		case *ast.ModuleStatement:
			c.collect(n.Body)
		case *ast.NativeBlock:
			c.collect(n.Body)
		}
	}
}

func (c *Checker) checkStatements(statements []ast.Statement, sc *scope) {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ast.ClassStatement:
			previous := c.current
			c.current = c.classes[n.Name]
			if c.current == nil {
				c.current = &classInfo{name: n.Name, superclass: expressionTypeName(n.Superclass), fields: map[string]*ast.FieldStatement{}, methods: map[string]*ast.MethodStatement{}}
			}
			c.checkSuperclass(n)
			classScope := &scope{parent: sc, values: map[string]symbol{"self": {typ: types.FromName(n.Name)}}}
			for name, field := range c.current.fields {
				classScope.values[name] = symbol{typ: fromTypeRef(field.Type), mutable: !field.ReadOnly, span: field.Span()}
			}
			c.checkStatements(n.Body, classScope)
			c.checkFieldInitialization(n)
			c.checkInterfaces(n)
			c.current = previous
		case *ast.RecordStatement:
			for _, member := range n.Body {
				field, ok := member.(*ast.RecordFieldStatement)
				if !ok {
					continue
				}
				if field.Type.Empty() {
					c.error(field.Span(), fmt.Sprintf("record field %s requires a type", field.Name))
				}
				for _, attribute := range field.Attributes {
					if attribute.Name == "gorm" && c.mode != "go" {
						c.error(attribute.Span(), "@gorm is only available in mode: go")
					}
					for _, argument := range attribute.Arguments {
						c.checkExpression(argument.Value, sc)
					}
				}
			}
		case *ast.RecordFieldStatement:
			// Checked as part of its enclosing record.
		case *ast.ModuleStatement:
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		case *ast.InterfaceStatement:
			for _, method := range n.Methods {
				c.checkMethod(method, sc)
			}
		case *ast.FieldStatement:
			if n.Value != nil {
				valueType := c.checkExpression(n.Value, sc)
				declared := fromTypeRef(n.Type)
				if !n.Type.Empty() && !types.Assignable(declared, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot initialize %s with %s", declared, valueType))
				}
			}
		case *ast.MethodStatement:
			c.checkMethod(n, sc)
		case *ast.VariableStatement:
			valueType := c.checkExpression(n.Value, sc)
			variableType := valueType
			if !n.Type.Empty() {
				variableType = fromTypeRef(n.Type)
				if !types.Assignable(variableType, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot assign %s to %s", valueType, variableType))
				}
			}
			if previous, exists := sc.values[n.Name]; exists {
				c.error(n.Span(), fmt.Sprintf("%s was already declared at %s; use = to reassign", n.Name, previous.span.Start))
			} else {
				sc.values[n.Name] = symbol{typ: variableType, mutable: n.Mutable, span: n.Span(), variable: n}
			}
			c.result.Variables[n] = variableType
		case *ast.AssignmentStatement:
			leftType := c.checkExpression(n.Target, sc)
			rightType := c.checkExpression(n.Value, sc)
			if identifier, ok := n.Target.(*ast.Identifier); ok && !strings.HasPrefix(identifier.Name, "@") {
				if value, exists := sc.lookup(identifier.Name); !exists {
					// Ruby constants and framework-provided setters remain legal.
					if c.mode != "ruby" && !isConstant(identifier.Name) {
						c.error(identifier.Span(), fmt.Sprintf("%s is not declared", identifier.Name))
					}
					if c.mode == "ruby" {
						leftType = types.Type{Kind: types.Any, Name: "Any"}
					}
				} else if value.variable != nil {
					// := declares; any later assignment requires a mutable binding in
					// targets such as TypeScript. Record that fact in the checked AST.
					value.variable.Mutable = true
				}
			}
			if leftType.Kind != types.Any && !types.Assignable(leftType, rightType) {
				c.error(n.Value.Span(), fmt.Sprintf("cannot assign %s to %s", rightType, leftType))
			}
		case *ast.ReturnStatement:
			actual := types.Type{Kind: types.Void, Name: "Void"}
			if n.Value != nil {
				actual = c.checkExpression(n.Value, sc)
			}
			if len(c.returns) > 0 {
				expected := c.returns[len(c.returns)-1]
				if !types.Assignable(expected, actual) {
					c.error(n.Span(), fmt.Sprintf("return type is %s, expected %s", actual, expected))
				}
			}
		case *ast.ExpressionStatement:
			c.checkExpression(n.Expression, sc)
		case *ast.IfStatement:
			c.checkExpression(n.Condition, sc)
			c.checkStatements(n.Then, &scope{parent: sc, values: map[string]symbol{}})
			for _, branch := range n.ElseIf {
				c.checkExpression(branch.Condition, sc)
				c.checkStatements(branch.Body, &scope{parent: sc, values: map[string]symbol{}})
			}
			c.checkStatements(n.Else, &scope{parent: sc, values: map[string]symbol{}})
		case *ast.WhileStatement:
			c.checkExpression(n.Condition, sc)
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		case *ast.NativeStatement:
			if c.mode != "ruby" {
				c.error(n.Span(), "Ruby-native statement is only available in mode: ruby")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
			}
		case *ast.NativeBlock:
			if c.mode != "ruby" {
				c.error(n.Span(), "Ruby-native block is only available in mode: ruby")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
			}
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		}
	}
}

func (c *Checker) checkMethod(method *ast.MethodStatement, parent *scope) {
	methodScope := &scope{parent: parent, values: map[string]symbol{}}
	for _, parameter := range method.Parameters {
		typ := fromTypeRef(parameter.Type)
		if parameter.Type.Empty() {
			typ = types.Type{Kind: types.Any, Name: "Any"}
		}
		if _, exists := methodScope.values[parameter.Name]; exists {
			c.error(parameter.Span(), fmt.Sprintf("parameter %s is duplicated", parameter.Name))
		}
		methodScope.values[parameter.Name] = symbol{typ: typ, mutable: true, span: parameter.Span()}
		if parameter.Default != nil {
			actual := c.checkExpression(parameter.Default, methodScope)
			if !types.Assignable(typ, actual) {
				c.error(parameter.Default.Span(), fmt.Sprintf("default value has type %s, expected %s", actual, typ))
			}
		}
	}
	returnType := fromTypeRef(method.ReturnType)
	if method.ReturnType.Empty() {
		returnType = types.Type{Kind: types.Void, Name: "Void"}
	}
	c.returns = append(c.returns, returnType)
	c.checkStatements(method.Body, methodScope)
	c.returns = c.returns[:len(c.returns)-1]
}

func (c *Checker) checkSuperclass(class *ast.ClassStatement) {
	name := expressionTypeName(class.Superclass)
	if name == "" || c.classes[name] != nil {
		return
	}
	if name == "ReactComponent" {
		if imported := c.resolution.Packages["react"]; imported != nil && imported.Path == "trb/platform/typescript/react" {
			return
		}
	}
	if imported, ok := c.resolution.ImportedType(name); ok && imported.Export.Kind == resolver.ClassExport {
		return
	}
	if _, ok := c.declarations().Type(name); ok {
		return
	}
	if c.mode == "ruby" {
		if c.resolution.NativeSyntax {
			return
		}
		c.error(class.Superclass.Span(), fmt.Sprintf("Ruby superclass %s requires import trb/platform/ruby/native or trb/platform/ruby/rails", name))
		return
	}
	c.error(class.Superclass.Span(), fmt.Sprintf("superclass %s is not declared or imported", name))
}

type methodSignature struct {
	returnType types.Type
	parameters []types.Type
	required   int
	variadic   bool
}

func (c *Checker) checkInterfaces(class *ast.ClassStatement) {
	for _, interfaceName := range class.Implements {
		required := map[string]methodSignature{}
		if local := c.interfaces[interfaceName]; local != nil {
			for _, method := range local.Methods {
				required[method.Name] = signatureFromMethod(method)
			}
		} else if imported, ok := c.resolution.ImportedType(interfaceName); ok && imported.Export.Kind == resolver.InterfaceExport {
			for name, member := range imported.Export.Members {
				required[name] = signatureFromResolvedMember(member)
			}
		} else {
			c.error(class.Span(), fmt.Sprintf("interface %s is not declared or imported", interfaceName))
			continue
		}
		for name, expected := range required {
			actual, ok := c.classMethodSignature(class.Name, name, map[string]bool{})
			if !ok {
				c.error(class.Span(), fmt.Sprintf("class %s does not implement %s.%s", class.Name, interfaceName, name))
				continue
			}
			if !sameSignature(expected, actual) {
				c.error(class.Span(), fmt.Sprintf("method %s.%s does not match interface %s", class.Name, name, interfaceName))
			}
		}
	}
}

func (c *Checker) classMethodSignature(className, memberName string, seen map[string]bool) (methodSignature, bool) {
	if className == "" || seen[className] {
		return methodSignature{}, false
	}
	seen[className] = true
	if info := c.classes[className]; info != nil {
		if method := info.methods[memberName]; method != nil {
			return signatureFromMethod(method), true
		}
		if signature, ok := c.classMethodSignature(info.superclass, memberName, seen); ok {
			return signature, true
		}
	}
	if binding, ok := c.resolution.TypeMember(className, memberName); ok && binding.Member != nil && binding.Member.Kind == resolver.FunctionExport {
		return signatureFromResolvedMember(*binding.Member), true
	}
	return methodSignature{}, false
}

func signatureFromMethod(method *ast.MethodStatement) methodSignature {
	result := methodSignature{returnType: methodReturnType(method)}
	for _, parameter := range method.Parameters {
		result.parameters = append(result.parameters, fromTypeRef(parameter.Type))
		if parameter.Default == nil && !parameter.Rest && !parameter.KeywordRest {
			result.required++
		}
		result.variadic = result.variadic || parameter.Rest || parameter.KeywordRest
	}
	return result
}

func signatureFromResolvedMember(member resolver.Member) methodSignature {
	return methodSignature{returnType: member.Type, parameters: append([]types.Type(nil), member.Parameters...), required: member.Required, variadic: member.Variadic}
}

func sameSignature(left, right methodSignature) bool {
	if left.required != right.required || left.variadic != right.variadic || len(left.parameters) != len(right.parameters) {
		return false
	}
	if !types.Assignable(left.returnType, right.returnType) || !types.Assignable(right.returnType, left.returnType) {
		return false
	}
	for i := range left.parameters {
		if !types.Assignable(left.parameters[i], right.parameters[i]) || !types.Assignable(right.parameters[i], left.parameters[i]) {
			return false
		}
	}
	return true
}

func (c *Checker) localMember(className, memberName string, seen map[string]bool) (types.Type, *ast.MethodStatement, bool) {
	if className == "" || seen[className] {
		return types.Type{}, nil, false
	}
	seen[className] = true
	if info := c.classes[className]; info != nil {
		if method := info.methods[memberName]; method != nil {
			return methodReturnType(method), method, true
		}
		if field := info.fields["@"+memberName]; field != nil {
			return fromTypeRef(field.Type), nil, true
		}
		if field := info.fields["@_"+strings.TrimPrefix(memberName, "_")]; field != nil {
			return fromTypeRef(field.Type), nil, true
		}
		if typ, method, ok := c.localMember(info.superclass, memberName, seen); ok {
			return typ, method, true
		}
	}
	return types.Type{}, nil, false
}

func (c *Checker) importedAncestorMember(className, memberName string, seen map[string]bool) (resolver.Binding, bool) {
	if className == "" || seen[className] {
		return resolver.Binding{}, false
	}
	seen[className] = true
	if binding, ok := c.resolution.TypeMember(className, memberName); ok {
		return binding, true
	}
	if info := c.classes[className]; info != nil {
		return c.importedAncestorMember(info.superclass, memberName, seen)
	}
	return resolver.Binding{}, false
}

func (c *Checker) declarations() *declaration.Catalog {
	if c.resolution.Declarations == nil {
		return declaration.NewCatalog()
	}
	return c.resolution.Declarations
}

func (c *Checker) declarationMember(className, memberName string, class bool, seen map[string]bool) (declaration.Member, bool) {
	if className == "" || seen[className] {
		return declaration.Member{}, false
	}
	seen[className] = true
	if info := c.classes[className]; info != nil {
		for _, mixinName := range info.mixins {
			if mixin, ok := c.declarations().Module(mixinName); ok {
				if member, exists := mixin.InstanceMembers[memberName]; exists {
					return member, true
				}
			}
		}
		if member, ok := c.declarationMember(info.superclass, memberName, class, seen); ok {
			return member, true
		}
	}
	return c.declarations().Member(className, memberName, class)
}

func (c *Checker) currentDeclarationMember(memberName string) (declaration.Member, bool) {
	if c.current == nil {
		return declaration.Member{}, false
	}
	return c.declarationMember(c.current.name, memberName, false, map[string]bool{})
}

func expressionTypeName(expression ast.Expression) string {
	switch node := expression.(type) {
	case *ast.Identifier:
		return node.Name
	case *ast.MemberExpression:
		prefix := expressionTypeName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	default:
		return ""
	}
}

func (c *Checker) checkFieldInitialization(class *ast.ClassStatement) {
	if c.current == nil || len(c.current.fields) == 0 {
		return
	}
	initialized := map[string]bool{}
	for name, field := range c.current.fields {
		initialized[name] = field.Value != nil
	}
	initialize := c.current.methods["initialize"]
	if initialize != nil {
		walkAssignments(initialize.Body, func(assignment *ast.AssignmentStatement) {
			if identifier, ok := assignment.Target.(*ast.Identifier); ok && strings.HasPrefix(identifier.Name, "@") {
				initialized[identifier.Name] = true
			}
		})
	}
	for name, ok := range initialized {
		if !ok {
			c.error(c.current.fields[name].Span(), fmt.Sprintf("field %s must be initialized in initialize() or at its declaration", name))
		}
	}
}

func walkAssignments(statements []ast.Statement, visit func(*ast.AssignmentStatement)) {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ast.AssignmentStatement:
			visit(n)
		case *ast.IfStatement:
			walkAssignments(n.Then, visit)
			for _, branch := range n.ElseIf {
				walkAssignments(branch.Body, visit)
			}
			walkAssignments(n.Else, visit)
		}
	}
}

func (c *Checker) checkExpression(expression ast.Expression, sc *scope) types.Type {
	if expression == nil {
		return types.Type{Kind: types.Void, Name: "Void"}
	}
	if typ, exists := c.result.Expressions[expression]; exists {
		return typ
	}
	typ := types.Type{Kind: types.Any, Name: "Any"}
	switch n := expression.(type) {
	case *ast.Identifier:
		if value, ok := sc.lookup(n.Name); ok {
			typ = value.typ
		} else if binding, ok := c.resolution.Symbols[n.Name]; ok {
			typ = binding.Type()
			c.result.References[n] = binding
		} else if member, ok := c.currentDeclarationMember(n.Name); ok {
			typ = member.Return
			c.external[n] = member
		} else if strings.HasPrefix(n.Name, "@") && c.current != nil {
			if field, ok := c.current.fields[n.Name]; ok {
				typ = fromTypeRef(field.Type)
			}
		} else if isConstant(n.Name) {
			typ = types.FromName(n.Name)
		}
	case *ast.Literal:
		switch n.Kind {
		case ast.StringLiteral:
			typ = types.FromName("String")
		case ast.IntegerLiteral:
			typ = types.FromName("Integer")
		case ast.FloatLiteral:
			typ = types.FromName("Float")
		case ast.BooleanLiteral:
			typ = types.FromName("Boolean")
		case ast.NilLiteral:
			typ = types.FromName("Nil")
		}
	case *ast.InterpolatedString:
		for _, part := range n.Parts {
			if part.Expression != nil {
				c.checkExpression(part.Expression, sc)
			}
		}
		typ = types.FromName("String")
	case *ast.SymbolLiteral:
		typ = types.FromName("String")
	case *ast.ArrayLiteral:
		element := types.Type{Kind: types.Any, Name: "Any"}
		if len(n.Elements) > 0 {
			element = c.checkExpression(n.Elements[0], sc)
			for _, item := range n.Elements[1:] {
				if current := c.checkExpression(item, sc); !types.Assignable(element, current) || !types.Assignable(current, element) {
					element = types.Type{Kind: types.Any, Name: "Any"}
				}
			}
		}
		typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{element}}
	case *ast.HashLiteral:
		for _, entry := range n.Entries {
			c.checkExpression(entry.Key, sc)
			c.checkExpression(entry.Value, sc)
		}
		typ = types.Type{Kind: types.Hash, Name: "Hash"}
	case *ast.UnaryExpression:
		operand := c.checkExpression(n.Operand, sc)
		if n.Operator == "!" || n.Operator == "not" {
			typ = types.FromName("Boolean")
		} else {
			typ = operand
		}
	case *ast.BinaryExpression:
		left := c.checkExpression(n.Left, sc)
		c.checkExpression(n.Right, sc)
		switch n.Operator {
		case "==", "!=", "<", "<=", ">", ">=", "=~", "!~", "&&", "||", "and", "or":
			typ = types.FromName("Boolean")
		default:
			typ = left
		}
	case *ast.MemberExpression:
		receiverType := c.checkExpression(n.Receiver, sc)
		if identifier, ok := n.Receiver.(*ast.Identifier); ok {
			if imported := c.resolution.Packages[identifier.Name]; imported != nil {
				if binding, exists := c.resolution.Member(identifier.Name, n.Name); exists {
					typ = binding.Type()
					c.result.References[n] = binding
				} else {
					c.error(n.Span(), fmt.Sprintf("package %s does not export %s", imported.Path, n.Name))
				}
				break
			}
		}
		if strings.HasPrefix(n.Name, "_") {
			self, ok := n.Receiver.(*ast.Identifier)
			if !ok || (self.Name != "self" && !strings.HasPrefix(self.Name, "@")) {
				c.error(n.Span(), fmt.Sprintf("private member %s cannot be accessed externally", n.Name))
			}
		}
		if record := c.records[receiverType.Name]; record != nil && record.byName[n.Name] != nil {
			typ = fromTypeRef(record.byName[n.Name].Type)
		} else if memberType, _, found := c.localMember(receiverType.Name, n.Name, map[string]bool{}); found {
			typ = memberType
		} else if binding, exists := c.importedAncestorMember(receiverType.Name, n.Name, map[string]bool{}); exists {
			typ = binding.Type()
			c.result.References[n] = binding
		} else if member, exists := c.declarationMember(receiverType.Name, n.Name, declarationClassAccess(n.Receiver), map[string]bool{}); exists {
			typ = member.Return
			c.external[n] = member
		} else if n.Name != "new" {
			if imported, exists := c.resolution.ImportedType(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiverType.Name, imported.Import.Path, n.Name))
			} else if declared, exists := c.declarations().Type(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("type %s provided by Rails has no member %s", declared.Name, n.Name))
			}
		}
	case *ast.CallExpression:
		calleeType := c.checkExpression(n.Callee, sc)
		argumentTypes := make([]types.Type, 0, len(n.Arguments))
		for _, arg := range n.Arguments {
			argumentTypes = append(argumentTypes, c.checkExpression(arg.Value, sc))
		}
		typ = calleeType
		if binding, ok := c.result.References[n.Callee]; ok {
			typ = binding.Type()
			c.checkImportedArguments(n.Span(), binding, n.Arguments, argumentTypes)
			if binding.Library != nil {
				typ = inferLibraryReturn(*binding.Library, argumentTypes)
			}
		}
		if member, ok := c.external[n.Callee]; ok {
			typ = c.checkDeclarationArguments(n.Span(), member, n.Arguments, argumentTypes)
		}
		if member, ok := n.Callee.(*ast.MemberExpression); ok && member.Name == "new" {
			if identifier, ok := member.Receiver.(*ast.Identifier); ok {
				typ = types.FromName(identifier.Name)
				if binding, imported := c.result.References[identifier]; imported && binding.Export != nil {
					if binding.Export.Kind == resolver.RecordExport {
						c.checkImportedRecordArguments(n, binding.Export)
					} else {
						c.checkImportedArguments(n.Span(), binding, n.Arguments, argumentTypes)
					}
				} else if record := c.records[identifier.Name]; record != nil {
					c.checkLocalRecordArguments(n, record)
				} else if info := c.classes[identifier.Name]; info != nil {
					c.checkArguments(n.Span(), info.methods["initialize"], n.Arguments, argumentTypes)
				}
			}
		} else if member, ok := n.Callee.(*ast.MemberExpression); ok {
			receiverType := c.checkExpression(member.Receiver, sc)
			if _, method, found := c.localMember(receiverType.Name, member.Name, map[string]bool{}); found && method != nil {
				typ = methodReturnType(method)
				c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
			}
		}
		if identifier, ok := n.Callee.(*ast.Identifier); ok {
			if _, imported := c.result.References[identifier]; imported {
				// The resolved import signature was checked above.
			} else if _, provided := c.external[identifier]; provided {
				// The library-provider signature was checked above.
			} else if c.current != nil && c.current.methods[identifier.Name] != nil {
				method := c.current.methods[identifier.Name]
				typ = methodReturnType(method)
				c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
			} else if method := c.functions[identifier.Name]; method != nil {
				typ = methodReturnType(method)
				c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
			} else if c.mode != "ruby" {
				c.error(identifier.Span(), fmt.Sprintf("function %s is not declared or imported", identifier.Name))
			} else if !c.resolution.NativeSyntax {
				c.error(identifier.Span(), fmt.Sprintf("Ruby function %s requires an explicit platform import", identifier.Name))
			}
		}
	case *ast.IndexExpression:
		receiver := c.checkExpression(n.Receiver, sc)
		indexType := c.checkExpression(n.Index, sc)
		if receiver.Kind == types.Array && len(receiver.Args) > 0 {
			typ = receiver.Args[0]
		} else if receiver.Name == "Tuple" {
			if literal, ok := n.Index.(*ast.Literal); ok && literal.Kind == ast.IntegerLiteral {
				if index, ok := integerLiteral(literal.Raw); ok && index >= 0 && index < len(receiver.Args) {
					typ = receiver.Args[index]
				}
			}
		} else if member, ok := c.declarationMember(receiver.Name, "[]", false, map[string]bool{}); ok {
			typ = c.checkDeclarationArguments(n.Span(), member, []ast.CallArgument{{Value: n.Index}}, []types.Type{indexType})
		}
	case *ast.BlockExpression:
		c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
	case *ast.NativeExpression:
		if c.mode != "ruby" {
			c.error(n.Span(), "Ruby-native expression is only available in mode: ruby")
		} else if !c.resolution.NativeSyntax {
			c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
		}
	}
	c.result.Expressions[expression] = typ
	return typ
}

func (c *Checker) checkLocalRecordArguments(call *ast.CallExpression, record *recordInfo) {
	fields := make([]resolver.RecordField, len(record.fields))
	for index, field := range record.fields {
		fields[index] = resolver.RecordField{Name: field.Name, Type: fromTypeRef(field.Type)}
	}
	c.checkRecordArguments(call, record.name, fields)
}

func (c *Checker) checkImportedRecordArguments(call *ast.CallExpression, record *resolver.Export) {
	c.checkRecordArguments(call, record.Name, record.Fields)
}

func (c *Checker) checkRecordArguments(call *ast.CallExpression, name string, fields []resolver.RecordField) {
	byName := map[string]resolver.RecordField{}
	for _, field := range fields {
		byName[field.Name] = field
	}
	used := map[string]bool{}
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			c.error(argument.Value.Span(), fmt.Sprintf("%s.new() uses keyword-only record fields", name))
			continue
		}
		field, ok := byName[argument.Name]
		if !ok {
			c.error(argument.Value.Span(), fmt.Sprintf("record %s has no field %s", name, argument.Name))
			continue
		}
		if used[argument.Name] {
			c.error(argument.Value.Span(), fmt.Sprintf("record field %s is provided more than once", argument.Name))
			continue
		}
		used[argument.Name] = true
		actual := c.result.Expressions[argument.Value]
		if !types.Assignable(field.Type, actual) {
			c.error(argument.Value.Span(), fmt.Sprintf("record field %s has type %s, expected %s", field.Name, actual, field.Type))
		}
	}
	for _, field := range fields {
		if !used[field.Name] {
			c.error(call.Span(), fmt.Sprintf("%s.new() is missing record field %s", name, field.Name))
		}
	}
}

func (c *Checker) checkImportedArguments(span token.Span, binding resolver.Binding, arguments []ast.CallArgument, actual []types.Type) {
	var parameters []types.Type
	required := 0
	variadic := false
	name := binding.Name
	if binding.Library != nil {
		for _, parameter := range binding.Library.Parameters {
			parameters = append(parameters, parameter.Type)
			if !parameter.Optional {
				required++
			}
		}
		variadic = binding.Library.Variadic
	} else if binding.Export != nil {
		parameters = append(parameters, binding.Export.Parameters...)
		required = binding.Export.Required
		variadic = binding.Export.Variadic
	} else if binding.Member != nil {
		parameters = append(parameters, binding.Member.Parameters...)
		required = binding.Member.Required
		variadic = binding.Member.Variadic
	}
	if len(arguments) < required || (!variadic && len(arguments) > len(parameters)) {
		if variadic {
			c.error(span, fmt.Sprintf("%s() expects at least %d arguments, got %d", name, required, len(arguments)))
		} else {
			c.error(span, fmt.Sprintf("%s() expects %d..%d arguments, got %d", name, required, len(parameters), len(arguments)))
		}
		return
	}
	for i, actualType := range actual {
		parameterIndex := i
		if parameterIndex >= len(parameters) {
			parameterIndex = len(parameters) - 1
		}
		if parameterIndex < 0 {
			continue
		}
		expected := parameters[parameterIndex]
		if !types.Assignable(expected, actualType) {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, name, actualType, expected))
		}
	}
}

func inferLibraryReturn(symbol stdlib.Symbol, arguments []types.Type) types.Type {
	argument := func(index int) types.Type {
		if index < 0 || index >= len(arguments) {
			return symbol.Return
		}
		return arguments[index]
	}
	switch symbol.Inference {
	case "argument_1":
		return argument(1)
	case "array_of_argument_1":
		return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{argument(1)}}
	default:
		return symbol.Return
	}
}

func (c *Checker) checkDeclarationArguments(span token.Span, member declaration.Member, arguments []ast.CallArgument, actual []types.Type) types.Type {
	bindings := map[string]types.Type{}
	typeParameters := map[string]bool{}
	for _, name := range member.TypeParameters {
		typeParameters[name] = true
	}
	byName := map[string]declaration.Parameter{}
	for _, parameter := range member.Parameters {
		byName[parameter.Name] = parameter
	}
	used := map[string]bool{}
	position := 0
	for index, argument := range arguments {
		var parameter declaration.Parameter
		found := false
		if argument.Name != "" {
			parameter, found = byName[argument.Name]
			if found {
				used[parameter.Name] = true
			}
		} else {
			for position < len(member.Parameters) && member.Parameters[position].Keyword {
				position++
			}
			if position < len(member.Parameters) {
				parameter, found = member.Parameters[position], true
				used[parameter.Name] = true
				position++
			}
		}
		if !found && member.Variadic && len(member.Parameters) > 0 {
			parameter, found = member.Parameters[len(member.Parameters)-1], true
		}
		if !found {
			if argument.Name != "" {
				c.error(argument.Value.Span(), fmt.Sprintf("%s() has no keyword argument %s", member.Name, argument.Name))
			} else {
				c.error(span, fmt.Sprintf("%s() expects at most %d arguments, got %d", member.Name, len(member.Parameters), len(arguments)))
			}
			continue
		}
		bindDeclarationType(parameter.Type, actual[index], typeParameters, bindings)
		expected := instantiateDeclarationType(parameter.Type, bindings)
		if !types.Assignable(expected, actual[index]) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, member.Name, actual[index], expected))
		}
	}
	for _, parameter := range member.Parameters {
		if !parameter.Optional && !used[parameter.Name] && !member.Variadic {
			c.error(span, fmt.Sprintf("%s() is missing required argument %s", member.Name, parameter.Name))
		}
	}
	return instantiateDeclarationType(member.Return, bindings)
}

func bindDeclarationType(pattern, actual types.Type, parameters map[string]bool, bindings map[string]types.Type) {
	if parameters[pattern.Name] {
		if _, exists := bindings[pattern.Name]; !exists {
			bindings[pattern.Name] = actual
		}
		return
	}
	if pattern.Name != actual.Name || len(pattern.Args) != len(actual.Args) {
		return
	}
	for index := range pattern.Args {
		bindDeclarationType(pattern.Args[index], actual.Args[index], parameters, bindings)
	}
}

func instantiateDeclarationType(input types.Type, bindings map[string]types.Type) types.Type {
	if replacement, ok := bindings[input.Name]; ok {
		replacement.Nullable = replacement.Nullable || input.Nullable
		return replacement
	}
	result := input
	result.Args = make([]types.Type, len(input.Args))
	for index, argument := range input.Args {
		result.Args[index] = instantiateDeclarationType(argument, bindings)
	}
	return result
}

func includedModule(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) != 2 || fields[0] != "include" {
		return ""
	}
	return strings.TrimSuffix(fields[1], ",")
}

func declarationClassAccess(expression ast.Expression) bool {
	switch node := expression.(type) {
	case *ast.Identifier:
		return isConstant(node.Name)
	case *ast.MemberExpression:
		return node.Namespace && declarationClassAccess(node.Receiver)
	default:
		return false
	}
}

func integerLiteral(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.ReplaceAll(raw, "_", ""))
	return value, err == nil
}

func methodReturnType(method *ast.MethodStatement) types.Type {
	if method == nil || method.ReturnType.Empty() {
		return types.Type{Kind: types.Void, Name: "Void"}
	}
	return fromTypeRef(method.ReturnType)
}

func (c *Checker) checkArguments(span token.Span, method *ast.MethodStatement, arguments []ast.CallArgument, actual []types.Type) {
	if method == nil {
		if len(arguments) > 0 {
			c.error(span, "constructor takes no arguments")
		}
		return
	}
	required := 0
	variadic := false
	for _, parameter := range method.Parameters {
		if parameter.Rest || parameter.KeywordRest {
			variadic = true
			continue
		}
		if parameter.Default == nil && !parameter.Keyword {
			required++
		}
	}
	if len(arguments) < required || (!variadic && len(arguments) > len(method.Parameters)) {
		c.error(span, fmt.Sprintf("%s() expects %d..%d arguments, got %d", method.Name, required, len(method.Parameters), len(arguments)))
		return
	}
	for i, argumentType := range actual {
		if i >= len(method.Parameters) {
			break
		}
		expected := fromTypeRef(method.Parameters[i].Type)
		if method.Parameters[i].Type.Empty() || method.Parameters[i].Rest || method.Parameters[i].KeywordRest {
			continue
		}
		if !types.Assignable(expected, argumentType) {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, method.Name, argumentType, expected))
		}
	}
}

func fromTypeRef(ref ast.TypeRef) types.Type {
	t := types.FromName(ref.Name)
	t.Nullable = ref.Nullable
	for _, argument := range ref.Arguments {
		t.Args = append(t.Args, fromTypeRef(argument))
	}
	if ref.Array {
		t = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{t}, Nullable: ref.Nullable}
	}
	return t
}

func isConstant(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func (c *Checker) error(span token.Span, message string) {
	c.diags = append(c.diags, diagnostic.Diagnostic{Severity: diagnostic.Error, Message: message, Span: span})
}
