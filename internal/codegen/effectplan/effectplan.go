// Package effectplan performs whole-project call-graph analysis for backend
// effects that remain intentionally absent from TypeRB source signatures.
package effectplan

import (
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/runtimeoperation"
	"github.com/type-rb/type-rb/internal/types"
)

// Plan records the declarations and expressions that transitively reach one
// of the backend effects selected by Options.
type Plan struct {
	Methods           map[*ir.Method]bool
	ParameterDefaults map[*ir.Method]bool
	// ClassConstructors includes direct field defaults and the class's own
	// initialize method, but never an inherited initializer.
	ClassConstructors   map[*ir.Class]bool
	RecordDefaults      map[*ir.Record]bool
	RecordFieldDefaults map[*ir.RecordField]bool
	// RecordConstructDefaults marks sites that evaluate an omitted selected
	// effect. RecordConstructSync marks sites that still need the declaration's
	// helper ABI but supplied every field whose default has that effect.
	RecordConstructDefaults map[*ir.RecordConstruct]bool
	RecordConstructSync     map[*ir.RecordConstruct]bool
	Lambdas                 map[*ir.Lambda]bool
	Calls                   map[*ir.Call]bool
	CallParameterDefaults   map[*ir.Call]bool
	EnumCalls               map[*ir.EnumCall]bool
	EnumCallDefaults        map[*ir.EnumCall]bool
	Expressions             map[ir.Expression]bool
	Iterations              map[*ir.Iterate]bool
	StructuredBlocks        map[*ir.StructuredBlock]bool
	// LambdaModules retains the source module that owns each first-class
	// function. Backend policy can use that identity without moving target-
	// specific suspension rules into shared TypeRB semantics.
	LambdaModules map[*ir.Lambda]string
	methodKeys    map[methodKey]bool
	recordKeys    map[recordKey]bool
}

type methodKey struct {
	module string
	owner  string
	name   string
}

type recordKey struct {
	module string
	name   string
}

// Method reports whether a named project method transitively reaches an
// effect root. Integrations use this stable identity when dispatch code is
// generated outside the module that owns the method.
func (p *Plan) Method(module, owner, name string) bool {
	return p != nil && p.methodKeys[methodKey{module: module, owner: owner, name: name}]
}

// RecordDefault reports whether evaluating a record's omitted field defaults
// transitively reaches an effect root.
func (p *Plan) RecordDefault(module, name string) bool {
	return p != nil && p.recordKeys[recordKey{module: module, name: name}]
}

// RecordDefaultFor reports the effect of one exact record declaration. Code
// generators use declaration identity so same-named records in namespaces do
// not accidentally share a hidden ABI.
func (p *Plan) RecordDefaultFor(record *ir.Record) bool {
	return p != nil && p.RecordDefaults[record]
}

// RecordFieldDefaultFor reports whether one field's default reaches an effect
// root. Calls use this finer-grained fact so explicitly supplied fields do not
// make an otherwise synchronous construction effectful.
func (p *Plan) RecordFieldDefaultFor(field *ir.RecordField) bool {
	return p != nil && p.RecordFieldDefaults[field]
}

type methodContext struct {
	module    string
	namespace string
	owner     declarationIdentity
	method    *ir.Method
	// fieldValues contains record fields that are already initialized at the
	// current default expression. Function-typed values may be backed by either
	// synchronous or backend-suspending callbacks.
	fieldValues map[string]bool
}

type classContext struct {
	identity   declarationIdentity
	namespace  string
	class      *ir.Class
	superclass declarationIdentity
	implements []declarationIdentity
	initialize *ir.Method
}

type interfaceContext struct {
	identity declarationIdentity
	methods  []*ir.Method
}

type recordContext struct {
	module    string
	namespace string
	identity  declarationIdentity
	record    *ir.Record
}

type aliasContext struct {
	identity  declarationIdentity
	module    string
	namespace string
	alias     *ir.TypeAlias
}

type analyzer struct {
	programs             []*ir.Program
	plan                 *Plan
	options              Options
	methods              []methodContext
	records              []recordContext
	methodInfo           map[*ir.Method]methodContext
	topMethods           map[string][]*ir.Method
	memberMethods        map[dispatchIdentity][]*ir.Method
	classDefinitions     map[declarationIdentity]*classContext
	interfaceDefinitions map[declarationIdentity]*interfaceContext
	recordDefinitions    map[declarationIdentity][]*ir.Record
	aliasDefinitions     map[declarationIdentity]declarationIdentity
	declarations         map[declarationIdentity]bool
	typeDeclarations     map[declarationIdentity]bool
	moduleDeclarations   map[declarationIdentity]bool
	aliases              []aliasContext
	classes              []*classContext
	lambdaBindings       map[functionBindingKey]*ir.Lambda
}

type declarationIdentity struct {
	module string
	name   string
	kind   identity.Kind
}

func (i declarationIdentity) empty() bool { return i.name == "" }

type dispatchIdentity struct {
	owner declarationIdentity
	name  string
	class bool
}

type functionBindingKey struct {
	module string
	method *ir.Method
	name   string
}

// Options chooses effect roots while retaining one call-graph model.
type Options struct {
	Intrinsic       func(string) bool
	Runtime         func(*ir.RuntimeBinding) bool
	Conversion      func(ir.ConversionKind) bool
	Transform       func(*ir.Transform) bool
	WebNext         bool
	CaptureLambdas  bool
	PassToFunctions bool
}

