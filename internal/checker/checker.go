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
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type Result struct {
	Program                    *ast.Program
	Expressions                map[ast.Expression]types.Type
	Conversions                map[ast.Expression]types.Type
	NullableUnwraps            map[ast.Expression]types.Type
	NativeResultBridges        map[ast.Expression]NativeResultBridge
	NativeCallResultBridges    map[*ast.CallExpression]NativeCallResultBridge
	Variables                  map[*ast.VariableStatement]types.Type
	Iterations                 map[*ast.IterationExpression]types.Type
	IterationBindings          map[*ast.IterationExpression][]types.Type
	LexicalBindings            map[*ast.Identifier]bool
	Constants                  map[ast.Expression]string
	ConstantOwners             map[*ast.VariableStatement]string
	Resolution                 resolver.Result
	References                 map[ast.Expression]resolver.Binding
	ExpressionDeclarations     map[ast.Expression]identity.Declaration
	ExpressionDispatches       map[ast.Expression]identity.Dispatch
	Declarations               map[ast.Statement]identity.Declaration
	MethodDispatches           map[*ast.MethodStatement]identity.Dispatch
	ResolvedTypes              map[token.Span]types.Type
	EnumConstructors           map[*ast.CallExpression]EnumVariant
	EnumArgumentIndexes        map[*ast.CallExpression][]int
	CasePatterns               map[ast.Expression]CasePattern
	CaseNarrowings             map[*ast.CaseStatement]CaseNarrowing
	GenericApplications        map[*ast.GenericExpression]GenericApplication
	CodecApplications          map[*ast.CallExpression]CodecApplication
	RecordConstructions        map[*ast.CallExpression]RecordConstruction
	CallSpecializationRequests map[*ast.CallExpression]CallSpecializationRequest
	CallSpecializations        map[*ast.CallExpression]CallSpecialization
	CallSignatures             map[*ast.CallExpression][]callsignature.Parameter
	DeclarationOnlyCalls       map[*ast.CallExpression]bool
	RawEnums                   map[*ast.EnumStatement]RawEnum
	EnumCalls                  map[*ast.CallExpression]EnumCall
	TypeAliases                map[*ast.TypeAliasStatement]TypeAlias
	InterfaceImplementations   map[*ast.ClassStatement][]InterfaceImplementation
	Newtypes                   map[*ast.NewtypeStatement]Newtype
	NewtypeCalls               map[*ast.CallExpression]NewtypeCall
	ResultTries                map[*ast.TryExpression]ResultTry
	ResultCatches              map[*ast.CatchExpression]ResultCatch
	StructuredBlocks           map[*ast.CallExpression]StructuredBlock
	ExternalMembers            map[ast.Expression]declaration.Member
	ClassFieldAccesses         map[*ast.MemberExpression]bool
	UnionMemberAccesses        map[*ast.MemberExpression][]UnionMemberAccess
	RuntimeDependencies        map[string]*stdlib.Package
	ImportUses                 map[*ast.ImportStatement]map[string]bool
	CompilerGeneratedStart     int
}

type CodecApplication struct {
	Operation string
	Schema    CodecSchema
}

type RecordConstruction struct {
	Fields      []resolver.RecordField
	Target      ast.Expression
	Declaration identity.Declaration
}

type CallSpecializationRequest struct {
	Request  packageextension.SpecializeCallRequest
	Receiver ast.Expression
}

type CallSpecialization struct {
	Callee    string
	Arguments []ast.Expression
}

type RawEnum struct {
	Type   types.Type
	Values map[string]RawEnumValue
}

type RawEnumValue struct {
	Raw  string
	Type types.Type
}

type EnumCall struct {
	EnumName      string
	Owner         string
	OwnerIdentity identity.Declaration
	Method        string
	Receiver      ast.Expression
	Reference     *resolver.Binding
	Raw           *RawEnum
}

type TypeAlias struct {
	Target                types.Type
	AuthoredTargetBinding *resolver.Binding
	TargetBinding         *resolver.Binding
	Variants              []EnumVariant
}

type InterfaceImplementation struct {
	Type          types.Type
	TargetBinding *resolver.Binding
}

type Newtype struct {
	Target types.Type
}

type NewtypeCall struct {
	Operation      string
	Type           types.Type
	Representation types.Type
}

// ResultTry describes the two Result variants used by a prefix try expression
// and the compatible Result boundary that receives its Err payload.
type ResultTry struct {
	SuccessType       types.Type
	ErrorType         types.Type
	ReturnSuccessType types.Type
	ReturnErrorType   types.Type
	ReturnType        types.Type
}

// ResultCatch describes the Result payload and the checked recovery branch of
// a catch expression. A nil HandlerResult denotes a diverging handler.
type ResultCatch struct {
	SuccessType     types.Type
	ErrorType       types.Type
	ResultType      types.Type
	HandlerResult   ast.Expression
	HandlerDiverges bool
}

type NativeResultBridge struct {
	Kind string
	Type types.Type
}

type NativeCallResultBridge struct {
	Kind       string
	Success    types.Type
	Error      types.Type
	ResultType types.Type
}

type StructuredBlock struct {
	Parameters     []types.Type
	Return         types.Type
	Result         ast.Expression
	ResultBoundary types.Type
	ResultType     types.Type
}

type CodecSchema struct {
	Type      types.Type
	Kind      string
	Module    string
	Reference *resolver.Binding
	Element   *CodecSchema
	Fields    []CodecField
	RawType   types.Type
	RawValues []CodecRawValue
}

type CodecRawValue struct {
	Member string
	Raw    string
}

type CodecField struct {
	Name     string
	WireName string
	Schema   *CodecSchema
}

type EnumField struct {
	Name      string
	Type      types.Type
	NamedOnly bool
}

type EnumVariant struct {
	EnumName      string
	Owner         string
	Declaration   identity.Declaration
	Name          string
	Fields        []EnumField
	TypeArguments []types.Type
	Reference     *resolver.Binding
}

type GenericApplication struct {
	Name                   string
	Kind                   string
	Owner                  string
	TypeParameters         []string
	TypeArguments          []types.Type
	OwnerArguments         []types.Type
	Parameters             []callsignature.Parameter
	ParameterResultBridges []resolver.NativeResultBridge
	CallResultBridge       resolver.NativeCallResultBridge
	ReturnType             types.Type
	Variadic               bool
	Specializer            string
	Source                 bool
	Declaration            identity.Declaration
	Dispatch               identity.Dispatch
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

type CaseNarrowing struct {
	Name     string
	Branches map[ast.Expression]types.Type
	Else     types.Type
}

type UnionMemberAccess struct {
	Alternative types.Type
	Member      types.Type
}

type symbol struct {
	typ                types.Type
	declared           types.Type
	mutable            bool
	constant           bool
	owner              string
	span               token.Span
	variable           *ast.VariableStatement
	used               *bool
	useKind            string
	mustUseResult      bool
	pending            *pendingEmptyCollection
	concurrentBorrowed bool
}

// pendingEmptyCollection is shared by every lexical reference found during the
// constraint pass. It resolves once, at the end of the first constraining
// statement owned by its declaration scope.
type pendingEmptyCollection struct {
	variable *ast.VariableStatement
	owner    *scope
	kind     types.Kind
	resolved types.Type
	blocked  bool
}

type emptyCollectionOutcome struct {
	typ         types.Type
	keySpan     token.Span
	blocked     bool
	blockedSpan token.Span
}

type emptyCollectionConstraint struct {
	typ  types.Type
	span token.Span
}

type emptyCollectionRegionConstraints struct {
	exact       *types.Type
	elements    []emptyCollectionConstraint
	keys        []emptyCollectionConstraint
	values      []emptyCollectionConstraint
	captured    bool
	captureSpan token.Span
	escaped     bool
	escapeSpan  token.Span
}

type emptyCollectionInferenceRegion struct {
	scope       *scope
	constraints map[*pendingEmptyCollection]*emptyCollectionRegionConstraints
}

func tracksUnusedBinding(name string) bool {
	return name != "_" && !strings.HasPrefix(name, "_")
}

type scope struct {
	parent           *scope
	values           map[string]symbol
	nullableMembers  map[nullableMemberKey]nullableMemberFact
	constantsAllowed bool
	constantOwner    string
	enumsAllowed     bool
}

type nullableMemberKey struct {
	rootName   string
	rootOffset int
	member     string
}

type nullableMemberFact struct {
	source types.Type
	valid  bool
}

func (s *scope) lookup(name string) (symbol, bool) {
	value, _, ok := s.lookupOwner(name)
	return value, ok
}

func (s *scope) lookupOwner(name string) (symbol, *scope, bool) {
	for current := s; current != nil; current = current.parent {
		if value, ok := current.values[name]; ok {
			if value.pending != nil && value.pending.resolved.Kind != "" {
				value.typ = value.pending.resolved
			}
			return value, current, true
		}
	}
	return symbol{}, nil, false
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

func (s *scope) setAssignmentType(name string, declared, flow types.Type) {
	value, _, ok := s.lookupOwner(name)
	if !ok {
		return
	}
	if value.declared.Kind == "" {
		value.declared = declared
	}
	value.typ = flow
	// Assignments inside a conditional or callback affect subsequent statements
	// in that path without leaking an unproven flow type into the parent path.
	s.values[name] = value
}

func (s *scope) nullableMember(key nullableMemberKey) (nullableMemberFact, bool) {
	for current := s; current != nil; current = current.parent {
		if fact, ok := current.nullableMembers[key]; ok {
			return fact, fact.valid
		}
	}
	return nullableMemberFact{}, false
}

func (s *scope) setNullableMember(key nullableMemberKey, source types.Type) {
	if s.nullableMembers == nil {
		s.nullableMembers = map[nullableMemberKey]nullableMemberFact{}
	}
	s.nullableMembers[key] = nullableMemberFact{source: source, valid: true}
}

func (s *scope) resetNullableMembers(rootName string, rootOffset int) {
	invalidated := map[nullableMemberKey]bool{}
	for current := s; current != nil; current = current.parent {
		for key := range current.nullableMembers {
			if key.rootName == rootName && key.rootOffset == rootOffset {
				invalidated[key] = true
			}
		}
	}
	if len(invalidated) == 0 {
		return
	}
	if s.nullableMembers == nil {
		s.nullableMembers = map[nullableMemberKey]nullableMemberFact{}
	}
	for key := range invalidated {
		s.nullableMembers[key] = nullableMemberFact{}
	}
}

type classInfo struct {
	name           string
	typeParameters []string
	superclass     string
	interfaces     []types.Type
	mixins         []string
	fields         map[string]*ast.FieldStatement
	methods        map[string]*ast.MethodStatement
}

type classMember struct {
	typ    types.Type
	method *ast.MethodStatement
	field  *ast.FieldStatement
	sig    *methodSignature
}

type recordInfo struct {
	name           string
	typeParameters []string
	fields         []*ast.RecordFieldStatement
	byName         map[string]*ast.RecordFieldStatement
}

type enumInfo struct {
	name           string
	typeParameters []string
	members        []string
	byName         map[string]*ast.EnumMemberStatement
	methods        map[string]*ast.MethodStatement
	raw            *RawEnum
}

type aliasInfo struct {
	statement      *ast.TypeAliasStatement
	typeParameters []string
	target         types.Type
}

type newtypeInfo struct {
	statement *ast.NewtypeStatement
	target    types.Type
}

type typeDeclaration struct {
	kind           string
	span           token.Span
	typeParameters []string
	identity       identity.Declaration
}

type Checker struct {
	mode                        string
	result                      Result
	diags                       []diagnostic.Diagnostic
	classes                     map[string]*classInfo
	records                     map[string]*recordInfo
	enums                       map[string]*enumInfo
	aliases                     map[string]*aliasInfo
	newtypes                    map[string]*newtypeInfo
	interfaces                  map[string]*ast.InterfaceStatement
	functions                   map[string]*ast.MethodStatement
	current                     *classInfo
	currentEnum                 *enumInfo
	classMethod                 bool
	initializing                int
	loopDepth                   int
	valueTransformDepth         int
	resultBoundaryBlockDepth    int
	controlBoundaries           []string
	moduleDepth                 int
	interfaceDepth              int
	runnableMain                *ast.MethodStatement
	returns                     []types.Type
	resolution                  resolver.Result
	external                    map[ast.Expression]declaration.Member
	declaredTypes               map[string]typeDeclaration
	enumCallee                  int
	enumPattern                 int
	enumPatternType             types.Type
	usedImports                 map[*ast.ImportStatement]map[string]bool
	allowUnusedImports          bool
	interactiveTopLevel         bool
	compilerGeneratedStart      int
	aliasCycles                 map[string]bool
	resultBoundaries            []resultBoundary
	directStructuredResultValue ast.Expression
	directStructuredResultKind  string
	declarationReferences       int
	declarationCalls            map[*ast.CallExpression]string
	inferenceOnly               bool
	emptyCollectionOutcomes     map[*ast.VariableStatement]emptyCollectionOutcome
	emptyCollectionRegions      []*emptyCollectionInferenceRegion
	pendingExpressions          map[ast.Expression]*pendingEmptyCollection
	callbackScopes              []*scope
	pendingEmptyCollections     int
	concurrentBlockScopes       []*scope
	borrowedExpressions         map[ast.Expression]bool
	concurrentFunctions         map[*ast.MethodStatement]bool
	concurrentConstructors      map[*ast.MethodStatement]bool
	concurrentClasses           map[string]bool
	currentMethod               *ast.MethodStatement
	currentMethodScopes         []*scope
	currentFieldClass           string
	concurrentMapDepth          int
	authoredMemberMethods       map[*ast.MemberExpression]*ast.MethodStatement
	concurrentInterfaceMembers  map[*ast.MemberExpression]bool
	authoredOwnedMethods        map[string]*ast.MethodStatement
	authoredTypes               map[string]string
	authoredEnumOwners          map[string]string
	authoredTypeIdentities      map[string]identity.Declaration
	authoredOwnerIdentities     map[string]identity.Declaration
	activeTypeParameters        map[string]int
	activeTypeOwner             string
	authoredCalls               map[*ast.MethodStatement]map[*ast.MethodStatement]bool
	authoredConstructorCalls    map[*ast.MethodStatement]map[*ast.MethodStatement]bool
	authoredOrdinaryCalls       map[*ast.MethodStatement]map[*ast.MethodStatement]bool
	authoredMethodConstructors  map[*ast.MethodStatement]map[string]bool
	authoredClassCalls          map[string]map[*ast.MethodStatement]bool
	authoredClassConstructors   map[string]map[string]bool
	concurrentCallRoots         map[*ast.MethodStatement]bool
	concurrentConstructorRoots  map[*ast.MethodStatement]bool
	concurrentOrdinaryRoots     map[*ast.MethodStatement]bool
	concurrentClassRoots        map[string]bool
	concurrentInitTargets       map[*ast.Identifier]bool
	recordDefaultUnavailable    map[string]bool
	recordDefaultCallee         *ast.Identifier
}

// resultBoundary is the lexical destination for prefix try. A boundary entry
// is pushed for every function-like scope, including scopes whose return type
// is not Result, so propagation never leaks into an enclosing function.
type resultBoundary struct {
	success types.Type
	failure types.Type
	result  types.Type
	valid   bool
	tries   []*ast.TryExpression
}

type Options struct {
	AllowUnusedImports     bool
	InteractiveTopLevel    bool
	RunnableMain           *ast.MethodStatement
	CompilerGeneratedStart int
}

func Check(program *ast.Program, resolution resolver.Result) (Result, []diagnostic.Diagnostic) {
	return CheckWithOptions(program, resolution, Options{})
}

func CheckWithOptions(program *ast.Program, resolution resolver.Result, options Options) (Result, []diagnostic.Diagnostic) {
	// Only programs that contain the opt-in mutable-empty form pay for the
	// constraint pass. The ordinary pass remains authoritative for diagnostics,
	// conversions, and typed IR consumed by every backend.
	var outcomes map[*ast.VariableStatement]emptyCollectionOutcome
	if statementsHaveFreshEmptyMutableCollection(program.Statements) {
		inference := newChecker(program, resolution, options)
		inference.inferenceOnly = true
		inference.allowUnusedImports = true
		inference.emptyCollectionOutcomes = map[*ast.VariableStatement]emptyCollectionOutcome{}
		inference.pendingExpressions = map[ast.Expression]*pendingEmptyCollection{}
		inference.checkProgram(program, false)
		outcomes = inference.emptyCollectionOutcomes
	}
	c := newChecker(program, resolution, options)
	c.emptyCollectionOutcomes = outcomes
	c.checkProgram(program, true)
	if concurrentFunctions, concurrentConstructors, concurrentClasses := c.resolvedConcurrentFunctions(); len(concurrentFunctions) > 0 || len(concurrentClasses) > 0 {
		audited := newChecker(program, resolution, options)
		audited.emptyCollectionOutcomes = outcomes
		audited.concurrentFunctions = concurrentFunctions
		audited.concurrentConstructors = concurrentConstructors
		audited.concurrentClasses = concurrentClasses
		audited.checkProgram(program, true)
		return audited.result, diagnostic.Normalize(audited.diags, "", diagnostic.TypeError)
	}
	return c.result, diagnostic.Normalize(c.diags, "", diagnostic.TypeError)
}

// statementsHaveFreshEmptyMutableCollection is a lightweight fast-path scan.
// Keep its expression descent exhaustive when adding syntax nodes that
// can contain authored statement bodies.
func statementsHaveFreshEmptyMutableCollection(statements []ast.Statement) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			if expressionHasFreshEmptyMutableCollection(node.Superclass) || statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		case *ast.RecordStatement:
			if statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		case *ast.EnumStatement:
			if statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		case *ast.EnumMemberStatement:
			if expressionHasFreshEmptyMutableCollection(node.RawValue) {
				return true
			}
			for _, attribute := range node.Attributes {
				for _, argument := range attribute.Arguments {
					if expressionHasFreshEmptyMutableCollection(argument.Value) {
						return true
					}
				}
			}
		case *ast.RecordFieldStatement:
			if expressionHasFreshEmptyMutableCollection(node.Default) {
				return true
			}
			for _, attribute := range node.Attributes {
				for _, argument := range attribute.Arguments {
					if expressionHasFreshEmptyMutableCollection(argument.Value) {
						return true
					}
				}
			}
		case *ast.ModuleStatement:
			if statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		case *ast.InterfaceStatement:
			for _, method := range node.Methods {
				if methodHasFreshEmptyMutableCollection(method) {
					return true
				}
			}
		case *ast.FieldStatement:
			if expressionHasFreshEmptyMutableCollection(node.Value) {
				return true
			}
		case *ast.MethodStatement:
			if methodHasFreshEmptyMutableCollection(node) {
				return true
			}
		case *ast.VariableStatement:
			if (node.Mutable && node.Type.Empty() && freshEmptyCollectionKind(node.Value) != "") ||
				expressionHasFreshEmptyMutableCollection(node.Value) {
				return true
			}
		case *ast.AssignmentStatement:
			if expressionHasFreshEmptyMutableCollection(node.Target) || expressionHasFreshEmptyMutableCollection(node.Value) {
				return true
			}
		case *ast.ReturnStatement:
			if expressionHasFreshEmptyMutableCollection(node.Value) {
				return true
			}
		case *ast.IfStatement:
			if expressionHasFreshEmptyMutableCollection(node) {
				return true
			}
		case *ast.CaseStatement:
			if expressionHasFreshEmptyMutableCollection(node) {
				return true
			}
		case *ast.WhileStatement:
			if expressionHasFreshEmptyMutableCollection(node.Condition) || statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		case *ast.ExpressionStatement:
			if expressionHasFreshEmptyMutableCollection(node.Expression) {
				return true
			}
		case *ast.NativeBlock:
			if statementsHaveFreshEmptyMutableCollection(node.Body) {
				return true
			}
		}
	}
	return false
}

func methodHasFreshEmptyMutableCollection(method *ast.MethodStatement) bool {
	if method == nil {
		return false
	}
	for _, parameter := range method.Parameters {
		if expressionHasFreshEmptyMutableCollection(parameter.Default) {
			return true
		}
	}
	return statementsHaveFreshEmptyMutableCollection(method.Body)
}

func expressionHasFreshEmptyMutableCollection(expression ast.Expression) bool {
	if expression == nil {
		return false
	}
	switch node := expression.(type) {
	case *ast.IfStatement:
		if expressionHasFreshEmptyMutableCollection(node.Condition) ||
			statementsHaveFreshEmptyMutableCollection(node.Then) ||
			statementsHaveFreshEmptyMutableCollection(node.Else) {
			return true
		}
		for _, branch := range node.ElseIf {
			if expressionHasFreshEmptyMutableCollection(branch.Condition) || statementsHaveFreshEmptyMutableCollection(branch.Body) {
				return true
			}
		}
	case *ast.CaseStatement:
		if expressionHasFreshEmptyMutableCollection(node.Value) ||
			statementsHaveFreshEmptyMutableCollection(node.Leading) ||
			statementsHaveFreshEmptyMutableCollection(node.Else) {
			return true
		}
		for _, branch := range node.Branches {
			if expressionHasFreshEmptyMutableCollection(branch.Value) || statementsHaveFreshEmptyMutableCollection(branch.Body) {
				return true
			}
			for _, alternative := range branch.Alternatives {
				if expressionHasFreshEmptyMutableCollection(alternative) {
					return true
				}
			}
		}
	case *ast.IterationExpression:
		return expressionHasFreshEmptyMutableCollection(node.Source) ||
			expressionHasFreshEmptyMutableCollection(node.SliceSize) ||
			expressionHasFreshEmptyMutableCollection(node.Initial) ||
			expressionHasFreshEmptyMutableCollection(node.Limit) ||
			node.Block != nil && statementsHaveFreshEmptyMutableCollection(node.Block.Body)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			if expressionHasFreshEmptyMutableCollection(part.Expression) {
				return true
			}
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			if expressionHasFreshEmptyMutableCollection(element) {
				return true
			}
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			if expressionHasFreshEmptyMutableCollection(entry.Key) || expressionHasFreshEmptyMutableCollection(entry.Value) {
				return true
			}
		}
	case *ast.JSXElement:
		if expressionHasFreshEmptyMutableCollection(node.Component) {
			return true
		}
		for _, attribute := range node.Attributes {
			if expressionHasFreshEmptyMutableCollection(attribute.Value) {
				return true
			}
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ast.JSXElement:
				if expressionHasFreshEmptyMutableCollection(item) {
					return true
				}
			case *ast.JSXExpression:
				if expressionHasFreshEmptyMutableCollection(item.Value) {
					return true
				}
			}
		}
	case *ast.UnaryExpression:
		return expressionHasFreshEmptyMutableCollection(node.Operand)
	case *ast.BinaryExpression:
		return expressionHasFreshEmptyMutableCollection(node.Left) || expressionHasFreshEmptyMutableCollection(node.Right)
	case *ast.RangeExpression:
		return expressionHasFreshEmptyMutableCollection(node.Start) || expressionHasFreshEmptyMutableCollection(node.End)
	case *ast.AttemptExpression:
		return expressionHasFreshEmptyMutableCollection(node.Value) || statementsHaveFreshEmptyMutableCollection(node.Body)
	case *ast.TryExpression:
		return expressionHasFreshEmptyMutableCollection(node.Value)
	case *ast.CatchExpression:
		return expressionHasFreshEmptyMutableCollection(node.Value) || statementsHaveFreshEmptyMutableCollection(node.Body)
	case *ast.LambdaExpression:
		for _, parameter := range node.Parameters {
			if expressionHasFreshEmptyMutableCollection(parameter.Default) {
				return true
			}
		}
		return statementsHaveFreshEmptyMutableCollection(node.Body)
	case *ast.CallExpression:
		if expressionHasFreshEmptyMutableCollection(node.Callee) {
			return true
		}
		for _, argument := range node.Arguments {
			if expressionHasFreshEmptyMutableCollection(argument.Value) {
				return true
			}
		}
		return node.Block != nil && statementsHaveFreshEmptyMutableCollection(node.Block.Body)
	case *ast.GenericExpression:
		return expressionHasFreshEmptyMutableCollection(node.Receiver)
	case *ast.MemberExpression:
		return expressionHasFreshEmptyMutableCollection(node.Receiver)
	case *ast.IndexExpression:
		return expressionHasFreshEmptyMutableCollection(node.Receiver) || expressionHasFreshEmptyMutableCollection(node.Index)
	case *ast.BlockExpression:
		return statementsHaveFreshEmptyMutableCollection(node.Body)
	}
	return false
}

func newChecker(program *ast.Program, resolution resolver.Result, options Options) Checker {
	importUses := map[*ast.ImportStatement]map[string]bool{}
	return Checker{
		mode: program.Mode,
		result: Result{
			Program:                    program,
			Expressions:                map[ast.Expression]types.Type{},
			Conversions:                map[ast.Expression]types.Type{},
			NullableUnwraps:            map[ast.Expression]types.Type{},
			NativeResultBridges:        map[ast.Expression]NativeResultBridge{},
			NativeCallResultBridges:    map[*ast.CallExpression]NativeCallResultBridge{},
			Variables:                  map[*ast.VariableStatement]types.Type{},
			Iterations:                 map[*ast.IterationExpression]types.Type{},
			IterationBindings:          map[*ast.IterationExpression][]types.Type{},
			LexicalBindings:            map[*ast.Identifier]bool{},
			Constants:                  map[ast.Expression]string{},
			ConstantOwners:             map[*ast.VariableStatement]string{},
			Resolution:                 resolution,
			References:                 map[ast.Expression]resolver.Binding{},
			ExpressionDeclarations:     map[ast.Expression]identity.Declaration{},
			ExpressionDispatches:       map[ast.Expression]identity.Dispatch{},
			Declarations:               map[ast.Statement]identity.Declaration{},
			MethodDispatches:           map[*ast.MethodStatement]identity.Dispatch{},
			ResolvedTypes:              map[token.Span]types.Type{},
			EnumConstructors:           map[*ast.CallExpression]EnumVariant{},
			EnumArgumentIndexes:        map[*ast.CallExpression][]int{},
			CasePatterns:               map[ast.Expression]CasePattern{},
			CaseNarrowings:             map[*ast.CaseStatement]CaseNarrowing{},
			GenericApplications:        map[*ast.GenericExpression]GenericApplication{},
			CodecApplications:          map[*ast.CallExpression]CodecApplication{},
			RecordConstructions:        map[*ast.CallExpression]RecordConstruction{},
			CallSpecializationRequests: map[*ast.CallExpression]CallSpecializationRequest{},
			CallSpecializations:        map[*ast.CallExpression]CallSpecialization{},
			CallSignatures:             map[*ast.CallExpression][]callsignature.Parameter{},
			DeclarationOnlyCalls:       map[*ast.CallExpression]bool{},
			RawEnums:                   map[*ast.EnumStatement]RawEnum{},
			EnumCalls:                  map[*ast.CallExpression]EnumCall{},
			TypeAliases:                map[*ast.TypeAliasStatement]TypeAlias{},
			InterfaceImplementations:   map[*ast.ClassStatement][]InterfaceImplementation{},
			Newtypes:                   map[*ast.NewtypeStatement]Newtype{},
			NewtypeCalls:               map[*ast.CallExpression]NewtypeCall{},
			ResultTries:                map[*ast.TryExpression]ResultTry{},
			ResultCatches:              map[*ast.CatchExpression]ResultCatch{},
			StructuredBlocks:           map[*ast.CallExpression]StructuredBlock{},
			ExternalMembers:            map[ast.Expression]declaration.Member{},
			ClassFieldAccesses:         map[*ast.MemberExpression]bool{},
			UnionMemberAccesses:        map[*ast.MemberExpression][]UnionMemberAccess{},
			RuntimeDependencies:        map[string]*stdlib.Package{},
			ImportUses:                 importUses,
			CompilerGeneratedStart:     options.CompilerGeneratedStart,
		},
		classes:                    map[string]*classInfo{},
		records:                    map[string]*recordInfo{},
		enums:                      map[string]*enumInfo{},
		aliases:                    map[string]*aliasInfo{},
		newtypes:                   map[string]*newtypeInfo{},
		interfaces:                 map[string]*ast.InterfaceStatement{},
		functions:                  map[string]*ast.MethodStatement{},
		resolution:                 resolution,
		external:                   map[ast.Expression]declaration.Member{},
		declaredTypes:              map[string]typeDeclaration{},
		usedImports:                importUses,
		allowUnusedImports:         options.AllowUnusedImports,
		interactiveTopLevel:        options.InteractiveTopLevel,
		compilerGeneratedStart:     options.CompilerGeneratedStart,
		runnableMain:               options.RunnableMain,
		aliasCycles:                map[string]bool{},
		declarationCalls:           map[*ast.CallExpression]string{},
		borrowedExpressions:        map[ast.Expression]bool{},
		concurrentFunctions:        map[*ast.MethodStatement]bool{},
		concurrentConstructors:     map[*ast.MethodStatement]bool{},
		concurrentClasses:          map[string]bool{},
		authoredMemberMethods:      map[*ast.MemberExpression]*ast.MethodStatement{},
		concurrentInterfaceMembers: map[*ast.MemberExpression]bool{},
		authoredOwnedMethods:       map[string]*ast.MethodStatement{},
		authoredTypes:              map[string]string{},
		authoredEnumOwners:         map[string]string{},
		authoredTypeIdentities:     map[string]identity.Declaration{},
		authoredOwnerIdentities:    map[string]identity.Declaration{},
		activeTypeParameters:       map[string]int{},
		authoredCalls:              map[*ast.MethodStatement]map[*ast.MethodStatement]bool{},
		authoredConstructorCalls:   map[*ast.MethodStatement]map[*ast.MethodStatement]bool{},
		authoredOrdinaryCalls:      map[*ast.MethodStatement]map[*ast.MethodStatement]bool{},
		authoredMethodConstructors: map[*ast.MethodStatement]map[string]bool{},
		authoredClassCalls:         map[string]map[*ast.MethodStatement]bool{},
		authoredClassConstructors:  map[string]map[string]bool{},
		concurrentCallRoots:        map[*ast.MethodStatement]bool{},
		concurrentConstructorRoots: map[*ast.MethodStatement]bool{},
		concurrentOrdinaryRoots:    map[*ast.MethodStatement]bool{},
		concurrentClassRoots:       map[string]bool{},
		concurrentInitTargets:      map[*ast.Identifier]bool{},
	}
}

func (c *Checker) checkProgram(program *ast.Program, checkUnusedImports bool) {
	if c.rubyNativeSyntax() {
		parser.NormalizeRubyNativeParameters(program.Statements)
	}
	if !c.inferenceOnly && c.resolution.Declarations != nil {
		for _, runtimeType := range c.resolution.Declarations.RuntimeTypesByModule[program.ModulePath] {
			c.requireRuntimeType(runtimeType)
		}
	}
	if !c.inferenceOnly {
		c.validateReservedKeywords(program.Statements)
	}
	c.indexAuthoredMethods(program.Statements, "")
	c.collect(program.Statements)
	if !c.inferenceOnly {
		c.validateTypeReferences(program.Statements, nil)
	}
	c.checkStatements(program.Statements, &scope{values: map[string]symbol{}, constantsAllowed: true, enumsAllowed: true})
	if checkUnusedImports && !c.allowUnusedImports {
		c.checkUnusedImports(program.Statements)
	}
}

func (c *Checker) indexAuthoredMethods(statements []ast.Statement, owner string) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.MethodStatement:
			if owner == "" && !node.Class {
				c.functions[node.Name] = node
				c.result.Declarations[node] = identity.Declaration{
					Module: c.result.Program.ModulePath, Name: node.Name, Kind: identity.Function,
				}
			} else if owner != "" {
				dispatch := identity.Dispatch{Owner: c.authoredOwnerIdentities[owner], Name: node.Name, Class: node.Class}
				c.result.MethodDispatches[node] = dispatch
				if node.Class {
					c.authoredOwnedMethods[authoredOwnedMethodKey(owner, node.Name)] = node
				}
			}
		case *ast.ClassStatement:
			qualified := nestedAuthoredOwner(owner, node.Name)
			c.registerAuthoredType(node, node.Name, qualified, identity.Class)
			if !strings.Contains(node.Name, "::") {
				c.authoredTypes[qualified] = node.Name
			}
			c.indexAuthoredMethods(node.Body, qualified)
		case *ast.RecordStatement:
			qualified := nestedAuthoredOwner(owner, node.Name)
			c.registerAuthoredType(node, node.Name, qualified, identity.Record)
			if !strings.Contains(node.Name, "::") {
				c.authoredTypes[qualified] = node.Name
			}
		case *ast.EnumStatement:
			qualified := nestedAuthoredOwner(owner, node.Name)
			c.registerAuthoredType(node, node.Name, qualified, identity.Enum)
			if !strings.Contains(node.Name, "::") {
				c.authoredTypes[qualified] = node.Name
				c.authoredEnumOwners[node.Name] = qualified
			}
			c.indexAuthoredMethods(node.Body, qualified)
		case *ast.InterfaceStatement:
			qualified := nestedAuthoredOwner(owner, node.Name)
			c.registerAuthoredType(node, node.Name, qualified, identity.Interface)
			for _, method := range node.Methods {
				dispatch := identity.Dispatch{Owner: c.authoredOwnerIdentities[qualified], Name: method.Name, Class: method.Class}
				c.result.MethodDispatches[method] = dispatch
			}
		case *ast.TypeAliasStatement:
			c.registerAuthoredType(node, node.Name, nestedAuthoredOwner(owner, node.Name), identity.TypeAlias)
		case *ast.NewtypeStatement:
			c.registerAuthoredType(node, node.Name, nestedAuthoredOwner(owner, node.Name), identity.Newtype)
		case *ast.ModuleStatement:
			qualified := nestedAuthoredOwner(owner, node.Name)
			declaration := identity.Declaration{Module: c.result.Program.ModulePath, Name: qualified, Kind: identity.Module}
			c.result.Declarations[node] = declaration
			c.authoredOwnerIdentities[qualified] = declaration
			c.indexAuthoredMethods(node.Body, qualified)
		}
	}
}

func (c *Checker) registerAuthoredType(statement ast.Statement, leaf, qualified string, kind identity.Kind) {
	declaration := identity.Declaration{Module: c.result.Program.ModulePath, Name: qualified, Kind: kind}
	c.result.Declarations[statement] = declaration
	c.authoredOwnerIdentities[qualified] = declaration
	if previous, exists := c.authoredTypeIdentities[leaf]; !exists || previous.Name == qualified {
		c.authoredTypeIdentities[leaf] = declaration
	}
}

func nestedAuthoredOwner(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + "::" + name
}

func authoredOwnedMethodKey(owner, name string) string {
	return owner + "\x00" + name
}

func (c *Checker) resolvedConcurrentFunctions() (map[*ast.MethodStatement]bool, map[*ast.MethodStatement]bool, map[string]bool) {
	// Constructor reachability stays distinct from ordinary calls so the audit
	// may initialize fresh receiver storage without treating aliased receivers as
	// owned. Class nodes retain field-default effects even without initialize().
	type reach struct {
		method      *ast.MethodStatement
		class       string
		constructor bool
	}
	reached := map[*ast.MethodStatement]bool{}
	reachedClasses := map[string]bool{}
	ordinary := map[*ast.MethodStatement]bool{}
	constructed := map[*ast.MethodStatement]bool{}
	expanded := map[*ast.MethodStatement]bool{}
	queue := make([]reach, 0, len(c.concurrentCallRoots)+len(c.concurrentClassRoots))
	for method := range c.concurrentOrdinaryRoots {
		queue = append(queue, reach{method: method})
	}
	for method := range c.concurrentConstructorRoots {
		queue = append(queue, reach{method: method, constructor: true})
	}
	for class := range c.concurrentClassRoots {
		queue = append(queue, reach{class: class})
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.class != "" {
			if reachedClasses[current.class] {
				continue
			}
			reachedClasses[current.class] = true
			if class := c.classes[current.class]; class != nil {
				if initialize := class.methods["initialize"]; initialize != nil {
					queue = append(queue, reach{method: initialize, constructor: true})
				}
			}
			for callee := range c.authoredClassCalls[current.class] {
				queue = append(queue, reach{method: callee})
			}
			for constructedClass := range c.authoredClassConstructors[current.class] {
				queue = append(queue, reach{class: constructedClass})
			}
			continue
		}
		method := current.method
		if method == nil {
			continue
		}
		reached[method] = true
		if current.constructor {
			constructed[method] = true
		} else {
			ordinary[method] = true
		}
		if expanded[method] {
			continue
		}
		expanded[method] = true
		for callee := range c.authoredCalls[method] {
			if c.authoredOrdinaryCalls[method][callee] {
				queue = append(queue, reach{method: callee})
			}
			if c.authoredConstructorCalls[method][callee] {
				queue = append(queue, reach{method: callee, constructor: true})
			}
		}
		for constructedClass := range c.authoredMethodConstructors[method] {
			queue = append(queue, reach{class: constructedClass})
		}
	}
	constructors := map[*ast.MethodStatement]bool{}
	for method := range constructed {
		if !ordinary[method] {
			constructors[method] = true
		}
	}
	return reached, constructors, reachedClasses
}

