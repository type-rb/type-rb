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
	Program             *ast.Program
	Expressions         map[ast.Expression]types.Type
	Variables           map[*ast.VariableStatement]types.Type
	Iterations          map[*ast.IterationExpression]types.Type
	Constants           map[ast.Expression]string
	ConstantOwners      map[*ast.VariableStatement]string
	Resolution          resolver.Result
	References          map[ast.Expression]resolver.Binding
	EnumConstructors    map[*ast.CallExpression]EnumVariant
	CasePatterns        map[ast.Expression]CasePattern
	GenericApplications map[*ast.GenericExpression]GenericApplication
}

type EnumField struct {
	Name string
	Type types.Type
}

type EnumVariant struct {
	EnumName      string
	Name          string
	Fields        []EnumField
	TypeArguments []types.Type
	Reference     *resolver.Binding
}

type GenericApplication struct {
	Name           string
	Kind           string
	TypeParameters []string
	TypeArguments  []types.Type
	Parameters     []types.Type
	ReturnType     types.Type
	Required       int
	Variadic       bool
}

type CaseBinding struct {
	Name  string
	Field EnumField
}

type CasePattern struct {
	Variant     EnumVariant
	Bindings    []CaseBinding
	PayloadEnum bool
}

type symbol struct {
	typ      types.Type
	mutable  bool
	constant bool
	owner    string
	span     token.Span
	variable *ast.VariableStatement
}

type scope struct {
	parent           *scope
	values           map[string]symbol
	constantsAllowed bool
	constantOwner    string
	enumsAllowed     bool
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

type classMember struct {
	typ    types.Type
	method *ast.MethodStatement
	field  *ast.FieldStatement
}

type recordInfo struct {
	name   string
	fields []*ast.RecordFieldStatement
	byName map[string]*ast.RecordFieldStatement
}

type enumInfo struct {
	name           string
	typeParameters []string
	members        []string
	byName         map[string]*ast.EnumMemberStatement
}

type typeDeclaration struct {
	kind           string
	span           token.Span
	typeParameters []string
}

type Checker struct {
	mode            string
	result          Result
	diags           []diagnostic.Diagnostic
	classes         map[string]*classInfo
	records         map[string]*recordInfo
	enums           map[string]*enumInfo
	interfaces      map[string]*ast.InterfaceStatement
	functions       map[string]*ast.MethodStatement
	current         *classInfo
	classMethod     bool
	initializing    int
	loopDepth       int
	moduleDepth     int
	returns         []types.Type
	resolution      resolver.Result
	external        map[ast.Expression]declaration.Member
	declaredTypes   map[string]typeDeclaration
	enumCallee      int
	enumPattern     int
	enumPatternType types.Type
}

func Check(program *ast.Program, resolution resolver.Result) (Result, []diagnostic.Diagnostic) {
	c := &Checker{
		mode: program.Mode,
		result: Result{
			Program:             program,
			Expressions:         map[ast.Expression]types.Type{},
			Variables:           map[*ast.VariableStatement]types.Type{},
			Iterations:          map[*ast.IterationExpression]types.Type{},
			Constants:           map[ast.Expression]string{},
			ConstantOwners:      map[*ast.VariableStatement]string{},
			Resolution:          resolution,
			References:          map[ast.Expression]resolver.Binding{},
			EnumConstructors:    map[*ast.CallExpression]EnumVariant{},
			CasePatterns:        map[ast.Expression]CasePattern{},
			GenericApplications: map[*ast.GenericExpression]GenericApplication{},
		},
		classes:       map[string]*classInfo{},
		records:       map[string]*recordInfo{},
		enums:         map[string]*enumInfo{},
		interfaces:    map[string]*ast.InterfaceStatement{},
		functions:     map[string]*ast.MethodStatement{},
		resolution:    resolution,
		external:      map[ast.Expression]declaration.Member{},
		declaredTypes: map[string]typeDeclaration{},
	}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok {
			c.functions[method.Name] = method
		}
	}
	c.collect(program.Statements)
	c.validateTypeReferences(program.Statements)
	c.checkStatements(program.Statements, &scope{values: map[string]symbol{}, constantsAllowed: true, enumsAllowed: true})
	return c.result, c.diags
}

func (c *Checker) validateTypeReferences(statements []ast.Statement) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			c.validateTypeReferences(node.Body)
		case *ast.RecordStatement:
			c.validateTypeReferences(node.Body)
		case *ast.EnumStatement:
			c.validateTypeReferences(node.Body)
		case *ast.EnumMemberStatement:
			for _, parameter := range node.Parameters {
				c.validateTypeReference(parameter.Type)
			}
		case *ast.RecordFieldStatement:
			c.validateTypeReference(node.Type)
		case *ast.ModuleStatement:
			c.validateTypeReferences(node.Body)
		case *ast.InterfaceStatement:
			for _, method := range node.Methods {
				c.validateMethodTypes(method)
			}
		case *ast.FieldStatement:
			c.validateTypeReference(node.Type)
		case *ast.MethodStatement:
			c.validateMethodTypes(node)
			c.validateTypeReferences(node.Body)
		case *ast.VariableStatement:
			c.validateTypeReference(node.Type)
		case *ast.IfStatement:
			c.validateTypeReferences(node.Then)
			for _, branch := range node.ElseIf {
				c.validateTypeReferences(branch.Body)
			}
			c.validateTypeReferences(node.Else)
		case *ast.CaseStatement:
			c.validateTypeReferences(node.Leading)
			for _, branch := range node.Branches {
				c.validateTypeReferences(branch.Body)
			}
			c.validateTypeReferences(node.Else)
		case *ast.WhileStatement:
			c.validateTypeReferences(node.Body)
		case *ast.ExpressionStatement:
			if iteration, ok := node.Expression.(*ast.IterationExpression); ok && iteration.Block != nil {
				c.validateTypeReferences(iteration.Block.Body)
			}
		case *ast.NativeBlock:
			c.validateTypeReferences(node.Body)
		}
	}
}

func (c *Checker) validateMethodTypes(method *ast.MethodStatement) {
	c.validateTypeReference(method.ReturnType)
	for _, parameter := range method.Parameters {
		c.validateTypeReference(parameter.Type)
	}
}

func (c *Checker) validateTypeReference(ref ast.TypeRef) {
	if ref.Empty() {
		return
	}
	for _, argument := range ref.Arguments {
		c.validateTypeReference(argument)
	}
	if expected, generic := c.genericTypeArity(ref.Name); generic {
		if len(ref.Arguments) != expected {
			c.error(ref.Span(), fmt.Sprintf("%s expects %d type argument(s), got %d", ref.Name, expected, len(ref.Arguments)))
		}
	} else if declaration, declared := c.declaredTypes[ref.Name]; declared && len(ref.Arguments) > 0 {
		c.error(ref.Span(), fmt.Sprintf("%s is not generic", declaration.kind+" "+ref.Name))
	} else if binding, imported := c.resolution.ImportedType(ref.Name); imported && len(binding.Export.TypeParameters) == 0 && len(ref.Arguments) > 0 {
		c.error(ref.Span(), fmt.Sprintf("%s is not generic", ref.Name))
	}
	if types.FromName(ref.Name).Kind != types.Hash {
		return
	}
	if len(ref.Arguments) == 0 && c.rubyNativeSyntax() {
		return
	}
	if len(ref.Arguments) != 2 {
		c.error(ref.Span(), fmt.Sprintf("Hash expects two type arguments, got %d", len(ref.Arguments)))
		return
	}
	key := fromTypeRef(ref.Arguments[0])
	if !portableHashKey(key) {
		c.error(ref.Arguments[0].Span(), fmt.Sprintf("Hash key type must be String or Integer, got %s", key))
	}
}