func Analyze(programs []*ir.Program, options Options) *Plan {
	plan := &Plan{
		Methods:                 map[*ir.Method]bool{},
		ParameterDefaults:       map[*ir.Method]bool{},
		ClassConstructors:       map[*ir.Class]bool{},
		RecordDefaults:          map[*ir.Record]bool{},
		RecordFieldDefaults:     map[*ir.RecordField]bool{},
		RecordConstructDefaults: map[*ir.RecordConstruct]bool{},
		RecordConstructSync:     map[*ir.RecordConstruct]bool{},
		Lambdas:                 map[*ir.Lambda]bool{},
		Calls:                   map[*ir.Call]bool{},
		CallParameterDefaults:   map[*ir.Call]bool{},
		EnumCalls:               map[*ir.EnumCall]bool{},
		EnumCallDefaults:        map[*ir.EnumCall]bool{},
		Expressions:             map[ir.Expression]bool{},
		Iterations:              map[*ir.Iterate]bool{},
		StructuredBlocks:        map[*ir.StructuredBlock]bool{},
		LambdaModules:           map[*ir.Lambda]string{},
		methodKeys:              map[methodKey]bool{},
		recordKeys:              map[recordKey]bool{},
	}
	analyzer := &analyzer{
		programs: programs, plan: plan, options: options, methodInfo: map[*ir.Method]methodContext{},
		topMethods:           map[string][]*ir.Method{},
		memberMethods:        map[dispatchIdentity][]*ir.Method{},
		classDefinitions:     map[declarationIdentity]*classContext{},
		interfaceDefinitions: map[declarationIdentity]*interfaceContext{},
		recordDefinitions:    map[declarationIdentity][]*ir.Record{},
		aliasDefinitions:     map[declarationIdentity]declarationIdentity{},
		declarations:         map[declarationIdentity]bool{},
		typeDeclarations:     map[declarationIdentity]bool{},
		moduleDeclarations:   map[declarationIdentity]bool{},
		lambdaBindings:       map[functionBindingKey]*ir.Lambda{},
	}
	for _, program := range programs {
		analyzer.collect(program.ModulePath, "", declarationIdentity{}, program.Statements)
	}
	analyzer.resolveAliasDefinitions()
	analyzer.normalizeDeclarationReferences()
	if options.WebNext {
		for _, method := range analyzer.methods {
			if method.owner.name == "Next" && method.method.Name == "call" {
				plan.Methods[method.method] = true
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for _, method := range analyzer.methods {
			defaultsReach := analyzer.parameterDefaultsReach(method, false)
			if defaultsReach && !plan.ParameterDefaults[method.method] {
				plan.ParameterDefaults[method.method] = true
				changed = true
			}
			bodyReaches := analyzer.statementsReach(method.method.Body, method, false)
			if !plan.Methods[method.method] && (defaultsReach || bodyReaches) {
				plan.Methods[method.method] = true
				changed = true
			}
		}
		for _, record := range analyzer.records {
			defaultsReach, fieldChanged := analyzer.recordDefaultsReach(record, false)
			if fieldChanged {
				changed = true
			}
			if defaultsReach && !plan.RecordDefaults[record.record] {
				plan.RecordDefaults[record.record] = true
				changed = true
			}
		}
		for _, class := range analyzer.classes {
			fieldsReach := analyzer.classFieldDefaultsReach(class, false)
			if fieldsReach && class.initialize != nil && !plan.Methods[class.initialize] {
				// Backend constructors evaluate direct field defaults as part of
				// initialize, so an explicit initializer owns the same hidden ABI.
				plan.Methods[class.initialize] = true
				changed = true
			}
			constructorReaches := fieldsReach || class.initialize != nil && plan.Methods[class.initialize]
			if constructorReaches && !plan.ClassConstructors[class.class] {
				plan.ClassConstructors[class.class] = true
				changed = true
			}
		}
		if analyzer.propagateDispatchEffects() {
			changed = true
		}
	}

	for _, method := range analyzer.methods {
		analyzer.parameterDefaultsReach(method, true)
		analyzer.statementsReach(method.method.Body, method, true)
		if plan.Methods[method.method] {
			plan.methodKeys[methodKey{module: method.module, owner: method.owner.name, name: method.method.Name}] = true
		}
	}
	for _, class := range analyzer.classes {
		analyzer.classFieldDefaultsReach(class, true)
	}
	for _, record := range analyzer.records {
		analyzer.recordDefaultsReach(record, true)
		if plan.RecordDefaults[record.record] {
			plan.recordKeys[recordKey{module: record.module, name: record.identity.name}] = true
		}
	}
	for _, program := range programs {
		analyzer.statementsReach(program.Statements, methodContext{module: program.ModulePath}, true)
	}
	return plan
}

func (a *analyzer) collect(module, namespace string, owner declarationIdentity, statements []ir.Statement) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			class := a.addClass(module, namespace, node)
			a.collect(module, class.identity.name, class.identity, node.Body)
		case *ir.Enum:
			identity := effectDeclarationIdentity(node.Declaration, module, qualifiedDeclarationName(namespace, node.Name))
			a.declarations[identity] = true
			a.typeDeclarations[identity] = true
			a.collect(module, identity.name, identity, node.Body)
		case *ir.Record:
			record := recordContext{
				module: module, namespace: namespace,
				identity: effectDeclarationIdentity(node.Declaration, module, qualifiedDeclarationName(namespace, node.Name)), record: node,
			}
			a.addRecord(record)
			a.collect(module, record.identity.name, record.identity, node.Body)
		case *ir.Interface:
			interfaceDeclaration := a.addInterface(module, namespace, node)
			for _, method := range node.Methods {
				a.addMethod(methodContext{module: module, namespace: interfaceDeclaration.identity.name, owner: interfaceDeclaration.identity, method: method})
			}
		case *ir.TypeAlias:
			a.addAlias(module, namespace, node)
		case *ir.Module:
			identity := effectDeclarationIdentity(node.Declaration, module, qualifiedDeclarationName(namespace, node.Name))
			a.declarations[identity] = true
			a.moduleDeclarations[identity] = true
			a.collect(module, identity.name, identity, node.Body)
		case *ir.Method:
			a.addMethod(methodContext{module: module, namespace: namespace, owner: owner, method: node})
		}
	}
}

func (a *analyzer) addClass(module, namespace string, class *ir.Class) *classContext {
	context := &classContext{
		identity:   effectDeclarationIdentity(class.Declaration, module, qualifiedDeclarationName(namespace, class.Name)),
		namespace:  namespace,
		class:      class,
		superclass: referencedDeclarationIdentity(class.Superclass, module),
	}
	for _, statement := range class.Body {
		method, ok := statement.(*ir.Method)
		if ok && method.Name == "initialize" && !method.Class {
			context.initialize = method
			break
		}
	}
	implementedTypes := class.ResolvedImplements
	implementedReferences := class.ResolvedImplementReferences
	if len(implementedTypes) == 0 {
		implementedTypes = class.Implements
		implementedReferences = class.ImplementReferences
	}
	for index, implemented := range implementedTypes {
		var reference *ir.Reference
		if index < len(implementedReferences) {
			reference = implementedReferences[index]
		}
		context.implements = append(context.implements, referencedTypeIdentity(implemented, reference, module))
	}
	a.classes = append(a.classes, context)
	a.classDefinitions[context.identity] = context
	a.declarations[context.identity] = true
	a.typeDeclarations[context.identity] = true
	return context
}

func (a *analyzer) addInterface(module, namespace string, declaration *ir.Interface) *interfaceContext {
	context := &interfaceContext{
		identity: effectDeclarationIdentity(declaration.Declaration, module, qualifiedDeclarationName(namespace, declaration.Name)),
		methods:  append([]*ir.Method(nil), declaration.Methods...),
	}
	a.interfaceDefinitions[context.identity] = context
	a.declarations[context.identity] = true
	a.typeDeclarations[context.identity] = true
	return context
}

func (a *analyzer) addRecord(record recordContext) {
	a.records = append(a.records, record)
	a.recordDefinitions[record.identity] = append(a.recordDefinitions[record.identity], record.record)
	a.declarations[record.identity] = true
	a.typeDeclarations[record.identity] = true
}

