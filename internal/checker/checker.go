// Package checker resolves names, infers local declaration types, validates
// assignments/returns, and records a type for every portable expression.
package checker

import (
	"fmt"
	"slices"
	"sort"
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
	Conversions         map[ast.Expression]types.Type
	Variables           map[*ast.VariableStatement]types.Type
	Iterations          map[*ast.IterationExpression]types.Type
	IterationBindings   map[*ast.IterationExpression][]types.Type
	LexicalBindings     map[*ast.Identifier]bool
	Constants           map[ast.Expression]string
	ConstantOwners      map[*ast.VariableStatement]string
	Resolution          resolver.Result
	References          map[ast.Expression]resolver.Binding
	EnumConstructors    map[*ast.CallExpression]EnumVariant
	CasePatterns        map[ast.Expression]CasePattern
	GenericApplications map[*ast.GenericExpression]GenericApplication
	CodecApplications   map[*ast.CallExpression]CodecApplication
	TypeAliases         map[*ast.TypeAliasStatement]TypeAlias
	StructuredBlocks    map[*ast.CallExpression]StructuredBlock
	ExternalMembers     map[ast.Expression]declaration.Member
	RuntimeDependencies map[string]*stdlib.Package
	ImportUses          map[*ast.ImportStatement]map[string]bool
}

type CodecApplication struct {
	Operation string
	Schema    CodecSchema
}

type TypeAlias struct {
	Target   types.Type
	Variants []EnumVariant
}

type StructuredBlock struct {
	Parameters []types.Type
	Return     types.Type
	Result     ast.Expression
}

type CodecSchema struct {
	Type      types.Type
	Kind      string
	Module    string
	Reference *resolver.Binding
	Element   *CodecSchema
	Fields    []CodecField
}

type CodecField struct {
	Name     string
	WireName string
	Schema   *CodecSchema
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
	MatchType   types.Type
	TypeUnion   bool
}

type symbol struct {
	typ      types.Type
	mutable  bool
	constant bool
	owner    string
	span     token.Span
	variable *ast.VariableStatement
	used     *bool
	useKind  string
}

func tracksUnusedBinding(name string) bool {
	return name != "_" && !strings.HasPrefix(name, "_")
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

func (s *scope) markUsed(name string) {
	for current := s; current != nil; current = current.parent {
		if value, ok := current.values[name]; ok {
			if value.used != nil {
				*value.used = true
			}
			return
		}
	}
}

type classInfo struct {
	name       string
	superclass string
	interfaces []string
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

type aliasInfo struct {
	statement      *ast.TypeAliasStatement
	typeParameters []string
	target         types.Type
}

type typeDeclaration struct {
	kind           string
	span           token.Span
	typeParameters []string
}

type Checker struct {
	mode               string
	result             Result
	diags              []diagnostic.Diagnostic
	classes            map[string]*classInfo
	records            map[string]*recordInfo
	enums              map[string]*enumInfo
	aliases            map[string]*aliasInfo
	interfaces         map[string]*ast.InterfaceStatement
	functions          map[string]*ast.MethodStatement
	current            *classInfo
	classMethod        bool
	initializing       int
	loopDepth          int
	moduleDepth        int
	interfaceDepth     int
	returns            []types.Type
	resolution         resolver.Result
	external           map[ast.Expression]declaration.Member
	declaredTypes      map[string]typeDeclaration
	enumCallee         int
	enumPattern        int
	enumPatternType    types.Type
	usedImports        map[*ast.ImportStatement]map[string]bool
	allowUnusedImports bool
	aliasCycles        map[string]bool
}

type Options struct {
	AllowUnusedImports bool
}

func Check(program *ast.Program, resolution resolver.Result) (Result, []diagnostic.Diagnostic) {
	return CheckWithOptions(program, resolution, Options{})
}

func CheckWithOptions(program *ast.Program, resolution resolver.Result, options Options) (Result, []diagnostic.Diagnostic) {
	importUses := map[*ast.ImportStatement]map[string]bool{}
	c := &Checker{
		mode: program.Mode,
		result: Result{
			Program:             program,
			Expressions:         map[ast.Expression]types.Type{},
			Conversions:         map[ast.Expression]types.Type{},
			Variables:           map[*ast.VariableStatement]types.Type{},
			Iterations:          map[*ast.IterationExpression]types.Type{},
			IterationBindings:   map[*ast.IterationExpression][]types.Type{},
			LexicalBindings:     map[*ast.Identifier]bool{},
			Constants:           map[ast.Expression]string{},
			ConstantOwners:      map[*ast.VariableStatement]string{},
			Resolution:          resolution,
			References:          map[ast.Expression]resolver.Binding{},
			EnumConstructors:    map[*ast.CallExpression]EnumVariant{},
			CasePatterns:        map[ast.Expression]CasePattern{},
			GenericApplications: map[*ast.GenericExpression]GenericApplication{},
			CodecApplications:   map[*ast.CallExpression]CodecApplication{},
			TypeAliases:         map[*ast.TypeAliasStatement]TypeAlias{},
			StructuredBlocks:    map[*ast.CallExpression]StructuredBlock{},
			ExternalMembers:     map[ast.Expression]declaration.Member{},
			RuntimeDependencies: map[string]*stdlib.Package{},
			ImportUses:          importUses,
		},
		classes:            map[string]*classInfo{},
		records:            map[string]*recordInfo{},
		enums:              map[string]*enumInfo{},
		aliases:            map[string]*aliasInfo{},
		interfaces:         map[string]*ast.InterfaceStatement{},
		functions:          map[string]*ast.MethodStatement{},
		resolution:         resolution,
		external:           map[ast.Expression]declaration.Member{},
		declaredTypes:      map[string]typeDeclaration{},
		usedImports:        importUses,
		allowUnusedImports: options.AllowUnusedImports,
		aliasCycles:        map[string]bool{},
	}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok {
			c.functions[method.Name] = method
		}
	}
	c.collect(program.Statements)
	c.validateTypeReferences(program.Statements)
	c.checkStatements(program.Statements, &scope{values: map[string]symbol{}, constantsAllowed: true, enumsAllowed: true})
	if !c.allowUnusedImports {
		c.checkUnusedImports(program.Statements)
	}
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
		case *ast.TypeAliasStatement:
			c.validateTypeReference(node.Target)
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
			c.validateExpressionTypeReferences(node.Value)
		case *ast.MethodStatement:
			c.validateMethodTypes(node)
			c.validateTypeReferences(node.Body)
		case *ast.VariableStatement:
			c.validateTypeReference(node.Type)
			c.validateExpressionTypeReferences(node.Value)
		case *ast.AssignmentStatement:
			c.validateExpressionTypeReferences(node.Target)
			c.validateExpressionTypeReferences(node.Value)
		case *ast.ReturnStatement:
			c.validateExpressionTypeReferences(node.Value)
		case *ast.IfStatement:
			c.validateExpressionTypeReferences(node.Condition)
			c.validateTypeReferences(node.Then)
			for _, branch := range node.ElseIf {
				c.validateExpressionTypeReferences(branch.Condition)
				c.validateTypeReferences(branch.Body)
			}
			c.validateTypeReferences(node.Else)
		case *ast.CaseStatement:
			c.validateExpressionTypeReferences(node.Value)
			c.validateTypeReferences(node.Leading)
			for _, branch := range node.Branches {
				c.validateTypeReferences(branch.Body)
			}
			c.validateTypeReferences(node.Else)
		case *ast.WhileStatement:
			c.validateExpressionTypeReferences(node.Condition)
			c.validateTypeReferences(node.Body)
		case *ast.ExpressionStatement:
			c.validateExpressionTypeReferences(node.Expression)
		case *ast.NativeBlock:
			c.validateTypeReferences(node.Body)
		}
	}
}

func (c *Checker) validateExpressionTypeReferences(expression ast.Expression) {
	switch node := expression.(type) {
	case nil:
		return
	case *ast.IfStatement:
		c.validateExpressionTypeReferences(node.Condition)
		c.validateTypeReferences(node.Then)
		for _, branch := range node.ElseIf {
			c.validateExpressionTypeReferences(branch.Condition)
			c.validateTypeReferences(branch.Body)
		}
		c.validateTypeReferences(node.Else)
	case *ast.CaseStatement:
		c.validateExpressionTypeReferences(node.Value)
		c.validateTypeReferences(node.Leading)
		for _, branch := range node.Branches {
			c.validateExpressionTypeReferences(branch.Value)
			c.validateTypeReferences(branch.Body)
		}
		c.validateTypeReferences(node.Else)
	case *ast.IterationExpression:
		c.validateExpressionTypeReferences(node.Source)
		c.validateExpressionTypeReferences(node.SliceSize)
		c.validateExpressionTypeReferences(node.Initial)
		if node.Block != nil {
			c.validateTypeReferences(node.Block.Body)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			c.validateExpressionTypeReferences(element)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			c.validateExpressionTypeReferences(entry.Key)
			c.validateExpressionTypeReferences(entry.Value)
		}
	case *ast.UnaryExpression:
		c.validateExpressionTypeReferences(node.Operand)
	case *ast.BinaryExpression:
		c.validateExpressionTypeReferences(node.Left)
		c.validateExpressionTypeReferences(node.Right)
	case *ast.RangeExpression:
		c.validateExpressionTypeReferences(node.Start)
		c.validateExpressionTypeReferences(node.End)
	case *ast.CallExpression:
		c.validateExpressionTypeReferences(node.Callee)
		for _, argument := range node.Arguments {
			c.validateExpressionTypeReferences(argument.Value)
		}
	case *ast.MemberExpression:
		c.validateExpressionTypeReferences(node.Receiver)
	case *ast.GenericExpression:
		c.validateExpressionTypeReferences(node.Receiver)
		for _, argument := range node.Arguments {
			c.validateTypeReference(argument)
		}
	case *ast.IndexExpression:
		c.validateExpressionTypeReferences(node.Receiver)
		c.validateExpressionTypeReferences(node.Index)
	case *ast.BlockExpression:
		c.validateTypeReferences(node.Body)
	}
}

func (c *Checker) validateMethodTypes(method *ast.MethodStatement) {
	c.validateTypeReference(method.ReturnType)
	for _, parameter := range method.Parameters {
		c.validateTypeReference(parameter.Type)
		c.validateExpressionTypeReferences(parameter.Default)
	}
}

