package nativepackage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type indexRequest struct {
	Modules []string `json:"modules"`
}

type indexResult struct {
	TypeScriptVersion string            `json:"typescriptVersion"`
	Modules           map[string]Module `json:"modules"`
}

func Generate(root, packageManager string, dependencies map[string]string) (*Catalog, error) {
	modules := make([]string, 0, len(dependencies))
	for name := range dependencies {
		modules = append(modules, name)
	}
	return GenerateModules(root, packageManager, dependencies, modules)
}

func GenerateModules(root, packageManager string, dependencies map[string]string, requestedModules []string) (*Catalog, error) {
	catalog := Empty(dependencies)
	if len(dependencies) == 0 {
		return catalog, nil
	}
	seen := map[string]bool{}
	modules := make([]string, 0, len(dependencies)+len(requestedModules))
	for name := range dependencies {
		seen[name] = true
		modules = append(modules, name)
	}
	for _, name := range requestedModules {
		if !seen[name] && catalog.Owns(name) {
			seen[name] = true
			modules = append(modules, name)
		}
	}
	sort.Strings(modules)
	requestData, err := json.Marshal(indexRequest{Modules: modules})
	if err != nil {
		return nil, err
	}
	temporaryDirectory, err := os.MkdirTemp("", "trb-native-types-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(temporaryDirectory) }()
	scriptPath := filepath.Join(temporaryDirectory, "index.cjs")
	requestPath := filepath.Join(temporaryDirectory, "request.json")
	if err := os.WriteFile(scriptPath, []byte(typeScriptIndexer), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(requestPath, requestData, 0o600); err != nil {
		return nil, err
	}
	var command *exec.Cmd
	switch packageManager {
	case "bun":
		command = exec.Command("bun", scriptPath, requestPath)
	case "npm":
		command = exec.Command("node", scriptPath, requestPath)
	default:
		return nil, fmt.Errorf("unsupported TypeScript package manager %q", packageManager)
	}
	command.Dir = root
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("index native TypeScript package types: %s", message)
	}
	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()
	var result indexResult
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode native TypeScript package indexer output: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode native TypeScript package indexer output: trailing JSON content")
	}
	catalog.TypeScriptVersion = result.TypeScriptVersion
	catalog.Modules = result.Modules
	return catalog, nil
}