func (c *Checker) genericTypeArity(name string) (int, bool) {
	if declaration, ok := c.declaredTypes[name]; ok && len(declaration.typeParameters) > 0 {
		return len(declaration.typeParameters), true
	}
	if binding, ok := c.resolution.ImportedType(name); ok && len(binding.Export.TypeParameters) > 0 {
		return len(binding.Export.TypeParameters), true
	}
	return 0, false
}

func (c *Checker) collect(statements []ast.Statement) {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ast.ClassStatement:
			if !c.declareType(n.Name, "class", n.Span()) {
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
			if !c.declareType(n.Name, "record", n.Span()) {
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
		case *ast.EnumStatement:
			if !c.declareType(n.Name, "enum", n.Span()) {
				continue
			}
			info := &enumInfo{name: n.Name, byName: map[string]*ast.EnumMemberStatement{}}
			for _, parameter := range n.TypeParameters {
				info.typeParameters = append(info.typeParameters, parameter.Name)
			}
			declaration := c.declaredTypes[n.Name]
			declaration.typeParameters = append([]string(nil), info.typeParameters...)
			c.declaredTypes[n.Name] = declaration
			for _, statement := range n.Body {
				member, ok := statement.(*ast.EnumMemberStatement)
				if !ok {
					continue
				}
				if previous := info.byName[member.Name]; previous != nil {
					c.error(member.Span(), fmt.Sprintf("enum member %s was already declared at %s", member.Name, previous.Span().Start))
					continue
				}
				info.members = append(info.members, member.Name)
				info.byName[member.Name] = member
			}
			c.enums[n.Name] = info
		case *ast.InterfaceStatement:
			if c.declareType(n.Name, "interface", n.Span()) {
				c.interfaces[n.Name] = n
			}
		case *ast.ModuleStatement:
			c.collect(n.Body)
		case *ast.NativeBlock:
			c.collect(n.Body)
		}
	}
}

func (c *Checker) declareType(name, kind string, span token.Span) bool {
	if previous, exists := c.declaredTypes[name]; exists {
		c.error(span, fmt.Sprintf("type %s is already declared as %s at %s", name, previous.kind, previous.span.Start))
		return false
	}
	c.declaredTypes[name] = typeDeclaration{kind: kind, span: span}
	return true
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
			owner := n.Name
			if sc.constantOwner != "" {
				owner = sc.constantOwner + "::" + n.Name
			}
			classScope := &scope{parent: sc, values: map[string]symbol{"self": {typ: types.FromName(n.Name)}}, constantsAllowed: true, constantOwner: owner}
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
		case *ast.EnumStatement:
			if !sc.enumsAllowed {
				c.error(n.Span(), fmt.Sprintf("enum %s may only be declared at top level or directly inside a module", n.Name))
			}
			if !isConstant(n.Name) {
				c.error(n.Span(), "enum name must begin with an uppercase letter")
			}
			c.checkTypeParameters(n.TypeParameters)
			info := c.enums[n.Name]
			if info != nil && len(info.members) == 0 {
				c.error(n.Span(), fmt.Sprintf("enum %s must declare at least one member", n.Name))
			}
			for _, statement := range n.Body {
				member, ok := statement.(*ast.EnumMemberStatement)
				if !ok {
					continue
				}
				if !isConstant(member.Name) {
					c.error(member.Span(), "enum member must begin with an uppercase letter")
				}
				if len(n.TypeParameters) > 0 && len(member.Parameters) == 0 {
					c.error(member.Span(), "payloadless members of generic enums are reserved until typed singleton construction is defined")
				}
				seenFields := map[string]bool{}
				for _, parameter := range member.Parameters {
					if parameter.Name == "" || parameter.Type.Empty() {
						c.error(parameter.Span(), fmt.Sprintf("enum payload %s requires a name and type", parameter.Name))
					}
					if parameter.Default != nil || parameter.Keyword || parameter.Rest || parameter.KeywordRest {
						c.error(parameter.Span(), "enum payload fields must be required positional values")
					}
					if seenFields[parameter.Name] {
						c.error(parameter.Span(), fmt.Sprintf("enum payload field %s is duplicated", parameter.Name))
					}
					seenFields[parameter.Name] = true
				}
			}
		case *ast.EnumMemberStatement:
			// Checked as part of its enclosing enum.
		case *ast.RecordFieldStatement:
			// Checked as part of its enclosing record.
		case *ast.ModuleStatement:
			owner := n.Name
			if sc.constantOwner != "" {
				owner = sc.constantOwner + "::" + n.Name
			}
			c.moduleDepth++
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}, constantsAllowed: true, constantOwner: owner, enumsAllowed: true})
			c.moduleDepth--
		case *ast.InterfaceStatement:
			for _, method := range n.Methods {
				c.checkMethod(method, sc)
			}
		case *ast.FieldStatement:
			if n.Value != nil {
				valueType := c.checkExpression(n.Value, sc)
				declared := fromTypeRef(n.Type)
				valueType = c.contextualizeHashLiteral(n.Value, declared, valueType)
				if !n.Type.Empty() && !types.Assignable(declared, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot initialize %s with %s", declared, valueType))
				}
			}
		case *ast.MethodStatement:
			c.checkMethod(n, sc)
		case *ast.VariableStatement:
			valueType := c.checkExpression(n.Value, sc)
			variableType := valueType
			if n.Constant {
				if n.Mutable {
					c.error(n.Span(), fmt.Sprintf("constant %s cannot be declared with mut", n.Name))
				}
				if !sc.constantsAllowed {
					c.error(n.Span(), fmt.Sprintf("constant %s may only be declared at top level or directly inside a module or class", n.Name))
				}
			}
			if !n.Type.Empty() {
				variableType = fromTypeRef(n.Type)
				valueType = c.contextualizeHashLiteral(n.Value, variableType, valueType)
				if !types.Assignable(variableType, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot assign %s to %s", valueType, variableType))
				}
			}
			if n.Mutable && valueType.Readonly && isReferenceType(variableType) {
				c.error(n.Value.Span(), fmt.Sprintf("cannot initialize mutable %s from an immutable value", n.Name))
			}
			if !n.Mutable && isReferenceType(variableType) {
				variableType.Readonly = true
			}
			if previous, exists := sc.values[n.Name]; exists {
				c.error(n.Span(), fmt.Sprintf("%s was already declared at %s; use = to reassign", n.Name, previous.span.Start))
			} else {
				if n.Constant {
					variableType.Readonly = true
				}
				sc.values[n.Name] = symbol{typ: variableType, mutable: n.Mutable && !n.Constant, constant: n.Constant, owner: sc.constantOwner, span: n.Span(), variable: n}
			}
			if n.Constant {
				c.result.ConstantOwners[n] = sc.constantOwner
			}
			c.result.Variables[n] = variableType
		case *ast.AssignmentStatement:
			leftType := c.checkExpression(n.Target, sc)
			rightType := c.checkExpression(n.Value, sc)
			rightType = c.contextualizeHashLiteral(n.Value, leftType, rightType)
			if identifier, ok := n.Target.(*ast.Identifier); ok && !strings.HasPrefix(identifier.Name, "@") {
				if _, exists := sc.lookup(identifier.Name); !exists {
					_, imported := c.result.References[identifier]
					if !imported && !c.rubyNativeSyntax() {
						c.error(identifier.Span(), fmt.Sprintf("%s is not declared", identifier.Name))
					}
					if !imported && c.rubyNativeSyntax() {
						// Explicit Ruby-native imports expose framework setters and
						// legacy Ruby assignments that have no TypeRB declaration.
						leftType = types.Type{Kind: types.Any, Name: "Any"}
					}
				}
			}
			if member, ok := n.Target.(*ast.MemberExpression); ok && c.readonlyClassField(member, sc) {
				c.error(member.Span(), fmt.Sprintf("field %s is readonly", member.Name))
			} else {
				c.requireMutable(n.Target, sc, "assignment")
			}
			assignedType := rightType
			if n.Operator != "=" {
				if index, ok := n.Target.(*ast.IndexExpression); ok && c.result.Expressions[index.Receiver].Kind == types.Hash {
					c.error(n.Span(), "Hash entry compound assignment is not supported; read and assign an explicit value")
					assignedType = types.Type{Kind: types.Invalid, Name: "Invalid"}
				} else {
					assignedType = c.checkBinaryOperator(n.Span(), strings.TrimSuffix(n.Operator, "="), leftType, rightType)
				}
			}
			if leftType.Kind != types.Any && !types.Assignable(leftType, assignedType) {
				c.error(n.Value.Span(), fmt.Sprintf("cannot assign %s to %s", assignedType, leftType))
			}
		case *ast.ReturnStatement:
			actual := types.Type{Kind: types.Void, Name: "Void"}
			if n.Value != nil {
				actual = c.checkExpression(n.Value, sc)
			}
			if len(c.returns) > 0 {
				expected := c.returns[len(c.returns)-1]
				actual = c.contextualizeHashLiteral(n.Value, expected, actual)
				if !types.Assignable(expected, actual) {
					c.error(n.Span(), fmt.Sprintf("return type is %s, expected %s", actual, expected))
				}
			}
		case *ast.BreakStatement:
			if c.loopDepth == 0 {
				c.error(n.Span(), "break is only valid inside while or an iteration block")
			}
		case *ast.NextStatement:
			if c.loopDepth == 0 {
				c.error(n.Span(), "next is only valid inside while or an iteration block")
			}
		case *ast.ExpressionStatement:
			c.checkExpression(n.Expression, sc)
		case *ast.IfStatement:
			c.checkBooleanCondition(n.Condition, sc, "if")
			c.checkStatements(n.Then, &scope{parent: sc, values: map[string]symbol{}})
			for _, branch := range n.ElseIf {
				c.checkBooleanCondition(branch.Condition, sc, "elsif")
				c.checkStatements(branch.Body, &scope{parent: sc, values: map[string]symbol{}})
			}
			c.checkStatements(n.Else, &scope{parent: sc, values: map[string]symbol{}})
		case *ast.CaseStatement:
			c.checkCase(n, sc)
		case *ast.WhileStatement:
			c.checkBooleanCondition(n.Condition, sc, "while")
			c.loopDepth++
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
			c.loopDepth--
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