func (c *Checker) validateTypeReference(ref ast.TypeRef) {
	if ref.Empty() {
		return
	}
	if len(ref.Union) > 0 {
		for _, alternative := range ref.Union {
			c.validateTypeReference(alternative)
		}
		return
	}
	if ref.Name == "Never" {
		c.error(ref.Span(), "Never is an internal compiler type and cannot be written in source")
		return
	}
	if _, _, compilerOwned := stdlib.LookupRuntimeExport(ref.Name); compilerOwned {
		_, declared := c.declaredTypes[ref.Name]
		_, imported := c.resolution.ImportedType(ref.Name)
		if !declared && !imported {
			c.error(ref.Span(), fmt.Sprintf("type %s is not declared or imported", ref.Name))
		}
	}
	for _, argument := range ref.Arguments {
		c.validateTypeReference(argument)
	}
	if binding, imported := c.resolution.ImportedType(ref.Name); imported {
		c.markImportUsed(binding)
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
	key := c.typeFromRef(ref.Arguments[0])
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
			info := &classInfo{name: n.Name, superclass: expressionTypeName(n.Superclass), interfaces: append([]string(nil), n.Implements...), fields: map[string]*ast.FieldStatement{}, methods: map[string]*ast.MethodStatement{}}
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
		case *ast.TypeAliasStatement:
			if !c.declareType(n.Name, "type alias", n.Span()) {
				continue
			}
			info := &aliasInfo{statement: n, target: fromTypeRef(n.Target)}
			for _, parameter := range n.TypeParameters {
				info.typeParameters = append(info.typeParameters, parameter.Name)
			}
			declaration := c.declaredTypes[n.Name]
			declaration.typeParameters = append([]string(nil), info.typeParameters...)
			c.declaredTypes[n.Name] = declaration
			c.aliases[n.Name] = info
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
	c.checkStatementSequence(statements, sc)
	c.checkUnusedBindings(sc)
}

func (c *Checker) checkStatementSequence(statements []ast.Statement, sc *scope) {
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
				classScope.values[name] = symbol{typ: c.typeFromRef(field.Type), mutable: !field.ReadOnly, span: field.Span()}
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
		case *ast.TypeAliasStatement:
			if !sc.enumsAllowed {
				c.error(n.Span(), fmt.Sprintf("type alias %s may only be declared at top level or directly inside a module", n.Name))
			}
			if !isConstant(n.Name) {
				c.error(n.Span(), "type alias name must begin with an uppercase letter")
			}
			c.checkTypeParameters(n.TypeParameters)
			if n.Target.Empty() {
				c.error(n.Span(), fmt.Sprintf("type alias %s requires a target type", n.Name))
			}
			aliasType := types.FromName(n.Name)
			for _, parameter := range n.TypeParameters {
				aliasType.Args = append(aliasType.Args, types.FromName(parameter.Name))
			}
			target := c.expandAlias(aliasType, map[string]bool{})
			alias := TypeAlias{Target: target}
			if variants, ok := c.enumVariants(aliasType); ok {
				alias.Variants = variants
			}
			c.result.TypeAliases[n] = alias
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
			c.interfaceDepth++
			for _, method := range n.Methods {
				c.checkMethod(method, sc)
			}
			c.interfaceDepth--
		case *ast.FieldStatement:
			if n.Value != nil {
				valueType := c.checkExpression(n.Value, sc)
				declared := c.typeFromRef(n.Type)
				valueType = c.contextualizeCollectionLiteral(n.Value, declared, valueType)
				if !n.Type.Empty() && !c.assignable(n.Value, declared, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot initialize %s with %s", declared, valueType))
				}
			}
		case *ast.MethodStatement:
			c.checkMethod(n, sc)
		case *ast.VariableStatement:
			valueType := c.checkExpression(n.Value, sc)
			c.checkStructuredBlockValue(n.Value)
			variableType := valueType
			if n.Name == "_" {
				c.error(n.Span(), "blank binding _ is only valid as a parameter or pattern binding")
			}
			if n.Constant {
				if n.Mutable {
					c.error(n.Span(), fmt.Sprintf("constant %s cannot be declared with mut", n.Name))
				}
				if !sc.constantsAllowed {
					c.error(n.Span(), fmt.Sprintf("constant %s may only be declared at top level or directly inside a module or class", n.Name))
				}
			}
			if !n.Type.Empty() {
				variableType = c.typeFromRef(n.Type)
				valueType = c.contextualizeCollectionLiteral(n.Value, variableType, valueType)
				if !c.assignable(n.Value, variableType, valueType) {
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
				declared := symbol{typ: variableType, mutable: n.Mutable && !n.Constant, constant: n.Constant, owner: sc.constantOwner, span: n.Span(), variable: n}
				if len(c.returns) > 0 && !n.Constant && tracksUnusedBinding(n.Name) {
					used := false
					declared.used = &used
					declared.useKind = "local variable"
				}
				sc.values[n.Name] = declared
			}
			if n.Constant {
				c.result.ConstantOwners[n] = sc.constantOwner
			}
			c.result.Variables[n] = variableType
		case *ast.AssignmentStatement:
			leftType := c.checkExpression(n.Target, sc)
			rightType := c.checkExpression(n.Value, sc)
			c.checkStructuredBlockValue(n.Value)
			rightType = c.contextualizeCollectionLiteral(n.Value, leftType, rightType)
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
					if leftType.Kind == types.Float && rightType.Kind == types.Int && isNonNullableNumber(leftType) && isNonNullableNumber(rightType) {
						c.recordIntegerToFloat(n.Value)
					}
				}
			}
			if n.Operator == "=" && leftType.Kind != types.Any {
				c.assignable(n.Value, leftType, rightType)
			}
			if leftType.Kind != types.Any && !types.Assignable(leftType, assignedType) {
				c.error(n.Value.Span(), fmt.Sprintf("cannot assign %s to %s", assignedType, leftType))
			}
		case *ast.ReturnStatement:
			actual := types.Type{Kind: types.Void, Name: "Void"}
			if n.Value != nil {
				actual = c.checkExpression(n.Value, sc)
				c.checkStructuredBlockValue(n.Value)
			}
			if len(c.returns) == 0 {
				c.error(n.Span(), "return is only valid inside a function or method")
			} else {
				expected := c.returns[len(c.returns)-1]
				actual = c.contextualizeCollectionLiteral(n.Value, expected, actual)
				if !c.assignable(n.Value, expected, actual) {
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
			if call, member, ok := c.structuredBlockCall(n.Expression); ok && member.Block.Structured {
				c.error(call.Span(), fmt.Sprintf("result of %s() must be assigned or returned", member.Name))
			}
		case *ast.IfStatement:
			c.checkIf(n, sc, false)
		case *ast.CaseStatement:
			c.checkCase(n, sc, false)
		case *ast.WhileStatement:
			c.checkBooleanCondition(n.Condition, sc, "while")
			c.loopDepth++
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
			c.loopDepth--
		case *ast.NativeStatement:
			if c.mode != "ruby" {
				c.error(n.Span(), "unsupported statement syntax in portable TypeRB")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
			}
		case *ast.NativeBlock:
			if c.mode != "ruby" {
				c.error(n.Span(), "unsupported block syntax in portable TypeRB")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
			}
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		}
	}
}

func (c *Checker) checkUnusedBindings(sc *scope) {
	type trackedBinding struct {
		name  string
		value symbol
	}
	tracked := make([]trackedBinding, 0, len(sc.values))
	for name, value := range sc.values {
		if value.used != nil && !*value.used {
			tracked = append(tracked, trackedBinding{name: name, value: value})
		}
	}
	sort.Slice(tracked, func(i, j int) bool {
		left := tracked[i].value.span.Start.Offset
		right := tracked[j].value.span.Start.Offset
		if left == right {
			return tracked[i].name < tracked[j].name
		}
		return left < right
	})
	for _, binding := range tracked {
		c.error(binding.value.span, fmt.Sprintf("%s %s is not used", binding.value.useKind, binding.name))
	}
}

func (c *Checker) markImportUsed(binding resolver.Binding) {
	if binding.Import == nil || binding.Import.Node == nil {
		return
	}
	c.markImportNodeUsed(binding.Import, binding.Name)
}

func (c *Checker) recordReference(expression ast.Expression, binding resolver.Binding) {
	c.result.References[expression] = binding
	c.markImportUsed(binding)
	if binding.Library != nil {
		for _, definition := range stdlib.RuntimeDependenciesForType(binding.Library.Return) {
			if definition.ModulePath == c.result.Program.ModulePath {
				continue
			}
			c.result.RuntimeDependencies[definition.Path] = definition
		}
		for _, runtimeType := range binding.Library.RuntimeDependencies {
			for _, definition := range stdlib.RuntimeDependenciesForType(runtimeType) {
				if definition.ModulePath != c.result.Program.ModulePath {
					c.result.RuntimeDependencies[definition.Path] = definition
				}
			}
		}
		for _, name := range binding.Library.RequiredSymbols {
			c.markImportedSymbolUsed(name)
		}
	}
}

func (c *Checker) markImportedSymbolUsed(name string) {
	if binding, ok := c.resolution.Symbols[name]; ok {
		c.markImportUsed(binding)
	}
}

func (c *Checker) markImportNodeUsed(imported *resolver.Import, symbolName string) {
	if imported == nil || imported.Node == nil {
		return
	}
	used := c.usedImports[imported.Node]
	if used == nil {
		used = map[string]bool{}
		c.usedImports[imported.Node] = used
	}
	used[""] = true
	if symbolName != "" {
		used[symbolName] = true
	}
}

func (c *Checker) checkUnusedImports(statements []ast.Statement) {
	for _, statement := range statements {
		node, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		imported := c.resolution.Imports[node]
		if imported == nil || imported.Definition != nil && (imported.Definition.NativeSyntax || imported.Definition.TypeProvider != "") {
			continue
		}
		used := c.usedImports[node]
		if len(node.Symbols) == 0 {
			if !used[""] {
				c.error(node.Span(), fmt.Sprintf("import %s is not used", node.Path))
			}
			continue
		}
		for _, name := range node.Symbols {
			if !used[name] {
				c.error(node.Span(), fmt.Sprintf("imported symbol %s is not used", name))
			}
		}
	}
}