func (c *Checker) recordAuthoredCall(call *ast.CallExpression) {
	constructorClass := c.authoredConstructorClass(call.Callee)
	if constructorClass != "" {
		if c.currentMethod != nil {
			classes := c.authoredMethodConstructors[c.currentMethod]
			if classes == nil {
				classes = map[string]bool{}
				c.authoredMethodConstructors[c.currentMethod] = classes
			}
			classes[constructorClass] = true
		}
		if c.currentFieldClass != "" {
			classes := c.authoredClassConstructors[c.currentFieldClass]
			if classes == nil {
				classes = map[string]bool{}
				c.authoredClassConstructors[c.currentFieldClass] = classes
			}
			classes[constructorClass] = true
		}
		if c.concurrentMapDepth > 0 {
			c.concurrentClassRoots[constructorClass] = true
		}
	}
	method := c.authoredCallTarget(call.Callee)
	if method == nil {
		return
	}
	constructorCall := constructorClass != "" && method.Name == "initialize" && !method.Class
	if c.currentFieldClass != "" && !constructorCall {
		calls := c.authoredClassCalls[c.currentFieldClass]
		if calls == nil {
			calls = map[*ast.MethodStatement]bool{}
			c.authoredClassCalls[c.currentFieldClass] = calls
		}
		calls[method] = true
	}
	if c.currentMethod != nil {
		calls := c.authoredCalls[c.currentMethod]
		if calls == nil {
			calls = map[*ast.MethodStatement]bool{}
			c.authoredCalls[c.currentMethod] = calls
		}
		calls[method] = true
		classified := c.authoredOrdinaryCalls
		if constructorCall {
			classified = c.authoredConstructorCalls
		}
		classifiedCalls := classified[c.currentMethod]
		if classifiedCalls == nil {
			classifiedCalls = map[*ast.MethodStatement]bool{}
			classified[c.currentMethod] = classifiedCalls
		}
		classifiedCalls[method] = true
	}
	if c.concurrentMapDepth > 0 {
		c.concurrentCallRoots[method] = true
		if constructorCall {
			c.concurrentConstructorRoots[method] = true
		} else {
			c.concurrentOrdinaryRoots[method] = true
		}
	}
}

func (c *Checker) authoredConstructorClass(expression ast.Expression) string {
	if generic, ok := expression.(*ast.GenericExpression); ok {
		return c.authoredConstructorClass(generic.Receiver)
	}
	member, ok := expression.(*ast.MemberExpression)
	if !ok || member.Name != "new" {
		return ""
	}
	owner := authoredReceiverName(member.Receiver)
	if c.classes[owner] == nil {
		return ""
	}
	return owner
}

func (c *Checker) authoredCallTarget(expression ast.Expression) *ast.MethodStatement {
	switch node := expression.(type) {
	case *ast.Identifier:
		return c.functions[node.Name]
	case *ast.GenericExpression:
		return c.authoredCallTarget(node.Receiver)
	case *ast.MemberExpression:
		if method := c.authoredMemberMethods[node]; method != nil {
			return method
		}
		owner := authoredReceiverName(node.Receiver)
		if node.Name == "new" {
			if class := c.classes[owner]; class != nil {
				return class.methods["initialize"]
			}
		}
		return c.authoredOwnedMethods[authoredOwnedMethodKey(owner, node.Name)]
	default:
		return nil
	}
}

func authoredReceiverName(expression ast.Expression) string {
	if generic, ok := expression.(*ast.GenericExpression); ok {
		return authoredReceiverName(generic.Receiver)
	}
	return expressionTypeName(expression)
}

func (c *Checker) validateTypeReferences(statements []ast.Statement, typeParameters map[string]bool) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			popOwner := c.pushActiveTypeOwner(nestedAuthoredOwner(c.activeTypeOwner, node.Name))
			classTypes := extendTypeParameters(typeParameters, node.TypeParameters)
			for _, implemented := range node.Implements {
				c.validateTypeReferenceInScope(implemented, classTypes)
			}
			c.validateTypeReferences(node.Body, classTypes)
			popOwner()
		case *ast.RecordStatement:
			popOwner := c.pushActiveTypeOwner(nestedAuthoredOwner(c.activeTypeOwner, node.Name))
			c.validateTypeReferences(node.Body, extendTypeParameters(typeParameters, node.TypeParameters))
			popOwner()
		case *ast.EnumStatement:
			popOwner := c.pushActiveTypeOwner(nestedAuthoredOwner(c.activeTypeOwner, node.Name))
			c.validateTypeReferences(node.Body, extendTypeParameters(typeParameters, node.TypeParameters))
			popOwner()
		case *ast.TypeAliasStatement:
			c.validateTypeReferenceInScope(node.Target, extendTypeParameters(typeParameters, node.TypeParameters))
		case *ast.NewtypeStatement:
			c.validateTypeReferenceInScope(node.Target, typeParameters)
		case *ast.EnumMemberStatement:
			for _, parameter := range node.Parameters {
				c.validateTypeReferenceInScope(parameter.Type, typeParameters)
			}
			c.validateExpressionTypeReferences(node.RawValue, typeParameters)
			for _, attribute := range node.Attributes {
				for _, argument := range attribute.Arguments {
					c.validateExpressionTypeReferences(argument.Value, typeParameters)
				}
			}
		case *ast.RecordFieldStatement:
			c.validateTypeReferenceInScope(node.Type, typeParameters)
			c.validateExpressionTypeReferences(node.Default, typeParameters)
			for _, attribute := range node.Attributes {
				for _, argument := range attribute.Arguments {
					c.validateExpressionTypeReferences(argument.Value, typeParameters)
				}
			}
		case *ast.ModuleStatement:
			popOwner := c.pushActiveTypeOwner(nestedAuthoredOwner(c.activeTypeOwner, node.Name))
			c.validateTypeReferences(node.Body, typeParameters)
			popOwner()
		case *ast.InterfaceStatement:
			popOwner := c.pushActiveTypeOwner(nestedAuthoredOwner(c.activeTypeOwner, node.Name))
			interfaceTypes := extendTypeParameters(typeParameters, node.TypeParameters)
			for _, method := range node.Methods {
				c.validateMethodTypes(method, extendTypeParameters(interfaceTypes, method.TypeParameters))
			}
			popOwner()
		case *ast.FieldStatement:
			c.validateTypeReferenceInScope(node.Type, typeParameters)
			c.validateExpressionTypeReferences(node.Value, typeParameters)
		case *ast.MethodStatement:
			methodTypes := extendTypeParameters(typeParameters, node.TypeParameters)
			c.validateMethodTypes(node, methodTypes)
			c.validateTypeReferences(node.Body, methodTypes)
		case *ast.VariableStatement:
			c.validateTypeReferenceInScope(node.Type, typeParameters)
			c.validateExpressionTypeReferences(node.Value, typeParameters)
		case *ast.AssignmentStatement:
			c.validateExpressionTypeReferences(node.Target, typeParameters)
			c.validateExpressionTypeReferences(node.Value, typeParameters)
		case *ast.ReturnStatement:
			c.validateExpressionTypeReferences(node.Value, typeParameters)
		case *ast.IfStatement:
			c.validateExpressionTypeReferences(node.Condition, typeParameters)
			c.validateTypeReferences(node.Then, typeParameters)
			for _, branch := range node.ElseIf {
				c.validateExpressionTypeReferences(branch.Condition, typeParameters)
				c.validateTypeReferences(branch.Body, typeParameters)
			}
			c.validateTypeReferences(node.Else, typeParameters)
		case *ast.CaseStatement:
			c.validateExpressionTypeReferences(node.Value, typeParameters)
			c.validateTypeReferences(node.Leading, typeParameters)
			for _, branch := range node.Branches {
				c.validateExpressionTypeReferences(branch.Value, typeParameters)
				for _, alternative := range branch.Alternatives {
					c.validateExpressionTypeReferences(alternative, typeParameters)
				}
				c.validateTypeReferences(branch.Body, typeParameters)
			}
			c.validateTypeReferences(node.Else, typeParameters)
		case *ast.WhileStatement:
			c.validateExpressionTypeReferences(node.Condition, typeParameters)
			c.validateTypeReferences(node.Body, typeParameters)
		case *ast.ExpressionStatement:
			c.validateExpressionTypeReferences(node.Expression, typeParameters)
		case *ast.NativeBlock:
			c.validateTypeReferences(node.Body, typeParameters)
		}
	}
}

func (c *Checker) validateExpressionTypeReferences(expression ast.Expression, typeParameters map[string]bool) {
	switch node := expression.(type) {
	case nil:
		return
	case *ast.IfStatement:
		c.validateExpressionTypeReferences(node.Condition, typeParameters)
		c.validateTypeReferences(node.Then, typeParameters)
		for _, branch := range node.ElseIf {
			c.validateExpressionTypeReferences(branch.Condition, typeParameters)
			c.validateTypeReferences(branch.Body, typeParameters)
		}
		c.validateTypeReferences(node.Else, typeParameters)
	case *ast.CaseStatement:
		c.validateExpressionTypeReferences(node.Value, typeParameters)
		c.validateTypeReferences(node.Leading, typeParameters)
		for _, branch := range node.Branches {
			c.validateExpressionTypeReferences(branch.Value, typeParameters)
			for _, alternative := range branch.Alternatives {
				c.validateExpressionTypeReferences(alternative, typeParameters)
			}
			c.validateTypeReferences(branch.Body, typeParameters)
		}
		c.validateTypeReferences(node.Else, typeParameters)
	case *ast.IterationExpression:
		c.validateExpressionTypeReferences(node.Source, typeParameters)
		c.validateExpressionTypeReferences(node.SliceSize, typeParameters)
		c.validateExpressionTypeReferences(node.Initial, typeParameters)
		c.validateExpressionTypeReferences(node.Limit, typeParameters)
		if node.Block != nil {
			c.validateTypeReferences(node.Block.Body, typeParameters)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			c.validateExpressionTypeReferences(element, typeParameters)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			c.validateExpressionTypeReferences(entry.Key, typeParameters)
			c.validateExpressionTypeReferences(entry.Value, typeParameters)
		}
	case *ast.JSXElement:
		c.validateExpressionTypeReferences(node.Component, typeParameters)
		for _, attribute := range node.Attributes {
			c.validateExpressionTypeReferences(attribute.Value, typeParameters)
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ast.JSXElement:
				c.validateExpressionTypeReferences(item, typeParameters)
			case *ast.JSXExpression:
				c.validateExpressionTypeReferences(item.Value, typeParameters)
			}
		}
	case *ast.UnaryExpression:
		c.validateExpressionTypeReferences(node.Operand, typeParameters)
	case *ast.BinaryExpression:
		c.validateExpressionTypeReferences(node.Left, typeParameters)
		c.validateExpressionTypeReferences(node.Right, typeParameters)
	case *ast.RangeExpression:
		c.validateExpressionTypeReferences(node.Start, typeParameters)
		c.validateExpressionTypeReferences(node.End, typeParameters)
	case *ast.AttemptExpression:
		c.validateExpressionTypeReferences(node.Value, typeParameters)
		c.validateTypeReferences(node.Body, typeParameters)
	case *ast.TryExpression:
		c.validateExpressionTypeReferences(node.Value, typeParameters)
	case *ast.CatchExpression:
		c.validateExpressionTypeReferences(node.Value, typeParameters)
		c.validateTypeReferences(node.Body, typeParameters)
	case *ast.LambdaExpression:
		for _, parameter := range node.Parameters {
			c.validateTypeReferenceInScope(parameter.Type, typeParameters)
		}
		c.validateTypeReferenceInScope(node.ReturnType, typeParameters)
		c.validateTypeReferences(node.Body, typeParameters)
	case *ast.CallExpression:
		c.validateExpressionTypeReferences(node.Callee, typeParameters)
		for _, argument := range node.Arguments {
			c.validateExpressionTypeReferences(argument.Value, typeParameters)
		}
	case *ast.MemberExpression:
		c.validateExpressionTypeReferences(node.Receiver, typeParameters)
	case *ast.GenericExpression:
		c.validateExpressionTypeReferences(node.Receiver, typeParameters)
		for _, argument := range node.Arguments {
			c.validateTypeReferenceInScope(argument, typeParameters)
		}
	case *ast.IndexExpression:
		c.validateExpressionTypeReferences(node.Receiver, typeParameters)
		c.validateExpressionTypeReferences(node.Index, typeParameters)
	case *ast.BlockExpression:
		c.validateTypeReferences(node.Body, typeParameters)
	}
}

func (c *Checker) validateMethodTypes(method *ast.MethodStatement, typeParameters map[string]bool) {
	c.validateTypeReferenceInScope(method.ReturnType, typeParameters)
	for _, parameter := range method.Parameters {
		c.validateTypeReferenceInScope(parameter.Type, typeParameters)
		c.validateExpressionTypeReferences(parameter.Default, typeParameters)
	}
}