func (a *analyzer) addAlias(module, namespace string, alias *ir.TypeAlias) {
	identity := effectDeclarationIdentity(alias.Declaration, module, qualifiedDeclarationName(namespace, alias.Name))
	a.declarations[identity] = true
	a.typeDeclarations[identity] = true
	if alias.Target.Kind == types.Named && alias.Target.Name != "" {
		a.aliases = append(a.aliases, aliasContext{identity: identity, module: module, namespace: namespace, alias: alias})
	}
}

func (a *analyzer) addMethod(method methodContext) {
	a.methods = append(a.methods, method)
	a.methodInfo[method.method] = method
	if method.owner.empty() {
		key := callableKey(method.module, method.method.Name)
		a.topMethods[key] = append(a.topMethods[key], method.method)
		return
	}
	key := dispatchIdentity{owner: method.owner, name: method.method.Name, class: method.method.Class}
	a.memberMethods[key] = append(a.memberMethods[key], method.method)
}

func qualifiedDeclarationName(namespace, name string) string {
	if namespace == "" || name == "" || strings.Contains(name, "::") {
		return name
	}
	return namespace + "::" + name
}

func effectDeclarationIdentity(declaration identity.Declaration, fallbackModule, fallbackName string) declarationIdentity {
	if declaration.Empty() {
		return declarationIdentity{module: fallbackModule, name: fallbackName}
	}
	return declarationIdentity{module: declaration.Module, name: declaration.Name, kind: declaration.Kind}
}

func (a *analyzer) resolveAliasDefinitions() {
	for _, context := range a.aliases {
		if context.alias.TargetReference != nil && context.alias.TargetReference.Package != "" {
			a.aliasDefinitions[context.identity] = referencedTypeIdentity(
				context.alias.Target, context.alias.TargetReference, context.module,
			)
			continue
		}
		if !context.alias.Target.Declaration.Empty() {
			a.aliasDefinitions[context.identity] = effectDeclarationIdentity(
				context.alias.Target.Declaration, context.module, context.alias.Target.Name,
			)
			continue
		}
		if target, ok := a.localTypeDeclarationIdentity(context.module, context.namespace, context.alias.Target.Name); ok {
			a.aliasDefinitions[context.identity] = target
			continue
		}
		a.aliasDefinitions[context.identity] = declarationIdentity{module: context.module, name: context.alias.Target.Name}
	}
}

func (a *analyzer) normalizeDeclarationReferences() {
	for _, class := range a.classes {
		class.superclass = a.resolveClassDeclarationReference(class, class.superclass)
		for index, implemented := range class.implements {
			class.implements[index] = a.resolveClassDeclarationReference(class, implemented)
		}
	}
}

func (a *analyzer) resolveClassDeclarationReference(class *classContext, identity declarationIdentity) declarationIdentity {
	if identity.empty() {
		return identity
	}
	if identity.module == class.identity.module {
		return a.declarationIdentityInNamespace(identity.module, class.namespace, identity.name)
	}
	return a.canonicalDeclarationIdentity(identity)
}

func (a *analyzer) canonicalDeclarationIdentity(identity declarationIdentity) declarationIdentity {
	seen := map[declarationIdentity]bool{}
	current := a.knownDeclarationIdentity(identity)
	for !current.empty() && !seen[current] {
		seen[current] = true
		target, alias := a.aliasDefinitions[current]
		if !alias {
			return current
		}
		current = a.knownDeclarationIdentity(target)
	}
	return current
}

func (a *analyzer) canonicalTypeDeclarationIdentity(identity declarationIdentity) declarationIdentity {
	seen := map[declarationIdentity]bool{}
	current := a.knownTypeDeclarationIdentity(identity)
	for !current.empty() && !seen[current] {
		seen[current] = true
		target, alias := a.aliasDefinitions[current]
		if !alias {
			return current
		}
		current = a.knownTypeDeclarationIdentity(target)
	}
	return current
}

func (a *analyzer) knownDeclarationIdentity(identity declarationIdentity) declarationIdentity {
	if identity.empty() || a.declarations[identity] {
		return identity
	}
	parts := strings.Split(identity.name, "::")
	if len(parts) > 1 {
		return identity
	}
	var match declarationIdentity
	for candidate := range a.typeDeclarations {
		if candidate.module != identity.module || identity.kind != "" && candidate.kind != identity.kind || !strings.HasSuffix(candidate.name, "::"+identity.name) {
			continue
		}
		if !match.empty() {
			return identity
		}
		match = candidate
	}
	if !match.empty() {
		return match
	}
	return identity
}

func (a *analyzer) knownTypeDeclarationIdentity(identity declarationIdentity) declarationIdentity {
	if identity.empty() || a.typeDeclarations[identity] {
		return identity
	}
	if strings.Contains(identity.name, "::") {
		return identity
	}
	var match declarationIdentity
	for candidate := range a.typeDeclarations {
		if candidate.module != identity.module || identity.kind != "" && candidate.kind != identity.kind || !strings.HasSuffix(candidate.name, "::"+identity.name) {
			continue
		}
		if !match.empty() {
			return identity
		}
		match = candidate
	}
	if !match.empty() {
		return match
	}
	return identity
}

func (a *analyzer) declarationIdentityInNamespace(module, namespace, name string) declarationIdentity {
	if identity, ok := a.localTypeDeclarationIdentity(module, namespace, name); ok {
		return a.canonicalTypeDeclarationIdentity(identity)
	}
	return a.canonicalTypeDeclarationIdentity(declarationIdentity{module: module, name: name})
}

func (a *analyzer) runtimeDeclarationIdentityInNamespace(module, namespace, name string) declarationIdentity {
	if identity, ok := localIdentityInNamespace(a.moduleDeclarations, module, namespace, name); ok {
		return identity
	}
	if identity, ok := a.localDeclarationIdentity(module, namespace, name); ok {
		if a.moduleDeclarations[identity] {
			return identity
		}
		return a.canonicalDeclarationIdentity(identity)
	}
	return a.canonicalDeclarationIdentity(declarationIdentity{module: module, name: name})
}

func (a *analyzer) localDeclarationIdentity(module, namespace, name string) (declarationIdentity, bool) {
	return localIdentityInNamespace(a.declarations, module, namespace, name)
}

func (a *analyzer) localTypeDeclarationIdentity(module, namespace, name string) (declarationIdentity, bool) {
	return localIdentityInNamespace(a.typeDeclarations, module, namespace, name)
}