func (c *Checker) checkBooleanCondition(expression ast.Expression, sc *scope, construct string) {
	typ := c.checkExpression(expression, sc)
	if typ.Kind == types.Invalid || typ.Kind == types.Never || typ.Kind == types.Bool && !typ.Nullable {
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

type controlFlowBranchResult struct {
	expression ast.Expression
	typ        types.Type
}

func (c *Checker) checkIf(node *ast.IfStatement, sc *scope, expression bool) types.Type {
	results := []controlFlowBranchResult{}
	c.checkBooleanCondition(node.Condition, sc, "if")
	if result := c.checkControlFlowBranch(node.Then, &scope{parent: sc, values: map[string]symbol{}}, node.Span(), "if", expression); result != nil {
		results = append(results, *result)
	}
	for _, branch := range node.ElseIf {
		c.checkBooleanCondition(branch.Condition, sc, "elsif")
		if result := c.checkControlFlowBranch(branch.Body, &scope{parent: sc, values: map[string]symbol{}}, node.Span(), "if", expression); result != nil {
			results = append(results, *result)
		}
	}
	if node.HasElse {
		if result := c.checkControlFlowBranch(node.Else, &scope{parent: sc, values: map[string]symbol{}}, node.Span(), "if", expression); result != nil {
			results = append(results, *result)
		}
	} else if expression {
		c.error(node.Span(), "if expression requires an else branch")
	}
	if !expression {
		return types.FromName("Void")
	}
	return c.controlFlowResultType("if", node.Span(), results)
}

func (c *Checker) checkCase(node *ast.CaseStatement, sc *scope, expression bool) types.Type {
	selectorType := c.checkExpression(node.Value, sc)
	if selectorType.Kind == types.Union {
		return c.checkUnionCase(node, sc, selectorType, expression)
	}
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
	results := []controlFlowBranchResult{}
	for _, branch := range node.Branches {
		previousPatternType := c.enumPatternType
		c.enumPatternType = selectorType
		c.enumPattern++
		branchType := c.checkExpression(branch.Value, sc)
		c.enumPattern--
		c.enumPatternType = previousPatternType
		variant, member := c.caseEnumVariant(branch.Value, selectorType)
		if !member || !c.typesEquivalent(selectorType, branchType) {
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
				declared := symbol{typ: field.Type, span: binding.Span()}
				if tracksUnusedBinding(binding.Name) {
					used := false
					declared.used = &used
					declared.useKind = "pattern binding"
				}
				branchScope.values[binding.Name] = declared
				bindings = append(bindings, CaseBinding{Name: binding.Name, Field: field})
			}
			c.result.CasePatterns[branch.Value] = CasePattern{
				Variant:     variant,
				Bindings:    bindings,
				PayloadEnum: enumHasPayload(variants),
			}
		}
		if result := c.checkControlFlowBranch(branch.Body, branchScope, branch.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	}
	if node.HasElse {
		if result := c.checkControlFlowBranch(node.Else, &scope{parent: sc, values: map[string]symbol{}}, node.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	} else if enum {
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
	if !expression {
		return types.FromName("Void")
	}
	return c.controlFlowResultType("case", node.Span(), results)
}

func (c *Checker) checkUnionCase(node *ast.CaseStatement, sc *scope, selectorType types.Type, expression bool) types.Type {
	for _, statement := range node.Leading {
		if _, comment := statement.(*ast.CommentStatement); !comment {
			c.error(statement.Span(), "case statements must be inside a when or else branch")
		}
	}
	c.checkStatements(node.Leading, &scope{parent: sc, values: map[string]symbol{}})

	seen := map[string]bool{}
	results := []controlFlowBranchResult{}
	for _, branch := range node.Branches {
		identifier, ok := branch.Value.(*ast.Identifier)
		matchType := types.Type{Kind: types.Invalid, Name: "Invalid"}
		if ok {
			candidate := types.FromName(identifier.Name)
			for _, alternative := range selectorType.Args {
				if types.Equivalent(alternative, candidate) {
					matchType = alternative
					break
				}
			}
		}
		if matchType.Kind == types.Invalid {
			c.error(branch.Value.Span(), fmt.Sprintf("when type must be an alternative of %s", selectorType))
		} else if !runtimeMatchableUnionType(matchType) {
			c.error(branch.Value.Span(), fmt.Sprintf("union type pattern does not yet support %s", matchType))
		} else if seen[matchType.String()] {
			c.error(branch.Value.Span(), fmt.Sprintf("union type %s is handled more than once", matchType))
		} else {
			seen[matchType.String()] = true
		}
		c.result.Expressions[branch.Value] = matchType

		branchScope := &scope{parent: sc, values: map[string]symbol{}}
		bindings := []CaseBinding{}
		if len(branch.Bindings) != 1 {
			c.error(branch.Value.Span(), fmt.Sprintf("union type pattern %s expects exactly one binding, got %d", matchType, len(branch.Bindings)))
		} else if matchType.Kind != types.Invalid {
			binding := branch.Bindings[0]
			declared := symbol{typ: matchType, span: binding.Span()}
			if tracksUnusedBinding(binding.Name) {
				used := false
				declared.used = &used
				declared.useKind = "pattern binding"
			}
			branchScope.values[binding.Name] = declared
			bindings = append(bindings, CaseBinding{Name: binding.Name, Field: EnumField{Type: matchType}})
		}
		c.result.CasePatterns[branch.Value] = CasePattern{Bindings: bindings, MatchType: matchType, TypeUnion: true}
		if result := c.checkControlFlowBranch(branch.Body, branchScope, branch.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	}

	if node.HasElse {
		if result := c.checkControlFlowBranch(node.Else, &scope{parent: sc, values: map[string]symbol{}}, node.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	} else {
		missing := []string{}
		for _, alternative := range selectorType.Args {
			if !seen[alternative.String()] {
				missing = append(missing, alternative.String())
			}
		}
		if len(missing) > 0 {
			c.error(node.Span(), fmt.Sprintf("case for %s is not exhaustive; missing %s", selectorType, strings.Join(missing, ", ")))
		}
	}
	if !expression {
		return types.FromName("Void")
	}
	return c.controlFlowResultType("case", node.Span(), results)
}

func (c *Checker) checkControlFlowBranch(body []ast.Statement, sc *scope, span token.Span, construct string, expression bool) *controlFlowBranchResult {
	if !expression {
		c.checkStatements(body, sc)
		return nil
	}
	resultIndex, result := controlFlowBranchExpression(body)
	if result == nil {
		c.checkStatements(body, sc)
		if terminalControlFlowTransfer(body) != nil {
			return &controlFlowBranchResult{typ: types.Type{Kind: types.Never, Name: "Never"}}
		}
		c.error(span, fmt.Sprintf("%s expression branch must end with an expression", construct))
		return &controlFlowBranchResult{typ: invalidType()}
	}
	c.checkStatementSequence(body[:resultIndex], sc)
	typ := c.checkExpression(result, sc)
	c.checkStatementSequence(body[resultIndex+1:], sc)
	c.checkUnusedBindings(sc)
	return &controlFlowBranchResult{expression: result, typ: typ}
}

func terminalControlFlowTransfer(body []ast.Statement) ast.Statement {
	for index := len(body) - 1; index >= 0; index-- {
		switch statement := body[index].(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		case *ast.ReturnStatement, *ast.BreakStatement, *ast.NextStatement:
			return statement
		default:
			return nil
		}
	}
	return nil
}

func controlFlowBranchExpression(body []ast.Statement) (int, ast.Expression) {
	for index := len(body) - 1; index >= 0; index-- {
		switch statement := body[index].(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		case *ast.ExpressionStatement:
			return index, statement.Expression
		default:
			if expression, ok := statement.(ast.Expression); ok {
				return index, expression
			}
			return index, nil
		}
	}
	return -1, nil
}

func (c *Checker) controlFlowResultType(construct string, span token.Span, results []controlFlowBranchResult) types.Type {
	if len(results) == 0 {
		c.error(span, fmt.Sprintf("%s expression has no value-producing branches", construct))
		return invalidType()
	}
	valueResults := make([]controlFlowBranchResult, 0, len(results))
	for _, result := range results {
		if result.typ.Kind != types.Never {
			valueResults = append(valueResults, result)
		}
	}
	if len(valueResults) == 0 {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	results = valueResults
	common := results[0].typ
	compatible := common.Kind != types.Invalid
	for index := 1; index < len(results); index++ {
		current := results[index]
		if current.typ.Kind == types.Invalid || common.Kind == types.Invalid {
			compatible = false
			continue
		}
		if types.Equivalent(common, current.typ) {
			continue
		}
		if common.Kind != types.Any && current.typ.Kind != types.Any && types.Assignable(common, current.typ) {
			c.recordAssignableConversion(current.expression, common, current.typ)
			continue
		}
		if common.Kind != types.Any && current.typ.Kind != types.Any && types.Assignable(current.typ, common) {
			for previous := 0; previous < index; previous++ {
				c.recordAssignableConversion(results[previous].expression, current.typ, results[previous].typ)
			}
			common = current.typ
			continue
		}
		c.error(current.expression.Span(), fmt.Sprintf("%s expression branches have incompatible types %s and %s", construct, common, current.typ))
		compatible = false
	}
	if !compatible {
		return invalidType()
	}
	return common
}

func runtimeMatchableUnionType(typ types.Type) bool {
	if typ.Nullable || len(typ.Args) > 0 {
		return false
	}
	switch typ.Kind {
	case types.Bool, types.Int, types.Float, types.String:
		return true
	default:
		return false
	}
}

func (c *Checker) enumVariants(typ types.Type) ([]EnumVariant, bool) {
	if typ.Nullable {
		return nil, false
	}
	if parameters, target, alias := c.aliasDefinition(typ.Name); alias {
		expanded := substituteType(target, typeSubstitutions(parameters, typ.Args))
		expanded = c.expandAlias(expanded, map[string]bool{})
		variants, ok := c.enumVariants(expanded)
		if !ok {
			return nil, false
		}
		result := make([]EnumVariant, len(variants))
		for index, variant := range variants {
			result[index] = variant
			result[index].EnumName = typ.Name
			result[index].TypeArguments = append([]types.Type(nil), typ.Args...)
			if reference, exists := c.resolution.TypeMember(typ.Name, variant.Name); exists {
				result[index].Reference = &reference
			}
		}
		return result, true
	}
	if info := c.enums[typ.Name]; info != nil {
		substitutions := typeSubstitutions(info.typeParameters, typ.Args)
		variants := make([]EnumVariant, 0, len(info.members))
		for _, name := range info.members {
			member := info.byName[name]
			variant := EnumVariant{EnumName: typ.Name, Name: name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, parameter := range member.Parameters {
				variant.Fields = append(variant.Fields, EnumField{Name: parameter.Name, Type: substituteType(c.typeFromRef(parameter.Type), substitutions)})
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
	if exported, ok := c.resolution.CompilerOwnedType(typ.Name); ok && exported.Kind == resolver.EnumExport {
		substitutions := typeSubstitutions(exported.TypeParameters, typ.Args)
		variants := make([]EnumVariant, 0, len(exported.EnumVariants))
		for _, imported := range exported.EnumVariants {
			variant := EnumVariant{EnumName: typ.Name, Name: imported.Name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, field := range imported.Fields {
				variant.Fields = append(variant.Fields, EnumField{Name: field.Name, Type: substituteType(field.Type, substitutions)})
			}
			variants = append(variants, variant)
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
	if !c.typesEquivalent(receiverType, selectorType) {
		return EnumVariant{}, false
	}
	variants, ok := c.enumVariants(receiverType)
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
		if !c.assignable(argument.Value, variant.Fields[index].Type, arguments[index]) {
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
		application.TypeArguments = append(application.TypeArguments, c.typeFromRef(argument))
	}

	if info := c.enums[name]; info != nil {
		application.Kind = "enum"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if info := c.aliases[name]; info != nil {
		application.Kind = "type_alias"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if method := c.functions[name]; method != nil {
		application.Kind = "function"
		for _, parameter := range method.TypeParameters {
			application.TypeParameters = append(application.TypeParameters, parameter.Name)
		}
		for _, parameter := range method.Parameters {
			application.Parameters = append(application.Parameters, c.typeFromRef(parameter.Type))
			if parameter.Default == nil && !parameter.Rest && !parameter.KeywordRest {
				application.Required++
			}
			application.Variadic = application.Variadic || parameter.Rest || parameter.KeywordRest
		}
		application.ReturnType = c.methodReturnType(method)
	} else if binding, ok := c.result.References[node.Receiver]; ok {
		if binding.Export != nil {
			application.TypeParameters = append([]string(nil), binding.Export.TypeParameters...)
			switch binding.Export.Kind {
			case resolver.EnumExport:
				application.Kind = "enum"
			case resolver.TypeAliasExport:
				application.Kind = "type_alias"
			case resolver.FunctionExport:
				application.Kind = "function"
				application.Parameters = append([]types.Type(nil), binding.Export.Parameters...)
				application.Required = binding.Export.Required
				application.Variadic = binding.Export.Variadic
				application.ReturnType = binding.Export.Type
			}
		} else if binding.Library != nil {
			application.Kind = "function"
			application.TypeParameters = append([]string(nil), binding.Library.TypeParameters...)
			for _, parameter := range binding.Library.Parameters {
				application.Parameters = append(application.Parameters, parameter.Type)
				if !parameter.Optional {
					application.Required++
				}
			}
			application.Variadic = binding.Library.Variadic
			application.ReturnType = binding.Library.Return
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
	if application.Kind == "enum" || application.Kind == "type_alias" {
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

func (c *Checker) checkCodecApplication(call *ast.CallExpression, intrinsic string, typ types.Type) {
	operation := ""
	switch intrinsic {
	case "trb.internal.json.decode", "trb.web.request_json":
		operation = "decode"
	case "trb.internal.json.encode", "trb.web.json":
		operation = "encode"
	default:
		return
	}
	schema, ok := c.codecSchema(call.Span(), typ, map[string]bool{})
	if ok {
		c.result.CodecApplications[call] = CodecApplication{Operation: operation, Schema: schema}
	}
}

func (c *Checker) codecSchema(span token.Span, typ types.Type, visiting map[string]bool) (CodecSchema, bool) {
	schema := CodecSchema{Type: typ}
	base := typ
	base.Nullable = false
	switch base.Kind {
	case types.Bool:
		schema.Kind = "boolean"
	case types.Int:
		schema.Kind = "integer"
	case types.Float:
		schema.Kind = "float"
	case types.String:
		schema.Kind = "string"
	case types.Array:
		if len(base.Args) != 1 {
			c.error(span, "JSON codec requires a typed Array<T>")
			return schema, false
		}
		element, ok := c.codecSchema(span, base.Args[0], visiting)
		if !ok {
			return schema, false
		}
		schema.Kind = "array"
		schema.Element = &element
	case types.Hash:
		if len(base.Args) != 2 || base.Args[0].Kind != types.String || base.Args[0].Nullable {
			c.error(span, "JSON codec requires Hash<String, V>")
			return schema, false
		}
		element, ok := c.codecSchema(span, base.Args[1], visiting)
		if !ok {
			return schema, false
		}
		schema.Kind = "hash"
		schema.Element = &element
	case types.Named:
		fields, module, reference, ok := c.codecRecord(base.Name)
		if !ok {
			c.error(span, fmt.Sprintf("JSON codec type %s must be a record or JSON-compatible built-in type", typ))
			return schema, false
		}
		key := module + "#" + base.Name
		if visiting[key] {
			c.error(span, fmt.Sprintf("recursive JSON codec record %s is not supported yet", base.Name))
			return schema, false
		}
		visiting[key] = true
		defer delete(visiting, key)
		schema.Kind = "record"
		schema.Module = module
		if reference != nil {
			copy := *reference
			schema.Reference = &copy
		}
		seen := map[string]bool{}
		for _, field := range fields {
			wireName := field.JSONName
			if wireName == "" {
				wireName = field.Name
			}
			if wireName == "-" || wireName == "" {
				c.error(span, fmt.Sprintf("record field %s has unsupported JSON name %q", field.Name, wireName))
				return schema, false
			}
			if seen[wireName] {
				c.error(span, fmt.Sprintf("record %s maps more than one field to JSON name %q", base.Name, wireName))
				return schema, false
			}
			seen[wireName] = true
			fieldSchema, fieldOK := c.codecSchema(span, field.Type, visiting)
			if !fieldOK {
				return schema, false
			}
			schema.Fields = append(schema.Fields, CodecField{Name: field.Name, WireName: wireName, Schema: &fieldSchema})
		}
	default:
		c.error(span, fmt.Sprintf("JSON codec type %s is not supported", typ))
		return schema, false
	}
	return schema, true
}

func (c *Checker) codecRecord(name string) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	if record := c.records[name]; record != nil {
		fields := make([]resolver.RecordField, len(record.fields))
		for index, field := range record.fields {
			fields[index] = resolver.RecordField{Name: field.Name, JSONName: checkerRecordJSONName(field), Type: c.typeFromRef(field.Type)}
		}
		return fields, c.result.Program.ModulePath, nil, true
	}
	if binding, ok := c.resolution.ImportedType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		copy := binding
		return append([]resolver.RecordField(nil), binding.Export.Fields...), binding.Import.RuntimePath(), &copy, true
	}
	for _, binding := range c.resolution.Symbols {
		if binding.Import == nil {
			continue
		}
		exported, ok := binding.Import.Exports[name]
		if !ok || exported.Kind != resolver.RecordExport {
			continue
		}
		copyExport := exported
		copy := resolver.Binding{Import: binding.Import, Name: name, Export: &copyExport}
		return append([]resolver.RecordField(nil), exported.Fields...), binding.Import.RuntimePath(), &copy, true
	}
	return nil, "", nil, false
}

func checkerRecordJSONName(field *ast.RecordFieldStatement) string {
	for _, attribute := range field.Attributes {
		if attribute.Name != "json" || len(attribute.Arguments) == 0 {
			continue
		}
		literal, ok := attribute.Arguments[0].Value.(*ast.Literal)
		if !ok || literal.Kind != ast.StringLiteral {
			continue
		}
		value, err := strconv.Unquote(literal.Raw)
		if err == nil {
			return strings.Split(value, ",")[0]
		}
	}
	return field.Name
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
		actualType = c.contextualizeCollectionLiteral(arguments[index].Value, expected, actualType)
		if !c.assignable(arguments[index].Value, expected, actualType) {
			c.error(arguments[index].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, name, actualType, expected))
		}
	}
}

func (c *Checker) checkUnaryOperator(span token.Span, operator string, operand types.Type) types.Type {
	if operand.Kind == types.Invalid {
		return invalidType()
	}
	if operand.Kind == types.Never {
		return types.Type{Kind: types.Never, Name: "Never"}
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
	if left.Kind == types.Never {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	if right.Kind == types.Never {
		if operator == "&&" || operator == "||" || operator == "and" || operator == "or" {
			if isNonNullable(left, types.Bool) {
				return types.FromName("Boolean")
			}
		} else {
			return types.Type{Kind: types.Never, Name: "Never"}
		}
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
		if isNonNullableNumber(left) && isNonNullableNumber(right) {
			return commonNumberType(left, right)
		}
	case "-", "*", "/", "**":
		if isNonNullableNumber(left) && isNonNullableNumber(right) {
			return commonNumberType(left, right)
		}
	case "%":
		if isNonNullable(left, types.Int) && isNonNullable(right, types.Int) {
			return types.FromName("Integer")
		}
	case "<", "<=", ">", ">=":
		if isNonNullableNumber(left) && isNonNullableNumber(right) {
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

func plainNumberType(kind types.Kind) types.Type {
	if kind == types.Float {
		return types.FromName("Float")
	}
	return types.FromName("Integer")
}

func commonNumberType(left, right types.Type) types.Type {
	if left.Kind == types.Float || right.Kind == types.Float {
		return types.FromName("Float")
	}
	return types.FromName("Integer")
}

func (c *Checker) assignable(expression ast.Expression, target, actual types.Type) bool {
	if !c.typesAssignable(target, actual) {
		return false
	}
	c.recordAssignableConversion(expression, target, actual)
	return true
}

func (c *Checker) typesAssignable(target, actual types.Type) bool {
	target = c.expandAlias(target, map[string]bool{})
	actual = c.expandAlias(actual, map[string]bool{})
	if types.Assignable(target, actual) {
		return true
	}
	if actual.Nullable && !target.Nullable {
		return false
	}
	target.Nullable = false
	actual.Nullable = false
	if target.Kind == types.Union {
		values := []types.Type{actual}
		if actual.Kind == types.Union {
			values = actual.Args
		}
		for _, value := range values {
			accepted := false
			for _, alternative := range target.Args {
				if c.typesAssignable(alternative, value) {
					accepted = true
					break
				}
			}
			if !accepted {
				return false
			}
		}
		return true
	}
	if actual.Kind == types.Union {
		for _, alternative := range actual.Args {
			if !c.typesAssignable(target, alternative) {
				return false
			}
		}
		return true
	}
	return target.Kind == types.Named && actual.Kind == types.Named && c.isInterface(target.Name) && c.classImplements(actual.Name, target.Name, map[string]bool{})
}

func (c *Checker) typesEquivalent(left, right types.Type) bool {
	left = c.expandAlias(left, map[string]bool{})
	right = c.expandAlias(right, map[string]bool{})
	return types.Equivalent(left, right)
}

func (c *Checker) isInterface(name string) bool {
	if c.interfaces[name] != nil {
		return true
	}
	binding, ok := c.resolution.ImportedType(name)
	return ok && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport
}

func (c *Checker) classImplements(className, interfaceName string, seen map[string]bool) bool {
	if className == "" || seen[className] {
		return false
	}
	seen[className] = true
	if info := c.classes[className]; info != nil {
		if slices.Contains(info.interfaces, interfaceName) {
			return true
		}
		return c.classImplements(info.superclass, interfaceName, seen)
	}
	binding, ok := c.resolution.ImportedType(className)
	if !ok || binding.Export == nil || binding.Export.Kind != resolver.ClassExport {
		return false
	}
	if slices.Contains(binding.Export.Interfaces, interfaceName) {
		return true
	}
	return c.classImplements(binding.Export.Superclass, interfaceName, seen)
}

func (c *Checker) recordAssignableConversion(expression ast.Expression, target, actual types.Type) {
	if expression != nil && target.Nullable && !actual.Nullable && actual.Kind != types.Nil {
		c.result.Conversions[expression] = target
		return
	}
	if target.Kind == types.Union && actual.Kind == types.Union && unionContainsKind(target, types.Float) && unionContainsKind(actual, types.Int) {
		c.result.Conversions[expression] = target
		return
	}
	if actual.Kind != types.Int || actual.Nullable || target.Nullable {
		return
	}
	if target.Kind == types.Float {
		c.recordIntegerToFloat(expression)
		return
	}
	if target.Kind == types.Union {
		for _, alternative := range target.Args {
			if alternative.Kind == types.Float && !alternative.Nullable {
				c.recordIntegerToFloat(expression)
				return
			}
		}
	}
}

func unionContainsKind(typ types.Type, kind types.Kind) bool {
	for _, alternative := range typ.Args {
		if alternative.Kind == kind && !alternative.Nullable {
			return true
		}
	}
	return false
}

func (c *Checker) recordIntegerToFloat(expression ast.Expression) {
	if expression != nil {
		c.result.Conversions[expression] = types.FromName("Float")
	}
}

func (c *Checker) portableEqualityOperands(left, right types.Type) bool {
	if left.Kind == types.Nil {
		return right.Nullable
	}
	if right.Kind == types.Nil {
		return left.Nullable
	}
	if left.Nullable || right.Nullable {
		return false
	}
	if isNonNullableNumber(left) && isNonNullableNumber(right) {
		return true
	}
	if left.Kind != right.Kind {
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
	seenPositionalDefault := false
	for _, parameter := range method.Parameters {
		typ := c.typeFromRef(parameter.Type)
		if parameter.Type.Empty() {
			typ = types.Type{Kind: types.Any, Name: "Any"}
		}
		if _, exists := methodScope.values[parameter.Name]; exists {
			c.error(parameter.Span(), fmt.Sprintf("parameter %s is duplicated", parameter.Name))
		}
		if !c.rubyNativeSyntax() && !parameter.Keyword && !parameter.Rest && !parameter.KeywordRest {
			if parameter.Default != nil {
				seenPositionalDefault = true
			} else if seenPositionalDefault {
				c.error(parameter.Span(), "required positional parameter cannot follow a default parameter")
			}
		}
		if parameter.Default != nil {
			actual := c.checkExpression(parameter.Default, methodScope)
			actual = c.contextualizeCollectionLiteral(parameter.Default, typ, actual)
			if !c.assignable(parameter.Default, typ, actual) {
				c.error(parameter.Default.Span(), fmt.Sprintf("default value has type %s, expected %s", actual, typ))
			}
		}
		methodScope.values[parameter.Name] = symbol{typ: typ, mutable: true, span: parameter.Span()}
	}
	returnType := c.typeFromRef(method.ReturnType)
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
	if c.interfaceDepth == 0 && returnType.Kind != types.Void && c.statementsFallThrough(method.Body) && !c.hasNativeImplicitReturn(method.Body) {
		c.error(method.Span(), fmt.Sprintf("%s() must return %s on every path", method.Name, returnType))
	}
	if method.Name == "initialize" && c.current != nil {
		c.initializing--
	}
	c.loopDepth = previousLoopDepth
	c.returns = c.returns[:len(c.returns)-1]
	c.classMethod = previousClassMethod
}

func (c *Checker) statementsFallThrough(statements []ast.Statement) bool {
	for _, statement := range statements {
		if !c.statementFallsThrough(statement) {
			return false
		}
	}
	return true
}

func (c *Checker) statementFallsThrough(statement ast.Statement) bool {
	switch node := statement.(type) {
	case *ast.ReturnStatement:
		return false
	case *ast.IfStatement:
		if !node.HasElse || c.statementsFallThrough(node.Then) || c.statementsFallThrough(node.Else) {
			return true
		}
		for _, branch := range node.ElseIf {
			if c.statementsFallThrough(branch.Body) {
				return true
			}
		}
		return false
	case *ast.CaseStatement:
		return c.caseFallsThrough(node)
	case *ast.NativeBlock:
		return c.statementsFallThrough(node.Body)
	case *ast.ExpressionStatement:
		return c.expressionFallsThrough(node.Expression)
	case *ast.VariableStatement:
		return c.expressionFallsThrough(node.Value)
	case *ast.AssignmentStatement:
		return c.expressionFallsThrough(node.Value)
	default:
		return true
	}
}

func (c *Checker) caseFallsThrough(node *ast.CaseStatement) bool {
	if !c.statementsFallThrough(node.Leading) {
		return false
	}
	if !node.HasElse && !c.caseCoversSelector(node) {
		return true
	}
	for _, branch := range node.Branches {
		if c.statementsFallThrough(branch.Body) {
			return true
		}
	}
	return node.HasElse && c.statementsFallThrough(node.Else)
}

func (c *Checker) caseCoversSelector(node *ast.CaseStatement) bool {
	selectorType := c.result.Expressions[node.Value]
	wanted := map[string]bool{}
	if selectorType.Kind == types.Union {
		for _, alternative := range selectorType.Args {
			wanted[alternative.String()] = true
		}
		for _, branch := range node.Branches {
			pattern := c.result.CasePatterns[branch.Value]
			if pattern.TypeUnion && pattern.MatchType.Kind != types.Invalid {
				delete(wanted, pattern.MatchType.String())
			}
		}
	} else {
		variants, ok := c.enumVariants(selectorType)
		if !ok {
			return false
		}
		for _, variant := range variants {
			wanted[variant.Name] = true
		}
		for _, branch := range node.Branches {
			pattern, ok := c.result.CasePatterns[branch.Value]
			if ok {
				delete(wanted, pattern.Variant.Name)
			}
		}
	}
	return len(wanted) == 0
}

func (c *Checker) expressionFallsThrough(expression ast.Expression) bool {
	if expression == nil {
		return true
	}
	return c.result.Expressions[expression].Kind != types.Never
}

func (c *Checker) hasNativeImplicitReturn(statements []ast.Statement) bool {
	if !c.rubyNativeSyntax() {
		return false
	}
	for index := len(statements) - 1; index >= 0; index-- {
		switch statement := statements[index].(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		case *ast.NativeStatement, *ast.NativeBlock:
			return true
		case *ast.ExpressionStatement:
			return c.nativeEscapeExpression(statement.Expression)
		default:
			return false
		}
	}
	return false
}

func (c *Checker) nativeEscapeExpression(expression ast.Expression) bool {
	switch node := expression.(type) {
	case *ast.NativeExpression:
		return true
	case *ast.Identifier:
		_, constant := c.result.Constants[node]
		if c.result.LexicalBindings[node] || constant {
			return false
		}
		if _, ok := c.result.References[node]; ok {
			return false
		}
		if _, ok := c.external[node]; ok {
			return false
		}
		if c.functions[node.Name] != nil || c.current != nil && c.current.methods[node.Name] != nil {
			return false
		}
		_, declared := c.declaredTypes[node.Name]
		return !declared
	case *ast.CallExpression:
		return c.nativeEscapeExpression(node.Callee)
	case *ast.MemberExpression:
		return c.nativeEscapeExpression(node.Receiver)
	case *ast.GenericExpression:
		return c.nativeEscapeExpression(node.Receiver)
	case *ast.IndexExpression:
		return c.nativeEscapeExpression(node.Receiver)
	default:
		return false
	}
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
			c.markImportNodeUsed(imported, "")
			return
		}
	}
	if imported, ok := c.resolution.ImportedType(name); ok && imported.Export.Kind == resolver.ClassExport {
		c.markImportUsed(imported)
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
				required[method.Name] = c.signatureFromMethod(method)
			}
		} else if imported, ok := c.resolution.ImportedType(interfaceName); ok && imported.Export.Kind == resolver.InterfaceExport {
			c.markImportUsed(imported)
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
			return c.signatureFromMethod(method), true
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

func (c *Checker) signatureFromMethod(method *ast.MethodStatement) methodSignature {
	result := methodSignature{returnType: c.methodReturnType(method)}
	for _, parameter := range method.Parameters {
		result.parameters = append(result.parameters, c.typeFromRef(parameter.Type))
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
			return classMember{typ: c.methodReturnType(method), method: method}, true
		}
		if !class {
			if field := info.fields["@"+memberName]; field != nil {
				return classMember{typ: c.typeFromRef(field.Type), field: field}, true
			}
			if field := info.fields["@_"+strings.TrimPrefix(memberName, "_")]; field != nil {
				return classMember{typ: c.typeFromRef(field.Type), field: field}, true
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
	case *ast.IfStatement:
		typ = c.checkIf(n, sc, true)
	case *ast.CaseStatement:
		typ = c.checkCase(n, sc, true)
	case *ast.Identifier:
		if n.Name == "_" {
			c.error(n.Span(), "blank binding _ cannot be used as a value")
			typ = invalidType()
			break
		}
		if value, ok := sc.lookup(n.Name); ok {
			typ = value.typ
			if !value.constant {
				c.result.LexicalBindings[n] = true
			}
			sc.markUsed(n.Name)
			if value.constant {
				c.result.Constants[n] = value.owner
			}
		} else if binding, ok := c.resolution.Symbols[n.Name]; ok {
			typ = binding.Type()
			c.recordReference(n, binding)
		} else if member, ok := c.currentDeclarationMember(n.Name); ok {
			typ = member.Return
			c.external[n] = member
			c.result.ExternalMembers[n] = member
		} else if strings.HasPrefix(n.Name, "@") && c.current != nil {
			if field, ok := c.current.fields[n.Name]; ok {
				typ = c.typeFromRef(field.Type)
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
		element := c.inferCollectionType(n.Elements, sc)
		typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{element}}
	case *ast.HashLiteral:
		if len(n.Entries) == 0 {
			typ = types.Type{Kind: types.Hash, Name: "Hash"}
			break
		}
		keyType := c.checkExpression(n.Entries[0].Key, sc)
		if !portableHashKey(keyType) && !c.rubyNativeSyntax() {
			c.error(n.Entries[0].Key.Span(), fmt.Sprintf("Hash key must be String or Integer, got %s", keyType))
		}
		values := []ast.Expression{n.Entries[0].Value}
		for _, entry := range n.Entries[1:] {
			currentKey := c.checkExpression(entry.Key, sc)
			if !portableHashKey(currentKey) && !c.rubyNativeSyntax() {
				c.error(entry.Key.Span(), fmt.Sprintf("Hash key must be String or Integer, got %s", currentKey))
			}
			if !types.Equivalent(keyType, currentKey) {
				c.error(entry.Key.Span(), fmt.Sprintf("Hash literal key type is %s, expected %s", currentKey, keyType))
			}
			values = append(values, entry.Value)
		}
		valueType := c.inferCollectionType(values, sc)
		typ = types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{keyType, valueType}}
	case *ast.UnaryExpression:
		operand := c.checkExpression(n.Operand, sc)
		typ = c.checkUnaryOperator(n.Span(), n.Operator, operand)
	case *ast.BinaryExpression:
		left := c.checkExpression(n.Left, sc)
		right := c.checkExpression(n.Right, sc)
		typ = c.checkBinaryOperator(n.Span(), n.Operator, left, right)
		if typ.Kind != types.Invalid && isNonNullableNumber(left) && isNonNullableNumber(right) && left.Kind != right.Kind {
			if left.Kind == types.Int {
				c.recordIntegerToFloat(n.Left)
			}
			if right.Kind == types.Int {
				c.recordIntegerToFloat(n.Right)
			}
		}
	case *ast.RangeExpression:
		start := c.checkExpression(n.Start, sc)
		end := c.checkExpression(n.End, sc)
		if start.Kind == types.Never || end.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
		} else if start.Kind != types.Int || end.Kind != types.Int {
			c.error(n.Span(), fmt.Sprintf("range endpoints must be Integer, got %s and %s", start, end))
		} else {
			typ = types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}
		}
	case *ast.IterationExpression:
		sourceType := c.checkExpression(n.Source, sc)
		if sourceType.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
			break
		}
		elementType, iterable := iterableElementType(sourceType)
		hashSource := sourceType.Kind == types.Hash && len(sourceType.Args) == 2
		if hashSource {
			elementType = sourceType.Args[1]
			iterable = true
		}
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
		hashEach := hashSource && n.Operation == "each"
		if hashSource && !hashEach {
			c.error(n.Span(), "Hash iteration supports only each in v0.1")
		}
		if hashEach && n.WithIndex {
			c.error(n.Span(), "Hash#each.with_index is not supported in v0.1")
		}
		itemType := elementType
		transform := n.Operation == "map" || n.Operation == "select" || n.Operation == "reduce"
		if n.Operation == "each_slice" && !hashSource {
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
			blockScope := &scope{parent: sc, values: map[string]symbol{}}
			accumulatorType := types.Type{Kind: types.Any, Name: "Any"}
			if n.Operation == "reduce" {
				if n.Initial == nil {
					c.error(n.Span(), "reduce expects exactly one positional initial value")
				} else {
					accumulatorType = c.checkExpression(n.Initial, sc)
				}
			}
			bindingTypes := []types.Type{itemType}
			switch {
			case hashEach:
				bindingTypes = []types.Type{sourceType.Args[0], sourceType.Args[1]}
			case n.Operation == "reduce":
				bindingTypes = []types.Type{accumulatorType, itemType}
				if n.WithIndex {
					c.error(n.Span(), "reduce.with_index is not supported; use an explicit counter")
				}
			case n.WithIndex:
				bindingTypes = []types.Type{itemType, types.FromName("Integer")}
			}
			c.result.IterationBindings[n] = append([]types.Type(nil), bindingTypes...)
			if len(n.Block.Parameters) != len(bindingTypes) {
				c.error(n.Block.Span(), fmt.Sprintf("%s block expects %d parameter(s), got %d", n.Operation, len(bindingTypes), len(n.Block.Parameters)))
			}
			for index, name := range n.Block.Parameters {
				parameterType := types.Type{Kind: types.Any, Name: "Any"}
				if index < len(bindingTypes) {
					parameterType = bindingTypes[index]
				}
				if _, duplicate := blockScope.values[name]; duplicate {
					c.error(n.Block.Span(), fmt.Sprintf("block parameter %s is duplicated", name))
					continue
				}
				declared := symbol{typ: parameterType, mutable: true, span: n.Block.Span()}
				if tracksUnusedBinding(name) {
					used := false
					declared.used = &used
					declared.useKind = "block parameter"
				}
				blockScope.values[name] = declared
			}
			if transform {
				if len(n.Block.Body) != 1 {
					c.error(n.Block.Span(), fmt.Sprintf("%s block must contain exactly one result expression in v0.1", n.Operation))
				}
				blockType := types.Type{Kind: types.Any, Name: "Any"}
				var resultExpression ast.Expression
				if len(n.Block.Body) == 1 {
					if result, ok := n.Block.Body[0].(*ast.ExpressionStatement); ok {
						resultExpression = result.Expression
						c.checkStatements(n.Block.Body, blockScope)
					} else if result, ok := n.Block.Body[0].(ast.Expression); ok {
						resultExpression = result
						blockType = c.checkExpression(result, blockScope)
						c.checkUnusedBindings(blockScope)
					} else {
						c.checkStatements(n.Block.Body, blockScope)
						c.error(n.Block.Body[0].Span(), fmt.Sprintf("%s block result must be an expression", n.Operation))
					}
				} else {
					c.checkStatements(n.Block.Body, blockScope)
				}
				if resultExpression != nil {
					if transfer := expressionReturn(resultExpression); transfer != nil {
						c.error(transfer.Span(), "return is not supported inside value-producing collection transformations yet")
					}
					blockType = c.result.Expressions[resultExpression]
				}
				c.result.Expressions[n.Block] = blockType
				switch n.Operation {
				case "map":
					typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{blockType}}
				case "select":
					if blockType.Kind != types.Bool || blockType.Nullable {
						c.error(n.Block.Span(), fmt.Sprintf("select block result must be Boolean, got %s", blockType))
					}
					typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{elementType}}
				case "reduce":
					if !c.assignable(resultExpression, accumulatorType, blockType) {
						c.error(n.Block.Span(), fmt.Sprintf("reduce block result is %s, expected %s", blockType, accumulatorType))
					}
					typ = accumulatorType
				}
			} else {
				c.loopDepth++
				c.checkStatements(n.Block.Body, blockScope)
				c.loopDepth--
				c.result.Expressions[n.Block] = types.Type{Kind: types.Void, Name: "Void"}
			}
		}
		if !transform {
			typ = types.Type{Kind: types.Void, Name: "Void"}
		}
	case *ast.GenericExpression:
		for _, argument := range n.Arguments {
			c.validateTypeReference(argument)
		}
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
		if receiverType.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
			break
		}
		if c.enumPattern > 0 && receiverType.Name == c.enumPatternType.Name && len(receiverType.Args) == 0 {
			receiverType = c.enumPatternType
			c.result.Expressions[n.Receiver] = receiverType
		} else if c.enumPattern > 0 && len(receiverType.Args) == 0 {
			if parameters, target, alias := c.aliasDefinition(receiverType.Name); alias {
				parameterSet := map[string]bool{}
				for _, parameter := range parameters {
					parameterSet[parameter] = true
				}
				bindings := map[string]types.Type{}
				bindDeclarationType(target, c.enumPatternType, parameterSet, bindings)
				if len(bindings) == len(parameters) {
					receiverType.Args = make([]types.Type, len(parameters))
					for index, parameter := range parameters {
						receiverType.Args[index] = bindings[parameter]
					}
					c.result.Expressions[n.Receiver] = receiverType
				}
			}
		}
		classAccess := c.classMemberAccess(n.Receiver, sc)
		if identifier, ok := n.Receiver.(*ast.Identifier); ok {
			if imported := c.resolution.Packages[identifier.Name]; imported != nil {
				c.markImportNodeUsed(imported, "")
				if binding, exists := c.resolution.Member(identifier.Name, n.Name); exists {
					typ = binding.Type()
					c.recordReference(n, binding)
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
				c.recordReference(n, binding)
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
				c.recordReference(n, binding)
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
			typ = c.typeFromRef(record.byName[n.Name].Type)
		} else if member, found := c.localMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); found {
			typ = member.typ
		} else if binding, exists := c.importedAncestorMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			typ = binding.Type()
			c.markImportedSymbolUsed(receiverType.Name)
			c.recordReference(n, binding)
		} else if member, exists := c.declarationMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			typ = member.Return
			c.external[n] = member
			c.result.ExternalMembers[n] = member
		} else if exported, exists := c.resolution.CompilerOwnedType(receiverType.Name); exists {
			if member, found := exported.Members[n.Name]; found && !member.Class {
				typ = member.Type
			} else if n.Name != "new" {
				c.error(n.Span(), fmt.Sprintf("type %s has no member %s", receiverType.Name, n.Name))
			}
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
				c.markImportUsed(imported)
				c.error(n.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiverType.Name, imported.Import.Path, n.Name))
			} else if declared, exists := c.declarations().Type(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("externally provided type %s has no member %s", declared.Name, n.Name))
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
			if binding, imported := c.result.References[generic.Receiver]; imported && binding.Library != nil && len(application.TypeArguments) == 1 {
				c.checkCodecApplication(n, binding.Library.Intrinsic, application.TypeArguments[0])
			}
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
			library := c.checkImportedArguments(n.Span(), binding, n.Arguments, argumentTypes, sc)
			if library != nil {
				receiverType := invalidType()
				if member, method := n.Callee.(*ast.MemberExpression); method && library.HasReceiver() {
					receiverType = c.result.Expressions[member.Receiver]
					if library.ReceiverMutable {
						c.requireMutable(member.Receiver, sc, binding.Name+"()")
					}
				}
				typ = inferLibraryReturn(*library, receiverType, argumentTypes)
				if unresolved := unresolvedLibraryTypeParameters(*library, typ); len(unresolved) > 0 {
					c.error(n.Span(), fmt.Sprintf("cannot infer %s for %s()", strings.Join(unresolved, ", "), binding.Name))
					typ = invalidType()
				}
				if (library.Intrinsic == "trb.internal.json.encode" || library.Intrinsic == "trb.web.json") && len(argumentTypes) >= 1 {
					c.checkCodecApplication(n, library.Intrinsic, argumentTypes[0])
				}
				c.checkLibraryEqualityRequirements(n.Span(), binding.Name, *library)
			}
		}
		if member, ok := c.external[n.Callee]; ok {
			var bindings map[string]types.Type
			typ, bindings = c.checkDeclarationArgumentsWithBindings(n.Span(), member, n.Arguments, argumentTypes)
			if blockType, checked := c.checkDeclarationBlock(n, member, sc, bindings); checked {
				typ = blockType
			}
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
			if local, found := c.localMember(receiverType.Name, member.Name, classAccess, map[string]bool{}); found && local.method != nil {
				typ = c.methodReturnType(local.method)
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
					typ = c.methodReturnType(method)
					c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
				}
			} else if method := c.functions[identifier.Name]; method != nil {
				if len(method.TypeParameters) > 0 {
					c.error(identifier.Span(), fmt.Sprintf("generic function %s requires explicit type arguments", identifier.Name))
				} else {
					typ = c.methodReturnType(method)
					c.checkArguments(n.Span(), method, n.Arguments, argumentTypes)
				}
			} else if c.mode != "ruby" {
				c.error(identifier.Span(), fmt.Sprintf("function %s is not declared or imported", identifier.Name))
			} else if !c.resolution.NativeSyntax {
				c.error(identifier.Span(), fmt.Sprintf("Ruby function %s requires an explicit platform import", identifier.Name))
			}
		}
		if n.Block != nil {
			if _, declared := c.external[n.Callee]; !declared {
				if c.mode == "ruby" && c.resolution.NativeSyntax {
					c.checkNativeCallBlock(n.Block, sc)
				} else {
					c.error(n.Block.Span(), "call blocks require a block-accepting package declaration")
				}
			}
		}
	case *ast.IndexExpression:
		receiver := c.checkExpression(n.Receiver, sc)
		indexType := c.checkExpression(n.Index, sc)
		if receiver.Kind == types.Never || indexType.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
		} else if receiver.Kind == types.Array && len(receiver.Args) > 0 {
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
			c.error(n.Span(), "unsupported expression syntax in portable TypeRB")
		} else if !c.resolution.NativeSyntax {
			c.error(n.Span(), "Ruby-native syntax requires import trb/platform/ruby/native or trb/platform/ruby/rails")
		}
	}
	c.result.Expressions[expression] = typ
	return typ
}

func expressionReturn(expression ast.Expression) ast.Statement {
	if expression == nil {
		return nil
	}
	switch node := expression.(type) {
	case *ast.IfStatement:
		if result := expressionReturn(node.Condition); result != nil {
			return result
		}
		groups := [][]ast.Statement{node.Then, node.Else}
		for _, branch := range node.ElseIf {
			groups = append(groups, branch.Body)
		}
		for _, group := range groups {
			if result := statementsReturn(group); result != nil {
				return result
			}
		}
	case *ast.CaseStatement:
		if result := expressionReturn(node.Value); result != nil {
			return result
		}
		for _, branch := range node.Branches {
			if result := statementsReturn(branch.Body); result != nil {
				return result
			}
		}
		return statementsReturn(node.Else)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			if result := expressionReturn(part.Expression); result != nil {
				return result
			}
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			if result := expressionReturn(element); result != nil {
				return result
			}
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			if result := expressionReturn(entry.Key); result != nil {
				return result
			}
			if result := expressionReturn(entry.Value); result != nil {
				return result
			}
		}
	case *ast.UnaryExpression:
		return expressionReturn(node.Operand)
	case *ast.BinaryExpression:
		if result := expressionReturn(node.Left); result != nil {
			return result
		}
		return expressionReturn(node.Right)
	case *ast.RangeExpression:
		if result := expressionReturn(node.Start); result != nil {
			return result
		}
		return expressionReturn(node.End)
	case *ast.CallExpression:
		if result := expressionReturn(node.Callee); result != nil {
			return result
		}
		for _, argument := range node.Arguments {
			if result := expressionReturn(argument.Value); result != nil {
				return result
			}
		}
		if node.Block != nil {
			return statementsReturn(node.Block.Body)
		}
	case *ast.GenericExpression:
		return expressionReturn(node.Receiver)
	case *ast.MemberExpression:
		return expressionReturn(node.Receiver)
	case *ast.IndexExpression:
		if result := expressionReturn(node.Receiver); result != nil {
			return result
		}
		return expressionReturn(node.Index)
	case *ast.BlockExpression:
		return statementsReturn(node.Body)
	}
	return nil
}

func statementsReturn(statements []ast.Statement) ast.Statement {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ReturnStatement:
			return node
		case *ast.ExpressionStatement:
			if result := expressionReturn(node.Expression); result != nil {
				return result
			}
		case *ast.IfStatement:
			if result := expressionReturn(node); result != nil {
				return result
			}
		case *ast.CaseStatement:
			if result := expressionReturn(node); result != nil {
				return result
			}
		case *ast.WhileStatement:
			if result := statementsReturn(node.Body); result != nil {
				return result
			}
		case *ast.NativeBlock:
			if result := statementsReturn(node.Body); result != nil {
				return result
			}
		}
	}
	return nil
}

func (c *Checker) inferCollectionType(expressions []ast.Expression, sc *scope) types.Type {
	if len(expressions) == 0 {
		return types.FromName("Any")
	}

	checked := make([]types.Type, len(expressions))
	for index, expression := range expressions {
		checked[index] = c.checkExpression(expression, sc)
	}

	common := checked[0]
	for _, current := range checked[1:] {
		joined, ok := types.CommonType(common, current)
		if !ok {
			common = types.FromName("Any")
			break
		}
		common = joined
	}
	for index, expression := range expressions {
		c.recordAssignableConversion(expression, common, checked[index])
	}
	return common
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
		fields[index] = resolver.RecordField{Name: field.Name, Type: c.typeFromRef(field.Type)}
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
		actual = c.contextualizeCollectionLiteral(argument.Value, field.Type, actual)
		if !c.assignable(argument.Value, field.Type, actual) {
			c.error(argument.Value.Span(), fmt.Sprintf("record field %s has type %s, expected %s", field.Name, actual, field.Type))
		}
	}
	for _, field := range fields {
		if !used[field.Name] {
			c.error(call.Span(), fmt.Sprintf("%s.new() is missing record field %s", name, field.Name))
		}
	}
}

func (c *Checker) checkImportedArguments(span token.Span, binding resolver.Binding, arguments []ast.CallArgument, actual []types.Type, sc *scope) *stdlib.Symbol {
	var parameters []types.Type
	var parameterIndexes []int
	required := 0
	variadic := false
	name := binding.Name
	var library *stdlib.Symbol
	if binding.Library != nil {
		specialized := stdlib.Instantiate(*binding.Library, actual)
		library = &specialized
		for _, parameter := range specialized.Parameters {
			parameters = append(parameters, parameter.Type)
			if !parameter.Optional {
				required++
			}
		}
		variadic = specialized.Variadic
		keywordAware := false
		for _, parameter := range specialized.Parameters {
			keywordAware = keywordAware || parameter.Keyword
		}
		for _, argument := range arguments {
			keywordAware = keywordAware || argument.Name != ""
		}
		if keywordAware {
			used := make([]bool, len(specialized.Parameters))
			position := 0
			for _, argument := range arguments {
				parameterIndex := -1
				if argument.Name != "" {
					for index, parameter := range specialized.Parameters {
						if parameter.Keyword && parameter.Name == argument.Name {
							parameterIndex = index
							break
						}
					}
					if parameterIndex < 0 {
						c.error(argument.Value.Span(), fmt.Sprintf("%s() has no keyword argument %s", name, argument.Name))
					}
				} else {
					for position < len(specialized.Parameters) && specialized.Parameters[position].Keyword {
						position++
					}
					if position < len(specialized.Parameters) {
						parameterIndex = position
						position++
					} else {
						c.error(argument.Value.Span(), fmt.Sprintf("%s() does not accept this positional argument", name))
					}
				}
				parameterIndexes = append(parameterIndexes, parameterIndex)
				if parameterIndex >= 0 {
					if used[parameterIndex] {
						c.error(argument.Value.Span(), fmt.Sprintf("%s() receives argument %s more than once", name, specialized.Parameters[parameterIndex].Name))
					}
					used[parameterIndex] = true
				}
			}
			for index, parameter := range specialized.Parameters {
				if !parameter.Optional && !used[index] {
					c.error(span, fmt.Sprintf("%s() is missing required argument %s", name, parameter.Name))
				}
			}
		}
	} else if binding.Export != nil {
		parameters = append(parameters, binding.Export.Parameters...)
		required = binding.Export.Required
		variadic = binding.Export.Variadic
	} else if binding.Member != nil {
		parameters = append(parameters, binding.Member.Parameters...)
		required = binding.Member.Required
		variadic = binding.Member.Variadic
	}
	if len(parameterIndexes) == 0 && (len(arguments) < required || (!variadic && len(arguments) > len(parameters))) {
		if variadic {
			c.error(span, fmt.Sprintf("%s() expects at least %d arguments, got %d", name, required, len(arguments)))
		} else {
			c.error(span, fmt.Sprintf("%s() expects %d..%d arguments, got %d", name, required, len(parameters), len(arguments)))
		}
		return library
	}
	for i, actualType := range actual {
		parameterIndex := i
		if len(parameterIndexes) > 0 {
			parameterIndex = parameterIndexes[i]
		}
		if parameterIndex >= len(parameters) {
			parameterIndex = len(parameters) - 1
		}
		if parameterIndex < 0 {
			continue
		}
		expected := parameters[parameterIndex]
		actualType = c.contextualizeCollectionLiteral(arguments[i].Value, expected, actualType)
		actual[i] = actualType
		assignable := c.assignable(arguments[i].Value, expected, actualType)
		if library != nil {
			assignable = libraryAssignable(expected, actualType)
		}
		if library != nil && parameterIndex < len(library.Parameters) && library.Parameters[parameterIndex].Exact {
			assignable = types.Equivalent(expected, actualType)
		}
		if !assignable {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, name, actualType, expected))
		} else if library != nil {
			c.recordAssignableConversion(arguments[i].Value, expected, actualType)
		}
		if library != nil && parameterIndex < len(library.Parameters) && library.Parameters[parameterIndex].Mutable {
			c.requireMutable(arguments[i].Value, sc, name+"()")
		}
	}
	return library
}

func libraryAssignable(expected, actual types.Type) bool {
	if expected.Kind == types.Any || actual.Kind == types.Any || expected.Kind == types.Invalid || actual.Kind == types.Invalid {
		return true
	}
	if expected.Kind == types.Array && actual.Kind == types.Array {
		if len(expected.Args) == 0 || len(actual.Args) == 0 {
			return true
		}
		return len(expected.Args) == 1 && len(actual.Args) == 1 && libraryAssignable(expected.Args[0], actual.Args[0])
	}
	if expected.Kind == types.Hash && actual.Kind == types.Hash {
		if len(expected.Args) == 0 || len(actual.Args) == 0 {
			return true
		}
		return len(expected.Args) == 2 && len(actual.Args) == 2 &&
			libraryAssignable(expected.Args[0], actual.Args[0]) && libraryAssignable(expected.Args[1], actual.Args[1])
	}
	return types.Assignable(expected, actual)
}

func (c *Checker) checkLibraryEqualityRequirements(span token.Span, name string, symbol stdlib.Symbol) {
	for _, typ := range symbol.EqualityTypes {
		if typ.Kind == types.Invalid || len(unresolvedLibraryTypeParameters(symbol, typ)) > 0 {
			continue
		}
		if !c.portableEqualityOperands(typ, typ) {
			c.error(span, fmt.Sprintf("portable equality is not defined for %s, required by %s()", typ, name))
		}
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
	case types.Array, types.Hash, types.StringBuilder, types.Named:
		return true
	default:
		return false
	}
}

func portableHashKey(typ types.Type) bool {
	return typ.Kind == types.Never || !typ.Nullable && (typ.Kind == types.String || typ.Kind == types.Int)
}

func (c *Checker) contextualizeCollectionLiteral(expression ast.Expression, expected, actual types.Type) types.Type {
	if expression == nil {
		return actual
	}
	if expected.Kind == types.Array && len(expected.Args) == 1 && actual.Kind == types.Array {
		literal, ok := expression.(*ast.ArrayLiteral)
		if !ok {
			return actual
		}
		for _, element := range literal.Elements {
			elementType := c.result.Expressions[element]
			elementType = c.contextualizeCollectionLiteral(element, expected.Args[0], elementType)
			if !c.assignable(element, expected.Args[0], elementType) {
				return actual
			}
		}
		c.result.Expressions[expression] = expected
		return expected
	}
	if expected.Kind != types.Hash || len(expected.Args) != 2 || actual.Kind != types.Hash {
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
			valueType = c.contextualizeCollectionLiteral(entry.Value, expected.Args[1], valueType)
			if !c.assignable(entry.Value, expected.Args[1], valueType) {
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

func unresolvedLibraryTypeParameters(symbol stdlib.Symbol, typ types.Type) []string {
	present := map[string]bool{}
	var visit func(types.Type)
	visit = func(current types.Type) {
		present[current.Name] = true
		for _, argument := range current.Args {
			visit(argument)
		}
	}
	visit(typ)
	var result []string
	for _, name := range symbol.TypeParameters {
		if present[name] {
			result = append(result, name)
		}
	}
	return result
}

func portableReceiverKind(kind types.Kind) bool {
	switch kind {
	case types.Bool, types.Int, types.Float, types.String, types.Bytes, types.StringBuilder, types.Array, types.Range, types.Hash:
		return true
	default:
		return false
	}
}

func (c *Checker) checkDeclarationArguments(span token.Span, member declaration.Member, arguments []ast.CallArgument, actual []types.Type) types.Type {
	result, _ := c.checkDeclarationArgumentsWithBindings(span, member, arguments, actual)
	return result
}

func (c *Checker) checkDeclarationArgumentsWithBindings(span token.Span, member declaration.Member, arguments []ast.CallArgument, actual []types.Type) (types.Type, map[string]types.Type) {
	if len(member.Alternatives) > 0 && declarationCallUsesPositionalArguments(arguments) {
		return c.checkDeclarationAlternativeArguments(span, member, arguments, actual), nil
	}
	if member.MinimumArguments > 0 && len(arguments) < member.MinimumArguments {
		c.error(span, fmt.Sprintf("%s() expects at least %d argument(s), got %d", member.Name, member.MinimumArguments, len(arguments)))
	}
	if member.MaximumArguments > 0 && len(arguments) > member.MaximumArguments {
		c.error(span, fmt.Sprintf("%s() expects at most %d argument(s), got %d", member.Name, member.MaximumArguments, len(arguments)))
	}
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
		if len(parameter.LiteralValues) > 0 && !declarationLiteralValueAccepted(argument.Value, parameter.LiteralValues) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() must be one of %s", index+1, member.Name, quotedDeclarationValues(parameter.LiteralValues)))
			continue
		}
		if len(parameter.LiteralArrays) > 0 && !declarationLiteralArrayAccepted(argument.Value, parameter.LiteralArrays) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() must match one of %s", index+1, member.Name, quotedDeclarationArrays(parameter.LiteralArrays)))
			continue
		}
		if len(parameter.LiteralArrayElements) > 0 && !declarationLiteralArrayElementsAccepted(argument.Value, parameter.LiteralArrayElements) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() must be a non-empty literal array containing only %s", index+1, member.Name, quotedDeclarationValues(parameter.LiteralArrayElements)))
			continue
		}
		bindDeclarationType(parameter.Type, actual[index], typeParameters, bindings)
		expected := instantiateDeclarationType(parameter.Type, bindings)
		actual[index] = c.contextualizeCollectionLiteral(argument.Value, expected, actual[index])
		if !c.assignable(argument.Value, expected, actual[index]) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, member.Name, actual[index], expected))
		}
	}
	for _, parameter := range member.Parameters {
		if !parameter.Optional && !used[parameter.Name] && !member.Variadic {
			c.error(span, fmt.Sprintf("%s() is missing required argument %s", member.Name, parameter.Name))
		}
	}
	return instantiateDeclarationType(member.Return, bindings), bindings
}

func (c *Checker) checkDeclarationBlock(call *ast.CallExpression, member declaration.Member, sc *scope, bindings map[string]types.Type) (types.Type, bool) {
	if bindings == nil {
		bindings = map[string]types.Type{}
	}
	if member.Block == nil {
		if call.Block != nil {
			c.error(call.Block.Span(), fmt.Sprintf("%s() does not accept a block", member.Name))
		}
		return types.Type{}, false
	}
	if call.Block == nil {
		c.error(call.Span(), fmt.Sprintf("%s() requires a block", member.Name))
		return types.Type{}, false
	}
	if member.Block.Structured && len(c.returns) == 0 {
		c.error(call.Span(), fmt.Sprintf("structured block %s() is only valid inside a function or method", member.Name))
	}
	if len(call.Block.Parameters) != len(member.Block.Parameters) {
		c.error(call.Block.Span(), fmt.Sprintf("%s block expects %d parameter(s), got %d", member.Name, len(member.Block.Parameters), len(call.Block.Parameters)))
	}
	blockScope := &scope{parent: sc, values: map[string]symbol{}}
	for index, name := range call.Block.Parameters {
		parameterType := types.Type{Kind: types.Any, Name: "Any"}
		if index < len(member.Block.Parameters) {
			parameterType = instantiateDeclarationType(member.Block.Parameters[index], bindings)
		}
		if _, duplicate := blockScope.values[name]; duplicate {
			c.error(call.Block.Span(), fmt.Sprintf("block parameter %s is duplicated", name))
			continue
		}
		declared := symbol{typ: parameterType, mutable: true, span: call.Block.Span()}
		if tracksUnusedBinding(name) {
			used := false
			declared.used = &used
			declared.useKind = "block parameter"
		}
		blockScope.values[name] = declared
	}
	if member.Block.Return.Name == "" {
		c.loopDepth++
		c.checkStatements(call.Block.Body, blockScope)
		c.loopDepth--
		c.result.Expressions[call.Block] = types.Type{Kind: types.Void, Name: "Void"}
		return instantiateDeclarationType(member.Return, bindings), true
	}

	resultIndex, resultExpression := controlFlowBranchExpression(call.Block.Body)
	if resultExpression == nil {
		c.checkStatements(call.Block.Body, blockScope)
		c.error(call.Block.Span(), fmt.Sprintf("%s block must end with a result expression", member.Name))
		return invalidType(), true
	}

	blockReturn := instantiateDeclarationType(member.Block.Return, bindings)
	c.returns = append(c.returns, blockReturn)
	previousLoopDepth := c.loopDepth
	c.loopDepth = 0
	c.checkStatementSequence(call.Block.Body[:resultIndex], blockScope)
	actual := c.checkExpression(resultExpression, blockScope)
	c.checkStatementSequence(call.Block.Body[resultIndex+1:], blockScope)
	c.checkUnusedBindings(blockScope)
	c.loopDepth = previousLoopDepth
	c.returns = c.returns[:len(c.returns)-1]

	typeParameters := map[string]bool{}
	for _, name := range member.TypeParameters {
		typeParameters[name] = true
	}
	bindDeclarationType(
		c.expandAlias(member.Block.Return, map[string]bool{}),
		c.expandAlias(actual, map[string]bool{}),
		typeParameters,
		bindings,
	)
	blockReturn = instantiateDeclarationType(member.Block.Return, bindings)
	if !c.assignable(resultExpression, blockReturn, actual) {
		c.error(resultExpression.Span(), fmt.Sprintf("%s block result has type %s, expected %s", member.Name, actual, blockReturn))
	}
	c.result.Expressions[call.Block] = blockReturn
	c.result.StructuredBlocks[call] = StructuredBlock{
		Parameters: instantiateDeclarationTypes(member.Block.Parameters, bindings),
		Return:     blockReturn,
		Result:     resultExpression,
	}
	return instantiateDeclarationType(member.Return, bindings), true
}

func (c *Checker) structuredBlockCall(expression ast.Expression) (*ast.CallExpression, declaration.Member, bool) {
	call, ok := expression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return nil, declaration.Member{}, false
	}
	member, ok := c.result.ExternalMembers[call.Callee]
	return call, member, ok && member.Block != nil
}