func (c *Checker) checkBooleanCondition(expression ast.Expression, sc *scope, construct string) {
	typ := c.checkExpression(expression, sc)
	if typ.Kind == types.Invalid || typ.Kind == types.Bool && !typ.Nullable {
		return
	}
	if typ.Kind == types.Any && c.mode == "ruby" && c.resolution.NativeSyntax {
		// Explicit Ruby-native projects may use values that their provider cannot
		// yet refine beyond Any. Truthiness remains confined to that escape hatch;
		// portable TypeRB conditions are always Boolean.
		return
	}
	c.error(expression.Span(), fmt.Sprintf("%s condition must be Boolean, got %s", construct, typ))
}

func (c *Checker) checkCase(node *ast.CaseStatement, sc *scope) {
	selectorType := c.checkExpression(node.Value, sc)
	variants, enum := c.enumVariants(selectorType)
	if !enum && selectorType.Kind != types.Invalid {
		c.error(node.Value.Span(), fmt.Sprintf("case value must be an enum, got %s", selectorType))
	}
	for _, statement := range node.Leading {
		if _, comment := statement.(*ast.CommentStatement); !comment {
			c.error(statement.Span(), "case statements must be inside a when or else branch")
		}
	}
	c.checkStatements(node.Leading, &scope{parent: sc, values: map[string]symbol{}})

	seen := map[string]bool{}
	for _, branch := range node.Branches {
		previousPatternType := c.enumPatternType
		c.enumPatternType = selectorType
		c.enumPattern++
		branchType := c.checkExpression(branch.Value, sc)
		c.enumPattern--
		c.enumPatternType = previousPatternType
		variant, member := c.caseEnumVariant(branch.Value, selectorType)
		if !member || !types.Equivalent(selectorType, branchType) {
			if selectorType.Kind != types.Invalid {
				c.error(branch.Value.Span(), fmt.Sprintf("when value must be a member of %s", selectorType))
			}
		} else if seen[variant.Name] {
			c.error(branch.Value.Span(), fmt.Sprintf("enum member %s is handled more than once", variant.Name))
		} else {
			seen[variant.Name] = true
		}

		branchScope := &scope{parent: sc, values: map[string]symbol{}}
		if member {
			if len(branch.Bindings) != len(variant.Fields) {
				c.error(branch.Value.Span(), fmt.Sprintf("enum pattern %s::%s expects %d binding(s), got %d", variant.EnumName, variant.Name, len(variant.Fields), len(branch.Bindings)))
			}
			bindings := make([]CaseBinding, 0, len(branch.Bindings))
			for index, binding := range branch.Bindings {
				if index >= len(variant.Fields) {
					break
				}
				if _, duplicate := branchScope.values[binding.Name]; duplicate {
					c.error(binding.Span(), fmt.Sprintf("enum pattern binding %s is duplicated", binding.Name))
					continue
				}
				field := variant.Fields[index]
				branchScope.values[binding.Name] = symbol{typ: field.Type, span: binding.Span()}
				bindings = append(bindings, CaseBinding{Name: binding.Name, Field: field})
			}
			c.result.CasePatterns[branch.Value] = CasePattern{
				Variant:     variant,
				Bindings:    bindings,
				PayloadEnum: enumHasPayload(variants),
			}
		}
		c.checkStatements(branch.Body, branchScope)
	}
	if node.HasElse {
		c.checkStatements(node.Else, &scope{parent: sc, values: map[string]symbol{}})
		return
	}
	if !enum {
		return
	}
	missing := make([]string, 0, len(variants))
	for _, variant := range variants {
		if !seen[variant.Name] {
			missing = append(missing, variant.Name)
		}
	}
	if len(missing) > 0 {
		c.error(node.Span(), fmt.Sprintf("case for %s is not exhaustive; missing %s", selectorType, strings.Join(missing, ", ")))
	}
}