func localIdentityInNamespace(candidates map[declarationIdentity]bool, module, namespace, name string) (declarationIdentity, bool) {
	for current := namespace; ; {
		candidateName := name
		if current != "" {
			candidateName = current + "::" + name
		}
		if candidate, ok := declarationIdentityNamed(candidates, module, candidateName); ok {
			return candidate, true
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
	return declarationIdentity{}, false
}

func declarationIdentityNamed(candidates map[declarationIdentity]bool, module, name string) (declarationIdentity, bool) {
	legacy := declarationIdentity{module: module, name: name}
	if candidates[legacy] {
		return legacy, true
	}
	var match declarationIdentity
	for candidate := range candidates {
		if candidate.module != module || candidate.name != name {
			continue
		}
		if !match.empty() {
			return declarationIdentity{}, false
		}
		match = candidate
	}
	return match, !match.empty()
}

func (a *analyzer) propagateDispatchEffects() bool {
	changed := a.propagateInterfaces()
	if a.propagateInheritance() {
		changed = true
	}
	return changed
}

func (a *analyzer) propagateInterfaces() bool {
	changed := false
	for _, class := range a.classes {
		for _, implemented := range class.implements {
			declaration := a.interfaceDefinitions[implemented]
			if declaration == nil {
				continue
			}
			for _, interfaceMethod := range declaration.methods {
				implementations := a.inheritedMemberMethods(class.identity, interfaceMethod.Name, interfaceMethod.Class)
				methods := append([]*ir.Method{interfaceMethod}, implementations...)
				if a.propagateMethodEffects(methods) {
					changed = true
				}
			}
		}
	}
	return changed
}

func (a *analyzer) propagateInheritance() bool {
	changed := false
	for _, class := range a.classes {
		lineage := a.classLineage(class)
		methodsByDispatch := map[struct {
			name  string
			class bool
		}][]*ir.Method{}
		for _, method := range a.methods {
			// Constructors own their initialization ABI and do not participate in
			// ordinary inherited method dispatch.
			owner := a.classDefinitions[method.owner]
			if owner != nil && method.method.Name != "initialize" && lineage[owner] {
				key := struct {
					name  string
					class bool
				}{name: method.method.Name, class: method.method.Class}
				methodsByDispatch[key] = append(methodsByDispatch[key], method.method)
			}
		}
		for _, methods := range methodsByDispatch {
			if a.propagateMethodEffects(methods) {
				changed = true
			}
		}
	}
	return changed
}

func (a *analyzer) propagateMethodEffects(methods []*ir.Method) bool {
	methodReaches := anyReached(a.plan.Methods, methods)
	defaultsReach := anyReached(a.plan.ParameterDefaults, methods)
	if !methodReaches && !defaultsReach {
		return false
	}
	changed := false
	for _, method := range methods {
		if !a.plan.Methods[method] {
			a.plan.Methods[method] = true
			changed = true
		}
		if defaultsReach && !a.plan.ParameterDefaults[method] {
			a.plan.ParameterDefaults[method] = true
			changed = true
		}
	}
	return changed
}

func (a *analyzer) classLineage(class *classContext) map[*classContext]bool {
	result := map[*classContext]bool{}
	var visit func(*classContext)
	visit = func(current *classContext) {
		if current == nil || result[current] {
			return
		}
		result[current] = true
		visit(a.classDefinitions[current.superclass])
	}
	visit(class)
	return result
}

func (a *analyzer) inheritedMemberMethods(owner declarationIdentity, name string, class bool) []*ir.Method {
	declaration := a.classDefinitions[owner]
	if declaration == nil {
		return a.memberMethods[dispatchIdentity{owner: owner, name: name, class: class}]
	}
	var result []*ir.Method
	seenClasses := map[*classContext]bool{}
	seenMethods := map[*ir.Method]bool{}
	var visit func(*classContext)
	visit = func(current *classContext) {
		if current == nil || seenClasses[current] {
			return
		}
		seenClasses[current] = true
		methods := a.memberMethods[dispatchIdentity{owner: current.identity, name: name, class: class}]
		if len(methods) > 0 {
			for _, method := range methods {
				if !seenMethods[method] {
					result = append(result, method)
					seenMethods[method] = true
				}
			}
			return
		}
		visit(a.classDefinitions[current.superclass])
	}
	visit(declaration)
	return result
}

func referencedDeclarationIdentity(expression ir.Expression, currentModule string) declarationIdentity {
	switch node := expression.(type) {
	case *ir.Identifier:
		if !node.Declaration.Empty() {
			return effectDeclarationIdentity(node.Declaration, currentModule, node.Name)
		}
		if node.Reference != nil && !node.Reference.Declaration.Empty() {
			return effectDeclarationIdentity(node.Reference.Declaration, currentModule, node.Name)
		}
		if node.Reference != nil && (node.Reference.Package != "" || node.Reference.Owner != "") {
			name := node.Name
			if node.Reference.Symbol != "" {
				name = node.Reference.Symbol
			}
			if node.Reference.Owner != "" {
				name = node.Reference.Owner
			}
			module := currentModule
			if node.Reference.Package != "" {
				module = node.Reference.Package
			}
			return declarationIdentity{module: module, name: name}
		}
		return declarationIdentity{module: currentModule, name: node.Name}
	case *ir.TypeApply:
		if !node.Declaration.Empty() {
			return effectDeclarationIdentity(node.Declaration, currentModule, "")
		}
		return referencedDeclarationIdentity(node.Receiver, currentModule)
	case *ir.Member:
		if !node.Declaration.Empty() {
			return effectDeclarationIdentity(node.Declaration, currentModule, node.Name)
		}
		if node.Reference != nil && !node.Reference.Declaration.Empty() {
			return effectDeclarationIdentity(node.Reference.Declaration, currentModule, node.Name)
		}
		if node.Reference != nil && (node.Reference.Package != "" || node.Reference.Owner != "") {
			name := node.Name
			if node.Reference.Symbol != "" {
				name = node.Reference.Symbol
			}
			if node.Reference.Owner != "" {
				name = node.Reference.Owner
			}
			module := currentModule
			if node.Reference.Package != "" {
				module = node.Reference.Package
			}
			return declarationIdentity{module: module, name: name}
		}
		if name := qualifiedExpressionName(node); name != "" {
			return declarationIdentity{module: currentModule, name: name}
		}
	}
	return declarationIdentity{}
}

func referencedTypeIdentity(typ types.Type, reference *ir.Reference, currentModule string) declarationIdentity {
	if reference != nil && !reference.Declaration.Empty() {
		return effectDeclarationIdentity(reference.Declaration, currentModule, typ.Name)
	}
	if !typ.Declaration.Empty() {
		return effectDeclarationIdentity(typ.Declaration, currentModule, typ.Name)
	}
	identity := declarationIdentity{module: currentModule, name: typ.Name}
	if reference == nil || reference.Package == "" {
		return identity
	}
	identity.module = reference.Package
	if reference.Symbol != "" {
		identity.name = reference.Symbol
	}
	return identity
}

func qualifiedExpressionName(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Name
	case *ir.Member:
		if !node.Namespace {
			return ""
		}
		prefix := qualifiedExpressionName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	case *ir.TypeApply:
		return qualifiedExpressionName(node.Receiver)
	}
	return ""
}

func anyReached(methods map[*ir.Method]bool, candidates []*ir.Method) bool {
	for _, method := range candidates {
		if methods[method] {
			return true
		}
	}
	return false
}

func anyRecordReached(records map[*ir.Record]bool, candidates []*ir.Record) bool {
	for _, record := range candidates {
		if records[record] {
			return true
		}
	}
	return false
}

func (a *analyzer) recordConstructDefaultsReach(construction *ir.RecordConstruct, records []*ir.Record) (bool, bool) {
	provided := map[string]bool{}
	for _, argument := range construction.Arguments {
		if argument.Name != "" {
			provided[argument.Name] = true
		}
	}
	omittedReaches := false
	targetHasEffects := false
	for _, record := range records {
		targetHasEffects = a.plan.RecordDefaults[record] || targetHasEffects
		for _, statement := range record.Body {
			field, ok := statement.(*ir.RecordField)
			if !ok || field.Default == nil || !a.plan.RecordFieldDefaults[field] {
				continue
			}
			targetHasEffects = true
			if !provided[field.Name] {
				omittedReaches = true
			}
		}
	}
	return omittedReaches, targetHasEffects
}

func callableKey(module, name string) string { return module + "\x00" + name }

func (a *analyzer) classFieldDefaultsReach(context *classContext, record bool) bool {
	reaches := false
	method := methodContext{module: context.identity.module, namespace: context.identity.name, owner: context.identity, method: context.initialize}
	for _, statement := range context.class.Body {
		field, ok := statement.(*ir.Field)
		if !ok {
			continue
		}
		reaches = a.expressionReaches(field.Value, method, record) || reaches
	}
	return reaches
}

func (a *analyzer) recordDefaultsReach(context recordContext, record bool) (bool, bool) {
	reaches := false
	changed := false
	method := methodContext{
		module: context.module, namespace: context.identity.name, owner: context.identity,
		fieldValues: map[string]bool{},
	}
	for _, statement := range context.record.Body {
		field, ok := statement.(*ir.RecordField)
		if !ok {
			continue
		}
		fieldReaches := a.expressionReaches(field.Default, method, record)
		if fieldReaches && !a.plan.RecordFieldDefaults[field] {
			a.plan.RecordFieldDefaults[field] = true
			changed = true
		}
		reaches = fieldReaches || reaches
		method.fieldValues[field.Name] = true
	}
	return reaches, changed
}

func (a *analyzer) parameterDefaultsReach(context methodContext, record bool) bool {
	reaches := false
	for _, parameter := range context.method.Parameters {
		reaches = a.expressionReaches(parameter.Default, context, record) || reaches
	}
	return reaches
}

func (a *analyzer) statementsReach(statements []ir.Statement, context methodContext, record bool) bool {
	suspends := false
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			identity := a.canonicalDeclarationIdentity(effectDeclarationIdentity(
				node.Declaration, context.module, qualifiedDeclarationName(context.namespace, node.Name),
			))
			classContext := methodContext{module: context.module, namespace: identity.name, owner: identity}
			suspends = a.statementsReach(node.Body, classContext, record) || suspends
		case *ir.Module:
			identity := effectDeclarationIdentity(node.Declaration, context.module, qualifiedDeclarationName(context.namespace, node.Name))
			moduleContext := context
			moduleContext.namespace = identity.name
			suspends = a.statementsReach(node.Body, moduleContext, record) || suspends
		case *ir.Field:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.Variable:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
			if lambda, ok := node.Value.(*ir.Lambda); ok {
				a.lambdaBindings[functionBindingKey{module: context.module, method: context.method, name: node.Name}] = lambda
			}
		case *ir.Assignment:
			suspends = a.expressionReaches(node.Target, context, record) || suspends
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.Return:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.ExpressionStatement:
			suspends = a.expressionReaches(node.Expression, context, record) || suspends
		case *ir.If:
			branchSuspends := a.expressionReaches(node.Condition, context, record)
			branchSuspends = a.statementsReach(node.Then, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ThenResult, context, record) || branchSuspends
			for _, branch := range node.ElseIf {
				branchSuspends = a.expressionReaches(branch.Condition, context, record) || branchSuspends
				branchSuspends = a.statementsReach(branch.Body, context, record) || branchSuspends
				branchSuspends = a.expressionReaches(branch.Result, context, record) || branchSuspends
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.Case:
			branchSuspends := a.statementsReach(node.Leading, context, record)
			branchSuspends = a.expressionReaches(node.Value, context, record) || branchSuspends
			for _, branch := range node.Branches {
				branchSuspends = a.expressionReaches(branch.Value, context, record) || branchSuspends
				branchSuspends = a.statementsReach(branch.Body, context, record) || branchSuspends
				branchSuspends = a.expressionReaches(branch.Result, context, record) || branchSuspends
				for _, alternative := range branch.Alternatives {
					branchSuspends = a.expressionReaches(alternative, context, record) || branchSuspends
				}
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.While:
			suspends = a.expressionReaches(node.Condition, context, record) || suspends
			suspends = a.statementsReach(node.Body, context, record) || suspends
		case *ir.Iterate:
			iterationSuspends := a.intrinsicReaches(node.Intrinsic)
			iterationSuspends = a.expressionReaches(node.Source, context, record) || iterationSuspends
			iterationSuspends = a.expressionReaches(node.SliceSize, context, record) || iterationSuspends
			iterationSuspends = a.statementsReach(node.Body, context, record) || iterationSuspends
			if record && iterationSuspends {
				a.plan.Iterations[node] = true
			}
			suspends = iterationSuspends || suspends
		case *ir.StructuredBlock:
			blockSuspends := a.intrinsicReaches(node.Intrinsic)
			blockSuspends = a.expressionReaches(node.Call, context, record) || blockSuspends
			blockSuspends = a.statementsReach(node.Body, context, record) || blockSuspends
			blockSuspends = a.expressionReaches(node.Value, context, record) || blockSuspends
			if record && blockSuspends {
				a.plan.StructuredBlocks[node] = true
			}
			suspends = blockSuspends || suspends
		case *ir.NativeBlock:
			suspends = a.statementsReach(node.Body, context, record) || suspends
		}
	}
	return suspends
}

func (a *analyzer) expressionReaches(expression ir.Expression, context methodContext, record bool) bool {
	if expression == nil {
		return false
	}
	suspends := false
	switch node := expression.(type) {
	case *ir.Lambda:
		// Creating a lambda never suspends the enclosing function. Its body owns
		// a separate backend-only suspension boundary.
		a.plan.LambdaModules[node] = context.module
		lambdaSuspends := a.statementsReach(node.Body, context, record)
		a.plan.Lambdas[node] = lambdaSuspends
		return a.options.CaptureLambdas && lambdaSuspends
	case *ir.InterpolatedString:
		for _, part := range node.Parts {
			suspends = a.expressionReaches(part.Expression, context, record) || suspends
		}
	case *ir.Array:
		for _, element := range node.Elements {
			suspends = a.expressionReaches(element, context, record) || suspends
		}
	case *ir.Hash:
		for _, entry := range node.Entries {
			suspends = a.expressionReaches(entry.Key, context, record) || suspends
			suspends = a.expressionReaches(entry.Value, context, record) || suspends
		}
	case *ir.JSXElement:
		suspends = a.expressionReaches(node.Component, context, record)
		for _, attribute := range node.Attributes {
			suspends = a.expressionReaches(attribute.Value, context, record) || suspends
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ir.JSXElement:
				suspends = a.expressionReaches(item, context, record) || suspends
			case *ir.JSXExpression:
				suspends = a.expressionReaches(item.Value, context, record) || suspends
			}
		}
	case *ir.Unary:
		suspends = a.expressionReaches(node.Operand, context, record)
	case *ir.Conversion:
		suspends = a.expressionReaches(node.Value, context, record)
		if a.options.Conversion != nil && a.options.Conversion(node.Kind) {
			suspends = true
		}
	case *ir.Binary:
		suspends = a.expressionReaches(node.Left, context, record)
		suspends = a.expressionReaches(node.Right, context, record) || suspends
	case *ir.Range:
		suspends = a.expressionReaches(node.Start, context, record)
		suspends = a.expressionReaches(node.End, context, record) || suspends
	case *ir.Transform:
		suspends = a.expressionReaches(node.Source, context, record)
		suspends = a.expressionReaches(node.Initial, context, record) || suspends
		suspends = a.expressionReaches(node.Limit, context, record) || suspends
		suspends = a.statementsReach(node.Body, context, record) || suspends
		suspends = a.expressionReaches(node.Result, context, record) || suspends
		if a.options.Transform != nil && a.options.Transform(node) {
			suspends = true
		}
	case *ir.RecordConstruct:
		suspends = a.expressionReaches(node.Target, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		suspends = a.recordConstructReaches(node, context, record) || suspends
	case *ir.Call:
		suspends = a.expressionReaches(node.Callee, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		if node.Block != nil {
			suspends = a.statementsReach(node.Block.Body, context, record) || suspends
		}
		parameterDefaultsReach := a.callTargetParameterDefaults(node.Callee, context)
		if record && parameterDefaultsReach {
			a.plan.CallParameterDefaults[node] = true
		}
		callSuspends := a.intrinsicReaches(referenceIntrinsic(node.Callee)) || a.runtimeReaches(referenceRuntime(node.Callee)) || a.options.WebNext && isWebNextCall(node.Callee) || a.callTargetReaches(node, node.Callee, context, record)
		if record && callSuspends {
			a.plan.Calls[node] = true
		}
		suspends = callSuspends || suspends
	case *ir.EnumConstruct:
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
	case *ir.EnumCall:
		suspends = a.expressionReaches(node.Receiver, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		classMember := false
		owner := effectDeclarationIdentity(node.OwnerIdentity, context.module, "")
		exactOwner := !node.OwnerIdentity.Empty()
		if node.Reference != nil && !node.Reference.Dispatch.Empty() {
			owner = effectDeclarationIdentity(node.Reference.Dispatch.Owner, context.module, "")
			classMember = node.Reference.Dispatch.Class
			exactOwner = true
		} else if node.Reference != nil && !node.Reference.Declaration.Empty() {
			owner = effectDeclarationIdentity(node.Reference.Declaration, context.module, "")
			classMember = node.Reference.ClassMember
			exactOwner = true
		}
		if !exactOwner {
			ownerName := node.Owner
			if ownerName == "" {
				ownerName = node.EnumName
			}
			owner = declarationIdentity{module: context.module, name: ownerName}
		}
		if !exactOwner && node.Reference != nil && (node.Reference.Package != "" || node.Reference.Owner != "") {
			if node.Reference.Package != "" {
				owner.module = node.Reference.Package
			}
			if node.Reference.Owner != "" {
				owner.name = node.Reference.Owner
			}
			classMember = node.Reference.ClassMember
		} else if !exactOwner && node.Owner == "" {
			owner = a.declarationIdentityInNamespace(context.module, context.namespace, node.EnumName)
		}
		owner = a.canonicalDeclarationIdentity(owner)
		targets := a.memberMethods[dispatchIdentity{owner: owner, name: node.Method, class: classMember}]
		if record && anyReached(a.plan.ParameterDefaults, targets) {
			a.plan.EnumCallDefaults[node] = true
		}
		targetSuspends := anyReached(a.plan.Methods, targets)
		if record && targetSuspends {
			a.plan.EnumCalls[node] = true
		}
		suspends = targetSuspends || suspends
	case *ir.TypeApply:
		suspends = a.expressionReaches(node.Receiver, context, record)
	case *ir.Member:
		suspends = a.expressionReaches(node.Receiver, context, record)
	case *ir.Index:
		suspends = a.expressionReaches(node.Receiver, context, record)
		suspends = a.expressionReaches(node.Index, context, record) || suspends
	case *ir.Block:
		suspends = a.statementsReach(node.Body, context, record)
	case *ir.If:
		suspends = a.statementsReach([]ir.Statement{node}, context, record)
	case *ir.Case:
		suspends = a.statementsReach([]ir.Statement{node}, context, record)
	}
	if record && suspends {
		a.plan.Expressions[expression] = true
	}
	return suspends
}

func (a *analyzer) callTargetReaches(call *ir.Call, callee ir.Expression, context methodContext, record bool) bool {
	if identity, ok := a.constructorIdentity(callee, context); ok {
		if class := a.classDefinitions[identity]; class != nil {
			return a.plan.ClassConstructors[class.class]
		}
	}
	if a.options.PassToFunctions && callee != nil && callee.ExprType().Kind == types.Function {
		if lambda, ok := callee.(*ir.Lambda); ok {
			return a.plan.Lambdas[lambda]
		}
		if identifier, ok := callee.(*ir.Identifier); ok {
			if lambda := a.lambdaBindings[functionBindingKey{module: context.module, method: context.method, name: identifier.Name}]; lambda != nil {
				return a.plan.Lambdas[lambda]
			}
			if context.fieldValues[identifier.Name] {
				return true
			}
			if context.method != nil {
				for _, parameter := range context.method.Parameters {
					if parameter.Name == identifier.Name {
						// The checked callee type above is authoritative. A source
						// parameter can retain a transparent function alias in IR.
						// A higher-order function must accept both synchronous and
						// backend-suspending callbacks.
						return true
					}
				}
			}
		}
	}
	switch node := callee.(type) {
	case *ir.TypeApply:
		return a.callTargetReaches(call, node.Receiver, context, record)
	case *ir.Identifier:
		if !node.Dispatch.Empty() {
			owner := effectDeclarationIdentity(node.Dispatch.Owner, context.module, "")
			return anyReached(a.plan.Methods, a.memberMethodsFor(owner, node.Dispatch.Name, node.Dispatch.Class))
		}
		if !node.Declaration.Empty() && node.Declaration.Kind == identity.Function {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(node.Declaration.Module, node.Declaration.Name)])
		}
		if node.Reference != nil && !node.Reference.Declaration.Empty() && node.Reference.Declaration.Kind == identity.Function {
			declaration := node.Reference.Declaration
			return anyReached(a.plan.Methods, a.topMethods[callableKey(declaration.Module, declaration.Name)])
		}
		if node.Reference != nil && node.Reference.Package != "" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		classMember := context.method != nil && context.method.Class
		if !context.owner.empty() && anyReached(a.plan.Methods, a.memberMethodsFor(context.owner, node.Name, classMember)) {
			return true
		}
		return anyReached(a.plan.Methods, a.topMethods[callableKey(context.module, node.Name)])
	case *ir.Member:
		if node.Reference != nil && node.Reference.Intrinsic != "" {
			return false
		}
		if owner, classMember, ok := a.memberTargetIdentity(node, context); ok {
			candidates := a.memberMethodsFor(owner, node.Name, classMember)
			if len(candidates) > 0 {
				return anyReached(a.plan.Methods, candidates)
			}
		}
		if node.Reference != nil && node.Reference.Package != "" && node.Reference.Owner == "" && node.Reference.ExportKind == "function" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		if member, ok := node.Receiver.(*ir.Identifier); ok && member.Reference != nil && member.Reference.Package != "" && member.Reference.ExportKind == "" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(member.Reference.Package, node.Name)])
		}
		return false
	default:
		return false
	}
}