func (c *Checker) checkStructuredBlockValue(expression ast.Expression) {
	call, member, ok := c.structuredBlockCall(expression)
	if !ok {
		if call, isCall := expression.(*ast.CallExpression); isCall && call.Block != nil {
			c.error(call.Span(), "call block cannot be used as a value without a structured package declaration")
		}
		return
	}
	if !member.Block.Structured {
		c.error(call.Span(), fmt.Sprintf("block call %s() cannot be used as a value", member.Name))
	}
}

func (c *Checker) checkNativeCallBlock(block *ast.BlockExpression, sc *scope) {
	blockScope := &scope{parent: sc, values: map[string]symbol{}}
	for _, name := range block.Parameters {
		if _, duplicate := blockScope.values[name]; duplicate {
			c.error(block.Span(), fmt.Sprintf("block parameter %s is duplicated", name))
			continue
		}
		blockScope.values[name] = symbol{typ: types.Type{Kind: types.Any, Name: "Any"}, mutable: true, span: block.Span()}
	}
	c.loopDepth++
	c.checkStatements(block.Body, blockScope)
	c.loopDepth--
	c.result.Expressions[block] = types.Type{Kind: types.Void, Name: "Void"}
}

func (c *Checker) checkDeclarationAlternativeArguments(span token.Span, member declaration.Member, arguments []ast.CallArgument, actual []types.Type) types.Type {
	candidates := make([]declaration.Signature, 0, len(member.Alternatives))
	for _, signature := range member.Alternatives {
		if declarationSignatureAcceptsArity(signature, len(arguments)) {
			candidates = append(candidates, signature)
		}
	}
	if len(candidates) == 0 {
		c.error(span, fmt.Sprintf("%s() does not accept %d positional arguments", member.Name, len(arguments)))
		return member.Return
	}
	for index, argument := range arguments {
		allowed := map[string]bool{}
		constrained := true
		for _, signature := range candidates {
			parameter := declarationSignatureParameter(signature, index)
			if len(parameter.LiteralValues) == 0 {
				constrained = false
				break
			}
			for _, value := range parameter.LiteralValues {
				allowed[value] = true
			}
		}
		if !constrained {
			continue
		}
		values := sortedDeclarationValues(allowed)
		if !declarationLiteralValueAccepted(argument.Value, values) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() must be one of %s", index+1, member.Name, quotedDeclarationValues(values)))
			return member.Return
		}
		filtered := candidates[:0]
		for _, signature := range candidates {
			if declarationLiteralValueAccepted(argument.Value, declarationSignatureParameter(signature, index).LiteralValues) {
				filtered = append(filtered, signature)
			}
		}
		candidates = filtered
	}
	selected := candidates[0]
	member.Parameters = selected.Parameters
	member.Return = selected.Return
	member.Variadic = selected.Variadic
	member.Alternatives = nil
	return c.checkDeclarationArguments(span, member, arguments, actual)
}