func (c *Checker) enumVariants(typ types.Type) ([]EnumVariant, bool) {
	if typ.Nullable {
		return nil, false
	}
	if info := c.enums[typ.Name]; info != nil {
		substitutions := typeSubstitutions(info.typeParameters, typ.Args)
		variants := make([]EnumVariant, 0, len(info.members))
		for _, name := range info.members {
			member := info.byName[name]
			variant := EnumVariant{EnumName: typ.Name, Name: name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, parameter := range member.Parameters {
				variant.Fields = append(variant.Fields, EnumField{Name: parameter.Name, Type: substituteType(fromTypeRef(parameter.Type), substitutions)})
			}
			variants = append(variants, variant)
		}
		return variants, true
	}
	if binding, ok := c.resolution.ImportedType(typ.Name); ok && binding.Export.Kind == resolver.EnumExport {
		substitutions := typeSubstitutions(binding.Export.TypeParameters, typ.Args)
		variants := make([]EnumVariant, 0, len(binding.Export.EnumVariants))
		for _, imported := range binding.Export.EnumVariants {
			variant := EnumVariant{EnumName: typ.Name, Name: imported.Name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, field := range imported.Fields {
				variant.Fields = append(variant.Fields, EnumField{Name: field.Name, Type: substituteType(field.Type, substitutions)})
			}
			if reference, exists := c.resolution.TypeMember(typ.Name, imported.Name); exists {
				variant.Reference = &reference
			}
			variants = append(variants, variant)
		}
		// Catalogs produced before payload metadata still expose member names.
		if len(variants) == 0 {
			for _, name := range binding.Export.EnumMembers {
				variants = append(variants, EnumVariant{EnumName: typ.Name, Name: name})
			}
		}
		return variants, true
	}
	return nil, false
}

func (c *Checker) enumMembers(typ types.Type) ([]string, bool) {
	variants, ok := c.enumVariants(typ)
	if !ok {
		return nil, false
	}
	members := make([]string, len(variants))
	for index, variant := range variants {
		members[index] = variant.Name
	}
	return members, true
}

func (c *Checker) caseEnumVariant(expression ast.Expression, selectorType types.Type) (EnumVariant, bool) {
	member, ok := expression.(*ast.MemberExpression)
	if !ok || !member.Namespace {
		return EnumVariant{}, false
	}
	receiverType := c.result.Expressions[member.Receiver]
	if !types.Equivalent(receiverType, selectorType) {
		return EnumVariant{}, false
	}
	variants, ok := c.enumVariants(selectorType)
	if !ok {
		return EnumVariant{}, false
	}
	for _, variant := range variants {
		if variant.Name == member.Name {
			return variant, true
		}
	}
	return EnumVariant{}, false
}

func enumHasPayload(variants []EnumVariant) bool {
	for _, variant := range variants {
		if len(variant.Fields) > 0 {
			return true
		}
	}
	return false
}

func enumVariantNamed(variants []EnumVariant, name string) (EnumVariant, bool) {
	for _, variant := range variants {
		if variant.Name == name {
			return variant, true
		}
	}
	return EnumVariant{}, false
}

func (c *Checker) enumVariantForMember(member *ast.MemberExpression) (EnumVariant, bool) {
	if member == nil || !member.Namespace {
		return EnumVariant{}, false
	}
	receiverType := c.result.Expressions[member.Receiver]
	variants, enum := c.enumVariants(receiverType)
	if !enum {
		return EnumVariant{}, false
	}
	return enumVariantNamed(variants, member.Name)
}

func (c *Checker) checkEnumConstructor(call *ast.CallExpression, variant EnumVariant, arguments []types.Type) {
	if len(variant.Fields) == 0 {
		c.error(call.Span(), fmt.Sprintf("enum member %s::%s has no payload and is not callable", variant.EnumName, variant.Name))
		return
	}
	if len(call.Arguments) != len(variant.Fields) {
		c.error(call.Span(), fmt.Sprintf("enum member %s::%s expects %d payload argument(s), got %d", variant.EnumName, variant.Name, len(variant.Fields), len(call.Arguments)))
	}
	for index, argument := range call.Arguments {
		if index >= len(variant.Fields) {
			break
		}
		if argument.Name != "" || argument.Splat != "" {
			c.error(argument.Value.Span(), "enum payload arguments must be positional values")
		}
		if !types.Assignable(variant.Fields[index].Type, arguments[index]) {
			c.error(argument.Value.Span(), fmt.Sprintf("enum payload argument %d has type %s, expected %s", index+1, arguments[index], variant.Fields[index].Type))
		}
	}
}

func (c *Checker) resolveGenericApplication(node *ast.GenericExpression) (GenericApplication, bool) {
	name := ""
	switch receiver := node.Receiver.(type) {
	case *ast.Identifier:
		name = receiver.Name
	case *ast.MemberExpression:
		name = receiver.Name
	}
	application := GenericApplication{Name: name}
	for _, argument := range node.Arguments {
		application.TypeArguments = append(application.TypeArguments, fromTypeRef(argument))
	}

	if info := c.enums[name]; info != nil {
		application.Kind = "enum"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if method := c.functions[name]; method != nil {
		application.Kind = "function"
		for _, parameter := range method.TypeParameters {
			application.TypeParameters = append(application.TypeParameters, parameter.Name)
		}
		for _, parameter := range method.Parameters {
			application.Parameters = append(application.Parameters, fromTypeRef(parameter.Type))
			if parameter.Default == nil && !parameter.Rest && !parameter.KeywordRest {
				application.Required++
			}
			application.Variadic = application.Variadic || parameter.Rest || parameter.KeywordRest
		}
		application.ReturnType = methodReturnType(method)
	} else if binding, ok := c.result.References[node.Receiver]; ok && binding.Export != nil {
		application.TypeParameters = append([]string(nil), binding.Export.TypeParameters...)
		switch binding.Export.Kind {
		case resolver.EnumExport:
			application.Kind = "enum"
		case resolver.FunctionExport:
			application.Kind = "function"
			application.Parameters = append([]types.Type(nil), binding.Export.Parameters...)
			application.Required = binding.Export.Required
			application.Variadic = binding.Export.Variadic
			application.ReturnType = binding.Export.Type
		}
	}
	if application.Kind == "" || len(application.TypeParameters) == 0 {
		c.error(node.Span(), fmt.Sprintf("%s is not a generic declaration", name))
		return application, false
	}
	if len(application.TypeArguments) != len(application.TypeParameters) {
		c.error(node.Span(), fmt.Sprintf("%s expects %d type argument(s), got %d", name, len(application.TypeParameters), len(application.TypeArguments)))
		application.ReturnType = invalidType()
		return application, true
	}
	substitutions := typeSubstitutions(application.TypeParameters, application.TypeArguments)
	if application.Kind == "enum" {
		application.ReturnType = types.Type{Kind: types.Named, Name: name, Args: append([]types.Type(nil), application.TypeArguments...)}
	} else {
		for index := range application.Parameters {
			application.Parameters[index] = substituteType(application.Parameters[index], substitutions)
		}
		application.ReturnType = substituteType(application.ReturnType, substitutions)
	}
	return application, true
}

func typeSubstitutions(parameters []string, arguments []types.Type) map[string]types.Type {
	result := map[string]types.Type{}
	for index, parameter := range parameters {
		if index < len(arguments) {
			result[parameter] = arguments[index]
		}
	}
	return result
}

func substituteType(typ types.Type, substitutions map[string]types.Type) types.Type {
	if replacement, ok := substitutions[typ.Name]; ok && typ.Kind == types.Named && len(typ.Args) == 0 {
		replacement.Nullable = replacement.Nullable || typ.Nullable
		replacement.Readonly = replacement.Readonly || typ.Readonly
		return replacement
	}
	result := typ
	result.Args = make([]types.Type, len(typ.Args))
	for index, argument := range typ.Args {
		result.Args[index] = substituteType(argument, substitutions)
	}
	return result
}

func (c *Checker) checkTypedArguments(span token.Span, name string, parameters []types.Type, required int, variadic bool, arguments []ast.CallArgument, actual []types.Type) {
	if len(arguments) < required || !variadic && len(arguments) > len(parameters) {
		c.error(span, fmt.Sprintf("%s() expects %d..%d arguments, got %d", name, required, len(parameters), len(arguments)))
		return
	}
	for index, actualType := range actual {
		parameterIndex := index
		if parameterIndex >= len(parameters) {
			parameterIndex = len(parameters) - 1
		}
		if parameterIndex < 0 {
			continue
		}
		expected := parameters[parameterIndex]
		actualType = c.contextualizeHashLiteral(arguments[index].Value, expected, actualType)
		if !types.Assignable(expected, actualType) {
			c.error(arguments[index].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, name, actualType, expected))
		}
	}
}

func (c *Checker) checkUnaryOperator(span token.Span, operator string, operand types.Type) types.Type {
	if operand.Kind == types.Invalid {
		return invalidType()
	}
	if operand.Kind == types.Any && c.rubyNativeSyntax() {
		if operator == "!" || operator == "not" {
			return types.FromName("Boolean")
		}
		return types.FromName("Any")
	}
	switch operator {
	case "!", "not":
		if isNonNullable(operand, types.Bool) {
			return types.FromName("Boolean")
		}
	case "+", "-":
		if isNonNullableNumber(operand) {
			return plainNumberType(operand.Kind)
		}
	case "~":
		if c.rubyNativeSyntax() {
			return types.FromName("Any")
		}
		c.error(span, "operator ~ is not part of portable TypeRB; use an explicit Ruby-native import")
		return invalidType()
	default:
		c.error(span, fmt.Sprintf("unknown unary operator %s", operator))
		return invalidType()
	}
	c.error(span, fmt.Sprintf("operator %s does not support %s", operator, operand))
	return invalidType()
}

func (c *Checker) checkBinaryOperator(span token.Span, operator string, left, right types.Type) types.Type {
	if left.Kind == types.Invalid || right.Kind == types.Invalid {
		return invalidType()
	}
	if c.rubyNativeSyntax() && (left.Kind == types.Any || right.Kind == types.Any) {
		switch operator {
		case "==", "!=", "!~", "<", "<=", ">", ">=":
			return types.FromName("Boolean")
		default:
			return types.FromName("Any")
		}
	}

	switch operator {
	case "&&", "||", "and", "or":
		if isNonNullable(left, types.Bool) && isNonNullable(right, types.Bool) {
			return types.FromName("Boolean")
		}
	case "+":
		if isNonNullable(left, types.String) && isNonNullable(right, types.String) {
			return types.FromName("String")
		}
		if sameNonNullableNumber(left, right) {
			return plainNumberType(left.Kind)
		}
	case "-", "*", "/", "**":
		if sameNonNullableNumber(left, right) {
			return plainNumberType(left.Kind)
		}
	case "%":
		if isNonNullable(left, types.Int) && isNonNullable(right, types.Int) {
			return types.FromName("Integer")
		}
	case "<", "<=", ">", ">=":
		if sameNonNullableNumber(left, right) {
			return types.FromName("Boolean")
		}
	case "==", "!=":
		if c.portableEqualityOperands(left, right) {
			return types.FromName("Boolean")
		}
	case "=~", "!~", "<=>", "|", "&", "^", "<<", ">>":
		if c.rubyNativeSyntax() {
			if operator == "!~" {
				return types.FromName("Boolean")
			}
			return types.FromName("Any")
		}
		c.error(span, fmt.Sprintf("operator %s is not part of portable TypeRB; use an explicit Ruby-native import", operator))
		return invalidType()
	default:
		c.error(span, fmt.Sprintf("unknown binary operator %s", operator))
		return invalidType()
	}
	c.error(span, fmt.Sprintf("operator %s does not support %s and %s", operator, left, right))
	return invalidType()
}

func (c *Checker) rubyNativeSyntax() bool {
	return c.mode == "ruby" && c.resolution.NativeSyntax
}

func isNonNullable(typ types.Type, kind types.Kind) bool {
	return typ.Kind == kind && !typ.Nullable
}

func isNonNullableNumber(typ types.Type) bool {
	return !typ.Nullable && (typ.Kind == types.Int || typ.Kind == types.Float)
}

func sameNonNullableNumber(left, right types.Type) bool {
	return isNonNullableNumber(left) && isNonNullableNumber(right) && left.Kind == right.Kind
}

func plainNumberType(kind types.Kind) types.Type {
	if kind == types.Float {
		return types.FromName("Float")
	}
	return types.FromName("Integer")
}

func (c *Checker) portableEqualityOperands(left, right types.Type) bool {
	if left.Kind == types.Nil {
		return right.Nullable
	}
	if right.Kind == types.Nil {
		return left.Nullable
	}
	if left.Nullable || right.Nullable || left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case types.Bool, types.Int, types.Float, types.String:
		return true
	default:
		leftVariants, leftEnum := c.enumVariants(left)
		rightVariants, rightEnum := c.enumVariants(right)
		return leftEnum && rightEnum && !enumHasPayload(leftVariants) && !enumHasPayload(rightVariants) && types.Equivalent(left, right)
	}
}

func invalidType() types.Type {
	return types.Type{Kind: types.Invalid, Name: "Invalid"}
}

func (c *Checker) checkMethod(method *ast.MethodStatement, parent *scope) {
	c.checkTypeParameters(method.TypeParameters)
	if len(method.TypeParameters) > 0 && (c.current != nil || c.moduleDepth > 0 || method.Name == "main") {
		c.error(method.Span(), "only non-main top-level functions may be generic in the initial generics subset")
	}
	previousClassMethod := c.classMethod
	c.classMethod = method.Class
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
			actual = c.contextualizeHashLiteral(parameter.Default, typ, actual)
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
	previousLoopDepth := c.loopDepth
	c.loopDepth = 0
	if method.Name == "initialize" && c.current != nil {
		c.initializing++
	}
	c.checkStatements(method.Body, methodScope)
	if method.Name == "initialize" && c.current != nil {
		c.initializing--
	}
	c.loopDepth = previousLoopDepth
	c.returns = c.returns[:len(c.returns)-1]
	c.classMethod = previousClassMethod
}

func (c *Checker) checkTypeParameters(parameters []ast.TypeParameter) {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if !isConstant(parameter.Name) {
			c.error(parameter.Span(), "type parameter must begin with an uppercase letter")
		}
		if seen[parameter.Name] {
			c.error(parameter.Span(), fmt.Sprintf("type parameter %s is duplicated", parameter.Name))
		}
		seen[parameter.Name] = true
	}
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
		if method := info.methods[memberName]; method != nil && !method.Class {
			return signatureFromMethod(method), true
		}
		if signature, ok := c.classMethodSignature(info.superclass, memberName, seen); ok {
			return signature, true
		}
	}
	if binding, ok := c.resolution.TypeMember(className, memberName); ok && binding.Member != nil && binding.Member.Kind == resolver.FunctionExport && !binding.Member.Class {
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

func (c *Checker) localMember(className, memberName string, class bool, seen map[string]bool) (classMember, bool) {
	if className == "" || seen[className] {
		return classMember{}, false
	}
	seen[className] = true
	if info := c.classes[className]; info != nil {
		if method := info.methods[memberName]; method != nil && method.Class == class {
			return classMember{typ: methodReturnType(method), method: method}, true
		}
		if !class {
			if field := info.fields["@"+memberName]; field != nil {
				return classMember{typ: fromTypeRef(field.Type), field: field}, true
			}
			if field := info.fields["@_"+strings.TrimPrefix(memberName, "_")]; field != nil {
				return classMember{typ: fromTypeRef(field.Type), field: field}, true
			}
		}
		if member, ok := c.localMember(info.superclass, memberName, class, seen); ok {
			return member, true
		}
	}
	return classMember{}, false
}

func (c *Checker) importedAncestorMember(className, memberName string, class bool, seen map[string]bool) (resolver.Binding, bool) {
	if className == "" || seen[className] {
		return resolver.Binding{}, false
	}
	seen[className] = true
	if binding, ok := c.resolution.TypeMember(className, memberName); ok && binding.Member != nil && binding.Member.Class == class {
		return binding, true
	}
	if info := c.classes[className]; info != nil {
		return c.importedAncestorMember(info.superclass, memberName, class, seen)
	}
	return resolver.Binding{}, false
}

func (c *Checker) readonlyClassField(member *ast.MemberExpression, sc *scope) bool {
	receiverType := c.result.Expressions[member.Receiver]
	if receiverType.Kind == types.Invalid || receiverType.Name == "" {
		receiverType = c.checkExpression(member.Receiver, sc)
	}
	if local, ok := c.localMember(receiverType.Name, member.Name, false, map[string]bool{}); ok && local.field != nil {
		return local.field.ReadOnly
	}
	if binding, ok := c.importedAncestorMember(receiverType.Name, member.Name, false, map[string]bool{}); ok && binding.Member != nil {
		return binding.Member.Readonly
	}
	return false
}

func (c *Checker) classMemberAccess(expression ast.Expression, sc *scope) bool {
	switch node := expression.(type) {
	case *ast.Identifier:
		if node.Name == "self" {
			return c.current != nil && c.classMethod
		}
		if _, exists := sc.lookup(node.Name); exists {
			return false
		}
		if declared, exists := c.declaredTypes[node.Name]; exists {
			return declared.kind == "class" || declared.kind == "record" || declared.kind == "module"
		}
		if _, exists := c.declarations().Type(node.Name); exists {
			return true
		}
		if binding, exists := c.result.References[node]; exists && binding.Export != nil {
			switch binding.Export.Kind {
			case resolver.ClassExport, resolver.RecordExport, resolver.ModuleExport:
				return true
			}
		}
	case *ast.MemberExpression:
		return node.Namespace
	}
	return false
}

func (c *Checker) memberKindMismatch(span token.Span, className, memberName string, class bool) {
	if class {
		c.error(span, fmt.Sprintf("class %s has no class member %s; %s is an instance member", className, memberName, memberName))
		return
	}
	c.error(span, fmt.Sprintf("class %s has no instance member %s; %s is a class member", className, memberName, memberName))
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
		case *ast.CaseStatement:
			for _, branch := range n.Branches {
				walkAssignments(branch.Body, visit)
			}
			walkAssignments(n.Else, visit)
		case *ast.ExpressionStatement:
			if iteration, ok := n.Expression.(*ast.IterationExpression); ok && iteration.Block != nil {
				walkAssignments(iteration.Block.Body, visit)
			}
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
			if value.constant {
				c.result.Constants[n] = value.owner
			}
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
		if len(n.Entries) == 0 {
			typ = types.Type{Kind: types.Hash, Name: "Hash"}
			break
		}
		keyType := c.checkExpression(n.Entries[0].Key, sc)
		valueType := c.checkExpression(n.Entries[0].Value, sc)
		if !portableHashKey(keyType) && !c.rubyNativeSyntax() {
			c.error(n.Entries[0].Key.Span(), fmt.Sprintf("Hash key must be String or Integer, got %s", keyType))
		}
		for _, entry := range n.Entries[1:] {
			currentKey := c.checkExpression(entry.Key, sc)
			currentValue := c.checkExpression(entry.Value, sc)
			if !portableHashKey(currentKey) && !c.rubyNativeSyntax() {
				c.error(entry.Key.Span(), fmt.Sprintf("Hash key must be String or Integer, got %s", currentKey))
			}
			if !types.Equivalent(keyType, currentKey) {
				c.error(entry.Key.Span(), fmt.Sprintf("Hash literal key type is %s, expected %s", currentKey, keyType))
			}
			if !types.Equivalent(valueType, currentValue) {
				valueType = types.FromName("Any")
			}
		}
		typ = types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{keyType, valueType}}
	case *ast.UnaryExpression:
		operand := c.checkExpression(n.Operand, sc)
		typ = c.checkUnaryOperator(n.Span(), n.Operator, operand)
	case *ast.BinaryExpression:
		left := c.checkExpression(n.Left, sc)
		right := c.checkExpression(n.Right, sc)
		typ = c.checkBinaryOperator(n.Span(), n.Operator, left, right)
	case *ast.RangeExpression:
		start := c.checkExpression(n.Start, sc)
		end := c.checkExpression(n.End, sc)
		if start.Kind != types.Int || end.Kind != types.Int {
			c.error(n.Span(), fmt.Sprintf("range endpoints must be Integer, got %s and %s", start, end))
		}
		typ = types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}
	case *ast.IterationExpression:
		sourceType := c.checkExpression(n.Source, sc)
		elementType, iterable := iterableElementType(sourceType)
		if !iterable && c.mode == "ruby" && c.resolution.NativeSyntax {
			// Ruby platform objects such as ActiveRecord::Relation participate in
			// native Enumerable even before a provider can expose their element
			// type. Keep the block portable while conservatively binding Any.
			elementType = types.Type{Kind: types.Any, Name: "Any"}
			iterable = true
		}
		if !iterable {
			c.error(n.Source.Span(), fmt.Sprintf("%s is not iterable", sourceType))
			elementType = types.Type{Kind: types.Any, Name: "Any"}
		}
		itemType := elementType
		if n.Operation == "each_slice" {
			if n.SliceSize == nil {
				c.error(n.Span(), "each_slice expects exactly one size argument")
			} else {
				sizeType := c.checkExpression(n.SliceSize, sc)
				if sizeType.Kind != types.Int {
					c.error(n.SliceSize.Span(), fmt.Sprintf("each_slice size must be Integer, got %s", sizeType))
				}
				if literal, ok := n.SliceSize.(*ast.Literal); ok {
					if size, valid := integerLiteral(literal.Raw); valid && size <= 0 {
						c.error(n.SliceSize.Span(), "each_slice size must be greater than zero")
					}
				}
			}
			itemType = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{elementType}}
		}
		c.result.Iterations[n] = itemType
		if n.Block != nil {
			expected := 1
			if n.WithIndex {
				expected = 2
			}
			if len(n.Block.Parameters) != expected {
				c.error(n.Block.Span(), fmt.Sprintf("%s block expects %d parameter(s), got %d", n.Operation, expected, len(n.Block.Parameters)))
			}
			blockScope := &scope{parent: sc, values: map[string]symbol{}}
			for index, name := range n.Block.Parameters {
				parameterType := itemType
				if index == 1 {
					parameterType = types.FromName("Integer")
				}
				if _, duplicate := blockScope.values[name]; duplicate {
					c.error(n.Block.Span(), fmt.Sprintf("block parameter %s is duplicated", name))
					continue
				}
				blockScope.values[name] = symbol{typ: parameterType, mutable: true, span: n.Block.Span()}
			}
			c.loopDepth++
			c.checkStatements(n.Block.Body, blockScope)
			c.loopDepth--
			c.result.Expressions[n.Block] = types.Type{Kind: types.Void, Name: "Void"}
		}
		typ = types.Type{Kind: types.Void, Name: "Void"}
	case *ast.GenericExpression:
		receiverType := c.checkExpression(n.Receiver, sc)
		application, ok := c.resolveGenericApplication(n)
		if !ok {
			typ = receiverType
			break
		}
		c.result.GenericApplications[n] = application
		typ = application.ReturnType
	case *ast.MemberExpression:
		receiverType := c.checkExpression(n.Receiver, sc)
		if c.enumPattern > 0 && receiverType.Name == c.enumPatternType.Name && len(receiverType.Args) == 0 {
			receiverType = c.enumPatternType
			c.result.Expressions[n.Receiver] = receiverType
		}
		classAccess := c.classMemberAccess(n.Receiver, sc)
		if receiverType.Kind == types.Array && n.Name == "push" {
			typ = types.Type{Kind: types.Void, Name: "Void"}
			break
		}
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
		if variants, enum := c.enumVariants(receiverType); enum {
			if !n.Namespace {
				c.error(n.Span(), fmt.Sprintf("enum member %s must be accessed with ::", n.Name))
				break
			}
			if expected, generic := c.genericTypeArity(receiverType.Name); generic && len(receiverType.Args) != expected {
				c.error(n.Receiver.Span(), fmt.Sprintf("%s expects %d type argument(s), got %d", receiverType.Name, expected, len(receiverType.Args)))
				break
			}
			variant, found := enumVariantNamed(variants, n.Name)
			if !found {
				c.error(n.Span(), fmt.Sprintf("enum %s has no member %s", receiverType.Name, n.Name))
				break
			}
			if binding, exists := c.resolution.TypeMember(receiverType.Name, n.Name); exists {
				c.result.References[n] = binding
			}
			typ = receiverType
			if len(variant.Fields) > 0 && c.enumCallee == 0 && c.enumPattern == 0 {
				c.error(n.Span(), fmt.Sprintf("enum member %s::%s requires %d payload argument(s)", receiverType.Name, n.Name, len(variant.Fields)))
			}
			break
		}
		if n.Name == "new" && !classAccess {
			constructorType := c.classes[receiverType.Name] != nil || c.records[receiverType.Name] != nil
			if imported, exists := c.resolution.ImportedType(receiverType.Name); exists && imported.Export != nil {
				constructorType = constructorType || imported.Export.Kind == resolver.ClassExport || imported.Export.Kind == resolver.RecordExport
			}
			if constructorType {
				c.memberKindMismatch(n.Span(), receiverType.Name, n.Name, false)
				break
			}
		}
		if !classAccess && !n.Namespace {
			if binding, exists := c.resolution.ReceiverMethod(receiverType, n.Name); exists {
				typ = binding.Type()
				c.result.References[n] = binding
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
		} else if member, found := c.localMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); found {
			typ = member.typ
		} else if binding, exists := c.importedAncestorMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			typ = binding.Type()
			c.result.References[n] = binding
		} else if member, exists := c.declarationMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			typ = member.Return
			c.external[n] = member
		} else if n.Name != "new" {
			if _, exists := c.localMember(receiverType.Name, n.Name, !classAccess, map[string]bool{}); exists {
				c.memberKindMismatch(n.Span(), receiverType.Name, n.Name, classAccess)
			} else if _, exists := c.importedAncestorMember(receiverType.Name, n.Name, !classAccess, map[string]bool{}); exists {
				c.memberKindMismatch(n.Span(), receiverType.Name, n.Name, classAccess)
			} else if _, exists := c.declarationMember(receiverType.Name, n.Name, !classAccess, map[string]bool{}); exists {
				c.memberKindMismatch(n.Span(), receiverType.Name, n.Name, classAccess)
			} else if c.classes[receiverType.Name] != nil {
				kind := "instance"
				if classAccess {
					kind = "class"
				}
				c.error(n.Span(), fmt.Sprintf("class %s has no %s member %s", receiverType.Name, kind, n.Name))
			} else if imported, exists := c.resolution.ImportedType(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiverType.Name, imported.Import.Path, n.Name))
			} else if declared, exists := c.declarations().Type(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("type %s provided by Rails has no member %s", declared.Name, n.Name))
			} else if portableReceiverKind(receiverType.Kind) {
				c.error(n.Span(), fmt.Sprintf("type %s has no member %s", receiverType, n.Name))
			}
		}
	case *ast.CallExpression:
		if member, ok := n.Callee.(*ast.MemberExpression); ok && member.Namespace {
			c.enumCallee++
		}
		calleeType := c.checkExpression(n.Callee, sc)
		if member, ok := n.Callee.(*ast.MemberExpression); ok && member.Namespace {
			c.enumCallee--
		}
		argumentTypes := make([]types.Type, 0, len(n.Arguments))
		for _, arg := range n.Arguments {
			argumentTypes = append(argumentTypes, c.checkExpression(arg.Value, sc))
		}
		typ = calleeType
		if generic, ok := n.Callee.(*ast.GenericExpression); ok {
			application := c.result.GenericApplications[generic]
			if application.Kind != "function" {
				c.error(n.Span(), fmt.Sprintf("generic %s %s is not callable", application.Kind, application.Name))
				break
			}
			c.checkTypedArguments(n.Span(), application.Name, application.Parameters, application.Required, application.Variadic, n.Arguments, argumentTypes)
			typ = application.ReturnType
			break
		}
		if member, ok := n.Callee.(*ast.MemberExpression); ok {
			if variant, enum := c.enumVariantForMember(member); enum {
				typ = calleeType
				c.checkEnumConstructor(n, variant, argumentTypes)
				if len(variant.Fields) > 0 {
					c.result.EnumConstructors[n] = variant
				}
				break
			}
		}
		if binding, ok := c.result.References[n.Callee]; ok {
			if binding.Export != nil && len(binding.Export.TypeParameters) > 0 {
				c.error(n.Callee.Span(), fmt.Sprintf("generic function %s requires explicit type arguments", binding.Name))
				break
			}
			typ = binding.Type()
			c.checkImportedArguments(n.Span(), binding, n.Arguments, argumentTypes, sc)
			if binding.Library != nil {
				receiverType := invalidType()
				if member, method := n.Callee.(*ast.MemberExpression); method && binding.Library.HasReceiver() {
					receiverType = c.result.Expressions[member.Receiver]
					if binding.Library.ReceiverMutable {
						c.requireMutable(member.Receiver, sc, binding.Name+"()")
					}
				}
				typ = inferLibraryReturn(*binding.Library, receiverType, argumentTypes)
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
						c.checkImportedArguments(n.Span(), binding, n.Arguments, argumentTypes, sc)
					}
				} else if record := c.records[identifier.Name]; record != nil {
					c.checkLocalRecordArguments(n, record)
				} else if info := c.classes[identifier.Name]; info != nil {
					c.checkArguments(n.Span(), info.methods["initialize"], n.Arguments, argumentTypes)
				}
			}
		} else if member, ok := n.Callee.(*ast.MemberExpression); ok {
			receiverType := c.checkExpression(member.Receiver, sc)
			classAccess := c.classMemberAccess(member.Receiver, sc)
			if receiverType.Kind == types.Array && member.Name == "push" {
				c.checkArrayPush(n, member, receiverType, argumentTypes, sc)
				typ = types.Type{Kind: types.Void, Name: "Void"}
			} else if local, found := c.localMember(receiverType.Name, member.Name, classAccess, map[string]bool{}); found && local.method != nil {
				typ = methodReturnType(local.method)
				c.checkArguments(n.Span(), local.method, n.Arguments, argumentTypes)
			}
		}
		if identifier, ok := n.Callee.(*ast.Identifier); ok {
			if _, imported := c.result.References[identifier]; imported {
				// The resolved import signature was checked above.
			} else if _, provided := c.external[identifier]; provided {
				// The library-provider signature was checked above.
			} else if c.current != nil && c.current.methods[identifier.Name] != nil {
				method := c.current.methods[identifier.Name]
				if method.Class != c.classMethod {
					c.memberKindMismatch(identifier.Span(), c.current.name, identifier.Name, c.classMethod)
				} else {
					typ = methodReturnType(method)
					c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
				}
			} else if method := c.functions[identifier.Name]; method != nil {
				if len(method.TypeParameters) > 0 {
					c.error(identifier.Span(), fmt.Sprintf("generic function %s requires explicit type arguments", identifier.Name))
				} else {
					typ = methodReturnType(method)
					c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
				}
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
			if indexType.Kind != types.Int || indexType.Nullable {
				c.error(n.Index.Span(), fmt.Sprintf("Array index must be Integer, got %s", indexType))
			}
			typ = receiver.Args[0]
		} else if receiver.Kind == types.Hash {
			if len(receiver.Args) != 2 {
				c.error(n.Receiver.Span(), "cannot index an untyped Hash; add Hash<K, V> annotation")
			} else {
				expectedKey := receiver.Args[0]
				if !types.Equivalent(expectedKey, indexType) {
					c.error(n.Index.Span(), fmt.Sprintf("Hash index has type %s, expected %s", indexType, expectedKey))
				}
				typ = receiver.Args[1]
			}
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

func iterableElementType(typ types.Type) (types.Type, bool) {
	if (typ.Kind == types.Array || typ.Kind == types.Range || typ.Kind == types.Iterable) && len(typ.Args) == 1 {
		return typ.Args[0], true
	}
	return types.Type{}, false
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
		actual = c.contextualizeHashLiteral(argument.Value, field.Type, actual)
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

func (c *Checker) checkImportedArguments(span token.Span, binding resolver.Binding, arguments []ast.CallArgument, actual []types.Type, sc *scope) {
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
		actualType = c.contextualizeHashLiteral(arguments[i].Value, expected, actualType)
		actual[i] = actualType
		if !types.Assignable(expected, actualType) {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, name, actualType, expected))
		}
		if binding.Library != nil && parameterIndex < len(binding.Library.Parameters) && binding.Library.Parameters[parameterIndex].Mutable {
			c.requireMutable(arguments[i].Value, sc, name+"()")
		}
	}
}