func (a *analyzer) recordConstructReaches(construction *ir.RecordConstruct, context methodContext, record bool) bool {
	declaration := effectDeclarationIdentity(construction.Declaration, context.module, "")
	if construction.Declaration.Empty() {
		if resolved, _, ok := a.expressionDeclarationIdentity(construction.Target, context); ok {
			declaration = resolved
		}
	}
	declaration = a.canonicalDeclarationIdentity(declaration)
	omittedDefaultsReach, targetHasEffects := a.recordConstructDefaultsReach(construction, a.recordDefinitions[declaration])
	if record {
		if omittedDefaultsReach {
			a.plan.RecordConstructDefaults[construction] = true
		} else if targetHasEffects {
			a.plan.RecordConstructSync[construction] = true
		}
	}
	return omittedDefaultsReach
}

func (a *analyzer) callTargetParameterDefaults(callee ir.Expression, context methodContext) bool {
	if identity, ok := a.constructorIdentity(callee, context); ok {
		class := a.classDefinitions[identity]
		return class != nil && class.initialize != nil && a.plan.ParameterDefaults[class.initialize]
	}
	switch node := callee.(type) {
	case *ir.TypeApply:
		return a.callTargetParameterDefaults(node.Receiver, context)
	case *ir.Identifier:
		if !node.Dispatch.Empty() {
			owner := effectDeclarationIdentity(node.Dispatch.Owner, context.module, "")
			return anyReached(a.plan.ParameterDefaults, a.memberMethodsFor(owner, node.Dispatch.Name, node.Dispatch.Class))
		}
		if !node.Declaration.Empty() && node.Declaration.Kind == identity.Function {
			return anyReached(a.plan.ParameterDefaults, a.topMethods[callableKey(node.Declaration.Module, node.Declaration.Name)])
		}
		if node.Reference != nil && !node.Reference.Declaration.Empty() && node.Reference.Declaration.Kind == identity.Function {
			declaration := node.Reference.Declaration
			return anyReached(a.plan.ParameterDefaults, a.topMethods[callableKey(declaration.Module, declaration.Name)])
		}
		if node.Reference != nil && node.Reference.Package != "" {
			return anyReached(a.plan.ParameterDefaults, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		classMember := context.method != nil && context.method.Class
		if !context.owner.empty() && anyReached(a.plan.ParameterDefaults, a.memberMethodsFor(context.owner, node.Name, classMember)) {
			return true
		}
		return anyReached(a.plan.ParameterDefaults, a.topMethods[callableKey(context.module, node.Name)])
	case *ir.Member:
		if owner, classMember, ok := a.memberTargetIdentity(node, context); ok {
			candidates := a.memberMethodsFor(owner, node.Name, classMember)
			if len(candidates) > 0 {
				return anyReached(a.plan.ParameterDefaults, candidates)
			}
		}
		if node.Reference != nil && node.Reference.Package != "" && node.Reference.Owner == "" && node.Reference.ExportKind == "function" {
			return anyReached(a.plan.ParameterDefaults, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		return false
	}
	return false
}

func (a *analyzer) memberMethodsFor(owner declarationIdentity, name string, class bool) []*ir.Method {
	if !a.moduleDeclarations[owner] {
		owner = a.canonicalDeclarationIdentity(owner)
	}
	if a.classDefinitions[owner] != nil {
		return a.inheritedMemberMethods(owner, name, class)
	}
	return a.memberMethods[dispatchIdentity{owner: owner, name: name, class: class}]
}

func (a *analyzer) memberTargetIdentity(member *ir.Member, context methodContext) (declarationIdentity, bool, bool) {
	if !member.Dispatch.Empty() {
		owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(member.Dispatch.Owner, context.module, ""))
		return owner, member.Dispatch.Class, true
	}
	if member.Reference != nil && !member.Reference.Dispatch.Empty() {
		owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(member.Reference.Dispatch.Owner, context.module, ""))
		return owner, member.Reference.Dispatch.Class, true
	}
	if member.Reference != nil && member.Reference.Owner != "" {
		module := context.module
		if member.Reference.Package != "" {
			module = member.Reference.Package
		}
		owner := a.canonicalDeclarationIdentity(declarationIdentity{module: module, name: member.Reference.Owner})
		return owner, member.Reference.ClassMember, true
	}
	return a.expressionDeclarationIdentity(member.Receiver, context)
}

func (a *analyzer) expressionDeclarationIdentity(expression ir.Expression, context methodContext) (declarationIdentity, bool, bool) {
	switch node := expression.(type) {
	case *ir.TypeApply:
		if !node.Dispatch.Empty() {
			owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Dispatch.Owner, context.module, ""))
			return owner, node.Dispatch.Class, true
		}
		if !node.Declaration.Empty() {
			declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Declaration, context.module, ""))
			return declaration, true, true
		}
		return a.expressionDeclarationIdentity(node.Receiver, context)
	case *ir.Identifier:
		if node.Name == "self" && !context.owner.empty() {
			return context.owner, context.method != nil && context.method.Class, true
		}
		if !node.Dispatch.Empty() {
			owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Dispatch.Owner, context.module, ""))
			return owner, node.Dispatch.Class, true
		}
		if !node.Declaration.Empty() {
			declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Declaration, context.module, node.Name))
			return declaration, true, true
		}
		if node.Reference != nil {
			if !node.Reference.Dispatch.Empty() {
				owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Reference.Dispatch.Owner, context.module, ""))
				return owner, node.Reference.Dispatch.Class, true
			}
			if !node.Reference.Declaration.Empty() {
				declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Reference.Declaration, context.module, node.Name))
				return declaration, true, true
			}
			if node.Reference.Owner != "" {
				module := context.module
				if node.Reference.Package != "" {
					module = node.Reference.Package
				}
				identity := a.canonicalDeclarationIdentity(declarationIdentity{module: module, name: node.Reference.Owner})
				return identity, true, true
			}
			if node.Reference.Package != "" {
				name := node.Name
				if node.Reference.Symbol != "" {
					name = node.Reference.Symbol
				}
				identity := a.canonicalDeclarationIdentity(declarationIdentity{module: node.Reference.Package, name: name})
				if a.declarations[identity] || declarationExportKind(node.Reference.ExportKind) {
					return identity, true, true
				}
			}
		}
		if !node.Lexical {
			identity := a.runtimeDeclarationIdentityInNamespace(context.module, context.namespace, node.Name)
			if a.declarations[identity] {
				return identity, true, true
			}
		}
	case *ir.Member:
		if !node.Dispatch.Empty() {
			owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Dispatch.Owner, context.module, ""))
			return owner, node.Dispatch.Class, true
		}
		if !node.Declaration.Empty() {
			declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Declaration, context.module, node.Name))
			return declaration, true, true
		}
		if node.Reference != nil && !node.Reference.Dispatch.Empty() {
			owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Reference.Dispatch.Owner, context.module, ""))
			return owner, node.Reference.Dispatch.Class, true
		}
		if node.Reference != nil && !node.Reference.Declaration.Empty() {
			declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Reference.Declaration, context.module, node.Name))
			return declaration, true, true
		}
		if node.Reference != nil && node.Reference.Owner != "" {
			module := context.module
			if node.Reference.Package != "" {
				module = node.Reference.Package
			}
			identity := a.canonicalDeclarationIdentity(declarationIdentity{module: module, name: node.Reference.Owner})
			return identity, true, true
		}
		if node.Reference != nil && node.Reference.Package != "" {
			name := node.Name
			if node.Reference.Symbol != "" {
				name = node.Reference.Symbol
			}
			identity := a.canonicalDeclarationIdentity(declarationIdentity{module: node.Reference.Package, name: name})
			if a.declarations[identity] || declarationExportKind(node.Reference.ExportKind) {
				return identity, true, true
			}
		}
		if node.Namespace {
			identity := a.runtimeDeclarationIdentityInNamespace(context.module, context.namespace, qualifiedExpressionName(node))
			if a.declarations[identity] {
				return identity, true, true
			}
		}
	case *ir.RecordConstruct:
		if !node.Declaration.Empty() {
			declaration := a.canonicalDeclarationIdentity(effectDeclarationIdentity(node.Declaration, context.module, ""))
			return declaration, false, true
		}
		return a.expressionDeclarationIdentity(node.Target, context)
	case *ir.Call:
		if identity, ok := a.constructorIdentity(node.Callee, context); ok {
			return identity, false, true
		}
	}
	if expression == nil || expression.ExprType().Name == "" {
		return declarationIdentity{}, false, false
	}
	if !expression.ExprType().Declaration.Empty() {
		identity := a.canonicalDeclarationIdentity(effectDeclarationIdentity(expression.ExprType().Declaration, context.module, expression.ExprType().Name))
		if a.declarations[identity] {
			return identity, false, true
		}
	}
	identity := a.declarationIdentityInNamespace(context.module, context.namespace, expression.ExprType().Name)
	if !a.declarations[identity] {
		return declarationIdentity{}, false, false
	}
	return identity, false, true
}