func declarationCallUsesPositionalArguments(arguments []ast.CallArgument) bool {
	for _, argument := range arguments {
		if argument.Name == "" {
			return true
		}
	}
	return false
}

func declarationSignatureAcceptsArity(signature declaration.Signature, count int) bool {
	required := 0
	for _, parameter := range signature.Parameters {
		if !parameter.Optional {
			required++
		}
	}
	return count >= required && (signature.Variadic || count <= len(signature.Parameters))
}

func declarationSignatureParameter(signature declaration.Signature, index int) declaration.Parameter {
	if index < len(signature.Parameters) {
		return signature.Parameters[index]
	}
	return signature.Parameters[len(signature.Parameters)-1]
}

func declarationLiteralValueAccepted(expression ast.Expression, allowed []string) bool {
	value, ok := declarationLiteralValue(expression)
	return ok && slices.Contains(allowed, value)
}

func declarationLiteralValue(expression ast.Expression) (string, bool) {
	switch literal := expression.(type) {
	case *ast.Literal:
		if literal.Kind != ast.StringLiteral {
			return "", false
		}
		if value, err := strconv.Unquote(literal.Raw); err == nil {
			return value, true
		}
		return strings.Trim(literal.Raw, "'\""), true
	case *ast.SymbolLiteral:
		return literal.Name, true
	default:
		return "", false
	}
}