func (c *Checker) checkArrayPush(call *ast.CallExpression, member *ast.MemberExpression, receiverType types.Type, arguments []types.Type, sc *scope) {
	c.requireMutable(member.Receiver, sc, "push()")
	if len(arguments) != 1 {
		c.error(call.Span(), fmt.Sprintf("push() expects 1 argument, got %d", len(arguments)))
		return
	}
	if len(receiverType.Args) > 0 && !types.Assignable(receiverType.Args[0], arguments[0]) {
		c.error(call.Arguments[0].Value.Span(), fmt.Sprintf("argument 1 to push() has type %s, expected %s", arguments[0], receiverType.Args[0]))
	}
}

func (c *Checker) requireMutable(expression ast.Expression, sc *scope, action string) {
	switch node := expression.(type) {
	case *ast.Identifier:
		value, exists := sc.lookup(node.Name)
		if exists {
			if strings.HasPrefix(node.Name, "@") && c.initializing > 0 {
				return
			}
			if !value.mutable || value.typ.Readonly {
				c.error(node.Span(), fmt.Sprintf("%s is immutable; declare it with mut to use %s", node.Name, action))
			}
			return
		}
		if binding, imported := c.result.References[node]; imported && binding.Export != nil {
			c.error(node.Span(), fmt.Sprintf("imported value %s is immutable", node.Name))
			return
		}
		// Ruby's native compatibility surface includes framework setters and
		// legacy `NAME = value` constant declarations which have no TypeRB
		// binding to inspect.
		if c.rubyNativeSyntax() {
			return
		}
	case *ast.MemberExpression:
		if node.Namespace && isConstant(node.Name) {
			c.error(node.Span(), fmt.Sprintf("constant %s is immutable", node.Name))
			return
		}
		c.requireMutable(node.Receiver, sc, action)
	case *ast.IndexExpression:
		c.requireMutable(node.Receiver, sc, action)
	default:
		c.error(expression.Span(), fmt.Sprintf("%s requires a mutable binding", action))
	}
}