func declarationExportKind(kind string) bool {
	switch kind {
	case "class", "record", "interface", "enum", "type_alias", "enum_alias", "newtype":
		return true
	default:
		return false
	}
}

func (a *analyzer) constructorIdentity(callee ir.Expression, context methodContext) (declarationIdentity, bool) {
	member, ok := callee.(*ir.Member)
	if !ok || member.Name != "new" {
		return declarationIdentity{}, false
	}
	identity, classMember, ok := a.expressionDeclarationIdentity(member.Receiver, context)
	if !ok || !classMember {
		return declarationIdentity{}, false
	}
	return a.canonicalDeclarationIdentity(identity), true
}

func referenceIntrinsic(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Reference != nil {
			return node.Reference.Intrinsic
		}
	case *ir.Member:
		if node.Reference != nil {
			return node.Reference.Intrinsic
		}
	case *ir.TypeApply:
		return referenceIntrinsic(node.Receiver)
	}
	return ""
}

func referenceRuntime(expression ir.Expression) *ir.RuntimeBinding {
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Reference != nil {
			return node.Reference.Runtime
		}
	case *ir.Member:
		if node.Reference != nil {
			return node.Reference.Runtime
		}
	case *ir.TypeApply:
		return referenceRuntime(node.Receiver)
	}
	return nil
}