func declarationLiteralArrayAccepted(expression ast.Expression, allowed [][]string) bool {
	literal, ok := expression.(*ast.ArrayLiteral)
	if !ok {
		return false
	}
	values := make([]string, len(literal.Elements))
	for index, element := range literal.Elements {
		value, ok := declarationLiteralValue(element)
		if !ok {
			return false
		}
		values[index] = value
	}
	for _, candidate := range allowed {
		if slices.Equal(values, candidate) {
			return true
		}
	}
	return false
}

func declarationLiteralArrayElementsAccepted(expression ast.Expression, allowed []string) bool {
	literal, ok := expression.(*ast.ArrayLiteral)
	if !ok || len(literal.Elements) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, element := range literal.Elements {
		value, ok := declarationLiteralValue(element)
		if !ok || !slices.Contains(allowed, value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func sortedDeclarationValues(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func quotedDeclarationValues(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = strconv.Quote(value)
	}
	return strings.Join(quoted, ", ")
}

func quotedDeclarationArrays(values [][]string) string {
	formatted := make([]string, len(values))
	for index, value := range values {
		symbols := make([]string, len(value))
		for elementIndex, element := range value {
			symbols[elementIndex] = ":" + element
		}
		formatted[index] = "[" + strings.Join(symbols, ", ") + "]"
	}
	return strings.Join(formatted, ", ")
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

func instantiateDeclarationTypes(input []types.Type, bindings map[string]types.Type) []types.Type {
	result := make([]types.Type, len(input))
	for index, typ := range input {
		result[index] = instantiateDeclarationType(typ, bindings)
	}
	return result
}

func instantiateEnumVariant(input EnumVariant, bindings map[string]types.Type) EnumVariant {
	result := input
	result.TypeArguments = instantiateDeclarationTypes(input.TypeArguments, bindings)
	result.Fields = make([]EnumField, len(input.Fields))
	for index, field := range input.Fields {
		result.Fields[index] = field
		result.Fields[index].Type = instantiateDeclarationType(field.Type, bindings)
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

func (c *Checker) methodReturnType(method *ast.MethodStatement) types.Type {
	if method == nil || method.ReturnType.Empty() {
		return types.Type{Kind: types.Void, Name: "Void"}
	}
	return c.typeFromRef(method.ReturnType)
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
		expected := c.typeFromRef(method.Parameters[i].Type)
		if method.Parameters[i].Type.Empty() || method.Parameters[i].Rest || method.Parameters[i].KeywordRest {
			continue
		}
		argumentType = c.contextualizeCollectionLiteral(arguments[i].Value, expected, argumentType)
		actual[i] = argumentType
		if !c.assignable(arguments[i].Value, expected, argumentType) {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, method.Name, argumentType, expected))
		}
	}
}

func fromTypeRef(ref ast.TypeRef) types.Type {
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = fromTypeRef(alternative)
		}
		return types.UnionOf(alternatives...)
	}
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

func (c *Checker) typeFromRef(ref ast.TypeRef) types.Type {
	return c.expandAlias(fromTypeRef(ref), map[string]bool{})
}

func (c *Checker) aliasDefinition(name string) ([]string, types.Type, bool) {
	if alias := c.aliases[name]; alias != nil {
		return alias.typeParameters, alias.target, true
	}
	if binding, imported := c.resolution.ImportedType(name); imported && binding.Export.Kind == resolver.TypeAliasExport {
		return binding.Export.TypeParameters, binding.Export.AliasTarget, true
	}
	if exported, exists := c.resolution.CompilerOwnedType(name); exists && exported.Kind == resolver.TypeAliasExport {
		return exported.TypeParameters, exported.AliasTarget, true
	}
	return nil, types.Type{}, false
}

func (c *Checker) expandAlias(typ types.Type, visiting map[string]bool) types.Type {
	if typ.Kind == types.Union {
		result := typ
		result.Args = make([]types.Type, len(typ.Args))
		for index, alternative := range typ.Args {
			result.Args[index] = c.expandAlias(alternative, visiting)
		}
		return result
	}
	arguments := make([]types.Type, len(typ.Args))
	for index, argument := range typ.Args {
		arguments[index] = c.expandAlias(argument, visiting)
	}
	typ.Args = arguments
	parameters, target, alias := c.aliasDefinition(typ.Name)
	if !alias {
		return typ
	}
	if visiting[typ.Name] {
		if !c.aliasCycles[typ.Name] {
			span := token.Span{}
			if local := c.aliases[typ.Name]; local != nil {
				span = local.statement.Span()
			}
			c.error(span, "type alias cycle involving "+typ.Name)
			c.aliasCycles[typ.Name] = true
		}
		return invalidType()
	}
	if len(parameters) != len(typ.Args) {
		return typ
	}
	visiting[typ.Name] = true
	expanded := substituteType(target, typeSubstitutions(parameters, typ.Args))
	expanded.Nullable = expanded.Nullable || typ.Nullable
	expanded.Readonly = expanded.Readonly || typ.Readonly
	expanded = c.expandAlias(expanded, visiting)
	delete(visiting, typ.Name)
	return expanded
}

func isConstant(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func (c *Checker) error(span token.Span, message string) {
	c.diags = append(c.diags, diagnostic.Diagnostic{Severity: diagnostic.Error, Message: message, Span: span})
}