func isReferenceType(typ types.Type) bool {
	switch typ.Kind {
	case types.Array, types.Hash, types.Named:
		return true
	default:
		return false
	}
}

func portableHashKey(typ types.Type) bool {
	return !typ.Nullable && (typ.Kind == types.String || typ.Kind == types.Int)
}

func (c *Checker) contextualizeHashLiteral(expression ast.Expression, expected, actual types.Type) types.Type {
	if expression == nil || expected.Kind != types.Hash || len(expected.Args) != 2 || actual.Kind != types.Hash {
		return actual
	}
	literal, ok := expression.(*ast.HashLiteral)
	if !ok {
		return actual
	}
	if len(literal.Entries) > 0 {
		if len(actual.Args) != 2 || !types.Equivalent(expected.Args[0], actual.Args[0]) {
			return actual
		}
		for _, entry := range literal.Entries {
			valueType := c.result.Expressions[entry.Value]
			valueType = c.contextualizeHashLiteral(entry.Value, expected.Args[1], valueType)
			if !types.Assignable(expected.Args[1], valueType) {
				return actual
			}
		}
	}
	c.result.Expressions[expression] = expected
	return expected
}

func inferLibraryReturn(symbol stdlib.Symbol, receiver types.Type, arguments []types.Type) types.Type {
	argument := func(index int) types.Type {
		if index < 0 || index >= len(arguments) {
			return symbol.Return
		}
		return arguments[index]
	}
	switch symbol.Inference {
	case "receiver":
		return receiver
	case "argument_1":
		return argument(1)
	case "array_of_argument_1":
		return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{argument(1)}}
	default:
		return symbol.Return
	}
}

func portableReceiverKind(kind types.Kind) bool {
	switch kind {
	case types.Bool, types.Int, types.Float, types.String, types.Array, types.Range, types.Hash:
		return true
	default:
		return false
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
		actual[index] = c.contextualizeHashLiteral(argument.Value, expected, actual[index])
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
		argumentType = c.contextualizeHashLiteral(arguments[i].Value, expected, argumentType)
		actual[i] = argumentType
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