func (c *Checker) validateTypeReferenceInScope(ref ast.TypeRef, typeParameters map[string]bool) {
	if ref.Empty() {
		return
	}
	defer func() {
		c.requireStandardResultRuntimeForSourceType(c.typeFromRefWithParameters(ref, typeParameters))
	}()
	if len(ref.Union) > 0 {
		for _, alternative := range ref.Union {
			c.validateTypeReferenceInScope(alternative, typeParameters)
		}
		return
	}
	if ref.FunctionReturn != nil {
		for _, parameter := range ref.FunctionParameters {
			c.validateTypeReferenceInScope(parameter, typeParameters)
		}
		c.validateTypeReferenceInScope(*ref.FunctionReturn, typeParameters)
		return
	}
	if literal, ok := types.LiteralFromSource(ref.Name); ok {
		if ref.Nullable || ref.Array || len(ref.Arguments) > 0 {
			c.error(ref.Span(), fmt.Sprintf("literal type %s cannot have type arguments, array, or nullable modifiers", literal))
		}
		return
	}
	if types.IsIntegerLiteralSource(ref.Name) {
		c.error(ref.Span(), portableIntegerLiteralRangeMessage)
		return
	}
	if ref.Name == "Never" {
		c.error(ref.Span(), "Never is an internal compiler type and cannot be written in source")
		return
	}
	semantic := types.FromName(ref.Name)
	if semantic.Kind != types.Named && semantic.Name != ref.Name {
		span := ref.Span()
		span.End = span.Start
		span.End.Offset += len(ref.Name)
		span.End.Column += len(ref.Name)
		c.diags = append(c.diags, diagnostic.Diagnostic{
			Code: diagnostic.TypeError, Severity: diagnostic.Error,
			Message: fmt.Sprintf("type name %s is not canonical; use %s", ref.Name, semantic.Name), Span: ref.Span(),
			Fixes: []diagnostic.Fix{{
				Message: fmt.Sprintf("replace %s with %s", ref.Name, semantic.Name),
				Edits:   []diagnostic.TextEdit{{Location: diagnostic.Location{Span: span}, Replacement: semantic.Name}},
			}},
		})
	}
	for _, argument := range ref.Arguments {
		c.validateTypeReferenceInScope(argument, typeParameters)
	}
	_, declared := c.declaredTypes[ref.Name]
	if !declared {
		declaration := c.authoredOwnerIdentities[ref.Name]
		declared = declaration.Kind.IsType()
	}
	binding, imported := c.importedTypeAt(ref.Name, ref.Span())
	_, parameter := typeParameters[ref.Name]
	if types.FromName(ref.Name).Kind == types.Named && !declared && !imported && !parameter {
		c.error(ref.Span(), fmt.Sprintf("type %s is not declared or imported", ref.Name))
	}
	if imported {
		c.markImportUsed(binding)
	}
	if expected, generic := c.genericTypeArityAt(ref.Name, ref.Span()); generic {
		if len(ref.Arguments) != expected {
			c.error(ref.Span(), fmt.Sprintf("%s expects %d type argument(s), got %d", ref.Name, expected, len(ref.Arguments)))
		}
	} else if declaration, declared := c.declaredTypes[ref.Name]; declared && len(ref.Arguments) > 0 {
		c.error(ref.Span(), fmt.Sprintf("%s is not generic", declaration.kind+" "+ref.Name))
	} else if binding, imported := c.importedTypeAt(ref.Name, ref.Span()); imported && len(binding.Export.TypeParameters) == 0 && len(ref.Arguments) > 0 {
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

func extendTypeParameters(parent map[string]bool, parameters []ast.TypeParameter) map[string]bool {
	if len(parameters) == 0 {
		return parent
	}
	result := make(map[string]bool, len(parent)+len(parameters))
	for name := range parent {
		result[name] = true
	}
	for _, parameter := range parameters {
		result[parameter.Name] = true
	}
	return result
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

func (c *Checker) genericTypeArityAt(name string, span token.Span) (int, bool) {
	if declaration, ok := c.declaredTypes[name]; ok && len(declaration.typeParameters) > 0 {
		return len(declaration.typeParameters), true
	}
	if binding, ok := c.importedTypeAt(name, span); ok && len(binding.Export.TypeParameters) > 0 {
		return len(binding.Export.TypeParameters), true
	}
	return 0, false
}

func (c *Checker) importedTypeAt(name string, span token.Span) (resolver.Binding, bool) {
	return c.resolution.ImportedTypeAt(name, c.generatedSpan(span))
}

func (c *Checker) generatedSpan(span token.Span) bool {
	return c.compilerGeneratedStart > 0 && span.Start.Offset >= c.compilerGeneratedStart
}

func (c *Checker) importBindingVisible(binding resolver.Binding, span token.Span) bool {
	return binding.Import == nil || !binding.Import.CompilerGenerated || c.compilerGeneratedStart > 0 && span.Start.Offset >= c.compilerGeneratedStart
}

func (c *Checker) importedValueAt(name string, span token.Span) (resolver.Binding, bool) {
	if c.generatedSpan(span) {
		binding, ok := c.resolution.GeneratedSymbols[name]
		return binding, ok
	}
	binding, ok := c.resolution.Symbols[name]
	return binding, ok
}

func (c *Checker) collect(statements []ast.Statement) {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ast.ClassStatement:
			if !c.declareType(n.Name, "class", n.Span()) {
				continue
			}
			info := &classInfo{name: n.Name, superclass: expressionTypeName(n.Superclass), fields: map[string]*ast.FieldStatement{}, methods: map[string]*ast.MethodStatement{}}
			for _, parameter := range n.TypeParameters {
				info.typeParameters = append(info.typeParameters, parameter.Name)
			}
			for _, implemented := range n.Implements {
				info.interfaces = append(info.interfaces, fromTypeRef(implemented))
			}
			declaration := c.declaredTypes[n.Name]
			declaration.typeParameters = append([]string(nil), info.typeParameters...)
			c.declaredTypes[n.Name] = declaration
			for _, member := range n.Body {
				switch m := member.(type) {
				case *ast.ExpressionStatement:
					if call, ok := m.Expression.(*ast.CallExpression); ok {
						c.declarationCalls[call] = n.Name
					}
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
			for _, parameter := range n.TypeParameters {
				info.typeParameters = append(info.typeParameters, parameter.Name)
			}
			declaration := c.declaredTypes[n.Name]
			declaration.typeParameters = append([]string(nil), info.typeParameters...)
			c.declaredTypes[n.Name] = declaration
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
			info := &enumInfo{name: n.Name, byName: map[string]*ast.EnumMemberStatement{}, methods: map[string]*ast.MethodStatement{}}
			for _, parameter := range n.TypeParameters {
				info.typeParameters = append(info.typeParameters, parameter.Name)
			}
			declaration := c.declaredTypes[n.Name]
			declaration.typeParameters = append([]string(nil), info.typeParameters...)
			c.declaredTypes[n.Name] = declaration
			for _, statement := range n.Body {
				switch member := statement.(type) {
				case *ast.EnumMemberStatement:
					if previous := info.byName[member.Name]; previous != nil {
						c.error(member.Span(), fmt.Sprintf("enum member %s was already declared at %s", member.Name, previous.Span().Start))
						continue
					}
					if previous := info.methods[member.Name]; previous != nil {
						c.error(member.Span(), fmt.Sprintf("enum member %s conflicts with a method declared at %s", member.Name, previous.Span().Start))
						continue
					}
					info.members = append(info.members, member.Name)
					info.byName[member.Name] = member
				case *ast.MethodStatement:
					if member.Name == "raw_value" || member.Name == "from_raw" {
						c.error(member.Span(), fmt.Sprintf("enum method name %s is reserved", member.Name))
						continue
					}
					if previous := info.methods[member.Name]; previous != nil {
						c.error(member.Span(), fmt.Sprintf("enum method %s was already declared at %s", member.Name, previous.Span().Start))
						continue
					}
					if previous := info.byName[member.Name]; previous != nil {
						c.error(member.Span(), fmt.Sprintf("enum method %s conflicts with a member declared at %s", member.Name, previous.Span().Start))
						continue
					}
					info.methods[member.Name] = member
				}
			}
			if raw, ok := rawEnumShape(n); ok {
				info.raw = &raw
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
		case *ast.NewtypeStatement:
			if !c.declareType(n.Name, "newtype", n.Span()) {
				continue
			}
			c.newtypes[n.Name] = &newtypeInfo{statement: n, target: fromTypeRef(n.Target)}
		case *ast.InterfaceStatement:
			if c.declareType(n.Name, "interface", n.Span()) {
				declaration := c.declaredTypes[n.Name]
				for _, parameter := range n.TypeParameters {
					declaration.typeParameters = append(declaration.typeParameters, parameter.Name)
				}
				c.declaredTypes[n.Name] = declaration
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
	c.declaredTypes[name] = typeDeclaration{kind: kind, span: span, identity: c.authoredTypeIdentities[name]}
	return true
}

func (c *Checker) checkStatements(statements []ast.Statement, sc *scope) {
	c.checkStatementSequence(statements, sc)
	if !c.inferenceOnly {
		c.checkUnusedBindings(sc)
	}
}

func (c *Checker) checkStatementSequence(statements []ast.Statement, sc *scope) {
	popTypeOwner := c.pushActiveTypeOwner(scopeConstantOwner(sc))
	defer popTypeOwner()
	for _, statement := range statements {
		var inferenceRegion *emptyCollectionInferenceRegion
		if c.inferenceOnly && c.pendingEmptyCollections > 0 {
			inferenceRegion = &emptyCollectionInferenceRegion{
				scope: sc,
			}
			c.emptyCollectionRegions = append(c.emptyCollectionRegions, inferenceRegion)
		}
		switch n := statement.(type) {
		case *ast.ClassStatement:
			popTypeParameters := c.pushActiveTypeParameters(n.TypeParameters)
			previous := c.current
			c.current = c.classes[n.Name]
			if c.current == nil {
				c.current = &classInfo{name: n.Name, superclass: expressionTypeName(n.Superclass), fields: map[string]*ast.FieldStatement{}, methods: map[string]*ast.MethodStatement{}}
			}
			c.checkTypeParameters(n.TypeParameters)
			c.checkSuperclass(n)
			owner := n.Name
			if sc.constantOwner != "" {
				owner = sc.constantOwner + "::" + n.Name
			}
			popTypeOwner := c.pushActiveTypeOwner(owner)
			selfType := types.FromName(n.Name)
			selfType.Declaration = c.result.Declarations[n]
			for _, parameter := range n.TypeParameters {
				selfType.Args = append(selfType.Args, types.FromName(parameter.Name))
			}
			classScope := &scope{parent: sc, values: map[string]symbol{"self": {typ: selfType}}, constantsAllowed: true, constantOwner: owner}
			for name, field := range c.current.fields {
				classScope.values[name] = symbol{typ: c.typeFromRef(field.Type), mutable: !field.ReadOnly, span: field.Span()}
			}
			c.checkStatements(n.Body, classScope)
			c.checkFieldInitialization(n)
			c.checkInterfaces(n)
			c.checkOverrides(n)
			c.current = previous
			popTypeOwner()
			popTypeParameters()
		case *ast.RecordStatement:
			popTypeParameters := c.pushActiveTypeParameters(n.TypeParameters)
			popTypeOwner := c.pushActiveTypeOwner(c.result.Declarations[n].Name)
			c.checkTypeParameters(n.TypeParameters)
			recordScope := &scope{parent: sc, values: map[string]symbol{}, constantOwner: c.result.Declarations[n].Name}
			unavailable := map[string]bool{}
			for _, member := range n.Body {
				if field, ok := member.(*ast.RecordFieldStatement); ok {
					unavailable[field.Name] = true
				}
			}
			seenDefault := false
			for _, member := range n.Body {
				field, ok := member.(*ast.RecordFieldStatement)
				if !ok {
					continue
				}
				if field.Type.Empty() {
					c.error(field.Span(), fmt.Sprintf("record field %s requires a type", field.Name))
				}
				fieldType := c.typeFromRef(field.Type)
				if field.Default != nil {
					seenDefault = true
					previousUnavailable := c.recordDefaultUnavailable
					c.recordDefaultUnavailable = unavailable
					actual := c.checkExpression(field.Default, recordScope)
					c.recordDefaultUnavailable = previousUnavailable
					actual = c.contextualizeCollectionLiteral(field.Default, fieldType, actual)
					if !c.assignable(field.Default, fieldType, actual) {
						c.error(field.Default.Span(), fmt.Sprintf("record field default has type %s, expected %s", actual, fieldType))
					}
				} else if seenDefault {
					c.error(field.Span(), "required record field cannot follow a default field")
				}
				for _, attribute := range field.Attributes {
					if attribute.Name == "gorm" && c.mode != "go" {
						c.error(attribute.Span(), "@gorm is only available in mode: go")
					}
					for _, argument := range attribute.Arguments {
						c.checkExpression(argument.Value, sc)
					}
				}
				recordScope.values[field.Name] = symbol{typ: fieldType, mutable: false, span: field.Span()}
				delete(unavailable, field.Name)
			}
			popTypeOwner()
			popTypeParameters()
		case *ast.EnumStatement:
			popTypeParameters := c.pushActiveTypeParameters(n.TypeParameters)
			popTypeOwner := c.pushActiveTypeOwner(c.result.Declarations[n].Name)
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
			rawCount := 0
			for _, statement := range n.Body {
				if member, ok := statement.(*ast.EnumMemberStatement); ok && member.RawValue != nil {
					rawCount++
				}
			}
			var raw *RawEnum
			if rawCount > 0 {
				raw = &RawEnum{Values: map[string]RawEnumValue{}}
				if len(n.TypeParameters) > 0 {
					c.error(n.Span(), "raw-value enums cannot declare type parameters")
				}
			}
			seenRaw := map[string]string{}
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
				if raw != nil {
					if len(member.Parameters) > 0 {
						c.error(member.Span(), "raw-value enum members cannot declare payload fields")
					}
					if member.RawValue == nil {
						c.error(member.Span(), "every member of a raw-value enum must declare a raw value")
					} else if value, key, ok := c.rawEnumLiteral(member.RawValue, sc); ok {
						if raw.Type.Kind == "" {
							raw.Type = value.Type
						} else if !types.Equivalent(raw.Type, value.Type) {
							c.error(member.RawValue.Span(), fmt.Sprintf("raw enum value has type %s, expected %s", value.Type, raw.Type))
						}
						if previous := seenRaw[key]; previous != "" {
							c.error(member.RawValue.Span(), fmt.Sprintf("raw enum value duplicates %s", previous))
						} else {
							seenRaw[key] = member.Name
						}
						raw.Values[member.Name] = value
					}
				} else if member.RawValue != nil {
					c.error(member.RawValue.Span(), "raw values cannot be mixed with ordinary enum members")
				}
				seenFields := map[string]bool{}
				for _, parameter := range member.Parameters {
					if parameter.Name == "" || parameter.Type.Empty() {
						c.error(parameter.Span(), fmt.Sprintf("enum payload %s requires a name and type", parameter.Name))
					}
					if parameter.Mutable {
						c.error(parameter.Span(), "enum payload fields cannot be declared with mut")
					}
					if parameter.Default != nil || parameter.Keyword || parameter.Rest || parameter.KeywordRest {
						c.error(parameter.Span(), "enum payload fields must be required positional or named-only values")
					}
					if seenFields[parameter.Name] {
						c.error(parameter.Span(), fmt.Sprintf("enum payload field %s is duplicated", parameter.Name))
					}
					seenFields[parameter.Name] = true
				}
				for _, attribute := range member.Attributes {
					for _, argument := range attribute.Arguments {
						c.checkExpression(argument.Value, sc)
					}
				}
			}
			if raw != nil {
				c.result.RawEnums[n] = *raw
				if info != nil {
					info.raw = raw
				}
			}
			previousClass, previousEnum := c.current, c.currentEnum
			if info != nil {
				c.current = &classInfo{name: info.name, methods: info.methods, fields: map[string]*ast.FieldStatement{}}
				c.currentEnum = info
			}
			selfType := types.FromName(n.Name)
			selfType.Declaration = c.result.Declarations[n]
			for _, parameter := range n.TypeParameters {
				selfType.Args = append(selfType.Args, types.FromName(parameter.Name))
			}
			enumScope := &scope{parent: sc, values: map[string]symbol{"self": {typ: selfType}}, constantOwner: c.authoredEnumOwners[n.Name]}
			for _, statement := range n.Body {
				if method, ok := statement.(*ast.MethodStatement); ok {
					c.checkMethod(method, enumScope)
				}
			}
			c.current, c.currentEnum = previousClass, previousEnum
			popTypeOwner()
			popTypeParameters()
		case *ast.EnumMemberStatement:
			// Checked as part of its enclosing enum.
		case *ast.TypeAliasStatement:
			popTypeParameters := c.pushActiveTypeParameters(n.TypeParameters)
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
			authoredTarget := fromTypeRef(n.Target)
			if binding, ok := c.authoredAliasTargetBinding(authoredTarget); ok {
				alias.AuthoredTargetBinding = &binding
			}
			if c.aliasTargetIsExternal(authoredTarget, map[string]bool{}) {
				if binding, ok := c.externalAliasTargetBinding(target.Name); ok {
					alias.TargetBinding = &binding
				}
			}
			if variants, ok := c.enumVariants(aliasType); ok {
				alias.Variants = variants
			}
			c.result.TypeAliases[n] = alias
			popTypeParameters()
		case *ast.NewtypeStatement:
			if !sc.enumsAllowed {
				c.error(n.Span(), fmt.Sprintf("newtype %s may only be declared at top level or directly inside a module", n.Name))
			}
			if !isConstant(n.Name) {
				c.error(n.Span(), "newtype name must begin with an uppercase letter")
			}
			if n.Target.Empty() {
				c.error(n.Span(), fmt.Sprintf("newtype %s requires a target type", n.Name))
				break
			}
			target := c.expandAlias(c.typeFromRef(n.Target), map[string]bool{})
			if target.Nullable {
				c.error(n.Target.Span(), fmt.Sprintf("newtype %s representation must be non-nullable; make %s nullable at use sites instead", n.Name, n.Name))
			}
			if target.Kind == types.Invalid || target.Kind == types.Never || target.Kind == types.Nil || target.Kind == types.Any || target.Kind == types.Void {
				c.error(n.Target.Span(), fmt.Sprintf("newtype %s requires a concrete value representation", n.Name))
			}
			if !newtypeRepresentationFullyInstantiated(target) {
				c.error(n.Target.Span(), fmt.Sprintf("newtype %s representation must be fully instantiated", n.Name))
			}
			if c.newtypeRepresentationCycle(n.Name, map[string]bool{}) {
				c.error(n.Target.Span(), "newtype representation cycle involving "+n.Name)
			}
			c.result.Newtypes[n] = Newtype{Target: target}
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
			popTypeParameters := c.pushActiveTypeParameters(n.TypeParameters)
			popTypeOwner := c.pushActiveTypeOwner(c.result.Declarations[n].Name)
			c.checkTypeParameters(n.TypeParameters)
			c.interfaceDepth++
			interfaceScope := &scope{parent: sc, values: map[string]symbol{}, constantOwner: c.result.Declarations[n].Name}
			for _, method := range n.Methods {
				c.checkMethod(method, interfaceScope)
			}
			c.interfaceDepth--
			popTypeOwner()
			popTypeParameters()
		case *ast.FieldStatement:
			if n.Value != nil {
				previousFieldClass := c.currentFieldClass
				if c.current != nil {
					c.currentFieldClass = c.current.name
				}
				valueScope := sc
				concurrentField := c.concurrentClasses[c.currentFieldClass]
				if concurrentField {
					// A field default is a synthetic constructor prologue. Keep class
					// fields outside its root so aliased reference fields are captures.
					valueScope = &scope{parent: sc, values: map[string]symbol{}}
					c.concurrentBlockScopes = append(c.concurrentBlockScopes, valueScope)
				}
				valueType := c.checkExpression(n.Value, valueScope)
				if concurrentField {
					c.concurrentBlockScopes = c.concurrentBlockScopes[:len(c.concurrentBlockScopes)-1]
				}
				c.currentFieldClass = previousFieldClass
				declared := c.typeFromRef(n.Type)
				valueType = c.contextualizeCollectionLiteral(n.Value, declared, valueType)
				if !n.Type.Empty() && !c.assignable(n.Value, declared, valueType) {
					c.error(n.Value.Span(), fmt.Sprintf("cannot initialize %s with %s", declared, valueType))
				}
			}
		case *ast.MethodStatement:
			c.checkMethod(n, sc)
		case *ast.VariableStatement:
			valueType := c.checkDirectStructuredResultValue(n.Value, sc, "variable declaration")
			c.checkStructuredBlockValue(n.Value)
			variableType := valueType
			emptyKind := freshEmptyCollectionKind(n.Value)
			var pending *pendingEmptyCollection
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
			if n.Mutable && n.Type.Empty() && emptyKind != "" {
				outcome, inferred := c.emptyCollectionOutcomes[n]
				if !c.inferenceOnly && inferred && outcome.typ.Kind != "" {
					variableType = outcome.typ
					valueType = c.contextualizeCollectionLiteral(n.Value, variableType, valueType)
					if variableType.Kind == types.Hash && len(variableType.Args) == 2 && !portableHashKey(variableType.Args[0]) && !c.rubyNativeSyntax() {
						span := outcome.keySpan
						if span == (token.Span{}) {
							span = n.Value.Span()
						}
						c.error(span, fmt.Sprintf("Hash key must be String or Integer, got %s", variableType.Args[0]))
					}
				} else {
					if c.inferenceOnly {
						variableType = provisionalEmptyCollectionType(emptyKind)
						valueType = c.contextualizeCollectionLiteral(n.Value, variableType, valueType)
					}
					pending = &pendingEmptyCollection{variable: n, owner: sc, kind: emptyKind}
					if c.inferenceOnly {
						c.pendingEmptyCollections++
					}
					if !c.inferenceOnly && inferred && outcome.blocked {
						pending.blocked = true
						c.error(outcome.blockedSpan, emptyCollectionInferenceMessage(emptyKind, true))
					}
				}
			}
			if c.inferenceOnly && n.Type.Empty() && emptyKind == "" {
				if escaped := c.pendingEmptyCollection(n.Value); escaped != nil {
					c.markEmptyCollectionEscape(escaped, n.Value.Span())
				}
			}
			if n.Mutable && valueType.Readonly && isReferenceType(variableType) {
				c.error(n.Value.Span(), fmt.Sprintf("cannot initialize mutable %s from an immutable value", n.Name))
			}
			if !n.Mutable && isReferenceType(variableType) {
				variableType.Readonly = true
			}
			if previous, exists := sc.values[n.Name]; exists {
				c.errorRelated(diagnostic.DuplicateBinding, n.Span(), fmt.Sprintf("%s was already declared; use = to reassign", n.Name), "first declaration", previous.span)
			} else {
				if n.Constant {
					variableType.Readonly = true
				}
				declared := symbol{typ: variableType, mutable: n.Mutable && !n.Constant, constant: n.Constant, owner: sc.constantOwner, span: n.Span(), variable: n, pending: pending}
				if c.concurrentBorrowedType(variableType) {
					declared.concurrentBorrowed = c.concurrentBorrowedExpression(n.Value, sc)
				}
				mustUseResult := len(c.returns) > 0 && !n.Constant && n.Name != "_" && c.isStandardResult(variableType)
				if len(c.returns) > 0 && !n.Constant && (tracksUnusedBinding(n.Name) || mustUseResult) {
					used := false
					declared.used = &used
					declared.useKind = "local variable"
					declared.mustUseResult = mustUseResult
				}
				sc.values[n.Name] = declared
			}
			if n.Constant {
				c.result.ConstantOwners[n] = sc.constantOwner
			}
			c.result.Variables[n] = variableType
		case *ast.AssignmentStatement:
			var initializationTarget *ast.Identifier
			if root := c.currentConcurrentBlockScope(); root != nil {
				if name, outer := concurrentAssignmentRoot(n.Target, sc, root); outer {
					if c.concurrentConstructorInitializesField(root, n.Target, name) {
						initializationTarget, _ = n.Target.(*ast.Identifier)
						c.concurrentInitTargets[initializationTarget] = true
					} else {
						c.error(n.Target.Span(), fmt.Sprintf("concurrent_map cannot assign to outer binding %s", name))
					}
				}
			}
			leftType := c.checkExpression(n.Target, sc)
			delete(c.concurrentInitTargets, initializationTarget)
			if identifier, ok := n.Target.(*ast.Identifier); ok {
				if value, exists := sc.lookup(identifier.Name); exists && value.declared.Kind != "" {
					leftType = value.declared
					c.result.Expressions[identifier] = leftType
					delete(c.result.NullableUnwraps, identifier)
				}
			}
			rightType := c.checkDirectStructuredResultValue(n.Value, sc, "assignment")
			c.checkStructuredBlockValue(n.Value)
			if c.inferenceOnly && n.Operator == "=" {
				if pending := c.pendingEmptyCollection(n.Target); pending != nil && freshEmptyCollectionKind(n.Value) == "" {
					if source := c.pendingEmptyCollection(n.Value); source != nil {
						c.markEmptyCollectionEscape(pending, n.Target.Span())
						c.markEmptyCollectionEscape(source, n.Value.Span())
					} else {
						c.constrainEmptyCollectionExactly(pending, rightType)
					}
				}
				if index, ok := n.Target.(*ast.IndexExpression); ok {
					if pending := c.pendingEmptyCollection(index.Receiver); pending != nil {
						keyType := c.result.Expressions[index.Index]
						c.constrainEmptyHash(pending, keyType, rightType, index.Index.Span(), n.Value.Span())
					}
				}
			}
			if _, member, ok := c.structuredBlockCall(n.Value); ok && member.Block != nil && member.Block.Structured {
				if _, identifier := n.Target.(*ast.Identifier); !identifier {
					c.error(n.Target.Span(), "a structured block assignment target must be a variable name")
				}
			}
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
				if _, direct := n.Target.(*ast.Identifier); direct {
					c.requireMutable(n.Target, sc, "assignment")
				} else {
					c.requireUnaliasedMutable(n.Target, sc, "assignment")
				}
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
			if identifier, ok := n.Target.(*ast.Identifier); ok {
				if n.Operator == "=" {
					if binding, _, exists := sc.lookupOwner(identifier.Name); exists &&
						c.concurrentBorrowedType(binding.typ) && c.concurrentBorrowedExpression(n.Value, sc) {
						markConcurrentBorrowed(sc, identifier.Name, binding)
					}
				}
				flowType := leftType
				if n.Operator == "=" {
					flowType = c.assignmentFlowType(leftType, assignedType)
				}
				sc.setAssignmentType(identifier.Name, leftType, flowType)
				if binding, exists := sc.lookup(identifier.Name); exists {
					sc.resetNullableMembers(identifier.Name, binding.span.Start.Offset)
				}
			}
		case *ast.ReturnStatement:
			if boundary := c.currentControlBoundary(); boundary != "" {
				c.error(n.Span(), fmt.Sprintf("return cannot cross the %s() block boundary", boundary))
			}
			if c.resultBoundaryBlockDepth > 0 {
				c.error(n.Span(), "return is not supported inside Result-boundary structured blocks; use try to abort with Err or return after the block")
			}
			actual := types.Type{Kind: types.Void, Name: "Void"}
			if n.Value != nil {
				actual = c.checkDirectStructuredResultValue(n.Value, sc, "return")
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
				if boundary := c.currentControlBoundary(); boundary != "" {
					c.error(n.Span(), fmt.Sprintf("break cannot cross the %s() block boundary", boundary))
				} else {
					c.error(n.Span(), "break is only valid inside while or an iteration block")
				}
			}
		case *ast.NextStatement:
			if c.loopDepth == 0 {
				if boundary := c.currentControlBoundary(); boundary != "" {
					c.error(n.Span(), fmt.Sprintf("next cannot cross the %s() block boundary", boundary))
				} else {
					c.error(n.Span(), "next is only valid inside while or an iteration block")
				}
			}
		case *ast.ExpressionStatement:
			typ := c.checkExpression(n.Expression, sc)
			if c.isStandardResult(typ) && !(c.interactiveTopLevel && len(c.returns) == 0) {
				c.error(n.Expression.Span(), "Result value must be used; handle it with try, catch, or case, or explicitly return, pass, or store it")
			}
			if call, member, ok := c.structuredBlockCall(n.Expression); ok && member.Block.Structured {
				c.error(call.Span(), fmt.Sprintf("result of %s() must be assigned or returned", member.Name))
			}
		case *ast.IfStatement:
			c.checkIf(n, sc, false)
		case *ast.CaseStatement:
			c.checkCase(n, sc, false)
		case *ast.WhileStatement:
			c.checkBooleanCondition(n.Condition, sc, "while")
			bodyScope, _ := c.nullableConditionScopes(n.Condition, sc)
			c.loopDepth++
			c.checkStatements(n.Body, bodyScope)
			c.loopDepth--
		case *ast.NativeStatement:
			if c.mode != "ruby" {
				c.error(n.Span(), "unsupported statement syntax in portable TypeRB")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires activate trb/platform/ruby/native or trb/platform/ruby/rails")
			}
		case *ast.NativeBlock:
			if c.mode != "ruby" {
				c.error(n.Span(), "unsupported block syntax in portable TypeRB")
			} else if !c.resolution.NativeSyntax {
				c.error(n.Span(), "Ruby-native syntax requires activate trb/platform/ruby/native or trb/platform/ruby/rails")
			}
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		}
		if inferenceRegion != nil {
			c.finishEmptyCollectionInferenceRegion(inferenceRegion)
			c.emptyCollectionRegions = c.emptyCollectionRegions[:len(c.emptyCollectionRegions)-1]
		}
	}
}

func (c *Checker) assignmentFlowType(declared, assigned types.Type) types.Type {
	if declared.Kind == types.Any || assigned.Kind == types.Invalid {
		return declared
	}
	if assigned.Kind == types.Nil && declared.Nullable {
		return assigned
	}
	if !declared.Nullable || assigned.Nullable {
		return declared
	}

	base := declared
	base.Nullable = false
	expandedBase := c.expandAlias(base, map[string]bool{})
	expandedAssigned := c.expandAlias(assigned, map[string]bool{})
	if types.Equivalent(expandedBase, expandedAssigned) {
		return assigned
	}
	if expandedBase.Kind == types.Float && expandedAssigned.Kind == types.Int && !expandedAssigned.Nullable {
		return base
	}
	return declared
}

func scopeConstantOwner(sc *scope) string {
	for current := sc; current != nil; current = current.parent {
		if current.constantOwner != "" {
			return current.constantOwner
		}
	}
	return ""
}

func freshEmptyCollectionKind(expression ast.Expression) types.Kind {
	switch literal := expression.(type) {
	case *ast.ArrayLiteral:
		if len(literal.Elements) == 0 {
			return types.Array
		}
	case *ast.HashLiteral:
		if len(literal.Entries) == 0 {
			return types.Hash
		}
	}
	return ""
}

func provisionalEmptyCollectionType(kind types.Kind) types.Type {
	anyType := types.FromName("Any")
	switch kind {
	case types.Array:
		return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{anyType}}
	case types.Hash:
		return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{anyType, anyType}}
	default:
		return invalidType()
	}
}

func (c *Checker) emptyCollectionRegion(pending *pendingEmptyCollection) *emptyCollectionInferenceRegion {
	if pending == nil || pending.resolved.Kind != "" || pending.blocked {
		return nil
	}
	for index := len(c.emptyCollectionRegions) - 1; index >= 0; index-- {
		region := c.emptyCollectionRegions[index]
		if region.scope == pending.owner {
			return region
		}
	}
	return nil
}

func (c *Checker) emptyCollectionConstraints(pending *pendingEmptyCollection) *emptyCollectionRegionConstraints {
	region := c.emptyCollectionRegion(pending)
	if region == nil {
		return nil
	}
	constraints := region.constraints[pending]
	if constraints == nil {
		if region.constraints == nil {
			region.constraints = map[*pendingEmptyCollection]*emptyCollectionRegionConstraints{}
		}
		constraints = &emptyCollectionRegionConstraints{}
		region.constraints[pending] = constraints
	}
	return constraints
}

func (c *Checker) constrainEmptyCollectionExactly(pending *pendingEmptyCollection, typ types.Type) {
	constraints := c.emptyCollectionConstraints(pending)
	if constraints == nil {
		return
	}
	typ = c.expandAlias(typ, map[string]bool{})
	valid := typ.Kind == types.Array && len(typ.Args) == 1 || typ.Kind == types.Hash && len(typ.Args) == 2
	if typ.Kind != pending.kind || !valid {
		return
	}
	typ.Readonly = false
	if constraints.exact == nil {
		constraints.exact = &typ
	}
}

func (c *Checker) constrainEmptyArray(pending *pendingEmptyCollection, typ types.Type, span token.Span) {
	if pending == nil || pending.kind != types.Array || typ.Kind == types.Invalid || typ.Kind == types.Never {
		return
	}
	constraints := c.emptyCollectionConstraints(pending)
	if constraints != nil {
		constraints.elements = append(constraints.elements, emptyCollectionConstraint{typ: typ, span: span})
	}
}

func (c *Checker) constrainEmptyHash(pending *pendingEmptyCollection, key, value types.Type, keySpan, valueSpan token.Span) {
	if pending == nil || pending.kind != types.Hash || key.Kind == types.Invalid || value.Kind == types.Invalid || key.Kind == types.Never || value.Kind == types.Never {
		return
	}
	constraints := c.emptyCollectionConstraints(pending)
	if constraints == nil {
		return
	}
	constraints.keys = append(constraints.keys, emptyCollectionConstraint{typ: key, span: keySpan})
	constraints.values = append(constraints.values, emptyCollectionConstraint{typ: value, span: valueSpan})
}

func (c *Checker) markEmptyCollectionEscape(pending *pendingEmptyCollection, span token.Span) {
	constraints := c.emptyCollectionConstraints(pending)
	if constraints == nil || constraints.escaped {
		return
	}
	constraints.escaped = true
	constraints.escapeSpan = span
}

func (c *Checker) markEmptyCollectionCapture(pending *pendingEmptyCollection, span token.Span) {
	constraints := c.emptyCollectionConstraints(pending)
	if constraints == nil || constraints.captured {
		return
	}
	constraints.captured = true
	constraints.captureSpan = span
}

func commonEmptyCollectionConstraint(constraints []emptyCollectionConstraint) types.Type {
	if len(constraints) == 0 {
		return types.Type{}
	}
	result := constraints[0].typ
	for _, constraint := range constraints[1:] {
		result, _ = types.CommonType(result, constraint.typ)
	}
	return result
}

func (c *Checker) finishEmptyCollectionInferenceRegion(region *emptyCollectionInferenceRegion) {
	// Exact typed contexts take precedence over write candidates. The ordinary
	// checker pass reports any incompatible write within the same statement.
	for pending, constraints := range region.constraints {
		if pending.resolved.Kind != "" || pending.blocked {
			continue
		}
		if constraints.escaped {
			pending.blocked = true
			c.pendingEmptyCollections--
			c.emptyCollectionOutcomes[pending.variable] = emptyCollectionOutcome{
				blocked:     true,
				blockedSpan: constraints.escapeSpan,
			}
			continue
		}
		inferred := types.Type{}
		if constraints.exact != nil {
			inferred = *constraints.exact
		} else {
			switch pending.kind {
			case types.Array:
				if element := commonEmptyCollectionConstraint(constraints.elements); element.Kind != "" {
					inferred = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{element}}
				}
			case types.Hash:
				if len(constraints.keys) > 0 {
					inferred = types.Type{
						Kind: types.Hash,
						Name: "Hash",
						Args: []types.Type{constraints.keys[0].typ, commonEmptyCollectionConstraint(constraints.values)},
					}
				}
			}
		}
		if inferred.Kind != "" {
			pending.resolved = inferred
			c.pendingEmptyCollections--
			outcome := emptyCollectionOutcome{typ: inferred}
			if len(constraints.keys) > 0 {
				outcome.keySpan = constraints.keys[0].span
			}
			c.emptyCollectionOutcomes[pending.variable] = outcome
			continue
		}
		if constraints.captured {
			pending.blocked = true
			c.pendingEmptyCollections--
			c.emptyCollectionOutcomes[pending.variable] = emptyCollectionOutcome{
				blocked:     true,
				blockedSpan: constraints.captureSpan,
			}
		}
	}
}

func (c *Checker) pendingEmptyCollection(expression ast.Expression) *pendingEmptyCollection {
	if !c.inferenceOnly {
		return nil
	}
	return c.pendingExpressions[expression]
}

func (c *Checker) constrainEmptyCollectionCall(call *ast.CallExpression, arguments []types.Type) {
	if !c.inferenceOnly || call == nil {
		return
	}
	binding, referenced := c.result.References[call.Callee]
	if !referenced || binding.Library == nil {
		return
	}
	library := binding.Library
	receiverMethod := library.HasReceiver()
	var receiver ast.Expression
	argumentOffset := 1
	if receiverMethod {
		member, ok := call.Callee.(*ast.MemberExpression)
		if !ok {
			return
		}
		receiver = member.Receiver
		argumentOffset = 0
	} else if len(call.Arguments) > 0 {
		receiver = call.Arguments[0].Value
	}
	pending := c.pendingEmptyCollection(receiver)
	if pending == nil {
		return
	}

	switch library.Intrinsic {
	case "trb.std.arrays.push", "trb.std.arrays.unshift":
		if argumentOffset < len(arguments) && argumentOffset < len(call.Arguments) {
			c.constrainEmptyArray(pending, arguments[argumentOffset], call.Arguments[argumentOffset].Value.Span())
		}
	case "trb.std.hashes.update":
		if argumentOffset >= len(arguments) || argumentOffset >= len(call.Arguments) {
			return
		}
		other := c.expandAlias(arguments[argumentOffset], map[string]bool{})
		if other.Kind == types.Hash && len(other.Args) == 2 {
			span := call.Arguments[argumentOffset].Value.Span()
			c.constrainEmptyHash(pending, other.Args[0], other.Args[1], span, span)
		}
	}
}

func scopeDescendsFrom(candidate, root *scope) bool {
	for current := candidate; current != nil; current = current.parent {
		if current == root {
			return true
		}
	}
	return false
}

func (c *Checker) pendingCollectionIsCaptured(pending *pendingEmptyCollection) bool {
	if pending == nil || len(c.callbackScopes) == 0 {
		return false
	}
	return !scopeDescendsFrom(pending.owner, c.callbackScopes[len(c.callbackScopes)-1])
}

func emptyCollectionInferenceMessage(kind types.Kind, escaped bool) string {
	suffix := "; add an explicit collection type annotation"
	if escaped {
		suffix = " before the value escapes" + suffix
	}
	if kind == types.Hash {
		return "cannot infer key and value types of empty Hash" + suffix
	}
	return "cannot infer element type of empty Array" + suffix
}

func (c *Checker) checkUnusedBindings(sc *scope) {
	if c.inferenceOnly {
		return
	}
	type trackedBinding struct {
		name  string
		value symbol
	}
	if !(c.interactiveTopLevel && sc.parent == nil) {
		unresolved := make([]trackedBinding, 0)
		for name, value := range sc.values {
			if value.pending != nil && !value.pending.blocked && value.pending.resolved.Kind == "" {
				unresolved = append(unresolved, trackedBinding{name: name, value: value})
			}
		}
		sort.Slice(unresolved, func(i, j int) bool {
			return unresolved[i].value.span.Start.Offset < unresolved[j].value.span.Start.Offset
		})
		for _, binding := range unresolved {
			c.error(binding.value.pending.variable.Value.Span(), emptyCollectionInferenceMessage(binding.value.pending.kind, false))
		}
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
		if binding.value.mustUseResult {
			c.errorCode(diagnostic.UnusedBinding, binding.value.span, fmt.Sprintf("Result binding %s must be used; handle it with try, catch, or case, or explicitly return, pass, or store it", binding.name))
			continue
		}
		c.errorCode(diagnostic.UnusedBinding, binding.value.span, fmt.Sprintf("%s %s is not used", binding.value.useKind, binding.name))
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
	if declaration := binding.DeclarationIdentity(); !declaration.Empty() {
		c.result.ExpressionDeclarations[expression] = declaration
	}
	if dispatch := binding.DispatchIdentity(); !dispatch.Empty() {
		c.result.ExpressionDispatches[expression] = dispatch
	}
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
			c.markImportedSymbolUsed(name, binding.Import != nil && binding.Import.CompilerGenerated)
		}
	}
}

func (c *Checker) markImportedSymbolUsed(name string, generated bool) {
	symbols := c.resolution.Symbols
	if generated {
		symbols = c.resolution.GeneratedSymbols
	}
	if binding, ok := symbols[name]; ok {
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
		if imported == nil || imported.DeclarationProvider || imported.Definition != nil && (imported.Definition.NativeSyntax || imported.Definition.TypeProvider != "") {
			continue
		}
		used := c.usedImports[node]
		if len(node.Symbols) == 0 {
			if !used[""] {
				c.diags = append(c.diags, diagnostic.Diagnostic{
					Code: diagnostic.UnusedBinding, Severity: diagnostic.Error,
					Message: fmt.Sprintf("import %s is not used", node.Path), Span: node.Span(),
					Fixes: []diagnostic.Fix{{Message: "remove unused import", Edits: []diagnostic.TextEdit{{Location: diagnostic.Location{Span: node.Span()}, Replacement: ""}}}},
				})
			}
			continue
		}
		for _, name := range node.Symbols {
			if !used[name] {
				c.errorCode(diagnostic.UnusedBinding, node.Span(), fmt.Sprintf("imported symbol %s is not used", name))
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
	thenScope, remainingScope := c.nullableConditionScopes(node.Condition, sc)
	if result := c.checkControlFlowBranch(node.Then, thenScope, node.Span(), "if", expression); result != nil {
		results = append(results, *result)
	}
	allMatchedBranchesDiverge := !c.statementsFallThrough(node.Then)
	for _, branch := range node.ElseIf {
		c.checkBooleanCondition(branch.Condition, remainingScope, "elsif")
		branchScope, nextScope := c.nullableConditionScopes(branch.Condition, remainingScope)
		if result := c.checkControlFlowBranch(branch.Body, branchScope, node.Span(), "if", expression); result != nil {
			results = append(results, *result)
		}
		allMatchedBranchesDiverge = allMatchedBranchesDiverge && !c.statementsFallThrough(branch.Body)
		remainingScope = nextScope
	}
	if node.HasElse {
		if result := c.checkControlFlowBranch(node.Else, &scope{parent: remainingScope, values: map[string]symbol{}}, node.Span(), "if", expression); result != nil {
			results = append(results, *result)
		}
	} else if expression {
		c.error(node.Span(), "if expression requires an else branch")
	}
	if !node.HasElse && allMatchedBranchesDiverge {
		c.promoteNullableNarrowings(sc, remainingScope)
	}
	if !expression {
		return types.FromName("Void")
	}
	return c.controlFlowResultType("if", node.Span(), results)
}

func (c *Checker) nullableConditionScopes(expression ast.Expression, sc *scope) (*scope, *scope) {
	matched := &scope{parent: sc, values: map[string]symbol{}}
	unmatched := sc
	binary, ok := expression.(*ast.BinaryExpression)
	if !ok || binary.Operator != "==" && binary.Operator != "!=" {
		return matched, unmatched
	}
	valueExpression, nilComparison := nullableComparisonValue(binary.Left, binary.Right)
	if !nilComparison {
		valueExpression, nilComparison = nullableComparisonValue(binary.Right, binary.Left)
	}
	if !nilComparison {
		return matched, unmatched
	}
	if identifier, ok := valueExpression.(*ast.Identifier); ok {
		value, found := sc.lookup(identifier.Name)
		if !found || !value.typ.Nullable {
			return matched, unmatched
		}
		if value.declared.Kind == "" {
			value.declared = value.typ
		}
		nonnull := value.typ
		nonnull.Nullable = false
		value.typ = nonnull
		if binary.Operator == "!=" {
			matched.values[identifier.Name] = value
		} else {
			unmatched = &scope{parent: sc, values: map[string]symbol{identifier.Name: value}}
		}
		return matched, unmatched
	}
	member, ok := valueExpression.(*ast.MemberExpression)
	if !ok {
		return matched, unmatched
	}
	key, source, stable := c.nullableMemberNarrowing(member, sc)
	if !stable || !source.Nullable {
		return matched, unmatched
	}
	if binary.Operator == "!=" {
		matched.setNullableMember(key, source)
	} else {
		unmatched = &scope{parent: sc, values: map[string]symbol{}}
		unmatched.setNullableMember(key, source)
	}
	return matched, unmatched
}

func nullableComparisonValue(value, nilValue ast.Expression) (ast.Expression, bool) {
	literal, ok := nilValue.(*ast.Literal)
	return value, ok && literal.Kind == ast.NilLiteral
}

func (c *Checker) nullableMemberNarrowing(member *ast.MemberExpression, sc *scope) (nullableMemberKey, types.Type, bool) {
	if member.Namespace || member.Safe {
		return nullableMemberKey{}, types.Type{}, false
	}
	receiverType, ok := c.result.Expressions[member.Receiver]
	if !ok || receiverType.Nullable || receiverType.Kind == types.Invalid {
		return nullableMemberKey{}, types.Type{}, false
	}
	fieldType, readonly, _, found := c.dataMember(receiverType, member.Name)
	if !found || !readonly {
		return nullableMemberKey{}, types.Type{}, false
	}
	rootName, rootOffset, path, stable := c.readonlyMemberPath(member.Receiver, sc)
	if !stable {
		return nullableMemberKey{}, types.Type{}, false
	}
	path = append(path, member.Name)
	key := nullableMemberKey{rootName: rootName, rootOffset: rootOffset, member: strings.Join(path, ".")}
	return key, fieldType, true
}

func (c *Checker) readonlyMemberPath(expression ast.Expression, sc *scope) (string, int, []string, bool) {
	switch value := expression.(type) {
	case *ast.Identifier:
		binding, ok := sc.lookup(value.Name)
		if !ok {
			return "", 0, nil, false
		}
		return value.Name, binding.span.Start.Offset, nil, true
	case *ast.MemberExpression:
		if value.Namespace || value.Safe {
			return "", 0, nil, false
		}
		receiverType, ok := c.result.Expressions[value.Receiver]
		if !ok || receiverType.Nullable || receiverType.Kind == types.Invalid {
			return "", 0, nil, false
		}
		if !c.readonlyDataMember(receiverType, value.Name) {
			return "", 0, nil, false
		}
		rootName, rootOffset, path, stable := c.readonlyMemberPath(value.Receiver, sc)
		if !stable {
			return "", 0, nil, false
		}
		return rootName, rootOffset, append(path, value.Name), true
	default:
		return "", 0, nil, false
	}
}

func (c *Checker) readonlyDataMember(receiver types.Type, name string) bool {
	receiver = c.expandAlias(receiver, map[string]bool{})
	if receiver.Kind != types.Union {
		_, readonly, _, found := c.dataMember(receiver, name)
		return found && readonly
	}
	if len(receiver.Args) == 0 {
		return false
	}
	for _, alternative := range receiver.Args {
		_, readonly, _, found := c.dataMember(alternative, name)
		if !found || !readonly {
			return false
		}
	}
	return true
}

func (c *Checker) promoteNullableNarrowings(target, narrowed *scope) {
	for current := narrowed; current != nil && current != target; current = current.parent {
		for name, value := range current.values {
			if value.declared.Nullable && !value.typ.Nullable {
				target.values[name] = value
			}
		}
		for key, fact := range current.nullableMembers {
			if fact.valid && fact.source.Nullable {
				target.setNullableMember(key, fact.source)
			}
		}
	}
}

func (c *Checker) checkCase(node *ast.CaseStatement, sc *scope, expression bool) types.Type {
	selectorType := c.checkExpression(node.Value, sc)
	borrowedSelector := c.concurrentBorrowedExpression(node.Value, sc)
	if literalCaseSelector(selectorType) {
		return c.checkLiteralCase(node, sc, selectorType, expression)
	}
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
		if len(branch.Alternatives) > 0 {
			for _, alternative := range branch.Alternatives {
				c.checkExpression(alternative, sc)
			}
			c.error(branch.Span(), "case alternatives are supported only for Integer or String literals")
		}
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
			patternFields := c.enumPatternFields(branch.Bindings, variant)
			bindings := make([]CaseBinding, 0, len(branch.Bindings))
			for index, binding := range branch.Bindings {
				if index >= len(patternFields) {
					break
				}
				if patternFields[index].Type.Kind == types.Invalid {
					continue
				}
				if _, duplicate := branchScope.values[binding.Name]; duplicate {
					c.error(binding.Span(), fmt.Sprintf("enum pattern binding %s is duplicated", binding.Name))
					continue
				}
				field := patternFields[index]
				declared := symbol{
					typ:                field.Type,
					span:               binding.Span(),
					concurrentBorrowed: borrowedSelector && c.concurrentBorrowedType(field.Type),
				}
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

func literalCaseSelector(selector types.Type) bool {
	if types.IsLiteral(selector) || selector.Kind == types.Int || selector.Kind == types.String {
		return true
	}
	if selector.Kind != types.Union || len(selector.Args) == 0 {
		return false
	}
	for _, alternative := range selector.Args {
		if !types.IsLiteral(alternative) {
			return false
		}
	}
	return true
}

func (c *Checker) checkLiteralCase(node *ast.CaseStatement, sc *scope, selectorType types.Type, expression bool) types.Type {
	for _, statement := range node.Leading {
		if _, comment := statement.(*ast.CommentStatement); !comment {
			c.error(statement.Span(), "case statements must be inside a when or else branch")
		}
	}
	c.checkStatements(node.Leading, &scope{parent: sc, values: map[string]symbol{}})

	exhaustive := selectorType.Kind == types.Union || types.IsLiteral(selectorType)
	wanted := map[string]types.Type{}
	if selectorType.Kind == types.Union {
		for _, alternative := range selectorType.Args {
			wanted[alternative.String()] = alternative
		}
	} else if types.IsLiteral(selectorType) {
		wanted[selectorType.String()] = selectorType
	}
	selectorBase := scalarType(selectorType)

	narrowing, narrows := c.discriminantNarrowing(node.Value, sc)
	if narrows {
		narrowing.Branches = map[ast.Expression]types.Type{}
	}
	seen := map[string]bool{}
	results := []controlFlowBranchResult{}
	for _, branch := range node.Branches {
		values := append([]ast.Expression{branch.Value}, branch.Alternatives...)
		matchedLiterals := []types.Type{}
		for _, value := range values {
			c.checkExpression(value, sc)
			literal, ok := literalExpressionType(value)
			if !ok {
				c.error(value.Span(), "when value must be an explicit Integer or String literal")
				continue
			}
			c.result.Expressions[value] = literal
			matchedLiterals = append(matchedLiterals, literal)
			if exhaustive {
				if _, exists := wanted[literal.String()]; !exists {
					c.error(value.Span(), fmt.Sprintf("when value %s is not an alternative of %s", literal, selectorType))
				}
			} else if !types.Equivalent(scalarType(literal), selectorBase) {
				c.error(value.Span(), fmt.Sprintf("when value has type %s, expected %s", scalarType(literal), selectorBase))
			}
			if seen[literal.String()] {
				c.error(value.Span(), fmt.Sprintf("literal %s is handled more than once", literal))
			} else {
				seen[literal.String()] = true
				delete(wanted, literal.String())
			}
		}

		branchScope := &scope{parent: sc, values: map[string]symbol{}}
		if narrows && len(matchedLiterals) > 0 {
			narrowedTypes := []types.Type{}
			for _, literal := range matchedLiterals {
				if narrowed, found := narrowing.typeForLiteral(literal); found {
					narrowedTypes = append(narrowedTypes, narrowed)
				}
			}
			if len(narrowedTypes) > 0 {
				narrowed := types.UnionOf(narrowedTypes...)
				value, _ := sc.lookup(narrowing.Name)
				value.typ = narrowed
				branchScope.values[narrowing.Name] = value
				narrowing.Branches[branch.Value] = narrowed
			}
		}
		if result := c.checkControlFlowBranch(branch.Body, branchScope, branch.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	}

	elseScope := &scope{parent: sc, values: map[string]symbol{}}
	if narrows && node.HasElse {
		remaining := make([]types.Type, 0, len(narrowing.Alternatives))
		for _, alternative := range narrowing.Alternatives {
			member, _, _, _ := c.dataMember(alternative, narrowing.Member)
			if !seen[member.String()] {
				remaining = append(remaining, alternative)
			}
		}
		if len(remaining) > 0 {
			narrowing.Else = types.UnionOf(remaining...)
			value, _ := sc.lookup(narrowing.Name)
			value.typ = narrowing.Else
			elseScope.values[narrowing.Name] = value
		}
	}
	if node.HasElse {
		if result := c.checkControlFlowBranch(node.Else, elseScope, node.Span(), "case", expression); result != nil {
			results = append(results, *result)
		}
	} else if exhaustive && len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for value := range wanted {
			missing = append(missing, value)
		}
		sort.Strings(missing)
		c.error(node.Span(), fmt.Sprintf("case for %s is not exhaustive; missing %s", selectorType, strings.Join(missing, ", ")))
	}
	if narrows {
		c.result.CaseNarrowings[node] = narrowing.CaseNarrowing
	}
	if !expression {
		return types.FromName("Void")
	}
	return c.controlFlowResultType("case", node.Span(), results)
}

type discriminantNarrowing struct {
	CaseNarrowing
	Member       string
	Alternatives []types.Type
	byLiteral    map[string][]types.Type
}

func (n discriminantNarrowing) typeForLiteral(literal types.Type) (types.Type, bool) {
	alternatives := n.byLiteral[literal.String()]
	if len(alternatives) == 0 {
		return types.Type{}, false
	}
	return types.UnionOf(alternatives...), true
}

func (c *Checker) discriminantNarrowing(expression ast.Expression, sc *scope) (discriminantNarrowing, bool) {
	member, ok := expression.(*ast.MemberExpression)
	if !ok || member.Namespace || member.Safe {
		return discriminantNarrowing{}, false
	}
	receiver, ok := member.Receiver.(*ast.Identifier)
	if !ok {
		return discriminantNarrowing{}, false
	}
	value, ok := sc.lookup(receiver.Name)
	if !ok {
		return discriminantNarrowing{}, false
	}
	receiverType := c.expandAlias(value.typ, map[string]bool{})
	if receiverType.Kind != types.Union {
		return discriminantNarrowing{}, false
	}
	result := discriminantNarrowing{
		CaseNarrowing: CaseNarrowing{Name: receiver.Name},
		Member:        member.Name,
		Alternatives:  append([]types.Type(nil), receiverType.Args...),
		byLiteral:     map[string][]types.Type{},
	}
	for _, alternative := range receiverType.Args {
		memberType, readonly, _, found := c.dataMember(alternative, member.Name)
		if !found || !readonly || !types.IsLiteral(memberType) {
			return discriminantNarrowing{}, false
		}
		result.byLiteral[memberType.String()] = append(result.byLiteral[memberType.String()], alternative)
	}
	return result, true
}

func (c *Checker) checkUnionCase(node *ast.CaseStatement, sc *scope, selectorType types.Type, expression bool) types.Type {
	borrowedSelector := c.concurrentBorrowedExpression(node.Value, sc)
	for _, statement := range node.Leading {
		if _, comment := statement.(*ast.CommentStatement); !comment {
			c.error(statement.Span(), "case statements must be inside a when or else branch")
		}
	}
	c.checkStatements(node.Leading, &scope{parent: sc, values: map[string]symbol{}})

	seen := map[string]bool{}
	results := []controlFlowBranchResult{}
	for _, branch := range node.Branches {
		if len(branch.Alternatives) > 0 {
			for _, alternative := range branch.Alternatives {
				c.checkExpression(alternative, sc)
			}
			c.error(branch.Span(), "case alternatives are supported only for Integer or String literals")
		}
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
			declared := symbol{
				typ:                matchType,
				span:               binding.Span(),
				concurrentBorrowed: borrowedSelector && c.concurrentBorrowedType(matchType),
			}
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
			// The alias is the authored runtime/type identity at this call site.
			// Do not retain the underlying enum's local namespace owner.
			result[index].Owner = ""
			result[index].Declaration = typ.Declaration
			result[index].TypeArguments = append([]types.Type(nil), typ.Args...)
			// An alias owns its generated constructors. Do not retain the
			// underlying enum package reference when the alias is local.
			result[index].Reference = nil
			if reference, exists := c.resolution.TypeMemberIdentity(typ.Declaration, variant.Name); exists {
				result[index].Reference = &reference
			} else if reference, exists := c.resolution.TypeMember(typ.Name, variant.Name); exists {
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
			declaration := typ.Declaration
			if declaration.Empty() {
				declaration = c.authoredTypeIdentities[typ.Name]
			}
			variant := EnumVariant{EnumName: typ.Name, Owner: c.authoredEnumOwners[typ.Name], Declaration: declaration, Name: name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, parameter := range member.Parameters {
				variant.Fields = append(variant.Fields, EnumField{Name: parameter.Name, Type: substituteType(c.typeFromRef(parameter.Type), substitutions), NamedOnly: parameter.NamedOnly})
			}
			variants = append(variants, variant)
		}
		return variants, true
	}
	binding, imported := c.resolution.ImportedTypeIdentity(typ.Declaration)
	if !imported {
		binding, imported = c.resolution.ImportedType(typ.Name)
	}
	if imported && binding.Export.Kind == resolver.EnumExport {
		substitutions := typeSubstitutions(binding.Export.TypeParameters, typ.Args)
		variants := make([]EnumVariant, 0, len(binding.Export.EnumVariants))
		for _, imported := range binding.Export.EnumVariants {
			variant := EnumVariant{EnumName: typ.Name, Declaration: binding.DeclarationIdentity(), Name: imported.Name, TypeArguments: append([]types.Type(nil), typ.Args...)}
			for _, field := range imported.Fields {
				fieldType := substituteType(field.Type, substitutions)
				fieldType = c.canonicalContractType(fieldType, c.activeTypeParameterSet())
				variant.Fields = append(variant.Fields, EnumField{Name: field.Name, Type: fieldType, NamedOnly: field.NamedOnly})
			}
			if reference, exists := c.resolution.TypeMemberIdentity(binding.DeclarationIdentity(), imported.Name); exists {
				variant.Reference = &reference
			}
			variants = append(variants, variant)
		}
		// Catalogs produced before payload metadata still expose member names.
		if len(variants) == 0 {
			for _, name := range binding.Export.EnumMembers {
				variants = append(variants, EnumVariant{EnumName: typ.Name, Declaration: binding.DeclarationIdentity(), Name: name})
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
				variant.Fields = append(variant.Fields, EnumField{Name: field.Name, Type: substituteType(field.Type, substitutions), NamedOnly: field.NamedOnly})
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

func enumVariantCallSignature(variant EnumVariant) ([]callsignature.Parameter, []string) {
	parameters := make([]callsignature.Parameter, len(variant.Fields))
	names := make([]string, len(variant.Fields))
	for index, field := range variant.Fields {
		kind := callsignature.Positional
		label := ""
		if field.NamedOnly {
			kind = callsignature.NamedOnly
			label = field.Name
		}
		parameters[index] = callsignature.Parameter{Kind: kind, Label: label, Type: field.Type, Presence: callsignature.Required}
		names[index] = field.Name
	}
	return parameters, names
}

func (c *Checker) enumPatternFields(bindings []ast.PatternBinding, variant EnumVariant) []EnumField {
	result := make([]EnumField, len(bindings))
	used := make([]bool, len(variant.Fields))
	position := 0
	qualified := variant.EnumName + "::" + variant.Name
	for index, binding := range bindings {
		fieldIndex := -1
		if binding.Label != "" {
			for candidate, field := range variant.Fields {
				if field.NamedOnly && field.Name == binding.Label {
					fieldIndex = candidate
					break
				}
			}
			if fieldIndex < 0 {
				positionalOnly := false
				for _, field := range variant.Fields {
					if !field.NamedOnly && field.Name == binding.Label {
						positionalOnly = true
						break
					}
				}
				if positionalOnly {
					c.error(binding.Span(), fmt.Sprintf("enum payload field %s is positional-only in pattern %s", binding.Label, qualified))
				} else {
					c.error(binding.Span(), fmt.Sprintf("enum pattern %s has no named payload field %s", qualified, binding.Label))
				}
			}
		} else {
			for position < len(variant.Fields) && variant.Fields[position].NamedOnly {
				position++
			}
			if position < len(variant.Fields) {
				fieldIndex = position
				position++
			} else {
				c.error(binding.Span(), fmt.Sprintf("enum pattern %s requires named bindings for its remaining payload fields", qualified))
			}
		}
		if fieldIndex < 0 || fieldIndex >= len(variant.Fields) {
			result[index] = EnumField{Type: invalidType()}
			continue
		}
		if used[fieldIndex] {
			c.error(binding.Span(), fmt.Sprintf("enum pattern %s binds payload field %s more than once", qualified, variant.Fields[fieldIndex].Name))
			result[index] = EnumField{Type: invalidType()}
			continue
		}
		used[fieldIndex] = true
		result[index] = variant.Fields[fieldIndex]
	}
	return result
}

func (c *Checker) checkEnumConstructor(call *ast.CallExpression, variant EnumVariant, arguments []types.Type) []int {
	if len(variant.Fields) == 0 {
		c.error(call.Span(), fmt.Sprintf("enum member %s::%s has no payload and is not callable", variant.EnumName, variant.Name))
		return nil
	}
	parameters, names := enumVariantCallSignature(variant)
	indexes := c.checkCallSignature(call.Span(), variant.EnumName+"::"+variant.Name, parameters, false, call.Arguments, arguments, names, nil)
	c.result.CallSignatures[call] = append([]callsignature.Parameter(nil), parameters...)
	return indexes
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

	if member, ok := node.Receiver.(*ast.MemberExpression); ok && !member.Namespace {
		receiver := c.result.Expressions[member.Receiver]
		classAccess := false
		if binding, found := c.result.References[node.Receiver]; found && binding.Library != nil && binding.Library.HasReceiver() && len(binding.Library.TypeParameters) > 0 {
			application.Kind = "method"
			application.Owner = receiver.Name
			application.OwnerArguments = append([]types.Type(nil), receiver.Args...)
			application.TypeParameters = append([]string(nil), binding.Library.TypeParameters...)
			for _, parameter := range binding.Library.Parameters {
				application.Parameters = append(application.Parameters, declaredCallParameter(parameter.Name, parameter.Type, parameter.Keyword, parameter.Optional))
			}
			application.Variadic = binding.Library.Variadic
			application.ReturnType = binding.Library.Return
			c.canonicalizeContractApplication(&application)
		} else if local, found := c.localMember(receiver.Name, member.Name, classAccess, map[string]bool{}); found && local.method != nil {
			local = c.specializeLocalClassMember(receiver, local)
			if len(local.method.TypeParameters) > 0 {
				application.Kind = "method"
				application.Source = true
				application.Owner = receiver.Name
				application.OwnerArguments = append([]types.Type(nil), receiver.Args...)
				for _, parameter := range local.method.TypeParameters {
					application.TypeParameters = append(application.TypeParameters, parameter.Name)
				}
				application.Parameters = append([]callsignature.Parameter(nil), local.sig.parameters...)
				application.Variadic = local.sig.variadic
				application.ReturnType = local.sig.returnType
			}
		} else if binding, found := c.importedAncestorMember(receiver.Name, member.Name, classAccess, map[string]bool{}); found && binding.Member != nil && len(binding.Member.TypeParameters) > 0 {
			binding = specializeResolvedClassMember(receiver, binding)
			application.Kind = "method"
			application.Source = sourceBinding(binding)
			application.Owner = receiver.Name
			application.OwnerArguments = append([]types.Type(nil), receiver.Args...)
			application.TypeParameters = append([]string(nil), binding.Member.TypeParameters...)
			application.Parameters = append([]callsignature.Parameter(nil), binding.Member.Parameters...)
			application.Variadic = binding.Member.Variadic
			application.ReturnType = binding.Member.Type
			application.CallResultBridge = binding.Member.CallResultBridge
			c.canonicalizeContractApplication(&application)
		} else if binding, found := c.resolution.InferredTypeMember(receiver.Name, member.Name); found && binding.Member != nil && len(binding.Member.TypeParameters) > 0 {
			binding = specializeResolvedClassMember(receiver, binding)
			application.Kind = "method"
			application.Source = sourceBinding(binding)
			application.Owner = receiver.Name
			application.OwnerArguments = append([]types.Type(nil), receiver.Args...)
			application.TypeParameters = append([]string(nil), binding.Member.TypeParameters...)
			application.Parameters = append([]callsignature.Parameter(nil), binding.Member.Parameters...)
			application.Variadic = binding.Member.Variadic
			application.ReturnType = binding.Member.Type
			application.CallResultBridge = binding.Member.CallResultBridge
			c.canonicalizeContractApplication(&application)
		} else if declared, found := c.external[node.Receiver]; found && len(declared.TypeParameters) > 0 {
			declared = c.specializeDeclarationMember(receiver, declared)
			application.Kind = "method"
			application.Specializer = declared.Specializer
			application.Owner = receiver.Name
			application.OwnerArguments = append([]types.Type(nil), receiver.Args...)
			application.TypeParameters = append([]string(nil), declared.TypeParameters...)
			for _, parameter := range declared.Parameters {
				application.Parameters = append(application.Parameters, declaredCallParameter(parameter.Name, parameter.Type, parameter.Keyword, parameter.Optional))
			}
			application.Variadic = declared.Variadic
			application.ReturnType = declared.Return
		}
	}
	if application.Kind != "" {
		// A generic member application has already been resolved from its
		// receiver. Continue below to validate and substitute method arguments.
	} else if info := c.classes[name]; info != nil {
		application.Kind = "class"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if info := c.records[name]; info != nil {
		application.Kind = "record"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if info := c.enums[name]; info != nil {
		application.Kind = "enum"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if info := c.aliases[name]; info != nil {
		application.Kind = "type_alias"
		application.TypeParameters = append([]string(nil), info.typeParameters...)
	} else if method := c.functions[name]; method != nil {
		application.Kind = "function"
		application.Source = true
		for _, parameter := range method.TypeParameters {
			application.TypeParameters = append(application.TypeParameters, parameter.Name)
		}
		signature := c.signatureFromMethod(method)
		application.Parameters = append(application.Parameters, signature.parameters...)
		application.Variadic = signature.variadic
		application.ReturnType = c.methodReturnType(method)
	} else if binding, ok := c.result.References[node.Receiver]; ok {
		if binding.Export != nil {
			application.TypeParameters = append([]string(nil), binding.Export.TypeParameters...)
			switch binding.Export.Kind {
			case resolver.ClassExport:
				application.Kind = "class"
			case resolver.RecordExport:
				application.Kind = "record"
			case resolver.EnumExport:
				application.Kind = "enum"
			case resolver.TypeAliasExport:
				application.Kind = "type_alias"
			case resolver.FunctionExport:
				application.Kind = "function"
				application.Source = sourceBinding(binding)
				application.Parameters = append([]callsignature.Parameter(nil), binding.Export.Parameters...)
				application.ParameterResultBridges = append([]resolver.NativeResultBridge(nil), binding.Export.ParameterResultBridges...)
				application.CallResultBridge = binding.Export.CallResultBridge
				application.Variadic = binding.Export.Variadic
				application.ReturnType = binding.Export.Type
				c.canonicalizeContractApplication(&application)
			}
		} else if binding.Library != nil {
			application.Kind = "function"
			application.TypeParameters = append([]string(nil), binding.Library.TypeParameters...)
			for _, parameter := range binding.Library.Parameters {
				application.Parameters = append(application.Parameters, declaredCallParameter(parameter.Name, parameter.Type, parameter.Keyword, parameter.Optional))
			}
			application.Variadic = binding.Library.Variadic
			application.ReturnType = binding.Library.Return
			c.canonicalizeContractApplication(&application)
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
	if application.Kind == "enum" || application.Kind == "type_alias" || application.Kind == "class" || application.Kind == "record" {
		application.ReturnType = types.Type{Kind: types.Named, Name: name, Args: append([]types.Type(nil), application.TypeArguments...)}
	} else {
		for index := range application.Parameters {
			application.Parameters[index].Type = substituteType(application.Parameters[index].Type, substitutions)
		}
		for index := range application.ParameterResultBridges {
			application.ParameterResultBridges[index] = substituteNativeResultBridge(application.ParameterResultBridges[index], substitutions)
		}
		application.CallResultBridge = substituteNativeCallResultBridge(application.CallResultBridge, substitutions)
		application.ReturnType = substituteType(application.ReturnType, substitutions)
	}
	return application, true
}

func (c *Checker) canonicalizeContractApplication(application *GenericApplication) {
	if application == nil {
		return
	}
	parameters := make(map[string]bool, len(application.TypeParameters))
	for _, parameter := range application.TypeParameters {
		parameters[parameter] = true
	}
	for index := range application.Parameters {
		application.Parameters[index].Type = c.canonicalContractType(application.Parameters[index].Type, parameters)
	}
	application.ReturnType = c.canonicalContractType(application.ReturnType, parameters)
}

func (c *Checker) canonicalContractSignature(parameters []callsignature.Parameter) []callsignature.Parameter {
	result := append([]callsignature.Parameter(nil), parameters...)
	for index := range result {
		result[index].Type = c.canonicalContractType(result[index].Type, c.activeTypeParameterSet())
	}
	return result
}

func declaredCallParameter(name string, typ types.Type, keyword, optional bool) callsignature.Parameter {
	kind := callsignature.Positional
	label := ""
	if keyword {
		kind = callsignature.NamedOnly
		label = name
	}
	presence := callsignature.Required
	if optional {
		presence = callsignature.Omittable
	}
	return callsignature.Parameter{Kind: kind, Label: label, Type: typ, Presence: presence}
}

func sourceBinding(binding resolver.Binding) bool {
	if binding.Export != nil && binding.Export.Source {
		return true
	}
	if binding.Import == nil {
		return false
	}
	return binding.Import.Kind == resolver.ProjectImport || binding.Import.Definition != nil && binding.Import.Definition.Source != ""
}

func substituteNativeResultBridge(bridge resolver.NativeResultBridge, substitutions map[string]types.Type) resolver.NativeResultBridge {
	bridge.Type = substituteType(bridge.Type, substitutions)
	bridge.Error = substituteType(bridge.Error, substitutions)
	return bridge
}

func substituteNativeCallResultBridge(bridge resolver.NativeCallResultBridge, substitutions map[string]types.Type) resolver.NativeCallResultBridge {
	bridge.Error = substituteType(bridge.Error, substitutions)
	return bridge
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
	case "trb.internal.json.decode", "trb.web.request_json", "trb.platform.typescript.browser.response_json":
		operation = "decode"
	case "trb.web.context_params":
		operation = "path_parameters"
	case "trb.web.request_query":
		operation = "query_parameters"
	case "trb.internal.json.encode", "trb.web.json", "trb.platform.typescript.browser.json_body":
		operation = "encode"
	default:
		return
	}
	var schema CodecSchema
	var ok bool
	if operation == "path_parameters" || operation == "query_parameters" {
		schema, ok = c.parameterSchema(call.Span(), typ, operation)
	} else {
		schema, ok = c.codecSchema(call.Span(), typ, map[string]bool{})
	}
	if ok {
		c.result.CodecApplications[call] = CodecApplication{Operation: operation, Schema: schema}
	}
}

func (c *Checker) recordCallSpecialization(call *ast.CallExpression, generic *ast.GenericExpression, application GenericApplication) {
	member, ok := generic.Receiver.(*ast.MemberExpression)
	if !ok || member.Namespace {
		return
	}
	typeArguments := make([]packageextension.Type, len(application.TypeArguments))
	for index, argument := range application.TypeArguments {
		typeArguments[index] = c.callSpecializationType(argument, true)
	}
	id := strconv.Itoa(call.Span().Start.Offset)
	c.result.CallSpecializationRequests[call] = CallSpecializationRequest{
		Request: packageextension.SpecializeCallRequest{
			ProtocolVersion: packageextension.ProtocolVersion,
			Provider:        application.Specializer,
			CallSite: packageextension.CallSite{
				ID:         id,
				ModulePath: c.result.Program.ModulePath,
			},
			TypeArguments: typeArguments,
		},
		Receiver: member.Receiver,
	}
}

func (c *Checker) callSpecializationType(typ types.Type, includeRecord bool) packageextension.Type {
	result := packageextension.Type{
		Kind:     string(typ.Kind),
		Name:     typ.Name,
		Nullable: typ.Nullable,
	}
	for _, argument := range typ.Args {
		result.Arguments = append(result.Arguments, c.callSpecializationType(argument, false))
	}
	expanded := c.expandAlias(typ, map[string]bool{})
	if _, _, newtype := c.newtypeDefinition(expanded.Name); newtype {
		representation := c.callSpecializationType(c.expandRepresentation(expanded, map[string]bool{}), false)
		result.Representation = &representation
	}
	if typ.Kind != types.Named {
		return result
	}
	result.Definition = c.callSpecializationDefinition(typ.Name)
	if !includeRecord {
		return result
	}
	fields, module, reference, ok := c.codecRecord(typ.Name)
	if !ok {
		return result
	}
	result.Definition = callSpecializationDefinition(module, reference)
	result.Record = &packageextension.Record{}
	for _, field := range fields {
		result.Record.Fields = append(result.Record.Fields, packageextension.Field{
			Name: field.Name,
			Type: c.callSpecializationType(field.Type, false),
		})
	}
	return result
}

func (c *Checker) callSpecializationDefinition(name string) *packageextension.Definition {
	if c.records[name] != nil || c.classes[name] != nil || c.enums[name] != nil || c.aliases[name] != nil || c.interfaces[name] != nil {
		return &packageextension.Definition{ModulePath: c.result.Program.ModulePath}
	}
	if binding, ok := c.resolution.ImportedType(name); ok {
		return callSpecializationDefinition(binding.Import.RuntimePath(), &binding)
	}
	if binding, ok := c.resolution.InferredType(name); ok {
		return callSpecializationDefinition(binding.Import.RuntimePath(), &binding)
	}
	return nil
}

func callSpecializationDefinition(module string, reference *resolver.Binding) *packageextension.Definition {
	result := &packageextension.Definition{ModulePath: module}
	if reference != nil && reference.Import != nil {
		result.ImportPath = reference.Import.Path
		if result.ModulePath == "" {
			result.ModulePath = reference.Import.RuntimePath()
		}
	}
	return result
}

func (c *Checker) parameterSchema(span token.Span, typ types.Type, operation string) (CodecSchema, bool) {
	base := c.expandRepresentation(typ, map[string]bool{})
	schema := CodecSchema{Type: base}
	base.Nullable = false
	if base.Kind != types.Named || typ.Nullable {
		c.error(span, fmt.Sprintf("web parameter binding type %s must be a non-nullable record", typ))
		return schema, false
	}
	fields, module, reference, ok := c.codecRecord(base.Name)
	if !ok {
		c.error(span, fmt.Sprintf("web parameter binding type %s must be a non-nullable record", typ))
		return schema, false
	}
	schema.Kind = "record"
	schema.Module = module
	if reference != nil {
		copy := *reference
		schema.Reference = &copy
	}
	for _, field := range fields {
		fieldSchema, fieldOK := c.parameterValueSchema(field.Type, operation == "query_parameters")
		if !fieldOK {
			if field.Type.Kind == types.Array && operation != "query_parameters" {
				c.error(span, fmt.Sprintf("path parameter field %s cannot be an Array", field.Name))
			} else if field.Type.Kind == types.Array {
				c.error(span, fmt.Sprintf("query parameter field %s must use Array<T> with a non-nullable scalar T", field.Name))
			} else {
				c.error(span, fmt.Sprintf("web parameter field %s has unsupported type %s", field.Name, field.Type))
			}
			return schema, false
		}
		// JSON field aliases describe JSON documents, not URL parameters. Keep
		// parameter names equal to the TypeRB field name until a dedicated
		// parameter annotation is introduced.
		schema.Fields = append(schema.Fields, CodecField{Name: field.Name, WireName: field.Name, Schema: &fieldSchema})
	}
	return schema, true
}

func (c *Checker) parameterValueSchema(typ types.Type, allowArray bool) (CodecSchema, bool) {
	expanded := c.expandRepresentation(typ, map[string]bool{})
	schema := CodecSchema{Type: expanded}
	base := expanded
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
		if !allowArray || len(base.Args) != 1 || base.Args[0].Nullable || base.Args[0].Kind == types.Array {
			return schema, false
		}
		element, ok := c.parameterValueSchema(base.Args[0], false)
		if !ok {
			return schema, false
		}
		schema.Kind = "array"
		schema.Element = &element
	case types.Named:
		if kind, module, reference, ok := c.codecTimeScalar(base.Name); ok {
			schema.Kind = kind
			schema.Module = module
			copy := reference
			schema.Reference = &copy
			break
		}
		raw, module, reference, ok := c.codecRawEnum(base.Name)
		if !ok {
			return schema, false
		}
		schema.Kind = "raw_enum"
		schema.Module = module
		schema.RawType = raw.Type
		if reference != nil {
			copy := *reference
			schema.Reference = &copy
		}
		names := make([]string, 0, len(raw.Values))
		for name := range raw.Values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			schema.RawValues = append(schema.RawValues, CodecRawValue{Member: name, Raw: raw.Values[name].Raw})
		}
	default:
		return schema, false
	}
	return schema, true
}

func (c *Checker) codecSchema(span token.Span, typ types.Type, visiting map[string]bool) (CodecSchema, bool) {
	return c.codecSchemaResolved(span, typ, visiting, false)
}

func (c *Checker) codecSchemaResolved(span token.Span, typ types.Type, visiting map[string]bool, catalogContext bool) (CodecSchema, bool) {
	// Codecs use the expanded representation of transparent aliases and nominal
	// newtypes. Generated helpers can then cross package boundaries without
	// exposing a source-only representation wrapper.
	base := c.expandCodecRepresentation(typ, map[string]bool{})
	schema := CodecSchema{Type: base}
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
		element, ok := c.codecSchemaResolved(span, base.Args[0], visiting, catalogContext)
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
		element, ok := c.codecSchemaResolved(span, base.Args[1], visiting, catalogContext)
		if !ok {
			return schema, false
		}
		schema.Kind = "hash"
		schema.Element = &element
	case types.Named:
		if kind, module, reference, ok := c.codecTimeScalar(base.Name); ok {
			schema.Kind = kind
			schema.Module = module
			copy := reference
			schema.Reference = &copy
			break
		}
		if raw, module, reference, ok := c.codecRawEnumResolved(base.Name, catalogContext); ok {
			schema.Kind = "raw_enum"
			schema.Module = module
			schema.RawType = raw.Type
			if reference != nil {
				copy := *reference
				schema.Reference = &copy
			}
			names := make([]string, 0, len(raw.Values))
			for name := range raw.Values {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				schema.RawValues = append(schema.RawValues, CodecRawValue{Member: name, Raw: raw.Values[name].Raw})
			}
			break
		}
		fields, module, reference, ok := c.codecRecordResolved(base.Name, catalogContext)
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
			fieldSchema, fieldOK := c.codecSchemaResolved(span, field.Type, visiting, catalogContext || reference != nil)
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

func (c *Checker) expandCodecRepresentation(typ types.Type, visiting map[string]bool) types.Type {
	expanded := c.expandRepresentation(typ, map[string]bool{})
	if expanded.Kind != types.Named || expanded.Name == "" || visiting[expanded.Name] {
		return expanded
	}
	catalogBinding, exists := c.resolution.CatalogType(expanded.Name)
	if !exists || catalogBinding.Export == nil || catalogBinding.Export.Kind != resolver.TypeAliasExport && catalogBinding.Export.Kind != resolver.NewtypeExport {
		return expanded
	}
	binding, exists := c.resolution.ContractType(expanded.Name)
	if !exists || binding.Export == nil || binding.Export.Kind != resolver.TypeAliasExport && binding.Export.Kind != resolver.NewtypeExport {
		return expanded
	}
	visiting[expanded.Name] = true
	target := binding.Export.AliasTarget
	if binding.Export.Kind == resolver.NewtypeExport {
		target = binding.Export.NewtypeTarget
	}
	target = substituteType(target, typeSubstitutions(binding.Export.TypeParameters, expanded.Args))
	result := c.expandCodecRepresentation(target, visiting)
	delete(visiting, expanded.Name)
	result.Nullable = result.Nullable || expanded.Nullable
	result.Readonly = result.Readonly || expanded.Readonly
	return result
}

func (c *Checker) codecTimeScalar(name string) (string, string, resolver.Binding, bool) {
	kinds := map[string]string{
		"Date":      "time_date",
		"TimeOfDay": "time_of_day",
		"DateTime":  "time_datetime",
		"Instant":   "time_instant",
		"Duration":  "time_duration",
		"TimeZone":  "time_zone",
	}
	kind, supported := kinds[name]
	if !supported {
		return "", "", resolver.Binding{}, false
	}
	binding, imported := c.resolution.ImportedType(name)
	if !imported || binding.Import == nil || binding.Import.Path != "trb/std/time" || binding.Export == nil || binding.Export.Kind != resolver.ClassExport {
		return "", "", resolver.Binding{}, false
	}
	return kind, binding.Import.RuntimePath(), binding, true
}

func (c *Checker) codecRawEnum(name string) (RawEnum, string, *resolver.Binding, bool) {
	return c.codecRawEnumResolved(name, false)
}

func (c *Checker) codecRawEnumResolved(name string, catalogContext bool) (RawEnum, string, *resolver.Binding, bool) {
	if catalogContext {
		if binding, ok := c.resolution.CatalogType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.EnumExport && binding.Export.EnumRawType.Kind != "" {
			copy := binding
			return rawEnumFromExport(binding.Export), binding.Import.RuntimePath(), &copy, true
		}
	}
	if binding, ok := c.resolution.ImportedType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.EnumExport && binding.Export.EnumRawType.Kind != "" {
		copy := binding
		return rawEnumFromExport(binding.Export), binding.Import.RuntimePath(), &copy, true
	}
	if enum := c.enums[name]; enum != nil && enum.raw != nil {
		return *enum.raw, c.result.Program.ModulePath, nil, true
	}
	if binding, ok := c.resolution.InferredType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.EnumExport && binding.Export.EnumRawType.Kind != "" {
		copy := binding
		return rawEnumFromExport(binding.Export), binding.Import.RuntimePath(), &copy, true
	}
	if binding, ok := c.resolution.ContractType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.EnumExport && binding.Export.EnumRawType.Kind != "" {
		copy := binding
		return rawEnumFromExport(binding.Export), binding.Import.RuntimePath(), &copy, true
	}
	return RawEnum{}, "", nil, false
}

func (c *Checker) codecRecord(name string) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	return c.codecRecordResolved(name, false)
}

func (c *Checker) codecRecordResolved(name string, catalogContext bool) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	if catalogContext {
		if binding, ok := c.resolution.CatalogType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
			copy := binding
			return append([]resolver.RecordField(nil), binding.Export.Fields...), binding.Import.RuntimePath(), &copy, true
		}
	}
	if binding, ok := c.resolution.ImportedType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		copy := binding
		return append([]resolver.RecordField(nil), binding.Export.Fields...), binding.Import.RuntimePath(), &copy, true
	}
	if record := c.records[name]; record != nil {
		fields := make([]resolver.RecordField, len(record.fields))
		for index, field := range record.fields {
			fields[index] = resolver.RecordField{Name: field.Name, JSONName: checkerRecordJSONName(field), Type: c.typeFromRef(field.Type)}
		}
		return fields, c.result.Program.ModulePath, nil, true
	}
	if binding, ok := c.resolution.InferredType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		copy := binding
		return append([]resolver.RecordField(nil), binding.Export.Fields...), binding.Import.RuntimePath(), &copy, true
	}
	if binding, ok := c.resolution.ContractType(name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		copy := binding
		return append([]resolver.RecordField(nil), binding.Export.Fields...), binding.Import.RuntimePath(), &copy, true
	}
	return nil, "", nil, false
}

func (c *Checker) jsxComponentProps(element *ast.JSXElement, nodeType types.Type) ([]resolver.RecordField, map[string]string, bool) {
	identifier, identifierComponent := element.Component.(*ast.Identifier)
	if !identifierComponent {
		binding, imported := c.result.References[element.Component]
		if !imported || binding.Member == nil || binding.Member.Kind != resolver.FunctionExport {
			return nil, nil, false
		}
		if !c.typesAssignable(nodeType, binding.Member.Type) {
			c.error(element.Component.Span(), fmt.Sprintf("JSX component %s must return %s", element.Name, nodeType))
			return nil, nil, false
		}
		if len(binding.Member.Parameters) == 0 {
			return nil, binding.Member.UnsupportedFields, true
		}
		if len(binding.Member.Parameters) != 1 {
			c.error(element.Component.Span(), fmt.Sprintf("JSX component %s must accept no parameters or one record parameter", element.Name))
			return nil, nil, false
		}
		fields, _, _, found := c.codecRecord(binding.Member.Parameters[0].Type.Name)
		if !found {
			c.error(element.Component.Span(), fmt.Sprintf("JSX component %s props must be a record", element.Name))
			return nil, nil, false
		}
		return fields, binding.Member.UnsupportedFields, true
	}
	if method := c.functions[identifier.Name]; method != nil {
		if !c.typesAssignable(nodeType, c.methodReturnType(method)) {
			c.error(identifier.Span(), fmt.Sprintf("JSX component %s must return %s", identifier.Name, nodeType))
			return nil, nil, false
		}
		if len(method.Parameters) == 0 {
			return nil, nil, true
		}
		if len(method.Parameters) != 1 {
			c.error(identifier.Span(), fmt.Sprintf("JSX component %s must accept no parameters or one record parameter", identifier.Name))
			return nil, nil, false
		}
		fields, _, _, found := c.codecRecord(c.typeFromRef(method.Parameters[0].Type).Name)
		if !found {
			c.error(method.Parameters[0].Span(), fmt.Sprintf("JSX component %s props must be a record", identifier.Name))
			return nil, nil, false
		}
		return fields, nil, true
	}
	binding, imported := c.result.References[identifier]
	if !imported || binding.Export == nil || binding.Export.Kind != resolver.FunctionExport {
		return nil, nil, false
	}
	if !c.typesAssignable(nodeType, binding.Export.Type) {
		c.error(identifier.Span(), fmt.Sprintf("JSX component %s must return %s", identifier.Name, nodeType))
		return nil, nil, false
	}
	if len(binding.Export.Parameters) == 0 {
		return nil, binding.Export.UnsupportedFields, true
	}
	if len(binding.Export.Parameters) != 1 {
		c.error(identifier.Span(), fmt.Sprintf("JSX component %s must accept no parameters or one record parameter", identifier.Name))
		return nil, nil, false
	}
	fields, _, _, found := c.codecRecord(binding.Export.Parameters[0].Type.Name)
	if !found {
		c.error(identifier.Span(), fmt.Sprintf("JSX component %s props must be a record", identifier.Name))
		return nil, nil, false
	}
	return fields, binding.Export.UnsupportedFields, true
}

func (c *Checker) jsxProvider(span token.Span) *stdlib.JSXProvider {
	var provider *stdlib.JSXProvider
	var selected *resolver.Import
	seen := map[*resolver.Import]bool{}
	for _, imported := range c.resolution.Capabilities {
		if imported == nil || imported.Definition == nil || imported.Definition.JSX == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		if provider != nil {
			paths := []string{selected.Path, imported.Path}
			sort.Strings(paths)
			c.error(span, fmt.Sprintf("JSX providers %s and %s cannot be imported together", paths[0], paths[1]))
			return provider
		}
		provider = imported.Definition.JSX
		selected = imported
	}
	if selected != nil {
		c.markImportNodeUsed(selected, "")
	}
	return provider
}

func (c *Checker) checkJSXProps(element *ast.JSXElement, fields []resolver.RecordField, unsupported map[string]string, attributes map[string]types.Type, nodeType types.Type) {
	declared := map[string]resolver.RecordField{}
	for _, field := range fields {
		declared[field.Name] = field
	}
	for name, actual := range attributes {
		if name == "key" || name == "ref" {
			continue
		}
		field, found := declared[name]
		if !found {
			if issue := unsupported[name]; issue != "" {
				c.error(element.Span(), fmt.Sprintf("JSX prop %s from native component %s cannot be represented safely: %s; use a TypeRB provider for this package", name, element.Name, issue))
				continue
			}
			c.error(element.Span(), fmt.Sprintf("JSX component %s has no prop %s", element.Name, name))
			continue
		}
		if !c.typesAssignable(field.Type, actual) && !c.jsxNodePropAssignable(field.Type, actual, nodeType) {
			c.error(element.Span(), fmt.Sprintf("JSX prop %s expects %s, got %s", name, field.Type, actual))
		}
	}
	for _, field := range fields {
		if field.Name == "children" && len(element.Children) > 0 {
			continue
		}
		if _, provided := attributes[field.Name]; provided || field.Optional || field.Type.Nullable {
			continue
		}
		c.error(element.Span(), fmt.Sprintf("JSX component %s requires prop %s", element.Name, field.Name))
	}
	if len(element.Children) > 0 {
		if _, acceptsChildren := declared["children"]; !acceptsChildren {
			c.error(element.Span(), fmt.Sprintf("JSX component %s does not accept children", element.Name))
		}
	}
}

func (c *Checker) jsxNodePropAssignable(expected, actual, nodeType types.Type) bool {
	expected = c.expandAlias(expected, map[string]bool{})
	expected.Nullable = false
	nodeType.Nullable = false
	if !types.Equivalent(expected, nodeType) {
		return false
	}
	return jsxRenderableType(c.expandAlias(actual, map[string]bool{}), nodeType)
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

func (c *Checker) checkTypedArgumentsWithNativeResultBridges(span token.Span, name string, parameters []types.Type, bridges []resolver.NativeResultBridge, required int, variadic bool, arguments []ast.CallArgument, actual []types.Type) {
	if len(bridges) == 0 {
		c.checkTypedArguments(span, name, parameters, required, variadic, arguments, actual)
		return
	}
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
		if parameterIndex < len(bridges) && bridges[parameterIndex].Kind != "" {
			c.checkNativeResultBridge(arguments[index].Value, expected, actualType, bridges[parameterIndex])
			continue
		}
		if !c.assignable(arguments[index].Value, expected, actualType) {
			c.error(arguments[index].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, name, actualType, expected))
		}
	}
}

func (c *Checker) checkUnaryOperator(span token.Span, operator string, operand types.Type) types.Type {
	operand = scalarType(c.expandAlias(operand, map[string]bool{}))
	if operand.Kind == types.Invalid {
		return invalidType()
	}
	if operand.Kind == types.Never {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	if operand.Kind == types.Any && c.rubyNativeSyntax() {
		if operator == "!" {
			return types.FromName("Boolean")
		}
		return types.FromName("Any")
	}
	switch operator {
	case "!":
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
	left = scalarType(c.expandAlias(left, map[string]bool{}))
	right = scalarType(c.expandAlias(right, map[string]bool{}))
	if left.Kind == types.Invalid || right.Kind == types.Invalid {
		return invalidType()
	}
	if left.Kind == types.Never {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	if right.Kind == types.Never {
		if operator == "&&" || operator == "||" {
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
	case "&&", "||":
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
	return !typ.Nullable && (typ.Kind == types.Int || typ.Kind == types.IntLiteral || typ.Kind == types.Float)
}

func scalarType(typ types.Type) types.Type {
	if base, literal := types.LiteralBase(typ); literal {
		base.Nullable = typ.Nullable
		return base
	}
	if typ.Kind == types.Union && len(typ.Args) > 0 {
		if base, ok := types.LiteralUnionBase(typ); ok {
			return base
		}
	}
	return typ
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
	if c.inferenceOnly {
		if pending := c.pendingEmptyCollection(expression); pending != nil {
			expanded := c.expandAlias(target, map[string]bool{})
			validCollection := expanded.Kind == types.Array && len(expanded.Args) == 1 ||
				expanded.Kind == types.Hash && len(expanded.Args) == 2
			if expanded.Kind == pending.kind && validCollection {
				c.constrainEmptyCollectionExactly(pending, expanded)
				return true
			}
			c.markEmptyCollectionEscape(pending, expression.Span())
		}
	}
	if !c.typesAssignable(target, actual) && !c.literalTargetAcceptsExpression(target, expression) {
		return false
	}
	c.recordAssignableConversion(expression, target, actual)
	return true
}

func (c *Checker) literalTargetAcceptsExpression(target types.Type, expression ast.Expression) bool {
	target = c.expandAlias(target, map[string]bool{})
	actual, ok := literalExpressionType(expression)
	if !ok {
		return false
	}
	if target.Kind == types.Union {
		for _, alternative := range target.Args {
			if types.Equivalent(alternative, actual) {
				return true
			}
		}
		return false
	}
	return types.Equivalent(target, actual)
}

func literalExpressionType(expression ast.Expression) (types.Type, bool) {
	switch value := expression.(type) {
	case *ast.Literal:
		if value.Kind != ast.IntegerLiteral && value.Kind != ast.StringLiteral {
			return types.Type{}, false
		}
		return types.LiteralFromSource(value.Raw)
	case *ast.UnaryExpression:
		literal, ok := value.Operand.(*ast.Literal)
		if value.Operator != "-" || !ok || literal.Kind != ast.IntegerLiteral {
			return types.Type{}, false
		}
		return types.LiteralFromSource("-" + literal.Raw)
	default:
		return types.Type{}, false
	}
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
	return target.Kind == types.Named && actual.Kind == types.Named && c.isInterface(target.Name) && c.classImplements(actual, target, map[string]bool{})
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
	if !ok {
		binding, ok = c.resolution.InferredType(name)
	}
	if !ok {
		binding, ok = c.resolution.ContractType(name)
	}
	return ok && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport
}

func (c *Checker) classImplements(classType, interfaceType types.Type, seen map[string]bool) bool {
	className := classType.Name
	if className == "" || seen[classType.String()] {
		return false
	}
	seen[classType.String()] = true
	if info := c.classes[className]; info != nil {
		substitutions := typeSubstitutions(info.typeParameters, classType.Args)
		for _, implemented := range info.interfaces {
			if c.typesEquivalent(substituteType(implemented, substitutions), interfaceType) {
				return true
			}
		}
		return c.classImplements(types.FromName(info.superclass), interfaceType, seen)
	}
	binding, ok := c.resolution.ImportedType(className)
	if !ok {
		binding, ok = c.resolution.InferredType(className)
	}
	if !ok {
		binding, ok = c.resolution.ContractType(className)
	}
	if !ok || binding.Export == nil || binding.Export.Kind != resolver.ClassExport {
		return false
	}
	substitutions := typeSubstitutions(binding.Export.TypeParameters, classType.Args)
	for _, implemented := range binding.Export.Interfaces {
		if c.typesEquivalent(substituteType(implemented, substitutions), interfaceType) {
			return true
		}
	}
	return c.classImplements(types.FromName(binding.Export.Superclass), interfaceType, seen)
}

func (c *Checker) recordAssignableConversion(expression ast.Expression, target, actual types.Type) {
	if expression != nil && target.Kind == types.Iterable && actual.Kind == types.Range {
		c.result.Conversions[expression] = target
		return
	}
	if expression != nil && target.Nullable && !actual.Nullable && actual.Kind != types.Nil {
		c.result.Conversions[expression] = target
		return
	}
	if target.Kind == types.Union && actual.Kind == types.Union && unionContainsKind(target, types.Float) && unionContainsKind(actual, types.Int) {
		c.result.Conversions[expression] = target
		return
	}
	if scalarType(actual).Kind != types.Int || actual.Nullable || target.Nullable {
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
	left = scalarType(left)
	right = scalarType(right)
	if left.Kind == types.Nil {
		return right.Nullable
	}
	if right.Kind == types.Nil {
		return left.Nullable
	}
	if left.Nullable || right.Nullable {
		return false
	}
	if types.Equivalent(left, right) {
		if _, _, newtype := c.newtypeDefinition(left.Name); newtype {
			representation := c.expandRepresentation(left, map[string]bool{})
			return representation.Kind != types.Invalid && c.portableEqualityOperands(representation, representation)
		}
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
	if c.inferenceOnly {
		// A named body is not part of the caller's inference region. Its declared
		// signature still constrains arguments at each call site.
		previousRegions := c.emptyCollectionRegions
		previousCallbacks := c.callbackScopes
		previousPending := c.pendingEmptyCollections
		c.emptyCollectionRegions = nil
		c.callbackScopes = nil
		c.pendingEmptyCollections = 0
		defer func() {
			c.emptyCollectionRegions = previousRegions
			c.callbackScopes = previousCallbacks
			c.pendingEmptyCollections = previousPending
		}()
	}
	runnableMain := method == c.runnableMain
	if runnableMain && (method.Class || len(method.TypeParameters) > 0 || len(method.Parameters) > 0 || !method.ReturnType.Empty()) {
		c.error(method.Span(), "runnable main must have signature def main()")
	}
	c.checkTypeParameters(method.TypeParameters)
	popTypeParameters := c.pushActiveTypeParameters(method.TypeParameters)
	defer popTypeParameters()
	if len(method.TypeParameters) > 0 {
		switch {
		case runnableMain:
			// The exact runnable entrypoint signature is diagnosed above.
		case c.current != nil && method.Class:
			c.error(method.Span(), "generic class methods are not supported; use a top-level generic function")
		case c.current == nil && (c.currentEnum != nil || c.moduleDepth > 0):
			c.error(method.Span(), "generic methods are supported on classes and as top-level functions")
		}
	}
	if c.current != nil && method.Class && len(c.current.typeParameters) > 0 {
		c.error(method.Span(), "class methods on a generic class cannot use the class type parameters")
	}
	if c.current != nil {
		ownerParameters := map[string]bool{}
		for _, parameter := range c.current.typeParameters {
			ownerParameters[parameter] = true
		}
		for _, parameter := range method.TypeParameters {
			if ownerParameters[parameter.Name] {
				c.error(parameter.Span(), fmt.Sprintf("method type parameter %s duplicates a class type parameter", parameter.Name))
			}
		}
	}
	previousClassMethod := c.classMethod
	c.classMethod = method.Class
	methodScope := &scope{parent: parent, values: map[string]symbol{}}
	previousMethod := c.currentMethod
	c.currentMethod = method
	c.currentMethodScopes = append(c.currentMethodScopes, methodScope)
	defer func() {
		c.currentMethodScopes = c.currentMethodScopes[:len(c.currentMethodScopes)-1]
		c.currentMethod = previousMethod
	}()
	if c.concurrentFunctions[method] {
		// Parameter defaults execute as part of the reached call and therefore
		// share its concurrency boundary with the authored body.
		c.concurrentBlockScopes = append(c.concurrentBlockScopes, methodScope)
	}
	seenPositionalDefault := false
	seenNamedOnly := false
	seenNamedDefault := false
	for _, parameter := range method.Parameters {
		typ := c.typeFromRef(parameter.Type)
		if parameter.Type.Empty() {
			typ = types.Type{Kind: types.Any, Name: "Any"}
		}
		if _, exists := methodScope.values[parameter.Name]; exists {
			c.error(parameter.Span(), fmt.Sprintf("parameter %s is duplicated", parameter.Name))
		}
		if !c.rubyNativeSyntax() {
			if parameter.Rest || parameter.KeywordRest {
				c.error(parameter.Span(), "rest parameters are not supported in portable TypeRB")
			}
			_, literalType := types.LiteralFromSource(parameter.Type.Name)
			if parameter.Keyword || parameter.NativeKeyword && !literalType {
				c.error(parameter.Span(), "Ruby keyword parameter syntax requires an explicit Ruby-native import; use bare * for a named-only TypeRB parameter")
			}
			if parameter.NamedOnly {
				seenNamedOnly = true
				if parameter.Default != nil {
					seenNamedDefault = true
				} else if seenNamedDefault {
					c.error(parameter.Span(), "required named-only parameter cannot follow a default parameter")
				}
			} else {
				if seenNamedOnly {
					c.error(parameter.Span(), "positional parameter cannot follow the named-only parameter separator")
				}
				if parameter.Default != nil {
					seenPositionalDefault = true
				} else if seenPositionalDefault {
					c.error(parameter.Span(), "required positional parameter cannot follow a default parameter")
				}
			}
		}
		if c.interfaceDepth > 0 && parameter.Default != nil {
			c.error(parameter.Span(), "interface parameters cannot have defaults")
		}
		if c.interfaceDepth > 0 && parameter.Mutable {
			c.error(parameter.Span(), "interface parameters cannot be declared with mut")
		}
		if parameter.Default != nil {
			actual := c.checkExpression(parameter.Default, methodScope)
			actual = c.contextualizeCollectionLiteral(parameter.Default, typ, actual)
			if !c.assignable(parameter.Default, typ, actual) {
				c.error(parameter.Default.Span(), fmt.Sprintf("default value has type %s, expected %s", actual, typ))
			}
		}
		bindingType := typ
		if !parameter.Mutable && isReferenceType(bindingType) {
			bindingType.Readonly = true
		}
		methodScope.values[parameter.Name] = symbol{
			typ:                bindingType,
			mutable:            parameter.Mutable,
			span:               parameter.Span(),
			concurrentBorrowed: c.concurrentFunctions[method] && c.concurrentBorrowedType(typ),
		}
	}
	returnType := c.typeFromRef(method.ReturnType)
	if method.ReturnType.Empty() {
		returnType = types.Type{Kind: types.Void, Name: "Void"}
	}
	c.returns = append(c.returns, returnType)
	c.resultBoundaries = append(c.resultBoundaries, c.resultBoundaryFor(returnType))
	previousLoopDepth := c.loopDepth
	c.loopDepth = 0
	if method.Name == "initialize" && c.current != nil {
		c.initializing++
	}
	c.checkStatements(method.Body, methodScope)
	if c.concurrentFunctions[method] {
		c.concurrentBlockScopes = c.concurrentBlockScopes[:len(c.concurrentBlockScopes)-1]
	}
	if c.interfaceDepth == 0 && returnType.Kind != types.Void && c.statementsFallThrough(method.Body) && !c.hasNativeImplicitReturn(method.Body) {
		c.error(method.Span(), fmt.Sprintf("%s() must return %s on every path", method.Name, returnType))
	}
	if method.Name == "initialize" && c.current != nil {
		c.initializing--
	}
	c.loopDepth = previousLoopDepth
	c.returns = c.returns[:len(c.returns)-1]
	c.resultBoundaries = c.resultBoundaries[:len(c.resultBoundaries)-1]
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
	if literalCaseSelector(selectorType) {
		if selectorType.Kind == types.Union {
			for _, alternative := range selectorType.Args {
				wanted[alternative.String()] = true
			}
		} else if types.IsLiteral(selectorType) {
			wanted[selectorType.String()] = true
		} else {
			return false
		}
		for _, branch := range node.Branches {
			for _, value := range append([]ast.Expression{branch.Value}, branch.Alternatives...) {
				if literal, ok := literalExpressionType(value); ok {
					delete(wanted, literal.String())
				}
			}
		}
	} else if selectorType.Kind == types.Union {
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
	if imported, ok := c.importedTypeAt(name, class.Superclass.Span()); ok && imported.Export.Kind == resolver.ClassExport {
		c.markImportUsed(imported)
		return
	}
	if c.declarationTypeVisible(name) {
		return
	}
	if c.mode == "ruby" {
		if c.resolution.NativeSyntax {
			return
		}
		c.error(class.Superclass.Span(), fmt.Sprintf("Ruby superclass %s requires activate trb/platform/ruby/native or trb/platform/ruby/rails", name))
		return
	}
	c.error(class.Superclass.Span(), fmt.Sprintf("superclass %s is not declared or imported", name))
}

type methodSignature struct {
	returnType types.Type
	parameters []callsignature.Parameter
	variadic   bool
}

func (c *Checker) checkInterfaces(class *ast.ClassStatement) {
	for _, reference := range class.Implements {
		interfaceType := c.typeFromRef(reference)
		interfaceName := interfaceType.Name
		if interfaceType.Kind != types.Named || interfaceType.Nullable {
			c.error(reference.Span(), fmt.Sprintf("implemented interface must be a non-nullable named type, got %s", interfaceType))
			continue
		}
		required := map[string]methodSignature{}
		externalTarget := c.aliasTargetIsExternal(fromTypeRef(reference), map[string]bool{})
		implementation := InterfaceImplementation{Type: interfaceType}
		externalBinding := resolver.Binding{}
		externalFound := false
		externalResolved := false
		if externalTarget {
			if binding, ok := c.externalAliasTargetBinding(interfaceName); ok {
				externalFound = true
				if binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport {
					externalBinding = binding
					externalResolved = true
					implementation.TargetBinding = &binding
				}
			}
		}
		c.result.InterfaceImplementations[class] = append(c.result.InterfaceImplementations[class], implementation)
		if externalFound && !externalResolved {
			c.error(class.Span(), fmt.Sprintf("implemented type %s must resolve to an interface", interfaceType))
			continue
		}
		if local := c.interfaces[interfaceName]; local != nil && !externalResolved {
			substitutions := typeSubstitutions(typeParameterNames(local.TypeParameters), interfaceType.Args)
			for _, method := range local.Methods {
				required[method.Name] = substituteMethodSignature(c.signatureFromMethod(method), substitutions)
			}
		} else if imported, ok := c.resolvedInterface(interfaceName); externalResolved || ok {
			if externalResolved {
				imported = externalBinding
			}
			c.markImportUsed(imported)
			substitutions := typeSubstitutions(imported.Export.TypeParameters, interfaceType.Args)
			for name, member := range imported.Export.Members {
				required[name] = substituteMethodSignature(signatureFromResolvedMember(member), substitutions)
			}
		} else {
			c.error(class.Span(), fmt.Sprintf("interface %s is not declared or imported", interfaceName))
			continue
		}
		for name, expected := range required {
			actual, ok := c.classMethodSignature(class.Name, name, map[string]bool{})
			if !ok {
				c.error(class.Span(), fmt.Sprintf("class %s does not implement %s.%s", class.Name, interfaceType, name))
				continue
			}
			if !c.sameSignature(expected, actual) {
				c.error(class.Span(), fmt.Sprintf("method %s.%s does not match interface %s", class.Name, name, interfaceType))
			}
		}
	}
}

func (c *Checker) resolvedInterface(name string) (resolver.Binding, bool) {
	lookups := []func(string) (resolver.Binding, bool){
		c.resolution.ImportedType,
		c.resolution.ContractType,
	}
	for _, lookup := range lookups {
		binding, ok := lookup(name)
		if ok && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport {
			return binding, true
		}
	}
	return resolver.Binding{}, false
}

func (c *Checker) checkOverrides(class *ast.ClassStatement) {
	if class == nil {
		return
	}
	superclass := expressionTypeName(class.Superclass)
	if superclass == "" {
		return
	}
	for _, statement := range class.Body {
		method, ok := statement.(*ast.MethodStatement)
		if !ok || method.Class || method.Name == "initialize" {
			continue
		}
		inherited, exists := c.classMethodSignature(superclass, method.Name, map[string]bool{})
		if !exists {
			continue
		}
		if !c.sameSignature(inherited, c.signatureFromMethod(method)) {
			c.error(method.Span(), fmt.Sprintf("method %s.%s does not match inherited method %s.%s", class.Name, method.Name, superclass, method.Name))
		}
	}
}

func typeParameterNames(parameters []ast.TypeParameter) []string {
	names := make([]string, len(parameters))
	for index, parameter := range parameters {
		names[index] = parameter.Name
	}
	return names
}

func substituteMethodSignature(signature methodSignature, substitutions map[string]types.Type) methodSignature {
	signature.returnType = substituteType(signature.returnType, substitutions)
	signature.parameters = append([]callsignature.Parameter(nil), signature.parameters...)
	for index := range signature.parameters {
		signature.parameters[index].Type = substituteType(signature.parameters[index].Type, substitutions)
	}
	return signature
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
	if member, ok := c.declarationMember(className, memberName, false, map[string]bool{}); ok {
		return signatureFromDeclarationMember(member), true
	}
	return methodSignature{}, false
}

func (c *Checker) signatureFromMethod(method *ast.MethodStatement) methodSignature {
	result := methodSignature{returnType: c.methodReturnType(method)}
	for _, parameter := range method.Parameters {
		kind := callsignature.Positional
		label := ""
		if parameter.NamedOnly || parameter.Keyword {
			kind = callsignature.NamedOnly
			label = parameter.Name
		}
		presence := callsignature.Omittable
		if parameter.Default == nil {
			presence = callsignature.Required
		}
		result.parameters = append(result.parameters, callsignature.Parameter{Kind: kind, Label: label, Type: c.typeFromRef(parameter.Type), Presence: presence})
		result.variadic = result.variadic || parameter.Rest || parameter.KeywordRest
	}
	return result
}

func signatureFromResolvedMember(member resolver.Member) methodSignature {
	return methodSignature{returnType: member.Type, parameters: append([]callsignature.Parameter(nil), member.Parameters...), variadic: member.Variadic}
}

func signatureFromDeclarationMember(member declaration.Member) methodSignature {
	result := methodSignature{returnType: member.Return, variadic: member.Variadic}
	for _, parameter := range member.Parameters {
		result.parameters = append(result.parameters, declaredCallParameter(parameter.Name, parameter.Type, parameter.Keyword, parameter.Optional))
	}
	return result
}

func (c *Checker) sameSignature(left, right methodSignature) bool {
	if left.variadic != right.variadic || len(left.parameters) != len(right.parameters) {
		return false
	}
	left.returnType = c.expandAlias(left.returnType, map[string]bool{})
	right.returnType = c.expandAlias(right.returnType, map[string]bool{})
	if !types.Assignable(left.returnType, right.returnType) || !types.Assignable(right.returnType, left.returnType) {
		return false
	}
	leftPositional := positionalSignatureParameters(left.parameters)
	rightPositional := positionalSignatureParameters(right.parameters)
	if len(leftPositional) != len(rightPositional) {
		return false
	}
	for index := range leftPositional {
		if !c.sameSignatureParameter(leftPositional[index], rightPositional[index]) {
			return false
		}
	}
	leftNamed := namedSignatureParameters(left.parameters)
	rightNamed := namedSignatureParameters(right.parameters)
	if len(leftNamed) != len(rightNamed) {
		return false
	}
	for label, leftParameter := range leftNamed {
		rightParameter, exists := rightNamed[label]
		if !exists || !c.sameSignatureParameter(leftParameter, rightParameter) {
			return false
		}
	}
	return true
}

func positionalSignatureParameters(parameters []callsignature.Parameter) []callsignature.Parameter {
	result := []callsignature.Parameter{}
	for _, parameter := range parameters {
		if parameter.Kind == callsignature.Positional {
			result = append(result, parameter)
		}
	}
	return result
}

func namedSignatureParameters(parameters []callsignature.Parameter) map[string]callsignature.Parameter {
	result := map[string]callsignature.Parameter{}
	for _, parameter := range parameters {
		if parameter.Kind == callsignature.NamedOnly {
			result[parameter.Label] = parameter
		}
	}
	return result
}

func (c *Checker) sameSignatureParameter(left, right callsignature.Parameter) bool {
	if left.Kind != right.Kind || left.Presence != right.Presence || left.Label != right.Label {
		return false
	}
	left.Type = c.expandAlias(left.Type, map[string]bool{})
	right.Type = c.expandAlias(right.Type, map[string]bool{})
	return types.Assignable(left.Type, right.Type) && types.Assignable(right.Type, left.Type)
}

func (c *Checker) localMember(className, memberName string, class bool, seen map[string]bool) (classMember, bool) {
	if className == "" || seen[className] {
		return classMember{}, false
	}
	seen[className] = true
	if info := c.interfaces[className]; info != nil && !class {
		for _, method := range info.Methods {
			if method.Name != memberName {
				continue
			}
			signature := c.signatureFromMethod(method)
			return classMember{typ: signature.returnType, method: method, sig: &signature}, true
		}
	}
	if info := c.enums[className]; info != nil {
		if method := info.methods[memberName]; method != nil && !class {
			signature := c.signatureFromMethod(method)
			return classMember{typ: signature.returnType, method: method, sig: &signature}, true
		}
		if info.raw != nil {
			switch {
			case memberName == "raw_value" && !class:
				signature := methodSignature{returnType: info.raw.Type}
				return classMember{typ: signature.returnType, sig: &signature}, true
			case memberName == "from_raw" && class:
				resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName(className), types.FromName("EnumValueError")}}
				signature := methodSignature{returnType: resultType, parameters: callsignature.FromPositionalTypes([]types.Type{info.raw.Type}, 1)}
				return classMember{typ: resultType, sig: &signature}, true
			}
		}
	}
	if info := c.classes[className]; info != nil {
		if method := info.methods[memberName]; method != nil && method.Class == class {
			signature := c.signatureFromMethod(method)
			return classMember{typ: signature.returnType, method: method, sig: &signature}, true
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

func (c *Checker) specializeLocalClassMember(receiver types.Type, member classMember) classMember {
	parameters := []string{}
	if info := c.classes[receiver.Name]; info != nil {
		parameters = info.typeParameters
	} else if info := c.interfaces[receiver.Name]; info != nil {
		parameters = typeParameterNames(info.TypeParameters)
	}
	if len(parameters) == 0 {
		return member
	}
	substitutions := typeSubstitutions(parameters, receiver.Args)
	if member.sig != nil {
		copy := *member.sig
		copy.returnType = substituteType(copy.returnType, substitutions)
		copy.parameters = append([]callsignature.Parameter(nil), copy.parameters...)
		for index := range copy.parameters {
			copy.parameters[index].Type = substituteType(copy.parameters[index].Type, substitutions)
		}
		member.typ = copy.returnType
		member.sig = &copy
	}
	if member.field != nil {
		member.typ = substituteType(member.typ, substitutions)
	}
	return member
}

// dataMember resolves storage-backed fields without selecting methods. It is
// used for safe common-member access across a union and for readonly
// discriminant analysis.
func (c *Checker) dataMember(receiver types.Type, name string) (types.Type, bool, bool, bool) {
	receiver = c.expandAlias(receiver, map[string]bool{})
	if receiver.Kind != types.Named {
		return types.Type{}, false, false, false
	}
	if record := c.records[receiver.Name]; record != nil {
		if field := record.byName[name]; field != nil {
			typ := substituteType(c.typeFromRef(field.Type), typeSubstitutions(record.typeParameters, receiver.Args))
			return typ, true, false, true
		}
	}
	if binding, imported := c.resolution.ImportedType(receiver.Name); imported && binding.Export != nil {
		if binding.Export.Kind == resolver.RecordExport {
			substitutions := typeSubstitutions(binding.Export.TypeParameters, receiver.Args)
			for _, field := range binding.Export.Fields {
				if field.Name == name {
					return substituteType(field.Type, substitutions), true, false, true
				}
			}
		}
	}
	if binding, inferred := c.resolution.InferredType(receiver.Name); inferred && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		substitutions := typeSubstitutions(binding.Export.TypeParameters, receiver.Args)
		for _, field := range binding.Export.Fields {
			if field.Name == name {
				return substituteType(field.Type, substitutions), true, false, true
			}
		}
	}
	if member, found := c.localMember(receiver.Name, name, false, map[string]bool{}); found && member.field != nil {
		member = c.specializeLocalClassMember(receiver, member)
		return member.typ, member.field.ReadOnly, true, true
	}
	if binding, found := c.importedAncestorMember(receiver.Name, name, false, map[string]bool{}); found && binding.Member != nil && binding.Member.Kind == resolver.ValueExport {
		binding = specializeResolvedClassMember(receiver, binding)
		return binding.Member.Type, binding.Member.Readonly, true, true
	}
	if exported, found := c.resolution.CompilerOwnedType(receiver.Name); found {
		if member, exists := exported.Members[name]; exists && !member.Class && member.Kind == resolver.ValueExport {
			return member.Type, member.Readonly, true, true
		}
	}
	return types.Type{}, false, false, false
}

func (c *Checker) unionDataMember(receiver types.Type, name string) (types.Type, []UnionMemberAccess, bool, bool) {
	receiver = c.expandAlias(receiver, map[string]bool{})
	if receiver.Kind != types.Union || len(receiver.Args) == 0 {
		return types.Type{}, nil, false, false
	}
	memberTypes := make([]types.Type, 0, len(receiver.Args))
	alternatives := make([]UnionMemberAccess, 0, len(receiver.Args))
	classField := false
	for index, alternative := range receiver.Args {
		member, _, storageField, found := c.dataMember(alternative, name)
		if !found || index > 0 && storageField != classField {
			return types.Type{}, nil, false, false
		}
		if index == 0 {
			classField = storageField
		}
		memberTypes = append(memberTypes, member)
		alternatives = append(alternatives, UnionMemberAccess{Alternative: alternative, Member: member})
	}
	return types.UnionOf(memberTypes...), alternatives, classField, true
}

func (c *Checker) specializeLocalEnumMember(receiver types.Type, member classMember) classMember {
	info := c.enums[receiver.Name]
	if info == nil || member.sig == nil || len(info.typeParameters) == 0 {
		return member
	}
	substitutions := typeSubstitutions(info.typeParameters, receiver.Args)
	copy := *member.sig
	copy.returnType = substituteType(copy.returnType, substitutions)
	copy.parameters = append([]callsignature.Parameter(nil), copy.parameters...)
	for index := range copy.parameters {
		copy.parameters[index].Type = substituteType(copy.parameters[index].Type, substitutions)
	}
	member.typ = copy.returnType
	member.sig = &copy
	return member
}

func specializeResolvedEnumMember(receiver types.Type, binding resolver.Binding) resolver.Binding {
	if binding.Export == nil || binding.Member == nil || binding.Member.EnumOwner == "" || len(binding.Export.TypeParameters) == 0 {
		return binding
	}
	substitutions := typeSubstitutions(binding.Export.TypeParameters, receiver.Args)
	copy := *binding.Member
	copy.Type = substituteType(copy.Type, substitutions)
	copy.Parameters = append([]callsignature.Parameter(nil), copy.Parameters...)
	for index := range copy.Parameters {
		copy.Parameters[index].Type = substituteType(copy.Parameters[index].Type, substitutions)
	}
	copy.CallResultBridge = substituteNativeCallResultBridge(copy.CallResultBridge, substitutions)
	binding.Member = &copy
	return binding
}

func specializeResolvedClassMember(receiver types.Type, binding resolver.Binding) resolver.Binding {
	if binding.Export == nil || binding.Member == nil || (binding.Export.Kind != resolver.ClassExport && binding.Export.Kind != resolver.RecordExport && binding.Export.Kind != resolver.InterfaceExport) || len(binding.Export.TypeParameters) == 0 {
		return binding
	}
	substitutions := typeSubstitutions(binding.Export.TypeParameters, receiver.Args)
	copy := *binding.Member
	copy.Type = substituteType(copy.Type, substitutions)
	copy.Parameters = append([]callsignature.Parameter(nil), copy.Parameters...)
	for index := range copy.Parameters {
		copy.Parameters[index].Type = substituteType(copy.Parameters[index].Type, substitutions)
	}
	copy.CallResultBridge = substituteNativeCallResultBridge(copy.CallResultBridge, substitutions)
	binding.Member = &copy
	return binding
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
	if binding, ok := c.resolution.InferredTypeMember(receiverType.Name, member.Name); ok && binding.Member != nil {
		return binding.Member.Readonly
	}
	return false
}

func (c *Checker) classMemberAccess(expression ast.Expression, sc *scope) bool {
	switch node := expression.(type) {
	case *ast.GenericExpression:
		return c.classMemberAccess(node.Receiver, sc)
	case *ast.Identifier:
		if node.Name == "self" {
			return c.current != nil && c.classMethod
		}
		if _, exists := sc.lookup(node.Name); exists {
			return false
		}
		if declared, exists := c.declaredTypes[node.Name]; exists {
			return declared.kind == "class" || declared.kind == "record" || declared.kind == "module" || declared.kind == "enum" || declared.kind == "newtype"
		}
		if c.declarationTypeVisible(node.Name) {
			return true
		}
		if binding, exists := c.result.References[node]; exists && binding.Export != nil {
			switch binding.Export.Kind {
			case resolver.ClassExport, resolver.RecordExport, resolver.ModuleExport, resolver.EnumExport, resolver.NewtypeExport:
				return true
			}
		}
	case *ast.MemberExpression:
		return node.Namespace
	}
	return false
}

func authoredOwnerAccess(expression ast.Expression, sc *scope) bool {
	switch node := expression.(type) {
	case *ast.GenericExpression:
		return authoredOwnerAccess(node.Receiver, sc)
	case *ast.Identifier:
		if _, exists := sc.lookup(node.Name); exists {
			return false
		}
		return isConstant(node.Name)
	case *ast.MemberExpression:
		return node.Namespace
	default:
		return false
	}
}

func (c *Checker) constructorType(name string) bool {
	if declaration, exists := c.declaredTypes[name]; exists {
		return declaration.kind == "class" || declaration.kind == "record" || declaration.kind == "newtype"
	}
	if c.declarationTypeVisible(name) {
		return true
	}
	if binding, exists := c.resolution.ImportedType(name); exists && binding.Export != nil {
		return binding.Export.Kind == resolver.ClassExport || binding.Export.Kind == resolver.RecordExport || binding.Export.Kind == resolver.NewtypeExport
	}
	if exported, exists := c.resolution.CompilerOwnedType(name); exists {
		return exported.Kind == resolver.ClassExport || exported.Kind == resolver.RecordExport || exported.Kind == resolver.NewtypeExport
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

func (c *Checker) declarationTypeVisible(name string) bool {
	declared, exists := c.declarations().Type(name)
	if !exists {
		return false
	}
	if declared.SourceModule == "" || c.result.Program == nil || declared.SourceModule == c.result.Program.ModulePath {
		return true
	}
	if _, local := c.declaredTypes[name]; local {
		return true
	}
	binding, imported := c.resolution.ImportedType(name)
	return imported && (binding.Import == nil || !binding.Import.CompilerGenerated)
}

func (c *Checker) declarationFunctionArgumentReference(call *ast.CallExpression, argumentIndex int) bool {
	if call == nil || argumentIndex < 0 || argumentIndex >= len(call.Arguments) || call.Arguments[argumentIndex].Name != "" || c.current == nil || c.result.Program == nil {
		return false
	}
	if c.declarationCalls[call] != c.current.name {
		return false
	}
	target, ok := call.Arguments[argumentIndex].Value.(*ast.Identifier)
	if !ok {
		return false
	}
	binding, referenced := c.result.References[call.Callee]
	if !referenced || binding.Import == nil {
		return false
	}
	position := -1
	for index := range call.Arguments {
		if call.Arguments[index].Name != "" {
			continue
		}
		position++
		if index == argumentIndex {
			break
		}
	}
	packagePath := strings.TrimSuffix(binding.Import.RuntimePath(), "/index")
	for _, rule := range c.declarations().FunctionArgumentReferenceRules {
		if rule.Package == packagePath && rule.Function == binding.Name && rule.Argument == position &&
			rule.Owner.ModulePath == c.result.Program.ModulePath && rule.Owner.Name == c.current.name {
			for _, reference := range rule.Targets {
				if reference.Name == target.Name {
					return true
				}
			}
		}
	}
	return false
}

func (c *Checker) declarationOnlyClassBodyCall(call *ast.CallExpression) bool {
	if call == nil || c.result.Program == nil {
		return false
	}
	owner := c.declarationCalls[call]
	if owner == "" {
		return false
	}
	callee := call.Callee
	if generic, ok := callee.(*ast.GenericExpression); ok {
		callee = generic.Receiver
	}
	binding, referenced := c.result.References[callee]
	if !referenced || binding.Import == nil {
		return false
	}
	packagePath := strings.TrimSuffix(binding.Import.RuntimePath(), "/index")
	for _, rule := range c.declarations().ClassBodyDeclarationRules {
		if rule.Package == packagePath && rule.Function == binding.Name &&
			rule.Owner.ModulePath == c.result.Program.ModulePath && rule.Owner.Name == owner {
			return true
		}
	}
	return false
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

func (c *Checker) specializeDeclarationMember(receiver types.Type, member declaration.Member) declaration.Member {
	declared, ok := c.declarations().Type(receiver.Name)
	if !ok || len(declared.TypeParameters) == 0 || len(receiver.Args) == 0 {
		return member
	}
	bindings := map[string]types.Type{}
	for index, name := range declared.TypeParameters {
		if index < len(receiver.Args) {
			bindings[name] = receiver.Args[index]
		}
	}
	result := member
	result.Return = instantiateDeclarationType(member.Return, bindings)
	result.Parameters = append([]declaration.Parameter(nil), member.Parameters...)
	for index := range result.Parameters {
		result.Parameters[index].Type = instantiateDeclarationType(result.Parameters[index].Type, bindings)
	}
	result.Alternatives = append([]declaration.Signature(nil), member.Alternatives...)
	for index := range result.Alternatives {
		result.Alternatives[index].Return = instantiateDeclarationType(result.Alternatives[index].Return, bindings)
		result.Alternatives[index].Parameters = append([]declaration.Parameter(nil), result.Alternatives[index].Parameters...)
		for parameterIndex := range result.Alternatives[index].Parameters {
			result.Alternatives[index].Parameters[parameterIndex].Type = instantiateDeclarationType(result.Alternatives[index].Parameters[parameterIndex].Type, bindings)
		}
	}
	if member.Block != nil {
		block := *member.Block
		block.Parameters = instantiateDeclarationTypes(member.Block.Parameters, bindings)
		block.Return = instantiateDeclarationType(member.Block.Return, bindings)
		block.ResultBoundary = instantiateDeclarationType(member.Block.ResultBoundary, bindings)
		result.Block = &block
	}
	return result
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

func directCallIdentifier(expression ast.Expression) *ast.Identifier {
	switch node := expression.(type) {
	case *ast.Identifier:
		return node
	case *ast.GenericExpression:
		return directCallIdentifier(node.Receiver)
	default:
		return nil
	}
}

func (c *Checker) recordDefaultCallable(name string) bool {
	if c.functions[name] != nil {
		return true
	}
	if c.current != nil {
		if method := c.current.methods[name]; method != nil && method.Class == c.classMethod {
			return true
		}
	}
	return false
}

func (c *Checker) authoredTypeInScope(name string, sc *scope) (string, bool) {
	declaration, ok := c.authoredTypeIdentityInScope(name, sc)
	return declaration.LeafName(), ok
}

func (c *Checker) authoredTypeIdentityInScope(name string, sc *scope) (identity.Declaration, bool) {
	owner := ""
	for current := sc; current != nil; current = current.parent {
		if current.constantOwner != "" {
			owner = current.constantOwner
			break
		}
	}
	for current := owner; ; {
		qualified := nestedAuthoredOwner(current, name)
		if _, ok := c.authoredTypes[qualified]; ok {
			return c.authoredOwnerIdentities[qualified], true
		}
		separator := strings.LastIndex(current, "::")
		if separator < 0 {
			if current == "" {
				break
			}
			current = ""
		} else {
			current = current[:separator]
		}
	}
	return identity.Declaration{}, false
}

func (c *Checker) authoredOwnedMethodInScope(owner, name string, sc *scope) *ast.MethodStatement {
	if owner == "" {
		return nil
	}
	constantOwner := ""
	for current := sc; current != nil; current = current.parent {
		if current.constantOwner != "" {
			constantOwner = current.constantOwner
			break
		}
	}
	for current := constantOwner; ; {
		candidate := nestedAuthoredOwner(current, owner)
		if method := c.authoredOwnedMethods[authoredOwnedMethodKey(candidate, name)]; method != nil {
			return method
		}
		separator := strings.LastIndex(current, "::")
		if separator < 0 {
			if current == "" {
				break
			}
			current = ""
		} else {
			current = current[:separator]
		}
	}
	return nil
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
	case *ast.LambdaExpression:
		lambdaScope := &scope{parent: sc, values: map[string]symbol{}}
		parameterTypes := make([]types.Type, 0, len(n.Parameters))
		for _, parameter := range n.Parameters {
			if parameter.Type.Empty() {
				c.error(parameter.Span(), fmt.Sprintf("fn parameter %s requires a type", parameter.Name))
			}
			if parameter.Default != nil || parameter.NamedOnly || parameter.Keyword || parameter.Rest || parameter.KeywordRest {
				c.error(parameter.Span(), "fn parameters must be required positional parameters")
			}
			parameterType := c.typeFromRef(parameter.Type)
			if parameter.Type.Empty() {
				parameterType = invalidType()
			}
			if _, duplicate := lambdaScope.values[parameter.Name]; duplicate {
				c.error(parameter.Span(), fmt.Sprintf("fn parameter %s is duplicated", parameter.Name))
				continue
			}
			bindingType := parameterType
			if !parameter.Mutable && isReferenceType(bindingType) {
				bindingType.Readonly = true
			}
			declared := symbol{typ: bindingType, mutable: parameter.Mutable, span: parameter.Span()}
			if tracksUnusedBinding(parameter.Name) {
				used := false
				declared.used = &used
				declared.useKind = "fn parameter"
			}
			lambdaScope.values[parameter.Name] = declared
			parameterTypes = append(parameterTypes, parameterType)
		}
		returnType := types.FromName("Void")
		if !n.ReturnType.Empty() {
			returnType = c.typeFromRef(n.ReturnType)
		}
		c.returns = append(c.returns, returnType)
		c.resultBoundaries = append(c.resultBoundaries, c.resultBoundaryFor(returnType))
		previousLoopDepth := c.loopDepth
		previousValueTransformDepth := c.valueTransformDepth
		previousResultBoundaryBlockDepth := c.resultBoundaryBlockDepth
		previousControlBoundaries := c.controlBoundaries
		c.loopDepth = 0
		c.valueTransformDepth = 0
		c.resultBoundaryBlockDepth = 0
		c.controlBoundaries = nil
		if c.inferenceOnly {
			c.callbackScopes = append(c.callbackScopes, lambdaScope)
		}
		c.checkStatements(n.Body, lambdaScope)
		if c.inferenceOnly {
			c.callbackScopes = c.callbackScopes[:len(c.callbackScopes)-1]
		}
		if returnType.Kind != types.Void && c.statementsFallThrough(n.Body) {
			c.error(n.Span(), fmt.Sprintf("fn must return %s on every path", returnType))
		}
		c.loopDepth = previousLoopDepth
		c.valueTransformDepth = previousValueTransformDepth
		c.resultBoundaryBlockDepth = previousResultBoundaryBlockDepth
		c.controlBoundaries = previousControlBoundaries
		c.returns = c.returns[:len(c.returns)-1]
		c.resultBoundaries = c.resultBoundaries[:len(c.resultBoundaries)-1]
		typ = types.FunctionOf(parameterTypes, returnType)
	case *ast.Identifier:
		if n.Name == "_" {
			c.error(n.Span(), "blank binding _ cannot be used as a value")
			typ = invalidType()
			break
		}
		if value, owner, ok := sc.lookupOwner(n.Name); ok {
			typ = value.typ
			if root := c.currentConcurrentBlockScope(); root != nil && !scopeWithin(owner, root) && !c.concurrentInitTargets[n] {
				switch {
				case !c.concurrencySafeType(value.typ, map[string]bool{}):
					c.error(n.Span(), fmt.Sprintf("concurrent_map cannot capture %s because %s is not concurrency-safe", n.Name, value.typ))
				}
			}
			if c.inferenceOnly && value.pending != nil && value.pending.resolved.Kind == "" && !value.pending.blocked {
				c.pendingExpressions[n] = value.pending
				if c.pendingCollectionIsCaptured(value.pending) {
					c.markEmptyCollectionCapture(value.pending, n.Span())
				}
			}
			if value.declared.Nullable && !value.typ.Nullable {
				c.result.NullableUnwraps[n] = value.declared
			}
			if !value.constant {
				c.result.LexicalBindings[n] = true
			}
			sc.markUsed(n.Name)
			if value.constant {
				c.result.Constants[n] = value.owner
			}
		} else if binding, ok := c.importedValueAt(n.Name, n.Span()); ok {
			typ = c.resolvedBindingType(binding)
			c.recordReference(n, binding)
		} else if member, ok := c.currentDeclarationMember(n.Name); ok {
			typ = member.Return
			c.external[n] = member
			c.result.ExternalMembers[n] = member
		} else if strings.HasPrefix(n.Name, "@") && c.current != nil {
			if field, ok := c.current.fields[n.Name]; ok {
				typ = c.typeFromRef(field.Type)
			}
		} else if c.recordDefaultUnavailable[n.Name] && (c.recordDefaultCallee != n || !c.recordDefaultCallable(n.Name)) {
			c.error(n.Span(), fmt.Sprintf("record field default cannot reference current or later field %s", n.Name))
			typ = invalidType()
		} else if isConstant(n.Name) {
			typ = types.FromName(n.Name)
			if declaration := c.authoredTypeIdentities[n.Name]; !declaration.Empty() {
				c.result.ExpressionDeclarations[n] = declaration
			}
			if c.declarationReferences == 0 && !c.declarationTypeVisible(n.Name) {
				if declared, exists := c.declarations().Type(n.Name); exists && declared.SourceModule != "" {
					c.error(n.Span(), fmt.Sprintf("type %s is not declared or imported", n.Name))
					typ = invalidType()
				}
			}
		}
	case *ast.Literal:
		switch n.Kind {
		case ast.StringLiteral:
			typ = types.FromName("String")
		case ast.IntegerLiteral:
			if _, ok := types.ParsePortableIntegerLiteral(n.Raw); !ok {
				c.error(n.Span(), portableIntegerLiteralRangeMessage)
			}
			typ = types.FromName("Integer")
		case ast.FloatLiteral:
			if _, ok := types.ParsePortableFloatLiteral(n.Raw); !ok {
				c.error(n.Span(), portableFloatLiteralRangeMessage)
			}
			typ = types.FromName("Float")
		case ast.BooleanLiteral:
			typ = types.FromName("Boolean")
		case ast.NilLiteral:
			typ = types.FromName("Nil")
		}
	case *ast.InterpolatedString:
		for _, part := range n.Parts {
			if part.Expression != nil {
				actual := c.checkExpression(part.Expression, sc)
				expanded := scalarType(c.expandAlias(actual, map[string]bool{}))
				if expanded.Kind != types.Invalid && !isNonNullable(expanded, types.String) {
					c.error(part.Expression.Span(), fmt.Sprintf("string interpolation requires String, got %s", actual))
				}
			}
		}
		typ = types.FromName("String")
	case *ast.SymbolLiteral:
		typ = types.FromName("String")
	case *ast.ArrayLiteral:
		element := c.inferCollectionType(n.Elements, sc)
		if c.inferenceOnly {
			for _, value := range n.Elements {
				if pending := c.pendingEmptyCollection(value); pending != nil {
					c.markEmptyCollectionCapture(pending, value.Span())
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
		if c.inferenceOnly {
			for _, entry := range n.Entries {
				if pending := c.pendingEmptyCollection(entry.Key); pending != nil {
					c.markEmptyCollectionCapture(pending, entry.Key.Span())
				}
				if pending := c.pendingEmptyCollection(entry.Value); pending != nil {
					c.markEmptyCollectionCapture(pending, entry.Value.Span())
				}
			}
		}
		typ = types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{keyType, valueType}}
	case *ast.JSXElement:
		provider := c.jsxProvider(n.Span())
		nodeType := types.FromName("Any")
		if provider == nil {
			c.error(n.Span(), "JSX requires an imported JSX provider; import trb/platform/typescript/react for React")
		} else {
			nodeType = provider.Node
		}
		var props []resolver.RecordField
		var unsupportedProps map[string]string
		componentHasTypedProps := false
		if n.Component != nil {
			componentType := c.checkExpression(n.Component, sc)
			if componentType.Kind == types.Any || componentType.Kind == types.Invalid {
				c.error(n.Component.Span(), fmt.Sprintf("JSX component %s is not declared or imported", n.Name))
			} else {
				props, unsupportedProps, componentHasTypedProps = c.jsxComponentProps(n, nodeType)
			}
		}
		expectedAttributes := map[string]types.Type{}
		if componentHasTypedProps {
			for _, prop := range props {
				expectedAttributes[prop.Name] = prop.Type
			}
		} else if n.Component == nil && provider != nil {
			expectedAttributes = provider.IntrinsicAttributes
		}
		attributeTypes := map[string]types.Type{}
		for _, attribute := range n.Attributes {
			if _, duplicate := attributeTypes[attribute.Name]; duplicate {
				c.error(attribute.Span(), fmt.Sprintf("JSX attribute %s is already specified", attribute.Name))
				continue
			}
			if attribute.Boolean {
				attributeTypes[attribute.Name] = types.FromName("Boolean")
			} else {
				attributeTypes[attribute.Name] = c.checkExpression(attribute.Value, sc)
				if literalType, literal := literalExpressionType(attribute.Value); literal {
					attributeTypes[attribute.Name] = literalType
				}
				if expected, found := expectedAttributes[attribute.Name]; found {
					attributeTypes[attribute.Name] = c.contextualizeCollectionLiteral(attribute.Value, expected, attributeTypes[attribute.Name])
				}
			}
			if n.Component == nil && provider != nil {
				if expected, checked := provider.IntrinsicAttributes[attribute.Name]; checked && !c.typesAssignable(expected, attributeTypes[attribute.Name]) {
					c.error(attribute.Span(), fmt.Sprintf("JSX attribute %s expects %s, got %s", attribute.Name, expected, attributeTypes[attribute.Name]))
				}
			}
		}
		if componentHasTypedProps {
			c.checkJSXProps(n, props, unsupportedProps, attributeTypes, nodeType)
		}
		for _, child := range n.Children {
			switch item := child.(type) {
			case *ast.JSXElement:
				c.checkExpression(item, sc)
			case *ast.JSXExpression:
				childType := c.checkExpression(item.Value, sc)
				if provider != nil && !jsxRenderableType(c.expandAlias(childType, map[string]bool{}), nodeType) {
					c.error(item.Span(), fmt.Sprintf("JSX child must be renderable, got %s", childType))
				}
			}
		}
		typ = nodeType
	case *ast.UnaryExpression:
		operand := c.checkExpression(n.Operand, sc)
		typ = c.checkUnaryOperator(n.Span(), n.Operator, operand)
	case *ast.BinaryExpression:
		left := c.checkExpression(n.Left, sc)
		rightScope := sc
		if n.Operator == "&&" {
			rightScope, _ = c.nullableConditionScopes(n.Left, sc)
		} else if n.Operator == "||" {
			_, rightScope = c.nullableConditionScopes(n.Left, sc)
		}
		right := c.checkExpression(n.Right, rightScope)
		typ = c.checkBinaryOperator(n.Span(), n.Operator, left, right)
		if typ.Kind != types.Invalid && isNonNullableNumber(left) && isNonNullableNumber(right) && scalarType(left).Kind != scalarType(right).Kind {
			if scalarType(left).Kind == types.Int {
				c.recordIntegerToFloat(n.Left)
			}
			if scalarType(right).Kind == types.Int {
				c.recordIntegerToFloat(n.Right)
			}
		}
	case *ast.RangeExpression:
		start := c.checkExpression(n.Start, sc)
		end := c.checkExpression(n.End, sc)
		if start.Kind == types.Never || end.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
		} else if scalarType(start).Kind != types.Int || scalarType(end).Kind != types.Int {
			c.error(n.Span(), fmt.Sprintf("range endpoints must be Integer, got %s and %s", start, end))
		} else {
			typ = types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}
		}
	case *ast.AttemptExpression:
		// attempt is retained in the syntax AST only so the parser can issue the
		// focused 0.3 migration diagnostic. Never assign it executable effect
		// semantics in the checker.
		if n.Value != nil {
			c.checkExpression(n.Value, sc)
		} else {
			c.checkStatements(n.Body, &scope{parent: sc, values: map[string]symbol{}})
		}
		typ = invalidType()
	case *ast.TryExpression:
		typ = c.checkResultTry(n, sc)
	case *ast.CatchExpression:
		typ = c.checkResultCatch(n, sc)
	case *ast.IterationExpression:
		predicate := n.Operation == "any?" || n.Operation == "all?" || n.Operation == "none?" || n.Operation == "find" || n.Operation == "find_index"
		sortBy := n.Operation == "sort_by" || n.Operation == "sort_by_descending"
		transform := n.Operation == "map" || n.Operation == "concurrent_map" || n.Operation == "select" || n.Operation == "reduce" || predicate || sortBy
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
		if n.Operation == "concurrent_map" {
			if len(c.returns) == 0 && !c.interactiveTopLevel {
				c.error(n.Span(), "concurrent_map is only available inside a function or method")
			}
			if sourceType.Kind != types.Array {
				c.error(n.Source.Span(), fmt.Sprintf("concurrent_map is available only on Array, got %s", sourceType))
			}
			if n.WithIndex {
				c.error(n.Span(), "concurrent_map.with_index is not supported")
			}
			if n.Limit != nil {
				limitType := c.checkExpression(n.Limit, sc)
				if scalarType(limitType).Kind != types.Int {
					c.error(n.Limit.Span(), fmt.Sprintf("concurrent_map limit must be Integer, got %s", limitType))
				}
				if literal, ok := n.Limit.(*ast.Literal); ok {
					if limit, valid := integerLiteral(literal.Raw); valid && limit <= 0 {
						c.error(n.Limit.Span(), "concurrent_map limit must be greater than zero")
					}
				}
			}
		}
		hashEach := hashSource && n.Operation == "each"
		if hashSource && !hashEach {
			c.error(n.Span(), "Hash iteration supports only each in v0.1")
		}
		if hashEach && n.WithIndex {
			c.error(n.Span(), "Hash#each.with_index is not supported in v0.1")
		}
		itemType := elementType
		if sortBy && sourceType.Kind != types.Array {
			c.error(n.Source.Span(), fmt.Sprintf("%s is available only on Array, got %s", n.Operation, sourceType))
		}
		if predicate && n.WithIndex {
			c.error(n.Span(), n.Operation+".with_index is not supported; use each with an explicit accumulator")
		}
		if sortBy && n.WithIndex {
			c.error(n.Span(), n.Operation+".with_index is not supported")
		}
		if n.Operation == "each_slice" && !hashSource {
			if n.SliceSize == nil {
				c.error(n.Span(), "each_slice expects exactly one size argument")
			} else {
				sizeType := c.checkExpression(n.SliceSize, sc)
				if scalarType(sizeType).Kind != types.Int {
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
			borrowedIterationSource := c.concurrentBorrowedExpression(n.Source, sc)
			if n.Initial != nil {
				borrowedIterationSource = borrowedIterationSource || c.concurrentBorrowedExpression(n.Initial, sc)
			}
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
				declared := symbol{
					typ:     parameterType,
					mutable: true,
					span:    n.Block.Span(),
					concurrentBorrowed: c.concurrentBorrowedType(parameterType) &&
						(n.Operation == "concurrent_map" || borrowedIterationSource),
				}
				if tracksUnusedBinding(name) {
					used := false
					declared.used = &used
					declared.useKind = "block parameter"
				}
				blockScope.values[name] = declared
			}
			if transform {
				c.valueTransformDepth++
			}
			if n.Operation == "concurrent_map" {
				c.concurrentBlockScopes = append(c.concurrentBlockScopes, blockScope)
				c.concurrentMapDepth++
			}
			if transform {
				blockType := types.Type{Kind: types.Any, Name: "Any"}
				var resultExpression ast.Expression
				if transfer := statementsEscapingTransform(n.Block.Body, 0); transfer != nil {
					c.error(transfer.Span(), fmt.Sprintf("%s is not supported inside value-producing collection transformations yet", controlTransferKeyword(transfer)))
				}
				if len(n.Block.Body) == 0 {
					c.error(n.Block.Span(), fmt.Sprintf("%s block must end with a result expression", n.Operation))
				} else {
					lastIndex := len(n.Block.Body) - 1
					last := n.Block.Body[lastIndex]
					if result, ok := last.(*ast.ExpressionStatement); ok {
						resultExpression = result.Expression
						c.checkStatementSequence(n.Block.Body[:lastIndex], blockScope)
						c.checkExpression(resultExpression, blockScope)
						c.checkUnusedBindings(blockScope)
					} else if result, ok := last.(ast.Expression); ok {
						resultExpression = result
						c.checkStatementSequence(n.Block.Body[:lastIndex], blockScope)
						c.checkExpression(resultExpression, blockScope)
						c.checkUnusedBindings(blockScope)
					} else {
						c.error(last.Span(), fmt.Sprintf("%s block must end with a result expression", n.Operation))
						c.checkStatements(n.Block.Body, blockScope)
					}
				}
				if resultExpression != nil {
					blockType = c.result.Expressions[resultExpression]
				}
				c.result.Expressions[n.Block] = blockType
				switch n.Operation {
				case "map", "concurrent_map":
					typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{blockType}}
				case "select", "any?", "all?", "none?", "find", "find_index":
					if resultExpression != nil && (blockType.Kind != types.Bool || blockType.Nullable) {
						c.error(n.Block.Span(), fmt.Sprintf("%s block result must be Boolean, got %s", n.Operation, blockType))
					}
					if n.Operation == "select" {
						typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{elementType}}
					} else if n.Operation == "find" {
						typ = elementType
						typ.Nullable = true
					} else if n.Operation == "find_index" {
						typ = types.FromName("Integer")
						typ.Nullable = true
					} else {
						typ = types.FromName("Boolean")
					}
				case "reduce":
					if resultExpression != nil && !c.assignable(resultExpression, accumulatorType, blockType) {
						c.error(n.Block.Span(), fmt.Sprintf("reduce block result is %s, expected %s", blockType, accumulatorType))
					}
					typ = accumulatorType
				case "sort_by", "sort_by_descending":
					if resultExpression != nil && !portableOrderType(blockType) {
						c.error(n.Block.Span(), fmt.Sprintf("%s block result must have portable natural order, got %s", n.Operation, blockType))
					}
					typ = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{elementType}}
				}
			} else {
				c.loopDepth++
				c.checkStatements(n.Block.Body, blockScope)
				c.loopDepth--
				c.result.Expressions[n.Block] = types.Type{Kind: types.Void, Name: "Void"}
			}
			if transform {
				c.valueTransformDepth--
			}
			if n.Operation == "concurrent_map" {
				c.concurrentMapDepth--
				c.concurrentBlockScopes = c.concurrentBlockScopes[:len(c.concurrentBlockScopes)-1]
			}
		}
		if !transform {
			typ = types.Type{Kind: types.Void, Name: "Void"}
		}
	case *ast.GenericExpression:
		receiverType := c.checkExpression(n.Receiver, sc)
		application, ok := c.resolveGenericApplication(n)
		if !ok {
			typ = receiverType
			break
		}
		application.Declaration = c.result.ExpressionDeclarations[n.Receiver]
		application.Dispatch = c.result.ExpressionDispatches[n.Receiver]
		c.result.GenericApplications[n] = application
		if !application.Declaration.Empty() {
			c.result.ExpressionDeclarations[n] = application.Declaration
		}
		if !application.Dispatch.Empty() {
			c.result.ExpressionDispatches[n] = application.Dispatch
		}
		typ = application.ReturnType
		if application.Declaration.Kind.IsType() && typ.Kind == types.Named {
			typ.Declaration = application.Declaration
		}
	case *ast.MemberExpression:
		if controlType, handled := c.checkAssociationControlMember(n, sc); handled {
			typ = controlType
			break
		}
		receiverType := c.checkExpression(n.Receiver, sc)
		if n.Namespace {
			if declaration, authored := c.authoredTypeIdentityInScope(expressionTypeName(n), sc); authored {
				typ = types.FromName(declaration.LeafName())
				typ.Declaration = declaration
				c.result.ExpressionDeclarations[n] = declaration
				break
			}
		}
		if receiverType.Kind == types.Never {
			typ = types.Type{Kind: types.Never, Name: "Never"}
			break
		}
		if c.concurrencyInterfaceType(receiverType) {
			c.concurrentInterfaceMembers[n] = true
		}
		methodReceiverType := scalarType(c.expandAlias(receiverType, map[string]bool{}))
		dataReceiverType := c.expandAlias(receiverType, map[string]bool{})
		if dataReceiverType.Kind == types.Union && scalarType(dataReceiverType).Kind == types.Union && !n.Namespace && !n.Safe {
			memberType, alternatives, classField, found := c.unionDataMember(dataReceiverType, n.Name)
			if !found {
				c.error(n.Span(), fmt.Sprintf("union type %s has no common data member %s", receiverType, n.Name))
				typ = invalidType()
			} else {
				typ = memberType
				c.result.UnionMemberAccesses[n] = alternatives
				c.result.ClassFieldAccesses[n] = classField
			}
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
		if n.Namespace {
			if binding, exists := c.resolution.TypeMemberIdentity(receiverType.Declaration, n.Name); exists && binding.Export != nil && binding.Member == nil && binding.Export.Kind != resolver.ModuleExport {
				typ = c.resolvedBindingType(binding)
				c.recordReference(n, binding)
				break
			}
		}
		if target, _, newtype := c.newtypeDefinition(receiverType.Name); newtype {
			switch {
			case classAccess && n.Name == "new":
				typ = receiverType
			case !classAccess && n.Name == "value":
				typ = target
			default:
				kind := "instance"
				if classAccess {
					kind = "class"
				}
				c.error(n.Span(), fmt.Sprintf("newtype %s has no %s member %s", receiverType.Name, kind, n.Name))
				typ = invalidType()
			}
			break
		}
		if identifier, ok := n.Receiver.(*ast.Identifier); ok && !n.Namespace {
			if owner, imported := c.result.References[identifier]; imported && owner.Export != nil && owner.Export.NativeExported && owner.Export.Kind == resolver.FunctionExport {
				if member, found := owner.Export.Members[n.Name]; found {
					binding := resolver.Binding{Import: owner.Import, Export: owner.Export, Member: &member, Name: member.Name}
					typ = c.resolvedBindingType(binding)
					c.recordReference(n, binding)
				} else {
					c.error(n.Span(), fmt.Sprintf("native component %s has no member %s", identifier.Name, n.Name))
					typ = invalidType()
				}
				break
			}
		}
		if identifier, ok := n.Receiver.(*ast.Identifier); ok {
			if imported := c.resolution.Packages[identifier.Name]; imported != nil {
				c.markImportNodeUsed(imported, "")
				if binding, exists := c.resolution.Member(identifier.Name, n.Name); exists {
					typ = c.resolvedBindingType(binding)
					c.recordReference(n, binding)
				} else {
					c.error(n.Span(), fmt.Sprintf("package %s does not export %s", imported.Path, n.Name))
				}
				break
			}
		}
		if variants, enum := c.enumVariants(receiverType); enum && !n.Namespace {
			if _, variant := enumVariantNamed(variants, n.Name); variant {
				c.error(n.Span(), fmt.Sprintf("enum member %s must be accessed with ::", n.Name))
				break
			}
		}
		if variants, enum := c.enumVariants(receiverType); enum && n.Namespace {
			if expected, generic := c.genericTypeArity(receiverType.Name); generic && len(receiverType.Args) != expected {
				c.error(n.Receiver.Span(), fmt.Sprintf("%s expects %d type argument(s), got %d", receiverType.Name, expected, len(receiverType.Args)))
				break
			}
			variant, found := enumVariantNamed(variants, n.Name)
			if !found {
				c.error(n.Span(), fmt.Sprintf("enum %s has no member %s", receiverType.Name, n.Name))
				break
			}
			if variant.Reference != nil {
				c.recordReference(n, *variant.Reference)
			} else if binding, exists := c.resolution.TypeMemberIdentity(receiverType.Declaration, n.Name); exists {
				c.recordReference(n, binding)
			} else if binding, exists := c.resolution.TypeMember(receiverType.Name, n.Name); exists {
				c.recordReference(n, binding)
			}
			if !variant.Declaration.Empty() {
				c.result.ExpressionDeclarations[n] = variant.Declaration
			}
			typ = receiverType
			if len(variant.Fields) > 0 && c.enumCallee == 0 && c.enumPattern == 0 {
				c.error(n.Span(), fmt.Sprintf("enum member %s::%s requires %d payload argument(s)", receiverType.Name, n.Name, len(variant.Fields)))
			}
			break
		}
		if classAccess || authoredOwnerAccess(n.Receiver, sc) {
			if method := c.authoredOwnedMethodInScope(expressionTypeName(n.Receiver), n.Name, sc); method != nil {
				typ = c.methodReturnType(method)
				c.authoredMemberMethods[n] = method
				if dispatch := c.result.MethodDispatches[method]; !dispatch.Empty() {
					c.result.ExpressionDispatches[n] = dispatch
				}
				break
			}
		}
		if n.Name == "new" && !classAccess {
			if c.constructorType(receiverType.Name) {
				c.memberKindMismatch(n.Span(), receiverType.Name, n.Name, false)
				break
			}
		}
		if !classAccess && !n.Namespace {
			if binding, exists := c.resolution.ReceiverMethod(methodReceiverType, n.Name); exists {
				typ = c.resolvedBindingType(binding)
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
			typ = substituteType(c.typeFromRef(record.byName[n.Name].Type), typeSubstitutions(record.typeParameters, receiverType.Args))
		} else if member, found := c.localMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); found {
			member = c.specializeLocalEnumMember(receiverType, member)
			member = c.specializeLocalClassMember(receiverType, member)
			typ = member.typ
			if member.method != nil {
				if c.interfaces[receiverType.Name] == nil {
					c.authoredMemberMethods[n] = member.method
					if dispatch := c.result.MethodDispatches[member.method]; !dispatch.Empty() {
						c.result.ExpressionDispatches[n] = dispatch
					}
				}
			}
			c.result.ClassFieldAccesses[n] = member.field != nil
		} else if binding, exists := c.importedAncestorMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			binding = specializeResolvedEnumMember(receiverType, binding)
			binding = specializeResolvedClassMember(receiverType, binding)
			typ = c.resolvedBindingType(binding)
			classType := c.classes[receiverType.Name] != nil
			if imported, found := c.resolution.ImportedType(receiverType.Name); found && imported.Export != nil {
				classType = classType || imported.Export.Kind == resolver.ClassExport
			}
			c.result.ClassFieldAccesses[n] = classType && binding.Member != nil && binding.Member.Kind == resolver.ValueExport
			c.markImportedSymbolUsed(receiverType.Name, c.generatedSpan(n.Span()))
			c.recordReference(n, binding)
		} else if binding, exists := c.resolution.InferredTypeMember(dataReceiverType.Name, n.Name); exists && binding.Member != nil && binding.Member.Class == classAccess {
			binding = specializeResolvedEnumMember(dataReceiverType, binding)
			binding = specializeResolvedClassMember(dataReceiverType, binding)
			typ = c.resolvedBindingType(binding)
			c.result.ClassFieldAccesses[n] = binding.Export != nil && binding.Export.Kind == resolver.ClassExport && binding.Member.Kind == resolver.ValueExport
			c.recordReference(n, binding)
		} else if member, exists := c.declarationMember(receiverType.Name, n.Name, classAccess, map[string]bool{}); exists {
			member = c.specializeDeclarationMember(receiverType, member)
			typ = member.Return
			c.external[n] = member
			c.result.ExternalMembers[n] = member
		} else if exported, exists := c.resolution.CompilerOwnedType(receiverType.Name); exists {
			if member, found := exported.Members[n.Name]; found && !member.Class {
				binding := resolver.Binding{Export: &exported, Member: &member, Name: member.Name}
				binding = specializeResolvedClassMember(receiverType, binding)
				typ = c.resolvedBindingType(binding)
				c.result.ClassFieldAccesses[n] = exported.Kind == resolver.ClassExport && member.Kind == resolver.ValueExport
			} else if n.Name != "new" {
				c.error(n.Span(), fmt.Sprintf("type %s has no member %s", receiverType.Name, n.Name))
			}
		} else if n.Name == "new" && classAccess && c.constructorType(receiverType.Name) {
			// Constructors are validated against their initialize method or record
			// fields when the surrounding call expression is checked.
		} else {
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
				if imported.Export != nil && imported.Export.UnsupportedFields[n.Name] != "" {
					c.error(n.Span(), fmt.Sprintf("member %s from native type %s cannot be represented safely: %s; use a TypeRB provider for this package", n.Name, receiverType.Name, imported.Export.UnsupportedFields[n.Name]))
				} else {
					c.error(n.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiverType.Name, imported.Import.Path, n.Name))
				}
			} else if inferred, exists := c.resolution.InferredType(receiverType.Name); exists {
				c.markImportUsed(inferred)
				if inferred.Export != nil && inferred.Export.UnsupportedFields[n.Name] != "" {
					c.error(n.Span(), fmt.Sprintf("member %s from native type %s cannot be represented safely: %s; use a TypeRB provider for this package", n.Name, receiverType.Name, inferred.Export.UnsupportedFields[n.Name]))
				} else {
					c.error(n.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiverType.Name, inferred.Import.Path, n.Name))
				}
			} else if declared, exists := c.declarations().Type(receiverType.Name); exists {
				c.error(n.Span(), fmt.Sprintf("externally provided type %s has no member %s", declared.Name, n.Name))
			} else if portableReceiverKind(receiverType.Kind) {
				c.error(n.Span(), fmt.Sprintf("type %s has no member %s", receiverType, n.Name))
			}
		}
	case *ast.CallExpression:
		libraryBlockChecked := false
		if !c.rubyNativeSyntax() {
			seenNamed := false
			for _, argument := range n.Arguments {
				if argument.Splat != "" {
					c.error(argument.Value.Span(), "argument splats are not supported in portable TypeRB")
				}
				if argument.Name != "" {
					seenNamed = true
				} else if seenNamed {
					c.error(argument.Value.Span(), "positional argument cannot follow a named argument")
				}
			}
		}
		if member, ok := n.Callee.(*ast.MemberExpression); ok && member.Namespace {
			c.enumCallee++
		}
		previousRecordDefaultCallee := c.recordDefaultCallee
		c.recordDefaultCallee = directCallIdentifier(n.Callee)
		calleeType := c.checkExpression(n.Callee, sc)
		c.recordDefaultCallee = previousRecordDefaultCallee
		if member, ok := n.Callee.(*ast.MemberExpression); ok && member.Namespace {
			c.enumCallee--
		}
		c.recordAuthoredCall(n)
		c.checkConcurrentDynamicCall(n.Callee, calleeType)
		c.checkConcurrentNativeCall(n.Callee)
		if c.declarationOnlyClassBodyCall(n) {
			c.result.DeclarationOnlyCalls[n] = true
		}
		argumentTypes := make([]types.Type, 0, len(n.Arguments))
		for index, arg := range n.Arguments {
			declarationReference := c.declarationFunctionArgumentReference(n, index)
			if declarationReference {
				c.declarationReferences++
			}
			argumentTypes = append(argumentTypes, c.checkExpression(arg.Value, sc))
			if declarationReference {
				c.declarationReferences--
			}
		}
		c.constrainEmptyCollectionCall(n, argumentTypes)
		typ = calleeType
		if member, ok := n.Callee.(*ast.MemberExpression); ok {
			receiverType := c.result.Expressions[member.Receiver]
			if target, _, newtype := c.newtypeDefinition(receiverType.Name); newtype {
				switch member.Name {
				case "new":
					for _, argument := range n.Arguments {
						if argument.Name != "" {
							c.error(argument.Value.Span(), fmt.Sprintf("%s.new() has no named argument %s", receiverType.Name, argument.Name))
						}
					}
					c.checkTypedArguments(n.Span(), receiverType.Name+".new", []types.Type{target}, 1, false, n.Arguments, argumentTypes)
					if n.Block != nil {
						c.error(n.Block.Span(), receiverType.Name+".new() does not accept a block")
					}
					typ = receiverType
					c.result.NewtypeCalls[n] = NewtypeCall{Operation: "new", Type: receiverType, Representation: target}
				case "value":
					c.checkTypedArguments(n.Span(), receiverType.Name+".value", nil, 0, false, n.Arguments, argumentTypes)
					if n.Block != nil {
						c.error(n.Block.Span(), receiverType.Name+".value() does not accept a block")
					}
					typ = target
					c.result.NewtypeCalls[n] = NewtypeCall{Operation: "value", Type: receiverType, Representation: target}
				}
				break
			}
		}
		if parameters, returned, callable := types.FunctionSignature(calleeType); callable {
			for _, argument := range n.Arguments {
				if argument.Name != "" || argument.Splat != "" {
					c.error(argument.Value.Span(), "fn values accept positional arguments only")
				}
			}
			c.checkTypedArguments(n.Span(), "fn", parameters, len(parameters), false, n.Arguments, argumentTypes)
			if n.Block != nil {
				c.error(n.Block.Span(), "fn values do not accept call blocks")
			}
			typ = returned
			break
		}
		if generic, ok := n.Callee.(*ast.GenericExpression); ok {
			application := c.result.GenericApplications[generic]
			if application.Kind != "function" && application.Kind != "method" {
				c.error(n.Span(), fmt.Sprintf("generic %s %s is not callable", application.Kind, application.Name))
				break
			}
			c.checkCallSignature(n.Span(), application.Name, application.Parameters, application.Variadic, n.Arguments, argumentTypes, nil, application.ParameterResultBridges)
			if application.Source {
				c.result.CallSignatures[n] = append([]callsignature.Parameter(nil), application.Parameters...)
			}
			typ = application.ReturnType
			c.checkNativeCallResultBridge(n, typ, application.CallResultBridge)
			if len(application.TypeArguments) == 1 {
				if binding, imported := c.result.References[generic.Receiver]; imported && binding.Library != nil {
					c.checkCodecApplication(n, binding.Library.Intrinsic, application.TypeArguments[0])
				} else if member, provided := c.external[generic.Receiver]; provided {
					c.checkCodecApplication(n, member.Intrinsic, application.TypeArguments[0])
				}
			}
			if application.Specializer != "" {
				c.recordCallSpecialization(n, generic, application)
			}
			break
		}
		if member, ok := n.Callee.(*ast.MemberExpression); ok {
			if variant, enum := c.enumVariantForMember(member); enum {
				typ = calleeType
				indexes := c.checkEnumConstructor(n, variant, argumentTypes)
				if len(variant.Fields) > 0 {
					c.result.EnumConstructors[n] = variant
					c.result.EnumArgumentIndexes[n] = indexes
				}
				break
			}
		}
		if binding, ok := c.result.References[n.Callee]; ok {
			if member, memberCall := n.Callee.(*ast.MemberExpression); memberCall && binding.Member != nil {
				binding = specializeResolvedEnumMember(c.result.Expressions[member.Receiver], binding)
				binding = specializeResolvedClassMember(c.result.Expressions[member.Receiver], binding)
			}
			if binding.Member != nil && binding.Library == nil && len(binding.Member.TypeParameters) > 0 {
				c.error(n.Callee.Span(), fmt.Sprintf("generic method %s requires explicit type arguments", binding.Name))
				break
			}
			if binding.Member == nil && binding.Export != nil && binding.Export.Kind == resolver.FunctionExport && len(binding.Export.TypeParameters) > 0 {
				c.error(n.Callee.Span(), fmt.Sprintf("generic function %s requires explicit type arguments", binding.Name))
				break
			}
			typ = c.resolvedBindingType(binding)
			library := c.checkImportedArguments(n, binding, argumentTypes, sc)
			if binding.Member != nil {
				c.checkNativeCallResultBridge(n, typ, binding.Member.CallResultBridge)
			} else if binding.Export != nil {
				c.checkNativeCallResultBridge(n, typ, binding.Export.CallResultBridge)
			}
			if binding.Member != nil && binding.Member.EnumOwner != "" {
				copy := binding
				call := EnumCall{EnumName: binding.Member.EnumOwner, Owner: binding.Member.EnumOwner, OwnerIdentity: binding.DeclarationIdentity(), Method: binding.Name, Reference: &copy}
				if member, ok := n.Callee.(*ast.MemberExpression); ok {
					call.Receiver = member.Receiver
				}
				if binding.Member.Generated != "" && binding.Export != nil {
					raw := rawEnumFromExport(binding.Export)
					call.Raw = &raw
				}
				c.result.EnumCalls[n] = call
				if binding.Member.Generated == "from_raw" {
					c.requireRuntimeType(binding.Member.Type)
				}
			}
			if library != nil {
				receiverType := invalidType()
				if member, method := n.Callee.(*ast.MemberExpression); method && library.HasReceiver() {
					receiverType = c.result.Expressions[member.Receiver]
					if library.ReceiverMutable {
						c.requireUnaliasedMutable(member.Receiver, sc, binding.Name+"()")
					}
				}
				typ = inferLibraryReturn(*library, receiverType, argumentTypes)
				if unresolved := unresolvedLibraryTypeParameters(*library, typ); len(unresolved) > 0 {
					c.error(n.Span(), fmt.Sprintf("cannot infer %s for %s()", strings.Join(unresolved, ", "), binding.Name))
					typ = invalidType()
				}
				if (library.Intrinsic == "trb.internal.json.encode" || library.Intrinsic == "trb.web.json" || library.Intrinsic == "trb.platform.typescript.browser.json_body") && len(argumentTypes) >= 1 {
					c.checkCodecApplication(n, library.Intrinsic, argumentTypes[0])
				}
				c.checkLibraryEqualityRequirements(n.Span(), binding.Name, *library)
				c.checkLibraryOrderingRequirements(n.Span(), binding.Name, *library)
				if library.Block != nil {
					member := declaration.Member{
						Name:   binding.Name,
						Return: typ,
						Block: &declaration.Block{
							Parameters:      append([]types.Type(nil), library.Block.Parameters...),
							ControlBoundary: library.Block.ControlBoundary,
						},
					}
					if blockType, checked := c.checkDeclarationBlock(n, member, sc, nil); checked {
						typ = blockType
					}
					libraryBlockChecked = true
				}
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
			switch receiver := member.Receiver.(type) {
			case *ast.Identifier:
				identifier := receiver
				typ = types.FromName(identifier.Name)
				if binding, imported := c.result.References[identifier]; imported && binding.Export != nil {
					if binding.Export.Kind == resolver.RecordExport {
						c.checkImportedRecordArguments(n, binding)
					} else {
						c.checkImportedArguments(n, binding, argumentTypes, sc)
					}
				} else if record := c.records[identifier.Name]; record != nil {
					c.checkLocalRecordArguments(n, record, c.authoredTypeIdentities[identifier.Name])
				} else if info := c.classes[identifier.Name]; info != nil {
					c.checkArguments(n, info.methods["initialize"], argumentTypes)
				}
			case *ast.GenericExpression:
				application := c.result.GenericApplications[receiver]
				typ = application.ReturnType
				substitutions := typeSubstitutions(application.TypeParameters, application.TypeArguments)
				if record := c.records[application.Name]; record != nil {
					fields := make([]resolver.RecordField, len(record.fields))
					for index, field := range record.fields {
						fields[index] = resolver.RecordField{Name: field.Name, Type: substituteType(c.typeFromRef(field.Type), substitutions), HasDefault: field.Default != nil}
					}
					c.checkRecordArguments(n, record.name, fields, application.Declaration)
				} else if info := c.classes[application.Name]; info != nil {
					if initialize := info.methods["initialize"]; initialize != nil {
						signature := c.signatureFromMethod(initialize)
						for index := range signature.parameters {
							signature.parameters[index].Type = substituteType(signature.parameters[index].Type, substitutions)
						}
						names := make([]string, len(initialize.Parameters))
						for index, parameter := range initialize.Parameters {
							names[index] = parameter.Name
						}
						c.checkCallSignature(n.Span(), application.Name+".new", signature.parameters, signature.variadic, n.Arguments, argumentTypes, names, nil)
						c.result.CallSignatures[n] = append([]callsignature.Parameter(nil), signature.parameters...)
					}
				} else if binding, imported := c.result.References[receiver.Receiver]; imported && binding.Export != nil {
					exported := *binding.Export
					if exported.Kind == resolver.RecordExport {
						fields := append([]resolver.RecordField(nil), exported.Fields...)
						for index := range fields {
							fields[index].Type = substituteType(fields[index].Type, substitutions)
							fields[index].ResultBridge = substituteNativeResultBridge(fields[index].ResultBridge, substitutions)
						}
						c.checkRecordArguments(n, exported.Name, fields, binding.DeclarationIdentity())
					} else {
						parameters := append([]callsignature.Parameter(nil), exported.Parameters...)
						for index := range parameters {
							parameters[index].Type = substituteType(parameters[index].Type, substitutions)
						}
						c.checkCallSignature(n.Span(), exported.Name+".new", parameters, exported.Variadic, n.Arguments, argumentTypes, nil, exported.ParameterResultBridges)
						if sourceBinding(binding) {
							c.result.CallSignatures[n] = c.canonicalContractSignature(parameters)
						}
					}
				}
			case *ast.MemberExpression:
				if receiver.Namespace {
					if binding, imported := c.result.References[receiver]; imported && binding.Export != nil {
						typ = c.resolvedBindingType(binding)
						if binding.Export.Kind == resolver.RecordExport {
							c.checkImportedRecordArguments(n, binding)
						} else {
							c.checkImportedArguments(n, binding, argumentTypes, sc)
						}
					} else {
						name, local := c.authoredTypeInScope(expressionTypeName(receiver), sc)
						if record := c.records[name]; local && record != nil {
							typ = types.FromName(name)
							c.checkLocalRecordArguments(n, record, c.result.ExpressionDeclarations[receiver])
						} else if info := c.classes[name]; local && info != nil {
							typ = types.FromName(name)
							c.checkArguments(n, info.methods["initialize"], argumentTypes)
						}
					}
				}
			}
		} else if member, ok := n.Callee.(*ast.MemberExpression); ok {
			receiverType := c.checkExpression(member.Receiver, sc)
			classAccess := c.classMemberAccess(member.Receiver, sc)
			if local, found := c.localMember(receiverType.Name, member.Name, classAccess, map[string]bool{}); found {
				local = c.specializeLocalEnumMember(receiverType, local)
				local = c.specializeLocalClassMember(receiverType, local)
				if local.method != nil && len(local.method.TypeParameters) > 0 {
					c.error(n.Callee.Span(), fmt.Sprintf("generic method %s requires explicit type arguments", member.Name))
					break
				}
				if local.sig != nil {
					typ = local.sig.returnType
					names := []string(nil)
					if local.method != nil {
						names = make([]string, len(local.method.Parameters))
						for index, parameter := range local.method.Parameters {
							names[index] = parameter.Name
						}
					}
					c.checkCallSignature(n.Span(), member.Name, local.sig.parameters, local.sig.variadic, n.Arguments, argumentTypes, names, nil)
					if local.method != nil {
						c.result.CallSignatures[n] = append([]callsignature.Parameter(nil), local.sig.parameters...)
					}
				}
				if local.method != nil {
					if dispatch := c.result.MethodDispatches[local.method]; !dispatch.Empty() {
						c.result.ExpressionDispatches[n.Callee] = dispatch
					}
				}
				if info := c.enums[receiverType.Name]; info != nil && (local.method != nil || info.raw != nil && (member.Name == "raw_value" || member.Name == "from_raw")) {
					ownerIdentity := receiverType.Declaration
					if ownerIdentity.Empty() {
						ownerIdentity = c.authoredTypeIdentities[receiverType.Name]
					}
					call := EnumCall{EnumName: receiverType.Name, Owner: c.authoredEnumOwners[receiverType.Name], OwnerIdentity: ownerIdentity, Method: member.Name, Receiver: member.Receiver}
					if info.raw != nil && (member.Name == "raw_value" || member.Name == "from_raw") {
						call.Raw = info.raw
					}
					c.result.EnumCalls[n] = call
					if member.Name == "from_raw" {
						c.requireRuntimeType(typ)
					}
				}
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
					c.checkArguments(n, method, argumentTypes)
					if dispatch := c.result.MethodDispatches[method]; !dispatch.Empty() {
						c.result.ExpressionDispatches[identifier] = dispatch
					}
					if c.currentEnum != nil {
						c.result.EnumCalls[n] = EnumCall{
							EnumName: c.currentEnum.name, Owner: c.authoredEnumOwners[c.currentEnum.name],
							OwnerIdentity: c.authoredTypeIdentities[c.currentEnum.name], Method: identifier.Name,
						}
					}
				}
			} else if method := c.functions[identifier.Name]; method != nil {
				if len(method.TypeParameters) > 0 {
					c.error(identifier.Span(), fmt.Sprintf("generic function %s requires explicit type arguments", identifier.Name))
				} else {
					typ = c.methodReturnType(method)
					c.checkArguments(n, method, argumentTypes)
					if declaration := c.result.Declarations[method]; !declaration.Empty() {
						c.result.ExpressionDeclarations[identifier] = declaration
					}
				}
			} else if c.mode != "ruby" {
				c.error(identifier.Span(), fmt.Sprintf("function %s is not declared or imported", identifier.Name))
			} else if !c.resolution.NativeSyntax {
				c.error(identifier.Span(), fmt.Sprintf("Ruby function %s requires an explicit platform import", identifier.Name))
			}
		}
		if n.Block != nil && !libraryBlockChecked {
			if _, declared := c.external[n.Callee]; !declared {
				if blockMember, provided := c.declarationFunctionBlock(n, argumentTypes); provided {
					if blockType, checked := c.checkDeclarationBlock(n, blockMember, sc, nil); checked {
						typ = blockType
					}
				} else if c.mode == "ruby" && c.resolution.NativeSyntax {
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
		} else if receiver.Kind == types.String {
			if indexType.Kind != types.Int || indexType.Nullable {
				c.error(n.Index.Span(), fmt.Sprintf("String index must be Integer, got %s", indexType))
			}
			typ = types.FromName("String")
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
			c.error(n.Span(), "Ruby-native syntax requires activate trb/platform/ruby/native or trb/platform/ruby/rails")
		}
	}
	if call, ok := expression.(*ast.CallExpression); ok {
		if construction, found := c.result.RecordConstructions[call]; found && typ.Kind == types.Named {
			typ.Declaration = construction.Declaration
		} else if variant, found := c.result.EnumConstructors[call]; found && typ.Kind == types.Named {
			typ.Declaration = variant.Declaration
		} else if member, constructor := call.Callee.(*ast.MemberExpression); constructor && member.Name == "new" && typ.Kind == types.Named {
			if declaration := c.result.ExpressionDeclarations[member.Receiver]; declaration.Kind.IsType() {
				typ.Declaration = declaration
			}
		}
	}
	typ = c.canonicalType(typ, c.activeTypeParameterSet())
	if member, ok := expression.(*ast.MemberExpression); ok && typ.Nullable {
		if key, _, stable := c.nullableMemberNarrowing(member, sc); stable {
			if fact, narrowed := sc.nullableMember(key); narrowed {
				typ.Nullable = false
				c.result.NullableUnwraps[member] = fact.source
			}
		}
	}
	c.result.Expressions[expression] = typ
	if c.currentConcurrentBlockScope() != nil && c.concurrentBorrowedType(typ) {
		c.borrowedExpressions[expression] = c.computeConcurrentBorrowedExpression(expression, sc)
	}
	return typ
}

func (c *Checker) checkConcurrentNativeCall(callee ast.Expression) {
	if c.currentConcurrentBlockScope() == nil {
		return
	}
	reference := callee
	if generic, ok := callee.(*ast.GenericExpression); ok {
		reference = generic.Receiver
	}
	binding, exists := c.result.References[reference]
	if !exists || binding.Export == nil || binding.Export.Runtime == nil || binding.Export.Runtime.PropagatesExecutionScope {
		return
	}
	c.error(callee.Span(), fmt.Sprintf("native function %s has no concurrency-safe execution-scope contract", binding.Name))
}

func (c *Checker) checkConcurrentDynamicCall(callee ast.Expression, calleeType types.Type) {
	if c.currentConcurrentBlockScope() == nil || calleeType.Kind == types.Invalid {
		return
	}
	reference := callee
	if generic, ok := reference.(*ast.GenericExpression); ok {
		reference = generic.Receiver
	}
	if member, ok := reference.(*ast.MemberExpression); ok && c.concurrentInterfaceMembers[member] {
		c.error(callee.Span(), fmt.Sprintf("concurrent_map cannot call interface method %s without an explicit concurrency-safety contract", member.Name))
		return
	}
	if scalarType(calleeType).Kind == types.Function {
		c.error(callee.Span(), "concurrent_map cannot call a function value without an explicit concurrency-safety contract")
	}
}

func jsxRenderableType(typ, nodeType types.Type) bool {
	if base, literal := types.LiteralBase(typ); literal {
		typ = base
	}
	if typ.Nullable {
		typ.Nullable = false
	}
	switch typ.Kind {
	case types.Any, types.Invalid, types.Never, types.Nil, types.String, types.Int, types.Float, types.Bool:
		return true
	case types.Array:
		return len(typ.Args) == 1 && jsxRenderableType(typ.Args[0], nodeType)
	case types.Named:
		return types.Equivalent(typ, nodeType)
	case types.Union:
		for _, alternative := range typ.Args {
			if !jsxRenderableType(alternative, nodeType) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (c *Checker) checkAssociationControlMember(node *ast.MemberExpression, sc *scope) (types.Type, bool) {
	control := node.Name
	if control != "load" && control != "reload" && control != "loaded?" {
		return types.Type{}, false
	}
	associationNode, ok := node.Receiver.(*ast.MemberExpression)
	if !ok || associationNode.Namespace || associationNode.Safe {
		return types.Type{}, false
	}
	receiverType := c.checkExpression(associationNode.Receiver, sc)
	association, ok := c.declarationMember(receiverType.Name, associationNode.Name, false, map[string]bool{})
	if !ok || !strings.HasPrefix(association.Intrinsic, "trb.orm.association.value.") {
		return types.Type{}, false
	}
	c.external[associationNode] = association
	c.result.ExternalMembers[associationNode] = association
	c.result.Expressions[associationNode] = association.Return

	resultType := association.Return
	if control == "loaded?" {
		resultType = types.FromName("Boolean")
	}
	member := declaration.Member{
		Name: control, Kind: declaration.Method,
		Intrinsic: strings.Replace(association.Intrinsic, ".value.", "."+strings.TrimSuffix(control, "?")+".", 1),
		Return:    resultType, Provider: association.Provider,
	}
	c.external[node] = member
	c.result.ExternalMembers[node] = member
	return resultType, true
}

func (c *Checker) checkResultTry(node *ast.TryExpression, sc *scope) types.Type {
	resultType := c.checkExpression(node.Value, sc)
	c.checkStructuredResultPlacement(node, node.Value, "try")
	success, failure, expanded, ok := c.standardResultParts(resultType)
	if !ok {
		c.error(node.Value.Span(), fmt.Sprintf("try requires the standard Result<T, E>, got %s", resultType))
		return invalidType()
	}
	if c.valueTransformDepth > 0 {
		c.error(node.Span(), "try is not supported inside value-producing collection transformations; use each or handle the Result explicitly")
	}
	if len(c.resultBoundaries) == 0 {
		c.error(node.Span(), "try is only valid inside a function or method that returns Result<T, E>")
		return success
	}
	boundaryIndex := len(c.resultBoundaries) - 1
	boundary := &c.resultBoundaries[boundaryIndex]
	if !boundary.valid {
		returnType := types.FromName("Void")
		if len(c.returns) > 0 {
			returnType = c.returns[len(c.returns)-1]
		}
		c.error(node.Span(), fmt.Sprintf("try requires the enclosing function to return Result<T, E>, got %s", returnType))
		return success
	}
	if !c.typesAssignable(boundary.failure, failure) {
		c.error(node.Span(), fmt.Sprintf("try cannot propagate %s through Result error type %s; use catch to convert the error explicitly", failure, boundary.failure))
		return success
	}
	c.requireRuntimeType(expanded)
	c.requireRuntimeType(boundary.result)
	c.result.ResultTries[node] = ResultTry{
		SuccessType:       success,
		ErrorType:         failure,
		ReturnSuccessType: boundary.success,
		ReturnErrorType:   boundary.failure,
		ReturnType:        boundary.result,
	}
	boundary.tries = append(boundary.tries, node)
	return success
}

func (c *Checker) checkResultCatch(node *ast.CatchExpression, sc *scope) types.Type {
	resultType := c.checkExpression(node.Value, sc)
	c.checkStructuredResultPlacement(node, node.Value, "catch")
	success, failure, expanded, ok := c.standardResultParts(resultType)
	if !ok {
		c.error(node.Value.Span(), fmt.Sprintf("catch requires the standard Result<T, E>, got %s", resultType))
		return invalidType()
	}
	c.requireRuntimeType(expanded)

	handlerScope := &scope{parent: sc, values: map[string]symbol{}}
	if node.Binding.Name != "_" {
		declared := symbol{
			typ:                failure,
			span:               node.Binding.Span(),
			concurrentBorrowed: c.concurrentBorrowedExpression(node.Value, sc) && c.concurrentBorrowedType(failure),
		}
		if tracksUnusedBinding(node.Binding.Name) {
			used := false
			declared.used = &used
			declared.useKind = "catch binding"
		}
		handlerScope.values[node.Binding.Name] = declared
	}

	semantic := ResultCatch{SuccessType: success, ErrorType: failure, ResultType: expanded}
	resultIndex, resultExpression := controlFlowBranchExpression(node.Body)
	if resultExpression == nil {
		c.checkStatements(node.Body, handlerScope)
		c.checkUnusedBindings(handlerScope)
		semantic.HandlerDiverges = terminalControlFlowTransfer(node.Body) != nil || !c.statementsFallThrough(node.Body)
		if !semantic.HandlerDiverges {
			c.error(node.Span(), fmt.Sprintf("catch handler must return %s or transfer control", success))
		}
	} else {
		c.checkStatementSequence(node.Body[:resultIndex], handlerScope)
		handlerType := c.checkExpression(resultExpression, handlerScope)
		handlerType = c.contextualizeCollectionLiteral(resultExpression, success, handlerType)
		c.checkStatementSequence(node.Body[resultIndex+1:], handlerScope)
		c.checkUnusedBindings(handlerScope)
		if handlerType.Kind == types.Never {
			semantic.HandlerDiverges = true
		} else {
			if !c.assignable(resultExpression, success, handlerType) {
				c.error(resultExpression.Span(), fmt.Sprintf("catch handler has type %s, expected %s", handlerType, success))
			}
			semantic.HandlerResult = resultExpression
		}
	}
	c.result.ResultCatches[node] = semantic
	return success
}

func (c *Checker) standardResultParts(typ types.Type) (types.Type, types.Type, types.Type, bool) {
	expanded := c.expandAlias(typ, map[string]bool{})
	if expanded.Nullable || expanded.Name != "Result" || len(expanded.Args) != 2 || !c.standardResultAvailable() {
		return types.Type{}, types.Type{}, expanded, false
	}
	return expanded.Args[0], expanded.Args[1], expanded, true
}

func (c *Checker) isStandardResult(typ types.Type) bool {
	_, _, _, ok := c.standardResultParts(typ)
	return ok
}

func (c *Checker) resultBoundaryFor(returnType types.Type) resultBoundary {
	success, failure, result, ok := c.standardResultParts(returnType)
	return resultBoundary{success: success, failure: failure, result: result, valid: ok}
}

func (c *Checker) checkDirectStructuredResultValue(expression ast.Expression, sc *scope, kind string) types.Type {
	previous := c.directStructuredResultValue
	previousKind := c.directStructuredResultKind
	c.directStructuredResultValue = expression
	c.directStructuredResultKind = kind
	result := c.checkExpression(expression, sc)
	c.directStructuredResultValue = previous
	c.directStructuredResultKind = previousKind
	return result
}

func (c *Checker) checkStructuredResultPlacement(wrapper, operand ast.Expression, keyword string) {
	call, member, ok := c.structuredBlockCall(operand)
	if !ok || !member.Block.Structured {
		return
	}
	semantic, checked := c.result.StructuredBlocks[call]
	if !checked || semantic.ResultBoundary.Kind == "" || semantic.ResultBoundary.Kind == types.Never {
		return
	}
	if c.directStructuredResultValue != wrapper || c.directStructuredResultKind == "assignment" {
		c.error(wrapper.Span(), fmt.Sprintf("%s over a structured block must be the direct value of a variable declaration or return", keyword))
	}
}

func (c *Checker) standardResultAvailable() bool {
	const module = "trb/std/result/index"
	if c.result.Program.ModulePath == module {
		return true
	}
	// A source declaration named Result shadows the compiler-owned enum. Shape
	// compatibility is not enough: try and catch are defined only for the
	// standard failure representation.
	if _, declared := c.declaredTypes["Result"]; declared {
		return false
	}
	if binding, visible := c.resolution.Symbols["Result"]; visible {
		return binding.Import != nil && binding.Import.RuntimePath() == module
	}
	if binding, inferred := c.resolution.InferredType("Result"); inferred {
		return binding.Import != nil && binding.Import.RuntimePath() == module
	}
	if dependency := c.result.RuntimeDependencies["trb/std/result"]; dependency != nil {
		return dependency.ModulePath == module
	}
	_, ok := c.resolution.CompilerOwnedType("Result")
	return ok
}

func (c *Checker) requireRuntimeType(typ types.Type) {
	for _, definition := range stdlib.RuntimeDependenciesForType(typ) {
		if definition != nil && definition.ModulePath != c.result.Program.ModulePath {
			c.result.RuntimeDependencies[definition.Path] = definition
		}
	}
}

func (c *Checker) requireStandardResultRuntimeForSourceType(typ types.Type) {
	if typ.Name == "Result" && c.standardResultAvailable() {
		if definition, _, ok := stdlib.LookupRuntimeExport("Result"); ok && definition.ModulePath != c.result.Program.ModulePath {
			c.result.RuntimeDependencies[definition.Path] = definition
		}
	}
	for _, argument := range typ.Args {
		c.requireStandardResultRuntimeForSourceType(argument)
	}
}

func (c *Checker) declarationFunctionBlock(call *ast.CallExpression, arguments []types.Type) (declaration.Member, bool) {
	binding, referenced := c.result.References[call.Callee]
	if !referenced || binding.Import == nil || c.current == nil {
		return declaration.Member{}, false
	}
	packagePath := strings.TrimSuffix(binding.Import.RuntimePath(), "/index")
	for _, rule := range c.declarations().FunctionBlockRules {
		if rule.Package != packagePath || rule.Function != binding.Name || !c.currentInherits(rule.EnclosingSuperclass) {
			continue
		}
		if rule.TypeArgument < 0 || rule.TypeArgument >= len(arguments) {
			return declaration.Member{}, false
		}
		argument := arguments[rule.TypeArgument]
		if argument.Name == "" || argument.Kind == types.Any || argument.Kind == types.Invalid {
			return declaration.Member{}, false
		}
		blockType := types.FromName(argument.Name + rule.ParameterTypeSuffix)
		if _, declared := c.declarations().Type(blockType.Name); !declared {
			return declaration.Member{}, false
		}
		block := &declaration.Block{Parameters: []types.Type{blockType}, Return: blockType}
		return declaration.Member{
			Name: binding.Name, Kind: declaration.Method, Return: types.FromName("Void"),
			Provider: rule.Package, Block: block,
		}, true
	}
	return declaration.Member{}, false
}

func (c *Checker) currentInherits(name string) bool {
	if c.current == nil {
		return false
	}
	current := c.current.superclass
	seen := map[string]bool{}
	for current != "" && !seen[current] {
		if current == name {
			return true
		}
		seen[current] = true
		if local := c.classes[current]; local != nil {
			current = local.superclass
			continue
		}
		if declared, ok := c.declarations().Type(current); ok {
			current = declared.Superclass
			continue
		}
		break
	}
	return false
}

func expressionEscapingTransform(expression ast.Expression, loopDepth int) ast.Statement {
	if expression == nil {
		return nil
	}
	switch node := expression.(type) {
	case *ast.LambdaExpression:
		// A lambda owns return and starts a separate loop-control context.
		return nil
	case *ast.IfStatement:
		if result := expressionEscapingTransform(node.Condition, loopDepth); result != nil {
			return result
		}
		groups := [][]ast.Statement{node.Then, node.Else}
		for _, branch := range node.ElseIf {
			groups = append(groups, branch.Body)
		}
		for _, group := range groups {
			if result := statementsEscapingTransform(group, loopDepth); result != nil {
				return result
			}
		}
	case *ast.CaseStatement:
		if result := expressionEscapingTransform(node.Value, loopDepth); result != nil {
			return result
		}
		for _, branch := range node.Branches {
			if result := statementsEscapingTransform(branch.Body, loopDepth); result != nil {
				return result
			}
		}
		return statementsEscapingTransform(node.Else, loopDepth)
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			if result := expressionEscapingTransform(part.Expression, loopDepth); result != nil {
				return result
			}
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			if result := expressionEscapingTransform(element, loopDepth); result != nil {
				return result
			}
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			if result := expressionEscapingTransform(entry.Key, loopDepth); result != nil {
				return result
			}
			if result := expressionEscapingTransform(entry.Value, loopDepth); result != nil {
				return result
			}
		}
	case *ast.JSXElement:
		if result := expressionEscapingTransform(node.Component, loopDepth); result != nil {
			return result
		}
		for _, attribute := range node.Attributes {
			if result := expressionEscapingTransform(attribute.Value, loopDepth); result != nil {
				return result
			}
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ast.JSXElement:
				if result := expressionEscapingTransform(item, loopDepth); result != nil {
					return result
				}
			case *ast.JSXExpression:
				if result := expressionEscapingTransform(item.Value, loopDepth); result != nil {
					return result
				}
			}
		}
	case *ast.UnaryExpression:
		return expressionEscapingTransform(node.Operand, loopDepth)
	case *ast.BinaryExpression:
		if result := expressionEscapingTransform(node.Left, loopDepth); result != nil {
			return result
		}
		return expressionEscapingTransform(node.Right, loopDepth)
	case *ast.RangeExpression:
		if result := expressionEscapingTransform(node.Start, loopDepth); result != nil {
			return result
		}
		return expressionEscapingTransform(node.End, loopDepth)
	case *ast.AttemptExpression:
		if result := expressionEscapingTransform(node.Value, loopDepth); result != nil {
			return result
		}
		return statementsEscapingTransform(node.Body, loopDepth)
	case *ast.TryExpression:
		return expressionEscapingTransform(node.Value, loopDepth)
	case *ast.CatchExpression:
		if result := expressionEscapingTransform(node.Value, loopDepth); result != nil {
			return result
		}
		return statementsEscapingTransform(node.Body, loopDepth)
	case *ast.IterationExpression:
		if result := expressionEscapingTransform(node.Source, loopDepth); result != nil {
			return result
		}
		if result := expressionEscapingTransform(node.SliceSize, loopDepth); result != nil {
			return result
		}
		if result := expressionEscapingTransform(node.Initial, loopDepth); result != nil {
			return result
		}
		if result := expressionEscapingTransform(node.Limit, loopDepth); result != nil {
			return result
		}
		if node.Block != nil {
			// The nested iteration owns break/next, while return still escapes it.
			return statementsEscapingTransform(node.Block.Body, loopDepth+1)
		}
	case *ast.CallExpression:
		if result := expressionEscapingTransform(node.Callee, loopDepth); result != nil {
			return result
		}
		for _, argument := range node.Arguments {
			if result := expressionEscapingTransform(argument.Value, loopDepth); result != nil {
				return result
			}
		}
		if node.Block != nil {
			return statementsEscapingTransform(node.Block.Body, loopDepth+1)
		}
	case *ast.GenericExpression:
		return expressionEscapingTransform(node.Receiver, loopDepth)
	case *ast.MemberExpression:
		return expressionEscapingTransform(node.Receiver, loopDepth)
	case *ast.IndexExpression:
		if result := expressionEscapingTransform(node.Receiver, loopDepth); result != nil {
			return result
		}
		return expressionEscapingTransform(node.Index, loopDepth)
	case *ast.BlockExpression:
		return statementsEscapingTransform(node.Body, loopDepth)
	}
	return nil
}

func statementsEscapingTransform(statements []ast.Statement, loopDepth int) ast.Statement {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ReturnStatement:
			return node
		case *ast.BreakStatement, *ast.NextStatement:
			if loopDepth == 0 {
				return node
			}
		case *ast.VariableStatement:
			if result := expressionEscapingTransform(node.Value, loopDepth); result != nil {
				return result
			}
		case *ast.AssignmentStatement:
			if result := expressionEscapingTransform(node.Target, loopDepth); result != nil {
				return result
			}
			if result := expressionEscapingTransform(node.Value, loopDepth); result != nil {
				return result
			}
		case *ast.ExpressionStatement:
			if result := expressionEscapingTransform(node.Expression, loopDepth); result != nil {
				return result
			}
		case *ast.IfStatement:
			if result := expressionEscapingTransform(node, loopDepth); result != nil {
				return result
			}
		case *ast.CaseStatement:
			if result := expressionEscapingTransform(node, loopDepth); result != nil {
				return result
			}
		case *ast.WhileStatement:
			if result := expressionEscapingTransform(node.Condition, loopDepth); result != nil {
				return result
			}
			if result := statementsEscapingTransform(node.Body, loopDepth+1); result != nil {
				return result
			}
		case *ast.NativeBlock:
			if result := statementsEscapingTransform(node.Body, loopDepth); result != nil {
				return result
			}
		}
	}
	return nil
}

func controlTransferKeyword(statement ast.Statement) string {
	switch statement.(type) {
	case *ast.BreakStatement:
		return "break"
	case *ast.NextStatement:
		return "next"
	default:
		return "return"
	}
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

func (c *Checker) checkLocalRecordArguments(call *ast.CallExpression, record *recordInfo, declaration identity.Declaration) {
	fields := make([]resolver.RecordField, len(record.fields))
	for index, field := range record.fields {
		fields[index] = resolver.RecordField{Name: field.Name, Type: c.typeFromRef(field.Type), HasDefault: field.Default != nil}
	}
	c.checkRecordArguments(call, record.name, fields, declaration)
}

func (c *Checker) checkImportedRecordArguments(call *ast.CallExpression, binding resolver.Binding) {
	c.checkRecordArguments(call, binding.Export.Name, binding.Export.Fields, binding.DeclarationIdentity())
}

func (c *Checker) checkRecordArguments(call *ast.CallExpression, name string, fields []resolver.RecordField, declaration identity.Declaration) {
	var target ast.Expression
	if member, ok := call.Callee.(*ast.MemberExpression); ok && member.Name == "new" {
		target = member.Receiver
	}
	resolvedFields := append([]resolver.RecordField(nil), fields...)
	for index := range resolvedFields {
		resolvedFields[index].Type = c.canonicalContractType(resolvedFields[index].Type, c.activeTypeParameterSet())
	}
	c.result.RecordConstructions[call] = RecordConstruction{Fields: resolvedFields, Target: target, Declaration: declaration}
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
		if field.ResultBridge.Kind != "" {
			c.checkNativeResultBridge(argument.Value, field.Type, actual, field.ResultBridge)
		} else if !c.assignable(argument.Value, field.Type, actual) {
			c.error(argument.Value.Span(), fmt.Sprintf("record field %s has type %s, expected %s", field.Name, actual, field.Type))
		}
	}
	for _, field := range fields {
		if !used[field.Name] && !field.Optional && !field.HasDefault {
			c.error(call.Span(), fmt.Sprintf("%s.new() is missing record field %s", name, field.Name))
		}
	}
}

func (c *Checker) checkImportedArguments(call *ast.CallExpression, binding resolver.Binding, actual []types.Type, sc *scope) *stdlib.Symbol {
	span := call.Span()
	arguments := call.Arguments
	var parameters []types.Type
	var parameterIndexes []int
	required := 0
	variadic := false
	signatureChecked := false
	name := binding.Name
	var library *stdlib.Symbol
	if binding.Library != nil {
		inferenceArguments := make([]types.Type, len(actual))
		for index, argument := range actual {
			inferenceArguments[index] = c.expandAlias(argument, map[string]bool{})
		}
		specialized := stdlib.Instantiate(*binding.Library, inferenceArguments)
		library = &specialized
		for index := range specialized.Parameters {
			parameter := &specialized.Parameters[index]
			parameter.Type = c.canonicalContractType(parameter.Type, c.activeTypeParameterSet())
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
						c.error(argument.Value.Span(), fmt.Sprintf("%s() has no named argument %s", name, argument.Name))
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
	} else if binding.Member != nil {
		signature := binding.Member.Parameters
		parameters = callsignature.Types(signature)
		variadic = binding.Member.Variadic
		bridges := []resolver.NativeResultBridge(nil)
		if binding.Export != nil {
			bridges = binding.Export.ParameterResultBridges
		}
		parameterIndexes = c.checkCallSignature(span, name, signature, variadic, arguments, actual, nil, bridges)
		signatureChecked = true
		if sourceBinding(binding) {
			c.result.CallSignatures[call] = c.canonicalContractSignature(signature)
		}
	} else if binding.Export != nil {
		signature := binding.Export.Parameters
		parameters = callsignature.Types(signature)
		variadic = binding.Export.Variadic
		parameterIndexes = c.checkCallSignature(span, name, signature, variadic, arguments, actual, nil, binding.Export.ParameterResultBridges)
		signatureChecked = true
		if sourceBinding(binding) {
			c.result.CallSignatures[call] = c.canonicalContractSignature(signature)
		}
	}
	if signatureChecked {
		return library
	}
	if !signatureChecked && len(parameterIndexes) == 0 && (len(arguments) < required || (!variadic && len(arguments) > len(parameters))) {
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
		resultBridge := resolver.NativeResultBridge{}
		if binding.Export != nil && parameterIndex < len(binding.Export.ParameterResultBridges) {
			resultBridge = binding.Export.ParameterResultBridges[parameterIndex]
		}
		assignable := false
		bridgeChecked := resultBridge.Kind != ""
		if bridgeChecked {
			assignable = c.checkNativeResultBridge(arguments[i].Value, expected, actualType, resultBridge)
		} else {
			assignable = c.assignable(arguments[i].Value, expected, actualType)
		}
		if library != nil {
			assignable = libraryAssignable(
				c.expandAlias(expected, map[string]bool{}),
				c.expandAlias(actualType, map[string]bool{}),
			)
		}
		if library != nil && parameterIndex < len(library.Parameters) && library.Parameters[parameterIndex].Exact {
			assignable = types.Equivalent(
				c.expandAlias(expected, map[string]bool{}),
				c.expandAlias(actualType, map[string]bool{}),
			)
		}
		if !assignable && !bridgeChecked {
			c.error(arguments[i].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", i+1, name, actualType, expected))
		} else if library != nil {
			c.recordAssignableConversion(arguments[i].Value, expected, actualType)
		}
		if library != nil && parameterIndex < len(library.Parameters) && library.Parameters[parameterIndex].Mutable {
			c.requireUnaliasedMutable(arguments[i].Value, sc, name+"()")
		}
	}
	return library
}

func (c *Checker) checkNativeResultBridge(expression ast.Expression, expected, actual types.Type, bridge resolver.NativeResultBridge) bool {
	expectedParameters, _, expectedOK := types.FunctionSignature(expected)
	actualParameters, actualResult, actualOK := types.FunctionSignature(actual)
	if !expectedOK || !actualOK || len(expectedParameters) != len(actualParameters) {
		c.error(expression.Span(), fmt.Sprintf("native result bridge requires callback type %s, got %s", expected, actual))
		return false
	}
	for index := range expectedParameters {
		if !c.typesEquivalent(expectedParameters[index], actualParameters[index]) {
			c.error(expression.Span(), fmt.Sprintf("native result bridge callback parameter %d has type %s, expected %s", index+1, actualParameters[index], expectedParameters[index]))
			return false
		}
	}
	actualSuccess, actualError, expandedResult, standard := c.standardResultParts(actualResult)
	if !standard {
		c.error(expression.Span(), fmt.Sprintf("native result bridge requires a callback returning the standard Result<T, E>, got %s", actualResult))
		return false
	}
	_, nativeSuccess, nativeOK := types.FunctionSignature(bridge.Type)
	if !nativeOK {
		c.error(expression.Span(), "native result bridge has an invalid provider callback type")
		return false
	}
	expectedSuccess := nativeSuccess
	if expectedSuccess.Kind == types.Void {
		expectedSuccess = types.FromName("Unit")
	}
	if !c.typesEquivalent(expectedSuccess, actualSuccess) {
		c.error(expression.Span(), fmt.Sprintf("native result bridge callback success type is %s, expected %s", actualSuccess, expectedSuccess))
		return false
	}
	if !c.typesAssignable(bridge.Error, actualError) {
		c.error(expression.Span(), fmt.Sprintf("native result bridge callback error type %s is not assignable to %s", actualError, bridge.Error))
		return false
	}
	c.requireRuntimeType(expandedResult)
	c.result.NativeResultBridges[expression] = NativeResultBridge{Kind: bridge.Kind, Type: bridge.Type}
	return true
}

func (c *Checker) checkNativeCallResultBridge(call *ast.CallExpression, returned types.Type, bridge resolver.NativeCallResultBridge) {
	if bridge.Kind == "" {
		return
	}
	if bridge.Kind != "promise_rejection_to_result" {
		c.error(call.Span(), fmt.Sprintf("unsupported native call result bridge %s", bridge.Kind))
		return
	}
	success, failure, expanded, standard := c.standardResultParts(returned)
	if !standard {
		c.error(call.Span(), fmt.Sprintf("native call result bridge requires the standard Result<T, E>, got %s", returned))
		return
	}
	if success.Kind == types.Void {
		c.error(call.Span(), "native Promise rejection bridge represents Promise<void> with Result<Unit, E>, not Result<Void, E>")
		return
	}
	if !c.typesEquivalent(failure, bridge.Error) {
		c.error(call.Span(), fmt.Sprintf("native call result bridge error type %s does not match Result error %s", bridge.Error, failure))
		return
	}
	if failure.Nullable || failure.Kind != types.String {
		c.error(call.Span(), fmt.Sprintf("native Promise rejection bridge currently requires String as its Result error type, got %s", failure))
		return
	}
	c.requireRuntimeType(expanded)
	c.result.NativeCallResultBridges[call] = NativeCallResultBridge{Kind: bridge.Kind, Success: success, Error: failure, ResultType: expanded}
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

func (c *Checker) checkLibraryOrderingRequirements(span token.Span, name string, symbol stdlib.Symbol) {
	for _, typ := range symbol.OrderingTypes {
		if typ.Kind == types.Invalid || len(unresolvedLibraryTypeParameters(symbol, typ)) > 0 {
			continue
		}
		if !portableOrderType(typ) {
			c.error(span, fmt.Sprintf("portable natural order is not defined for %s, required by %s()", typ, name))
		}
	}
}

func portableOrderType(typ types.Type) bool {
	typ = scalarType(typ)
	return !typ.Nullable && (typ.Kind == types.Int || typ.Kind == types.Float || typ.Kind == types.String)
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

func (c *Checker) requireUnaliasedMutable(expression ast.Expression, sc *scope, action string) {
	if c.currentConcurrentBlockScope() != nil {
		if name, typ, borrowed := concurrentBorrowedRoot(expression, sc); borrowed {
			c.error(expression.Span(), fmt.Sprintf("concurrent_map cannot mutate borrowed binding %s because %s is not uniquely owned", name, typ))
			return
		}
	}
	c.requireMutable(expression, sc, action)
}

func concurrentBorrowedRoot(expression ast.Expression, sc *scope) (string, types.Type, bool) {
	switch node := expression.(type) {
	case *ast.Identifier:
		value, exists := sc.lookup(node.Name)
		return node.Name, value.typ, exists && value.concurrentBorrowed
	case *ast.MemberExpression:
		return concurrentBorrowedRoot(node.Receiver, sc)
	case *ast.IndexExpression:
		return concurrentBorrowedRoot(node.Receiver, sc)
	case *ast.GenericExpression:
		return concurrentBorrowedRoot(node.Receiver, sc)
	default:
		return "", types.Type{}, false
	}
}

func markConcurrentBorrowed(sc *scope, name string, binding symbol) {
	for current := sc; current != nil; current = current.parent {
		value, exists := current.values[name]
		if !exists || !sameConcurrentBinding(value, binding) {
			continue
		}
		value.concurrentBorrowed = true
		current.values[name] = value
	}
}

func sameConcurrentBinding(left, right symbol) bool {
	if left.variable != nil || right.variable != nil {
		return left.variable != nil && left.variable == right.variable
	}
	return left.span == right.span
}

func (c *Checker) concurrentBorrowedType(typ types.Type) bool {
	typ = c.expandAlias(typ, map[string]bool{})
	if typ.Kind == types.Union {
		for _, alternative := range typ.Args {
			if c.concurrentBorrowedType(alternative) {
				return true
			}
		}
		return false
	}
	return isReferenceType(typ) && !c.concurrencySafeType(typ, map[string]bool{})
}

func (c *Checker) concurrentBorrowedExpression(expression ast.Expression, sc *scope) bool {
	if expression == nil || c.currentConcurrentBlockScope() == nil {
		return false
	}
	if typ := c.result.Expressions[expression]; !c.concurrentBorrowedType(typ) {
		return false
	}
	if borrowed, known := c.borrowedExpressions[expression]; known {
		return borrowed
	}
	borrowed := c.computeConcurrentBorrowedExpression(expression, sc)
	c.borrowedExpressions[expression] = borrowed
	return borrowed
}

func (c *Checker) computeConcurrentBorrowedExpression(expression ast.Expression, sc *scope) bool {
	switch node := expression.(type) {
	case *ast.Identifier:
		value, exists := sc.lookup(node.Name)
		return exists && value.concurrentBorrowed
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			if c.concurrentBorrowedExpression(element, sc) {
				return true
			}
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			if c.concurrentBorrowedExpression(entry.Key, sc) || c.concurrentBorrowedExpression(entry.Value, sc) {
				return true
			}
		}
	case *ast.MemberExpression:
		return c.concurrentBorrowedExpression(node.Receiver, sc)
	case *ast.IndexExpression:
		return c.concurrentBorrowedExpression(node.Receiver, sc)
	case *ast.GenericExpression:
		return c.concurrentBorrowedExpression(node.Receiver, sc)
	case *ast.CallExpression:
		if member, ok := node.Callee.(*ast.MemberExpression); ok && c.concurrentBorrowedExpression(member.Receiver, sc) {
			return true
		}
		for _, argument := range node.Arguments {
			if c.concurrentBorrowedExpression(argument.Value, sc) {
				return true
			}
		}
	case *ast.IterationExpression:
		if c.concurrentBorrowedExpression(node.Source, sc) || c.concurrentBorrowedExpression(node.Initial, sc) {
			return true
		}
		if node.Block != nil {
			_, result := controlFlowBranchExpression(node.Block.Body)
			return c.concurrentBorrowedExpression(result, sc)
		}
	case *ast.IfStatement:
		branches := [][]ast.Statement{node.Then, node.Else}
		for _, branch := range node.ElseIf {
			branches = append(branches, branch.Body)
		}
		for _, branch := range branches {
			_, result := controlFlowBranchExpression(branch)
			if c.concurrentBorrowedExpression(result, sc) {
				return true
			}
		}
	case *ast.CaseStatement:
		for _, branch := range node.Branches {
			_, result := controlFlowBranchExpression(branch.Body)
			if c.concurrentBorrowedExpression(result, sc) {
				return true
			}
		}
		_, result := controlFlowBranchExpression(node.Else)
		return c.concurrentBorrowedExpression(result, sc)
	case *ast.TryExpression:
		return c.concurrentBorrowedExpression(node.Value, sc)
	case *ast.CatchExpression:
		if c.concurrentBorrowedExpression(node.Value, sc) {
			return true
		}
		_, result := controlFlowBranchExpression(node.Body)
		return c.concurrentBorrowedExpression(result, sc)
	case *ast.AttemptExpression:
		if c.concurrentBorrowedExpression(node.Value, sc) {
			return true
		}
		_, result := controlFlowBranchExpression(node.Body)
		return c.concurrentBorrowedExpression(result, sc)
	case *ast.UnaryExpression:
		return c.concurrentBorrowedExpression(node.Operand, sc)
	case *ast.BinaryExpression:
		return c.concurrentBorrowedExpression(node.Left, sc) || c.concurrentBorrowedExpression(node.Right, sc)
	default:
		// Future reference-producing syntax stays borrowed until it gains an
		// explicit provenance rule here.
		return true
	}
	return false
}

func isReferenceType(typ types.Type) bool {
	switch typ.Kind {
	case types.Array, types.Hash, types.StringBuilder, types.Named:
		return true
	default:
		return false
	}
}

func (c *Checker) currentConcurrentBlockScope() *scope {
	if len(c.concurrentBlockScopes) == 0 {
		return nil
	}
	return c.concurrentBlockScopes[len(c.concurrentBlockScopes)-1]
}

func (c *Checker) currentMethodScope() *scope {
	if len(c.currentMethodScopes) == 0 {
		return nil
	}
	return c.currentMethodScopes[len(c.currentMethodScopes)-1]
}

func (c *Checker) concurrentConstructorInitializesField(root *scope, target ast.Expression, name string) bool {
	identifier, direct := target.(*ast.Identifier)
	// Only the constructor's own direct field target is fresh. A nested target
	// such as @items[0] may still refer to storage shared by Array elements.
	return direct && identifier.Name == name && strings.HasPrefix(name, "@") &&
		root == c.currentMethodScope() && c.currentMethod != nil && c.concurrentConstructors[c.currentMethod]
}

func (c *Checker) concurrencyInterfaceType(typ types.Type) bool {
	typ = c.expandAlias(typ, map[string]bool{})
	if typ.Kind != types.Named {
		return false
	}
	if c.interfaces[typ.Name] != nil {
		return true
	}
	if binding, exists := c.resolution.ImportedType(typ.Name); exists && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport {
		return true
	}
	if binding, exists := c.resolution.InferredType(typ.Name); exists && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport {
		return true
	}
	exported, exists := c.resolution.CompilerOwnedType(typ.Name)
	return exists && exported.Kind == resolver.InterfaceExport
}

func scopeWithin(candidate, root *scope) bool {
	for current := candidate; current != nil; current = current.parent {
		if current == root {
			return true
		}
	}
	return false
}

func concurrentAssignmentRoot(expression ast.Expression, sc, root *scope) (string, bool) {
	var identifier *ast.Identifier
	switch node := expression.(type) {
	case *ast.Identifier:
		identifier = node
	case *ast.MemberExpression:
		return concurrentAssignmentRoot(node.Receiver, sc, root)
	case *ast.IndexExpression:
		return concurrentAssignmentRoot(node.Receiver, sc, root)
	default:
		return "", false
	}
	_, owner, exists := sc.lookupOwner(identifier.Name)
	return identifier.Name, exists && !scopeWithin(owner, root)
}

// concurrencySafeType is intentionally conservative. A concurrent block may
// read scalar/value data from its lexical parent, while outer assignment is
// rejected separately. Containers, functions, Any, and class/interface
// instances require an explicit concurrency-safety contract that TypeRB does
// not expose yet.
func (c *Checker) concurrencySafeType(typ types.Type, visiting map[string]bool) bool {
	typ = c.expandAlias(typ, map[string]bool{})
	if typ.Nullable {
		typ.Nullable = false
	}
	if typ.Kind == types.Union {
		for _, alternative := range typ.Args {
			if !c.concurrencySafeType(alternative, visiting) {
				return false
			}
		}
		return true
	}
	switch typ.Kind {
	case types.Nil, types.Bool, types.Int, types.IntLiteral, types.Float, types.String, types.StringLiteral, types.Bytes, types.Void:
		return true
	case types.Range:
		return len(typ.Args) == 1 && c.concurrencySafeType(typ.Args[0], visiting)
	case types.Named:
		if target, binding, newtype := c.newtypeDefinition(typ.Name); newtype {
			if visiting[typ.Name] {
				return false
			}
			visiting[typ.Name] = true
			parameters := c.declaredTypes[typ.Name].typeParameters
			if binding != nil && binding.Export != nil {
				parameters = binding.Export.TypeParameters
			}
			target = substituteType(target, typeSubstitutions(parameters, typ.Args))
			safe := c.concurrencySafeType(target, visiting)
			delete(visiting, typ.Name)
			return safe
		}
		if visiting[typ.Name] {
			return false
		}
		if fields, _, binding, record := c.codecRecord(typ.Name); record {
			visiting[typ.Name] = true
			parameters := []string{}
			if local := c.records[typ.Name]; local != nil {
				parameters = local.typeParameters
			} else if binding != nil && binding.Export != nil {
				parameters = binding.Export.TypeParameters
			}
			substitutions := typeSubstitutions(parameters, typ.Args)
			for _, field := range fields {
				if !c.concurrencySafeType(substituteType(field.Type, substitutions), visiting) {
					delete(visiting, typ.Name)
					return false
				}
			}
			delete(visiting, typ.Name)
			return true
		}
		if variants, enum := c.enumVariants(typ); enum {
			visiting[typ.Name] = true
			for _, variant := range variants {
				for _, field := range variant.Fields {
					if !c.concurrencySafeType(field.Type, visiting) {
						delete(visiting, typ.Name)
						return false
					}
				}
			}
			delete(visiting, typ.Name)
			return true
		}
	}
	return false
}

func portableHashKey(typ types.Type) bool {
	typ = scalarType(typ)
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
	case types.Nil, types.Bool, types.Int, types.Float, types.String, types.Bytes, types.StringBuilder, types.Array, types.Range, types.Hash:
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
	positionalNames := map[string]bool{}
	for _, parameter := range member.Parameters {
		if parameter.Keyword {
			byName[parameter.Name] = parameter
		} else {
			positionalNames[parameter.Name] = true
		}
	}
	used := map[string]bool{}
	usedNamed := map[string]bool{}
	position := 0
	for index, argument := range arguments {
		if argument.Name != "" {
			if usedNamed[argument.Name] {
				c.error(argument.Value.Span(), fmt.Sprintf("%s() receives argument %s more than once", member.Name, argument.Name))
				continue
			}
			usedNamed[argument.Name] = true
		}
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
				if positionalNames[argument.Name] {
					c.error(argument.Value.Span(), fmt.Sprintf("%s is a positional-only parameter of %s()", argument.Name, member.Name))
				} else {
					c.error(argument.Value.Span(), fmt.Sprintf("%s() has no named argument %s", member.Name, argument.Name))
				}
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
		authoredActual := actual[index]
		actualForBoundary := authoredActual
		if parameter.RepresentationBoundary {
			actualForBoundary = c.expandRepresentation(actualForBoundary, map[string]bool{})
		}
		bindDeclarationType(parameter.Type, actualForBoundary, typeParameters, bindings)
		expected := instantiateDeclarationType(parameter.Type, bindings)
		actualForBoundary = c.contextualizeCollectionLiteral(argument.Value, expected, actualForBoundary)
		actual[index] = actualForBoundary
		if !c.assignable(argument.Value, expected, actualForBoundary) {
			c.error(argument.Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", index+1, member.Name, authoredActual, expected))
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
	boundaryError := types.Type{}
	if member.Block != nil {
		boundaryError = instantiateDeclarationType(member.Block.ResultBoundary, bindings)
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
	if member.Block.Structured && (member.Block.Return.Name != "" || boundaryError.Kind != "" && boundaryError.Kind != types.Never) {
		direct := c.directStructuredResultValue == call
		wrapped := false
		switch wrapper := c.directStructuredResultValue.(type) {
		case *ast.TryExpression:
			wrapped = wrapper.Value == call
		case *ast.CatchExpression:
			wrapped = wrapper.Value == call
		}
		if !direct && !wrapped {
			c.error(call.Span(), fmt.Sprintf("structured block %s() must be the direct value of a variable declaration, assignment, or return; a try or catch wrapper must also be direct", member.Name))
		}
	}
	if len(call.Block.Parameters) != len(member.Block.Parameters) {
		c.error(call.Block.Span(), fmt.Sprintf("%s block expects %d parameter(s), got %d", member.Name, len(member.Block.Parameters), len(call.Block.Parameters)))
	}
	blockScope := &scope{parent: sc, values: map[string]symbol{}}
	if c.inferenceOnly && !member.Block.Structured {
		c.callbackScopes = append(c.callbackScopes, blockScope)
		defer func() {
			c.callbackScopes = c.callbackScopes[:len(c.callbackScopes)-1]
		}()
	}
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
		callType := instantiateDeclarationType(member.Return, bindings)
		declaresResultBoundary := boundaryError.Kind != "" && boundaryError.Kind != types.Never
		if !declaresResultBoundary {
			previousLoopDepth := c.loopDepth
			if member.Block.ControlBoundary {
				c.loopDepth = 0
				c.controlBoundaries = append(c.controlBoundaries, member.Name)
			} else {
				c.loopDepth++
			}
			c.checkStatements(call.Block.Body, blockScope)
			if member.Block.ControlBoundary {
				c.controlBoundaries = c.controlBoundaries[:len(c.controlBoundaries)-1]
			}
			c.loopDepth = previousLoopDepth
			c.result.Expressions[call.Block] = types.Type{Kind: types.Void, Name: "Void"}
			return callType, true
		}

		boundarySuccess, boundaryFailure, boundaryResult, hasResultBoundary := c.standardResultParts(callType)
		if !hasResultBoundary {
			c.error(call.Span(), fmt.Sprintf("structured block %s() declares a Result boundary but returns %s", member.Name, callType))
		} else if !c.typesAssignable(boundaryError, boundaryFailure) || !c.typesAssignable(boundaryFailure, boundaryError) {
			c.error(call.Span(), fmt.Sprintf("structured block %s() Result error type %s does not match its boundary error type %s", member.Name, boundaryFailure, boundaryError))
			hasResultBoundary = false
		}
		boundary := resultBoundary{}
		if hasResultBoundary {
			boundary = resultBoundary{
				success: boundarySuccess,
				failure: boundaryError,
				result:  boundaryResult,
				valid:   true,
			}
		}
		c.resultBoundaries = append(c.resultBoundaries, boundary)
		boundaryIndex := len(c.resultBoundaries) - 1
		previousLoopDepth := c.loopDepth
		previousResultBoundaryBlockDepth := c.resultBoundaryBlockDepth
		if member.Block.ControlBoundary {
			c.loopDepth = 0
			c.controlBoundaries = append(c.controlBoundaries, member.Name)
		} else {
			c.loopDepth++
		}
		if boundary.valid {
			c.resultBoundaryBlockDepth++
		}
		c.checkStatements(call.Block.Body, blockScope)
		if member.Block.ControlBoundary {
			c.controlBoundaries = c.controlBoundaries[:len(c.controlBoundaries)-1]
		}
		c.loopDepth = previousLoopDepth
		c.resultBoundaryBlockDepth = previousResultBoundaryBlockDepth
		if boundary.valid {
			for _, resultTry := range c.resultBoundaries[boundaryIndex].tries {
				semantic := c.result.ResultTries[resultTry]
				semantic.ReturnSuccessType = boundary.success
				semantic.ReturnErrorType = boundary.failure
				semantic.ReturnType = boundary.result
				c.result.ResultTries[resultTry] = semantic
			}
		}
		c.resultBoundaries = c.resultBoundaries[:boundaryIndex]
		c.result.Expressions[call.Block] = types.Type{Kind: types.Void, Name: "Void"}
		c.result.StructuredBlocks[call] = StructuredBlock{
			Parameters:     instantiateDeclarationTypes(member.Block.Parameters, bindings),
			Return:         boundarySuccess,
			ResultBoundary: boundaryError,
			ResultType:     boundaryResult,
		}
		return callType, true
	}

	resultIndex, resultExpression := controlFlowBranchExpression(call.Block.Body)
	if resultExpression == nil {
		c.checkStatements(call.Block.Body, blockScope)
		c.error(call.Block.Span(), fmt.Sprintf("%s block must end with a result expression", member.Name))
		return invalidType(), true
	}

	blockReturn := instantiateDeclarationType(member.Block.Return, bindings)
	c.returns = append(c.returns, blockReturn)
	boundary := resultBoundary{}
	if boundaryError.Kind != "" && boundaryError.Kind != types.Never {
		boundary = resultBoundary{
			success: blockReturn,
			failure: boundaryError,
			result:  types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{blockReturn, boundaryError}},
			valid:   true,
		}
	}
	c.resultBoundaries = append(c.resultBoundaries, boundary)
	boundaryIndex := len(c.resultBoundaries) - 1
	previousLoopDepth := c.loopDepth
	previousResultBoundaryBlockDepth := c.resultBoundaryBlockDepth
	c.loopDepth = 0
	if boundary.valid {
		c.resultBoundaryBlockDepth++
	}
	c.checkStatementSequence(call.Block.Body[:resultIndex], blockScope)
	actual := c.checkExpression(resultExpression, blockScope)
	c.checkStatementSequence(call.Block.Body[resultIndex+1:], blockScope)
	c.checkUnusedBindings(blockScope)
	c.loopDepth = previousLoopDepth
	c.resultBoundaryBlockDepth = previousResultBoundaryBlockDepth
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
	boundaryError = instantiateDeclarationType(member.Block.ResultBoundary, bindings)
	resultBoundaryType := types.Type{}
	if boundaryError.Kind != "" && boundaryError.Kind != types.Never {
		resultBoundaryType = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{blockReturn, boundaryError}}
		for _, try := range c.resultBoundaries[boundaryIndex].tries {
			semantic := c.result.ResultTries[try]
			semantic.ReturnSuccessType = blockReturn
			semantic.ReturnErrorType = boundaryError
			semantic.ReturnType = resultBoundaryType
			c.result.ResultTries[try] = semantic
		}
	}
	c.resultBoundaries = c.resultBoundaries[:boundaryIndex]
	if !c.assignable(resultExpression, blockReturn, actual) {
		c.error(resultExpression.Span(), fmt.Sprintf("%s block result has type %s, expected %s", member.Name, actual, blockReturn))
	}
	c.result.Expressions[call.Block] = blockReturn
	c.result.StructuredBlocks[call] = StructuredBlock{
		Parameters:     instantiateDeclarationTypes(member.Block.Parameters, bindings),
		Return:         blockReturn,
		Result:         resultExpression,
		ResultBoundary: boundaryError,
		ResultType:     resultBoundaryType,
	}
	return instantiateDeclarationType(member.Return, bindings), true
}

func (c *Checker) currentControlBoundary() string {
	if len(c.controlBoundaries) == 0 {
		return ""
	}
	return c.controlBoundaries[len(c.controlBoundaries)-1]
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
	if c.inferenceOnly {
		c.callbackScopes = append(c.callbackScopes, blockScope)
		defer func() {
			c.callbackScopes = c.callbackScopes[:len(c.callbackScopes)-1]
		}()
	}
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

const portableIntegerLiteralRangeMessage = "Integer literal is outside the portable range -9007199254740991..9007199254740991"
const portableFloatLiteralRangeMessage = "Float literal is outside the finite binary64 range"

func (c *Checker) rawEnumLiteral(expression ast.Expression, sc *scope) (RawEnumValue, string, bool) {
	typ := c.checkExpression(expression, sc)
	switch value := expression.(type) {
	case *ast.Literal:
		switch value.Kind {
		case ast.StringLiteral:
			decoded, err := strconv.Unquote(value.Raw)
			if err != nil {
				c.error(value.Span(), "raw enum String value must be a valid string literal")
				return RawEnumValue{}, "", false
			}
			return RawEnumValue{Raw: value.Raw, Type: typ}, "string:" + decoded, true
		case ast.IntegerLiteral:
			parsed, err := strconv.ParseInt(strings.ReplaceAll(value.Raw, "_", ""), 10, 64)
			if err != nil {
				c.error(value.Span(), "raw enum Integer value is outside the portable range")
				return RawEnumValue{}, "", false
			}
			return RawEnumValue{Raw: value.Raw, Type: typ}, fmt.Sprintf("integer:%d", parsed), true
		}
	case *ast.UnaryExpression:
		literal, ok := value.Operand.(*ast.Literal)
		if value.Operator == "-" && ok && literal.Kind == ast.IntegerLiteral {
			parsed, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 64)
			if err == nil && parsed >= 0 {
				raw := "-" + literal.Raw
				return RawEnumValue{Raw: raw, Type: typ}, fmt.Sprintf("integer:%d", -parsed), true
			}
		}
	}
	c.error(expression.Span(), "raw enum values must be explicit String or Integer literals")
	return RawEnumValue{}, "", false
}

func rawEnumFromExport(exported *resolver.Export) RawEnum {
	result := RawEnum{Type: exported.EnumRawType, Values: map[string]RawEnumValue{}}
	for _, variant := range exported.EnumVariants {
		if variant.RawValue != "" {
			result.Values[variant.Name] = RawEnumValue{Raw: variant.RawValue, Type: exported.EnumRawType}
		}
	}
	return result
}

func rawEnumShape(enum *ast.EnumStatement) (RawEnum, bool) {
	result := RawEnum{Values: map[string]RawEnumValue{}}
	found := false
	for _, statement := range enum.Body {
		member, ok := statement.(*ast.EnumMemberStatement)
		if !ok || member.RawValue == nil {
			continue
		}
		found = true
		raw := ""
		typ := types.Type{}
		switch value := member.RawValue.(type) {
		case *ast.Literal:
			raw = value.Raw
			if value.Kind == ast.StringLiteral {
				typ = types.FromName("String")
			} else if value.Kind == ast.IntegerLiteral {
				typ = types.FromName("Integer")
			}
		case *ast.UnaryExpression:
			if literal, ok := value.Operand.(*ast.Literal); value.Operator == "-" && ok && literal.Kind == ast.IntegerLiteral {
				raw = "-" + literal.Raw
				typ = types.FromName("Integer")
			}
		}
		if result.Type.Kind == "" && typ.Kind != "" {
			result.Type = typ
		}
		if raw != "" {
			result.Values[member.Name] = RawEnumValue{Raw: raw, Type: typ}
		}
	}
	return result, found
}

func (c *Checker) methodReturnType(method *ast.MethodStatement) types.Type {
	if method == nil || method.ReturnType.Empty() {
		return types.Type{Kind: types.Void, Name: "Void"}
	}
	return c.typeFromRef(method.ReturnType)
}

func (c *Checker) checkArguments(call *ast.CallExpression, method *ast.MethodStatement, actual []types.Type) {
	if method == nil {
		if len(call.Arguments) > 0 {
			c.error(call.Span(), "constructor takes no arguments")
		}
		return
	}
	signature := c.signatureFromMethod(method)
	names := make([]string, len(method.Parameters))
	for index, parameter := range method.Parameters {
		names[index] = parameter.Name
	}
	c.checkCallSignature(call.Span(), method.Name, signature.parameters, signature.variadic, call.Arguments, actual, names, nil)
	c.result.CallSignatures[call] = append([]callsignature.Parameter(nil), signature.parameters...)
}

func (c *Checker) checkCallSignature(span token.Span, name string, parameters []callsignature.Parameter, variadic bool, arguments []ast.CallArgument, actual []types.Type, sourceNames []string, resultBridges []resolver.NativeResultBridge) []int {
	indexes := make([]int, len(arguments))
	for index := range indexes {
		indexes[index] = -1
	}
	used := make([]bool, len(parameters))
	position := 0
	for index, argument := range arguments {
		parameterIndex := -1
		if argument.Name != "" {
			for candidate, parameter := range parameters {
				if parameter.Kind == callsignature.NamedOnly && parameter.Label == argument.Name {
					parameterIndex = candidate
					break
				}
			}
			if parameterIndex < 0 {
				positionalOnly := false
				for candidate, sourceName := range sourceNames {
					if candidate < len(parameters) && parameters[candidate].Kind == callsignature.Positional && sourceName == argument.Name {
						positionalOnly = true
						break
					}
				}
				if positionalOnly {
					c.error(argument.Value.Span(), fmt.Sprintf("%s is a positional-only parameter of %s()", argument.Name, name))
				} else {
					c.error(argument.Value.Span(), fmt.Sprintf("%s() has no named argument %s", name, argument.Name))
				}
			}
		} else {
			for position < len(parameters) && parameters[position].Kind != callsignature.Positional {
				position++
			}
			if position < len(parameters) {
				parameterIndex = position
				position++
			} else if variadic && len(parameters) > 0 {
				parameterIndex = len(parameters) - 1
			} else {
				c.error(argument.Value.Span(), fmt.Sprintf("%s() does not accept this positional argument", name))
			}
		}
		indexes[index] = parameterIndex
		if parameterIndex < 0 {
			continue
		}
		if used[parameterIndex] && !(variadic && parameterIndex == len(parameters)-1) {
			label := parameters[parameterIndex].Label
			if label == "" && parameterIndex < len(sourceNames) {
				label = sourceNames[parameterIndex]
			}
			c.error(argument.Value.Span(), fmt.Sprintf("%s() receives argument %s more than once", name, label))
			continue
		}
		used[parameterIndex] = true
	}
	for index, parameter := range parameters {
		if parameter.Presence != callsignature.Required || used[index] || variadic && index == len(parameters)-1 {
			continue
		}
		label := parameter.Label
		if label == "" {
			label = strconv.Itoa(index + 1)
		}
		c.error(span, fmt.Sprintf("%s() is missing required argument %s", name, label))
	}
	for argumentIndex, parameterIndex := range indexes {
		if parameterIndex < 0 || parameterIndex >= len(parameters) {
			continue
		}
		expected := parameters[parameterIndex].Type
		argumentType := actual[argumentIndex]
		if expected.Kind == types.Any || expected.Kind == types.Invalid {
			if c.inferenceOnly {
				if pending := c.pendingEmptyCollection(arguments[argumentIndex].Value); pending != nil {
					c.markEmptyCollectionEscape(pending, arguments[argumentIndex].Value.Span())
				}
			}
			continue
		}
		argumentType = c.contextualizeCollectionLiteral(arguments[argumentIndex].Value, expected, argumentType)
		actual[argumentIndex] = argumentType
		bridge := resolver.NativeResultBridge{}
		if parameterIndex < len(resultBridges) {
			bridge = resultBridges[parameterIndex]
		}
		assignable := false
		if bridge.Kind != "" {
			assignable = c.checkNativeResultBridge(arguments[argumentIndex].Value, expected, argumentType, bridge)
		} else {
			assignable = c.assignable(arguments[argumentIndex].Value, expected, argumentType)
		}
		if !assignable {
			c.error(arguments[argumentIndex].Value.Span(), fmt.Sprintf("argument %d to %s() has type %s, expected %s", argumentIndex+1, name, argumentType, expected))
		}
	}
	return indexes
}

func fromTypeRef(ref ast.TypeRef) types.Type {
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = fromTypeRef(alternative)
		}
		return types.UnionOf(alternatives...)
	}
	if ref.FunctionReturn != nil {
		parameters := make([]types.Type, len(ref.FunctionParameters))
		for index, parameter := range ref.FunctionParameters {
			parameters[index] = fromTypeRef(parameter)
		}
		result := types.FunctionOf(parameters, fromTypeRef(*ref.FunctionReturn))
		result.Nullable = ref.Nullable
		return result
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
	return c.typeFromRefWithParameters(ref, c.activeTypeParameterSet())
}

func (c *Checker) typeFromRefWithParameters(ref ast.TypeRef, typeParameters map[string]bool) types.Type {
	authored := c.canonicalType(fromTypeRef(ref), typeParameters)
	result := c.expandAlias(fromTypeRef(ref), map[string]bool{})
	result = c.canonicalType(result, typeParameters)
	if !ref.Empty() {
		c.result.ResolvedTypes[ref.Span()] = authored
	}
	return result
}

func (c *Checker) canonicalType(typ types.Type, typeParameters map[string]bool) types.Type {
	for index := range typ.Args {
		typ.Args[index] = c.canonicalType(typ.Args[index], typeParameters)
	}
	if typ.Kind == types.Named && !typ.Declaration.Empty() {
		if declaration := c.authoredOwnerIdentities[typ.Declaration.Name]; declaration.Kind.IsType() {
			typ.Name = declaration.LeafName()
		}
		return typ
	}
	if typ.Kind != types.Named || typ.Name == "" || typeParameters[typ.Name] {
		return typ
	}
	if declaration := c.authoredTypeIdentity(typ.Name, c.activeTypeOwner); !declaration.Empty() {
		typ.Declaration = declaration
		typ.Name = declaration.LeafName()
		return typ
	}
	lookups := []func(string) (resolver.Binding, bool){
		c.resolution.ImportedType,
		c.resolution.InferredType,
		c.resolution.ContractType,
	}
	for _, lookup := range lookups {
		if binding, ok := lookup(typ.Name); ok {
			typ.Declaration = binding.DeclarationIdentity()
			return typ
		}
	}
	return typ
}

func (c *Checker) authoredTypeIdentity(name, owner string) identity.Declaration {
	if strings.Contains(name, "::") {
		declaration := c.authoredOwnerIdentities[name]
		if declaration.Kind.IsType() {
			return declaration
		}
		return identity.Declaration{}
	}
	for current := owner; ; {
		qualified := nestedAuthoredOwner(current, name)
		declaration := c.authoredOwnerIdentities[qualified]
		if declaration.Kind.IsType() {
			return declaration
		}
		separator := strings.LastIndex(current, "::")
		if separator < 0 {
			if current == "" {
				break
			}
			current = ""
		} else {
			current = current[:separator]
		}
	}
	return identity.Declaration{}
}

func (c *Checker) canonicalContractType(typ types.Type, typeParameters map[string]bool) types.Type {
	for index := range typ.Args {
		typ.Args[index] = c.canonicalContractType(typ.Args[index], typeParameters)
	}
	if typ.Kind != types.Named || typ.Name == "" || typeParameters[typ.Name] || !typ.Declaration.Empty() {
		return typ
	}
	if binding, ok := c.resolution.CatalogType(typ.Name); ok {
		typ.Declaration = binding.DeclarationIdentity()
	}
	return typ
}

func (c *Checker) resolvedBindingType(binding resolver.Binding) types.Type {
	typ := binding.Type()
	if binding.Export != nil && typ.Kind == types.Named && typ.Name == binding.Export.Name {
		typ.Declaration = binding.DeclarationIdentity()
	}
	return c.canonicalContractType(typ, c.activeTypeParameterSet())
}

func (c *Checker) activeTypeParameterSet() map[string]bool {
	result := map[string]bool{}
	for name, depth := range c.activeTypeParameters {
		if depth > 0 {
			result[name] = true
		}
	}
	return result
}

func (c *Checker) pushActiveTypeParameters(parameters []ast.TypeParameter) func() {
	for _, parameter := range parameters {
		c.activeTypeParameters[parameter.Name]++
	}
	return func() {
		for _, parameter := range parameters {
			c.activeTypeParameters[parameter.Name]--
		}
	}
}

func (c *Checker) pushActiveTypeOwner(owner string) func() {
	previous := c.activeTypeOwner
	c.activeTypeOwner = owner
	return func() { c.activeTypeOwner = previous }
}

func (c *Checker) aliasDefinition(name string) ([]string, types.Type, bool) {
	if alias := c.aliases[name]; alias != nil {
		return alias.typeParameters, alias.target, true
	}
	if binding, imported := c.resolution.ImportedType(name); imported && binding.Export.Kind == resolver.TypeAliasExport {
		return binding.Export.TypeParameters, binding.Export.AliasTarget, true
	}
	if binding, inferred := c.resolution.InferredType(name); inferred && binding.Export.Kind == resolver.TypeAliasExport {
		return binding.Export.TypeParameters, binding.Export.AliasTarget, true
	}
	if exported, exists := c.resolution.CompilerOwnedType(name); exists && exported.Kind == resolver.TypeAliasExport {
		return exported.TypeParameters, exported.AliasTarget, true
	}
	if exported, exists := c.resolution.ContractTypeAlias(name); exists {
		return exported.TypeParameters, exported.AliasTarget, true
	}
	return nil, types.Type{}, false
}

func (c *Checker) aliasTargetIsExternal(target types.Type, seen map[string]bool) bool {
	if target.Kind != types.Named || target.Name == "" {
		return false
	}
	if alias := c.aliases[target.Name]; alias != nil {
		if argument, ok := aliasTypeParameterArgument(alias.typeParameters, alias.target, target.Args); ok {
			return c.aliasTargetIsExternal(argument, seen)
		}
		if c.aliasCycles[target.Name] {
			return false
		}
		key := target.String()
		if seen[key] {
			return false
		}
		seen[key] = true
		expanded := substituteType(alias.target, typeSubstitutions(alias.typeParameters, target.Args))
		return c.aliasTargetIsExternal(expanded, seen)
	}
	if _, local := c.declaredTypes[target.Name]; local {
		return false
	}
	binding, imported := c.resolution.ImportedType(target.Name)
	if !imported {
		return false
	}
	if binding.Export != nil && binding.Export.Kind == resolver.TypeAliasExport {
		if argument, ok := aliasTypeParameterArgument(binding.Export.TypeParameters, binding.Export.AliasTarget, target.Args); ok {
			return c.aliasTargetIsExternal(argument, seen)
		}
	}
	return true
}

func aliasTypeParameterArgument(parameters []string, target types.Type, arguments []types.Type) (types.Type, bool) {
	if target.Kind != types.Named || len(target.Args) != 0 {
		return types.Type{}, false
	}
	for index, parameter := range parameters {
		if target.Name == parameter && index < len(arguments) {
			return arguments[index], true
		}
	}
	return types.Type{}, false
}

func (c *Checker) externalAliasTargetBinding(name string) (resolver.Binding, bool) {
	lookups := []func(string) (resolver.Binding, bool){
		c.resolution.ImportedType,
		c.resolution.ContractType,
	}
	for _, lookup := range lookups {
		if binding, ok := lookup(name); ok {
			return binding, true
		}
	}
	return resolver.Binding{}, false
}

func (c *Checker) authoredAliasTargetBinding(target types.Type) (resolver.Binding, bool) {
	if target.Kind != types.Named || target.Name == "" {
		return resolver.Binding{}, false
	}
	if c.aliases[target.Name] != nil {
		return resolver.Binding{}, false
	}
	if _, local := c.declaredTypes[target.Name]; local {
		return resolver.Binding{}, false
	}
	return c.resolution.ImportedType(target.Name)
}

func (c *Checker) newtypeDefinition(name string) (types.Type, *resolver.Binding, bool) {
	if defined := c.newtypes[name]; defined != nil {
		return defined.target, nil, true
	}
	if binding, imported := c.resolution.ImportedType(name); imported && binding.Export != nil && binding.Export.Kind == resolver.NewtypeExport {
		copy := binding
		return binding.Export.NewtypeTarget, &copy, true
	}
	if binding, inferred := c.resolution.InferredType(name); inferred && binding.Export != nil && binding.Export.Kind == resolver.NewtypeExport {
		copy := binding
		return binding.Export.NewtypeTarget, &copy, true
	}
	if exported, exists := c.resolution.CompilerOwnedType(name); exists && exported.Kind == resolver.NewtypeExport {
		return exported.NewtypeTarget, nil, true
	}
	return types.Type{}, nil, false
}

func (c *Checker) expandRepresentation(typ types.Type, visiting map[string]bool) types.Type {
	typ = c.expandAlias(typ, map[string]bool{})
	arguments := make([]types.Type, len(typ.Args))
	for index, argument := range typ.Args {
		arguments[index] = c.expandRepresentation(argument, visiting)
	}
	typ.Args = arguments
	target, _, newtype := c.newtypeDefinition(typ.Name)
	if !newtype {
		return typ
	}
	if visiting[typ.Name] {
		return invalidType()
	}
	visiting[typ.Name] = true
	representation := c.expandRepresentation(target, visiting)
	delete(visiting, typ.Name)
	representation.Nullable = representation.Nullable || typ.Nullable
	representation.Readonly = representation.Readonly || typ.Readonly
	return representation
}

func (c *Checker) newtypeRepresentationCycle(name string, visiting map[string]bool) bool {
	if visiting[name] {
		return true
	}
	target, _, ok := c.newtypeDefinition(name)
	if !ok {
		return false
	}
	visiting[name] = true
	target = c.expandAlias(target, map[string]bool{})
	var cycle func(types.Type) bool
	cycle = func(typ types.Type) bool {
		if _, _, defined := c.newtypeDefinition(typ.Name); defined && c.newtypeRepresentationCycle(typ.Name, visiting) {
			return true
		}
		for _, argument := range typ.Args {
			if cycle(argument) {
				return true
			}
		}
		return false
	}
	result := cycle(target)
	delete(visiting, name)
	return result
}

func newtypeRepresentationFullyInstantiated(typ types.Type) bool {
	switch typ.Kind {
	case types.Array, types.Range, types.Iterable:
		if len(typ.Args) != 1 {
			return false
		}
	case types.Hash:
		if len(typ.Args) != 2 {
			return false
		}
	case types.Function, types.Union:
		if len(typ.Args) == 0 {
			return false
		}
	}
	for _, argument := range typ.Args {
		if !newtypeRepresentationFullyInstantiated(argument) {
			return false
		}
	}
	return true
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
	c.errorCode(diagnostic.TypeError, span, message)
}

func (c *Checker) errorCode(code diagnostic.Code, span token.Span, message string) {
	if c.inferenceOnly {
		return
	}
	c.diags = append(c.diags, diagnostic.Diagnostic{Code: code, Severity: diagnostic.Error, Message: message, Span: span})
}

func (c *Checker) errorRelated(code diagnostic.Code, span token.Span, message, relatedMessage string, relatedSpan token.Span) {
	if c.inferenceOnly {
		return
	}
	c.diags = append(c.diags, diagnostic.Diagnostic{
		Code: code, Severity: diagnostic.Error, Message: message, Span: span,
		Related: []diagnostic.RelatedInformation{{Message: relatedMessage, Location: diagnostic.Location{Span: relatedSpan}}},
	})
}