func (a *analyzer) intrinsicReaches(intrinsic string) bool {
	return a.options.Intrinsic != nil && a.options.Intrinsic(intrinsic)
}

func (a *analyzer) runtimeReaches(binding *ir.RuntimeBinding) bool {
	return binding != nil && a.options.Runtime != nil && a.options.Runtime(binding)
}

// ORMOperation identifies intrinsics that may execute database work. It is
// shared by TypeScript suspension and portable execution-scope propagation.
func ORMOperation(intrinsic string) bool {
	return runtimeoperation.ORMExecution(intrinsic)
}

// ExecutionScope computes the functions that must receive the compiler-owned
// cancellation/deadline scope. The list contains operations whose native
// implementations can observe cancellation, plus web runtime boundaries that
// forward a child scope.
func ExecutionScope(programs []*ir.Program) *Plan {
	return Analyze(programs, Options{
		WebNext: true, CaptureLambdas: true,
		Intrinsic: func(intrinsic string) bool {
			return runtimeoperation.Describe(intrinsic).PropagatesExecutionScope
		},
		Runtime: func(binding *ir.RuntimeBinding) bool {
			return binding.PropagatesExecutionScope
		},
		Transform: func(transform *ir.Transform) bool {
			return transform.Operation == "concurrent_map"
		},
	})
}

func isWebNextCall(callee ir.Expression) bool {
	member, ok := callee.(*ir.Member)
	return ok && member.Name == "call" && member.Receiver.ExprType().Name == "Next"
}