const typeScriptIndexer = `
const fs = require("node:fs");
const path = require("node:path");
const { createRequire } = require("node:module");

const request = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const localRequire = createRequire(path.join(process.cwd(), "package.json"));
let ts;
try {
	ts = localRequire("typescript");
} catch (error) {
	throw new Error("TypeScript is not installed in this project; run trb install after adding it to devDependencies");
}
const typeScriptMajor = Number.parseInt(ts.version.split(".")[0], 10);
if (typeScriptMajor !== 6) {
	throw new Error(
		"TypeRB native package indexing supports TypeScript 6.x; found " + ts.version +
		". Set devDependencies.typescript to \"^6.0.0\" and run trb install"
	);
}

const compilerOptions = {
	target: ts.ScriptTarget.ESNext,
	module: ts.ModuleKind.ESNext,
	moduleResolution: ts.ModuleResolutionKind.Bundler,
	jsx: ts.JsxEmit.ReactJSX,
	strict: true,
	skipLibCheck: true,
	allowJs: true,
};
const containingFile = path.join(process.cwd(), "__trb_native_types__.tsx");
const resolutions = new Map();
for (const moduleName of request.modules) {
	const resolved = ts.resolveModuleName(moduleName, containingFile, compilerOptions, ts.sys).resolvedModule;
	if (resolved) resolutions.set(moduleName, resolved.resolvedFileName);
}
const program = ts.createProgram([...resolutions.values()], compilerOptions);
const checker = program.getTypeChecker();
const modules = {};

function wire(kind, name = "", args = [], nullable = false) {
	const result = { kind };
	if (name) result.name = name;
	if (args.length) result.args = args;
	if (nullable) result.nullable = true;
	return result;
}

function sameType(left, right) {
	return JSON.stringify(left) === JSON.stringify(right);
}

function nullable(type) {
	return { ...type, nullable: true };
}

function sourceLooksReact(symbol) {
	return !!symbol?.declarations?.some((declaration) => /(?:@types[/\\]react|react[/\\])/.test(declaration.getSourceFile().fileName));
}

function reactType(type) {
	const symbol = type.aliasSymbol || type.getSymbol?.();
	const name = symbol?.getName?.() || "";
	if (name === "ReactNode" && sourceLooksReact(symbol)) return wire("named", "ReactNode");
	if ((name === "Element" || name === "ReactElement") && sourceLooksReact(symbol)) return wire("named", "ReactNode");
	const text = checker.typeToString(type);
	for (const eventName of ["MouseEvent", "ChangeEvent", "FormEvent", "KeyboardEvent", "SyntheticEvent"]) {
		if (text.startsWith(eventName + "<") || text.startsWith("React." + eventName + "<")) {
			return wire("named", eventName);
		}
	}
	return null;
}

function portableType(type, state, depth = 0) {
	if (depth > 12) return { error: "type nesting exceeds the native bridge limit" };
	const react = reactType(type);
	if (react) return { type: react };
	if (type.flags & ts.TypeFlags.Any) return { error: "uses TypeScript any" };
	if (type.flags & ts.TypeFlags.Unknown) return { error: "uses TypeScript unknown" };
	if (type.flags & ts.TypeFlags.Never) return { error: "uses TypeScript never" };
	if (type.flags & ts.TypeFlags.Void) return { type: wire("void", "Void") };
	if (type.flags & ts.TypeFlags.StringLike) return { type: wire("string", "String") };
	if (type.flags & ts.TypeFlags.NumberLike) return { type: wire("float", "Float") };
	if (type.flags & ts.TypeFlags.BooleanLike) return { type: wire("bool", "Boolean") };
	if (type.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined)) return { type: wire("nil", "Nil") };
	if (type.isUnion?.()) {
		let isNullable = false;
		const alternatives = [];
		for (const alternative of type.types) {
			if (alternative.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined)) {
				isNullable = true;
				continue;
			}
			if (alternative.flags & ts.TypeFlags.Never) continue;
			const converted = portableType(alternative, state, depth + 1);
			if (converted.error) return converted;
			if (!alternatives.some((existing) => sameType(existing, converted.type))) alternatives.push(converted.type);
		}
		if (alternatives.length === 0) return { error: "contains no representable union alternative" };
		if (alternatives.length === 1) return { type: isNullable ? nullable(alternatives[0]) : alternatives[0] };
		return { type: wire("union", "Union", alternatives, isNullable) };
	}
	if (checker.isArrayType(type)) {
		const arguments = checker.getTypeArguments(type);
		if (arguments.length !== 1) return { error: "has an unsupported Array shape" };
		const element = portableType(arguments[0], state, depth + 1);
		if (element.error) return element;
		return { type: wire("array", "Array", [element.type]) };
	}
	if (checker.isTupleType(type)) return { error: "uses a TypeScript tuple" };
	const signatures = checker.getSignaturesOfType(type, ts.SignatureKind.Call);
	if (signatures.length > 0) {
		if (signatures.length !== 1) return { error: "uses overloaded call signatures" };
		const signature = signatures[0];
		if (signature.typeParameters?.length) return { error: "uses generic call signatures" };
		const parameters = [];
		for (const parameter of signature.getParameters()) {
			if (parameter.flags & ts.SymbolFlags.Optional) return { error: "uses optional callback parameters" };
			const declaration = parameter.valueDeclaration || parameter.declarations?.[0];
			if (declaration?.dotDotDotToken) return { error: "uses rest callback parameters" };
			const converted = portableType(checker.getTypeOfSymbolAtLocation(parameter, declaration || state.fallbackNode), state, depth + 1);
			if (converted.error) return converted;
			parameters.push(converted.type);
		}
		const returned = portableType(checker.getReturnTypeOfSignature(signature), state, depth + 1);
		if (returned.error) return returned;
		return { type: wire("function", "Function", [...parameters, returned.type]) };
	}
	return { error: "uses unsupported TypeScript type " + checker.typeToString(type) };
}

function recordFields(type, state, hint) {
	const alternatives = type.isUnion?.() ? type.types.filter((item) => !(item.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined | ts.TypeFlags.Never))) : [type];
	const names = new Set();
	for (const alternative of alternatives) {
		for (const property of checker.getPropertiesOfType(alternative)) names.add(property.getName());
	}
	if (names.size > 512) return { error: "object exposes more than 512 properties" };
	const fields = [];
	const unsupportedFields = {};
	for (const name of [...names].sort()) {
		const convertedTypes = [];
		let optional = false;
		let issue = "";
		for (const alternative of alternatives) {
			const property = checker.getPropertyOfType(alternative, name);
			if (!property) {
				optional = true;
				continue;
			}
			optional ||= !!(property.flags & ts.SymbolFlags.Optional);
			const declaration = property.valueDeclaration || property.declarations?.[0];
			const converted = portableType(checker.getTypeOfSymbolAtLocation(property, declaration || state.fallbackNode), state, 1);
			if (converted.error) {
				issue = converted.error;
				break;
			}
			if (!convertedTypes.some((existing) => sameType(existing, converted.type))) convertedTypes.push(converted.type);
		}
		if (issue || convertedTypes.length === 0) {
			unsupportedFields[name] = issue || "has no representable type";
			continue;
		}
		let fieldType = convertedTypes.length === 1 ? convertedTypes[0] : wire("union", "Union", convertedTypes);
		if (optional && !fieldType.nullable) fieldType = nullable(fieldType);
		fields.push({ name, type: fieldType, ...(optional ? { optional: true } : {}) });
	}
	return { fields, unsupportedFields };
}

function isRenderable(type) {
	if (reactType(type)?.name === "ReactNode") return true;
	if (type.isUnion?.()) return type.types.every((item) => item.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined) || isRenderable(item));
	return false;
}

function moduleSlug(name) {
	let result = "";
	for (const character of name) {
		result += /[A-Za-z0-9]/.test(character) ? character : "_" + character.codePointAt(0).toString(16) + "_";
	}
	return result || "Package";
}

function componentExport(type, state, exportName) {
	let signature = checker.getSignaturesOfType(type, ts.SignatureKind.Call).find((candidate) => isRenderable(checker.getReturnTypeOfSignature(candidate)));
	if (!signature) {
		for (const candidate of checker.getSignaturesOfType(type, ts.SignatureKind.Construct)) {
			const instance = checker.getReturnTypeOfSignature(candidate);
			if (checker.getPropertyOfType(instance, "render")) {
				signature = candidate;
				break;
			}
		}
	}
	if (!signature) return null;
	const parameters = signature.getParameters();
	if (parameters.length > 1) return { error: "React component has more than one props parameter" };
	if (parameters.length === 0) return { export: { kind: "component", type: wire("named", "ReactNode") } };
	const parameter = parameters[0];
	const declaration = parameter.valueDeclaration || parameter.declarations?.[0];
	const propsType = checker.getTypeOfSymbolAtLocation(parameter, declaration || state.fallbackNode);
	const props = recordFields(propsType, state, exportName + "Props");
	if (props.error) return props;
	const recordName = "Native_" + state.moduleSlug + "_" + exportName + "Props";
	state.records[recordName] = { kind: "record", type: wire("named", recordName), fields: props.fields, unsupportedFields: props.unsupportedFields };
	return {
		export: {
			kind: "component",
			type: wire("named", "ReactNode"),
			parameters: [wire("named", recordName)],
			required: 1,
			unsupportedFields: props.unsupportedFields,
		},
	};
}

function ordinaryExport(type, state, exportName) {
	const signatures = checker.getSignaturesOfType(type, ts.SignatureKind.Call);
	if (signatures.length !== 1) return { error: signatures.length > 1 ? "uses overloaded call signatures" : "is not a supported function or React component" };
	const signature = signatures[0];
	if (signature.typeParameters?.length) return { error: "uses generic call signatures" };
	const parameters = [];
	let required = 0;
	let variadic = false;
	for (const parameter of signature.getParameters()) {
		const declaration = parameter.valueDeclaration || parameter.declarations?.[0];
		variadic ||= !!declaration?.dotDotDotToken;
		if (!(parameter.flags & ts.SymbolFlags.Optional) && !declaration?.questionToken && !declaration?.initializer && !declaration?.dotDotDotToken) required++;
		const converted = portableType(checker.getTypeOfSymbolAtLocation(parameter, declaration || state.fallbackNode), state, 0);
		if (converted.error) return { error: "parameter " + parameter.getName() + " " + converted.error };
		parameters.push(converted.type);
	}
	const returned = portableType(checker.getReturnTypeOfSignature(signature), state, 0);
	if (returned.error) return { error: "return type " + returned.error };
	return { export: { kind: "function", type: returned.type, parameters, required, ...(variadic ? { variadic: true } : {}) } };
}

for (const moduleName of request.modules) {
	const resolved = resolutions.get(moduleName);
	if (!resolved) {
		modules[moduleName] = { exports: {}, unsupported: { "*": "TypeScript could not resolve this package" } };
		continue;
	}
	const source = program.getSourceFile(resolved);
	const moduleSymbol = source && checker.getSymbolAtLocation(source);
	if (!moduleSymbol) {
		modules[moduleName] = { exports: {}, unsupported: { "*": "TypeScript could not read this package declaration" } };
		continue;
	}
	const state = { records: {}, moduleSlug: moduleSlug(moduleName), fallbackNode: source };
	const exports = {};
	const unsupported = {};
	for (const original of checker.getExportsOfModule(moduleSymbol).sort((left, right) => left.getName().localeCompare(right.getName()))) {
		const exportName = original.getName();
		if (exportName === "default") continue;
		let symbol = original;
		if (symbol.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
		const declaration = symbol.valueDeclaration || symbol.declarations?.[0] || source;
		const type = checker.getTypeOfSymbolAtLocation(symbol, declaration);
		const component = componentExport(type, state, exportName);
		const converted = component || ordinaryExport(type, state, exportName);
		if (converted.error) unsupported[exportName] = converted.error;
		else exports[exportName] = converted.export;
	}
	modules[moduleName] = { exports, ...(Object.keys(state.records).length ? { records: state.records } : {}), ...(Object.keys(unsupported).length ? { unsupported } : {}) };
}

process.stdout.write(JSON.stringify({ typescriptVersion: ts.version, modules }));
`
