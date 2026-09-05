package golang

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	if name == "trb.cli.run" {
		return g.cliRun(call)
	}
	if name == "trb.cli.fail" {
		g.cliFailure = true
		failure := "panic(" + g.cliApplicationFailureTypeName() + "(" + arguments[0] + "))"
		if call.ExprType().Kind == types.Never {
			return failure
		}
		return "func() " + g.goType(call.ExprType()) + " { " + failure + " }()"
	}
	if generated, ok := g.testIntrinsic(name, call, arguments); ok {
		return generated
	}
	if generated, ok := g.oidcIntrinsic(name, call, arguments); ok {
		return generated
	}
	if value, ok := g.timeIntrinsic(name, call, arguments); ok {
		return value
	}
	if name == "trb.internal.runtime.fail" {
		if call.ExprType().Kind == types.Never {
			return "panic(" + arguments[0] + ")"
		}
		return "func() " + g.goType(call.ExprType()) + " { panic(" + arguments[0] + ") }()"
	}
	if name == "trb.jobs.perform_later" || name == "trb.jobs.perform_in" || name == "trb.jobs.perform_at" {
		return g.jobsPerformLater(call, arguments)
	}
	if name == "trb.jobs.sql.enqueue" || name == "trb.jobs.sql.enqueue_at" {
		return g.jobsAdapterEnqueue(name, call, arguments)
	}
	unicodeAlias := "unicode"
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Alias != "" {
		unicodeAlias = goImportAlias(reference.Alias)
	}
	unicodeCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return unicodeAlias + "." + goMethodName(symbol)
		}
		return unicodeAlias + ".Unicode" + goMethodName(symbol)
	}
	filesystemResultType := func() (string, string, string) {
		result := call.ExprType()
		if len(result.Args) != 2 {
			return g.goType(result), "any", "any"
		}
		return g.goType(result), g.goType(result.Args[0]), g.goType(result.Args[1])
	}
	filesystemOK := func(value string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.filesystemResultAlias()
		return alias + ".NewResultOk[" + successType + ", " + errorType + "](" + value + ")"
	}
	resultError := func(value string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.filesystemResultAlias()
		return alias + ".NewResultErr[" + successType + ", " + errorType + "](" + value + ")"
	}
	numberParseError := func(kind, input, message string) string {
		alias := g.typeAliases["NumberParseErrorKind"]
		if alias == "" {
			alias = "__trb_errors"
		}
		value := g.goType(types.FromName("NumberParseError")) + "{Kind: " + alias + "." + goConstantIdentifier("NumberParseErrorKind", kind) + ", Input: " + input + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	hexDecodeError := func(kind, input, index, message string) string {
		alias := g.typeAliases["Hex::DecodeErrorKind"]
		kindName := goConstantIdentifier("Hex::DecodeErrorKind", kind)
		if alias != "" {
			kindName = alias + "." + kindName
		}
		value := g.goType(types.FromName("Hex::DecodeError")) + "{Kind: " + kindName + ", Input: " + input + ", Index: " + index + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	base64DecodeError := func(kind, input, index, message string) string {
		alias := g.typeAliases["Base64::DecodeErrorKind"]
		kindName := goConstantIdentifier("Base64::DecodeErrorKind", kind)
		if alias != "" {
			kindName = alias + "." + kindName
		}
		value := g.goType(types.FromName("Base64::DecodeError")) + "{Kind: " + kindName + ", Input: " + input + ", Index: " + index + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	percentDecodeError := func(kind, input, message string) string {
		alias := g.typeAliases["URL::DecodeErrorKind"]
		kindName := goConstantIdentifier("URL::DecodeErrorKind", kind)
		if alias != "" {
			kindName = alias + "." + kindName
		}
		value := g.goType(types.FromName("URL::DecodeError")) + "{Kind: " + kindName + ", Input: " + input + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	indexLookupError := func(index, size, message string) string {
		value := g.goType(types.FromName("IndexLookupError")) + "{Index: " + index + ", Size: " + size + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	sliceRangeError := func(start, end, exclusive, size, message string) string {
		value := g.goType(types.FromName("SliceRangeError")) + "{Start: " + start + ", Finish: " + end + ", Exclusive: " + exclusive + ", Size: " + size + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}
	keyLookupError := func(key, message string) string {
		value := g.goType(types.FromName("KeyLookupError")) + "{Key: " + key + ", Message: " + strconv.Quote(message) + "}"
		return resultError(value)
	}

	processError := func(operation, command, message string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		value := errorType + "{Operation: " + strconv.Quote(operation) + ", Command: " + command + ", Message: " + message + "}"
		return alias + ".NewResultErr[" + successType + ", " + errorType + "](" + value + ")"
	}
	switch name {
	case "trb.std.path.join":
		g.requireImport("runtime", "")
		g.requireImport("strings", "")
		return pathJoinExpression(arguments[0], arguments[1], `runtime.GOOS == "windows"`)
	case "trb.std.io.puts":
		g.requireImport("fmt", "")
		if len(call.Arguments) == 1 {
			argumentType := call.Arguments[0].Value.ExprType()
			if argumentType.Kind == types.Array {
				return "fmt.Println(" + g.portableArrayString(arguments[0], argumentType) + ")"
			}
			if argumentType.Kind == types.Float && argumentType.Nullable {
				return "fmt.Println(func(value *float64) any { if value == nil { return nil }; return " + g.portableFloatString("*value") + " }(" + arguments[0] + "))"
			}
			if argumentType.Kind == types.Float {
				return "fmt.Println(" + g.portableFloatString(arguments[0]) + ")"
			}
			if argumentType.Nullable {
				baseType := argumentType
				baseType.Nullable = false
				goType := g.goType(baseType)
				if goType != "any" && !strings.HasPrefix(goType, "*") {
					return "fmt.Println(func(value *" + goType + ") any { if value == nil { return nil }; return *value }(" + arguments[0] + "))"
				}
			}
		}
		return "fmt.Println(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.url.encode_component":
		g.requireImport("strings", "")
		return "func(value string) string { const hexadecimal = \"0123456789ABCDEF\"; var builder strings.Builder; for _, octet := range []byte(value) { unreserved := octet >= 'A' && octet <= 'Z' || octet >= 'a' && octet <= 'z' || octet >= '0' && octet <= '9' || octet == '-' || octet == '.' || octet == '_' || octet == '~'; if unreserved { builder.WriteByte(octet) } else { builder.WriteByte('%'); builder.WriteByte(hexadecimal[octet>>4]); builder.WriteByte(hexadecimal[octet&15]) } }; return builder.String() }(" + arguments[0] + ")"
	case "trb.std.url.decode_component":
		g.requireImport("unicode/utf8", "utf8")
		resultType, _, _ := filesystemResultType()
		invalidEscape := percentDecodeError("InvalidEscape", "input", "invalid percent escape in URL component")
		invalidUtf8 := percentDecodeError("InvalidUtf8", "input", "decoded URL component is not valid UTF-8")
		return "func() " + resultType + " { input := " + arguments[0] + "; characters := []rune(input); value := make([]byte, 0, len(input)); hexadecimal := func(character rune) (byte, bool) { switch { case character >= '0' && character <= '9': return byte(character - '0'), true; case character >= 'A' && character <= 'F': return byte(character - 'A' + 10), true; case character >= 'a' && character <= 'f': return byte(character - 'a' + 10), true; default: return 0, false } }; for index := 0; index < len(characters); index++ { character := characters[index]; if character != '%' { value = append(value, []byte(string(character))...); continue }; if index+2 >= len(characters) { return " + invalidEscape + " }; high, highOK := hexadecimal(characters[index+1]); low, lowOK := hexadecimal(characters[index+2]); if !highOK || !lowOK { return " + invalidEscape + " }; value = append(value, high<<4|low); index += 2 }; if !utf8.Valid(value) { return " + invalidUtf8 + " }; return " + filesystemOK("string(value)") + " }()"
	case "trb.std.dir.children":
		return g.filesystemChildren(call, arguments)
	case "trb.std.dir.create_all":
		return g.filesystemCreateAll(call, arguments)
	case "trb.std.dir.root_children":
		return g.anchoredChildren(call, arguments)
	case "trb.std.dir.root_create_all":
		return g.anchoredCreateAll(call, arguments)
	case "trb.std.file.read", "trb.std.file.read_text":
		g.requireImport("io", "")
		g.temporary++
		id := strconv.Itoa(g.temporary)
		handle := "__trbFileReadHandle" + id
		limit := "__trbFileReadLimit" + id
		path := "__trbFileReadPath" + id
		data := "__trbFileReadData" + id
		readError := "__trbFileReadError" + id
		resultType, _, _ := filesystemResultType()
		result := call.ExprType()
		errorType := types.FromName("FileSystemError")
		if len(result.Args) == 2 {
			errorType = result.Args[1]
		}
		operation := strings.TrimPrefix(name, "trb.std.file.")
		invalid := g.filesystemResourceError(errorType, operation, path, strconv.Quote("max_bytes must be non-negative"), "InvalidLimit", handle+".Rooted")
		tooLarge := g.filesystemResourceError(errorType, operation, path, strconv.Quote("file exceeds max_bytes"), "TooLarge", handle+".Rooted")
		failure := g.filesystemResourceNativeError(operation, path, readError, handle+".Rooted")
		value := data
		failureCheck := ""
		if name == "trb.std.file.read_text" {
			g.requireImport("unicode/utf8", "utf8")
			invalidEncoding := g.filesystemResourceError(errorType, operation, path, strconv.Quote("file is not valid UTF-8"), "InvalidEncoding", handle+".Rooted")
			value = "string(" + data + ")"
			failureCheck = "if !utf8.Valid(" + data + ") { return " + resultError(invalidEncoding) + " }; "
		}
		return "func() " + resultType + " { " + handle + " := " + arguments[0] + "; " + limit + " := " + arguments[1] + "; " + path + " := " + handle + ".Path; if " + limit + " < 0 { return " + resultError(invalid) + " }; " + data + ", " + readError + " := io.ReadAll(io.LimitReader(" + handle + ", int64(" + limit + ")+1)); if " + readError + " != nil { return " + resultError(failure) + " }; if len(" + data + ") > " + limit + " { return " + resultError(tooLarge) + " }; " + failureCheck + "return " + filesystemOK(value) + " }()"
	case "trb.std.file.write", "trb.std.file.write_text":
		g.requireImport("io", "")
		g.temporary++
		id := strconv.Itoa(g.temporary)
		handle := "__trbFileWriteHandle" + id
		path := "__trbFileWritePath" + id
		data := "__trbFileWriteData" + id
		written := "__trbFileWriteCount" + id
		writeError := "__trbFileWriteError" + id
		resultType, successType, _ := filesystemResultType()
		operation := strings.TrimPrefix(name, "trb.std.file.")
		failure := g.filesystemResourceNativeError(operation, path, writeError, handle+".Rooted")
		value := arguments[1]
		if name == "trb.std.file.write_text" {
			value = "[]byte(" + value + ")"
		}
		return "func() " + resultType + " { " + handle + " := " + arguments[0] + "; " + data + " := " + value + "; " + path + " := " + handle + ".Path; " + written + ", " + writeError + " := " + handle + ".Write(" + data + "); if " + writeError + " == nil && " + written + " != len(" + data + ") { " + writeError + " = io.ErrShortWrite }; if " + writeError + " != nil { return " + resultError(failure) + " }; return " + filesystemOK(successType+"{}") + " }()"
	case "trb.internal.process.arguments":
		g.requireImport("os", "")
		return g.arrayReference("append([]string{}, os.Args[1:]...)")
	case "trb.internal.process.environment":
		g.requireImport("os", "")
		return "func() *string { value, found := os.LookupEnv(" + arguments[0] + "); if !found { return nil }; return &value }()"
	case "trb.internal.process.working_directory":
		g.requireImport("os", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { directory, err := os.Getwd(); if err != nil { return " + processError("working_directory", strconv.Quote(""), "err.Error()") + " }; return " + filesystemOK("directory") + " }()"
	case "trb.internal.process.run":
		g.requireImport("bytes", "")
		g.requireImport("errors", "")
		g.requireImport("os/exec", "exec")
		g.requireImport("strings", "")
		resultType, successType, _ := filesystemResultType()
		value := successType + "{Status: status, Stdout: strings.ToValidUTF8(stdout.String(), \"�\"), Stderr: strings.ToValidUTF8(stderr.String(), \"�\"), Success: status == 0}"
		return "func() " + resultType + " { commandName := " + arguments[0] + "; commandArguments := " + g.arrayValues(arguments[1]) + "; process := exec.Command(commandName, commandArguments...); var stdout bytes.Buffer; var stderr bytes.Buffer; process.Stdout = &stdout; process.Stderr = &stderr; err := process.Run(); status := 0; if err != nil { var exitError *exec.ExitError; if errors.As(err, &exitError) { status = exitError.ExitCode() } else { return " + processError("run", "commandName", "err.Error()") + " } }; return " + filesystemOK(value) + " }()"
	case "trb.internal.json.parse":
		return g.jsonParse(call, arguments[0], false)
	case "trb.internal.json.parse_jsonc":
		if reference := expressionReference(call.Callee); reference != nil && reference.Package == "trb/std/jsonc/index" && strings.TrimSuffix(g.modulePath, "/index") != strings.TrimSuffix(reference.Package, "/index") {
			alias := reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(reference.Package))
			}
			return goImportAlias(alias) + ".Parse(" + arguments[0] + ")"
		}
		return g.jsonParse(call, arguments[0], true)
	case "trb.internal.json.stringify":
		return g.jsonStringify(call, arguments[0])
	case "trb.internal.json.decode":
		return g.jsonDecode(call, arguments[0])
	case "trb.internal.json.encode":
		return g.jsonEncode(call, arguments[0])
	case "trb.web.request_json":
		return g.webRequestJSON(call, arguments[0])
	case "trb.web.request_query":
		return g.webParameterBinding(call, arguments[0], "query")
	case "trb.web.context_params":
		return g.webParameterBinding(call, arguments[0], "path")
	case "trb.web.context_with":
		return g.webContextWith(call, arguments)
	case "trb.web.context_with_request":
		return g.webContextWithRequest(call, arguments)
	case "trb.web.context_fetch":
		return g.webContextFetch(call, arguments)
	case "trb.web.json":
		return g.webJSON(call, arguments)
	case "trb.web.serve":
		webAlias := "web"
		if reference := expressionReference(call.Callee); reference != nil {
			if alias := g.referenceAlias(reference); alias != "" {
				webAlias = alias
			}
		}
		config := webAlias + ".WebServerConfig{Host: \"0.0.0.0\", Port: 3000, BodyLimitBytes: 1048576, ShutdownTimeoutMilliseconds: 10000}"
		if len(arguments) > 0 {
			config = arguments[0]
		}
		return "trbWebServe(" + config + ")"
	case "trb.web.testing.dispatch":
		return "trbWebDispatch(__trbScope, " + arguments[0] + ")"
	case "trb.web.middleware.logger.call":
		return g.webLogger(call, arguments)
	case "trb.web.middleware.timeout.call":
		return g.webTimeout(call, arguments)
	case "trb.web.middleware.compression.gzip":
		return g.webGzip(arguments[0])
	case "trb.orm.where":
		return g.ormWhere(call)
	case "trb.orm.distinct", "trb.orm.query.distinct":
		return g.ormDistinct(call)
	case "trb.orm.select", "trb.orm.query.select":
		return g.ormSelect(call)
	case "trb.orm.group", "trb.orm.query.group":
		return g.ormGroup(call)
	case "trb.orm.group.having":
		return g.ormGroupHaving(call, arguments)
	case "trb.orm.group.count":
		return g.ormGroupCount(call, arguments)
	case "trb.orm.group.sum", "trb.orm.group.average", "trb.orm.group.minimum", "trb.orm.group.maximum":
		return g.ormGroupAggregate(call, arguments, name[strings.LastIndex(name, ".")+1:])
	case "trb.orm.join":
		return g.ormJoin(call, "INNER JOIN")
	case "trb.orm.left_join":
		return g.ormJoin(call, "LEFT JOIN")
	case "trb.orm.where_exists":
		return g.ormWhereExists(call, false)
	case "trb.orm.where_not_exists":
		return g.ormWhereExists(call, true)
	case "trb.orm.using":
		return g.ormUsing(call, arguments)
	case "trb.orm.not":
		return g.ormNot(call)
	case "trb.orm.order":
		return g.ormClassOrder(call)
	case "trb.orm.limit":
		return g.ormClassInteger(call, goORMLimit)
	case "trb.orm.offset":
		return g.ormClassInteger(call, goORMOffset)
	case "trb.orm.lock":
		return g.ormClassTerminal(call, goORMLock)
	case "trb.orm.all":
		return g.ormClassTerminal(call, goORMLoader)
	case "trb.orm.first":
		return g.ormClassTerminal(call, goORMFirst)
	case "trb.orm.count":
		return g.ormClassTerminal(call, goORMCount)
	case "trb.orm.to_sql":
		return g.ormClassTerminal(call, goORMToSQL)
	case "trb.orm.explain":
		return g.ormClassTerminal(call, goORMExplain)
	case "trb.orm.preload":
		return g.ormClassPreload(call)
	case "trb.orm.find_by":
		return g.ormFindBy(call)
	case "trb.orm.exists":
		return g.ormExists(call)
	case "trb.orm.pluck", "trb.orm.query.pluck":
		return g.ormProjection(call, arguments, "pluck")
	case "trb.orm.pick", "trb.orm.query.pick":
		return g.ormProjection(call, arguments, "pick")
	case "trb.orm.ids", "trb.orm.query.ids":
		return g.ormProjection(call, arguments, "ids")
	case "trb.orm.sum", "trb.orm.query.sum":
		return g.ormAggregate(call, arguments, "sum")
	case "trb.orm.average", "trb.orm.query.average":
		return g.ormAggregate(call, arguments, "average")
	case "trb.orm.minimum", "trb.orm.query.minimum":
		return g.ormAggregate(call, arguments, "minimum")
	case "trb.orm.maximum", "trb.orm.query.maximum":
		return g.ormAggregate(call, arguments, "maximum")
	case "trb.orm.find":
		return g.ormFind(call)
	case "trb.orm.build":
		return g.ormBuild(call)
	case "trb.orm.create":
		return g.ormCreate(call)
	case "trb.orm.scope.find":
		return g.ormScopeFind(call, arguments)
	case "trb.orm.scope.build":
		return g.ormScopeBuild(call, arguments)
	case "trb.orm.scope.create":
		return g.ormScopeCreate(call, arguments)
	case "trb.orm.draft.save":
		return g.ormDraftSave(call)
	case "trb.orm.insert_all":
		return g.ormInsertAll(call)
	case "trb.orm.insert_if_absent":
		return g.ormInsertIfAbsent(call)
	case "trb.orm.draft.upsert":
		return g.ormUpsert(call)
	case "trb.orm.upsert_all":
		return g.ormUpsertAll(call)
	case "trb.orm.with":
		return g.ormWith(call)
	case "trb.orm.update":
		return g.ormUpdate(call)
	case "trb.orm.changes.save":
		return g.ormChangesSave(call)
	case "trb.orm.delete":
		return g.ormDelete(call)
	case "trb.orm.destroy":
		return g.ormDestroy(call)
	case "trb.orm.update_all":
		return g.ormClassUpdateAll(call)
	case "trb.orm.delete_all":
		return g.ormClassTerminal(call, goORMDeleteAll)
	case "trb.orm.destroy_all":
		return g.ormClassTerminal(call, goORMDestroyAll)
	case "trb.orm.query.where":
		return g.ormQueryWhere(call, arguments)
	case "trb.orm.query.join":
		return g.ormJoin(call, "INNER JOIN")
	case "trb.orm.query.left_join":
		return g.ormJoin(call, "LEFT JOIN")
	case "trb.orm.query.where_exists":
		return g.ormWhereExists(call, false)
	case "trb.orm.query.where_not_exists":
		return g.ormWhereExists(call, true)
	case "trb.orm.query.not":
		return g.ormQueryNot(call, arguments)
	case "trb.orm.query.or":
		return g.ormQueryOr(call, arguments)
	case "trb.orm.query.find_by":
		return g.ormQueryFindBy(call, arguments)
	case "trb.orm.query.exists":
		return g.ormQueryTerminal(call, arguments, goORMExists)
	case "trb.orm.query.update_all":
		return g.ormQueryUpdateAll(call, arguments)
	case "trb.orm.query.delete_all":
		return g.ormQueryTerminal(call, arguments, goORMDeleteAll)
	case "trb.orm.query.destroy_all":
		return g.ormQueryTerminal(call, arguments, goORMDestroyAll)
	case "trb.orm.query.order":
		return g.ormOrder(call, arguments)
	case "trb.orm.query.limit":
		return g.ormQueryInteger(call, arguments, goORMLimit)
	case "trb.orm.query.offset":
		return g.ormQueryInteger(call, arguments, goORMOffset)
	case "trb.orm.query.lock":
		return g.ormQueryTerminal(call, arguments, goORMLock)
	case "trb.orm.query.all":
		return g.ormQueryTerminal(call, arguments, goORMLoader)
	case "trb.orm.query.first":
		return g.ormQueryTerminal(call, arguments, goORMFirst)
	case "trb.orm.query.count":
		return g.ormQueryTerminal(call, arguments, goORMCount)
	case "trb.orm.query.to_sql":
		return g.ormQueryTerminal(call, arguments, goORMToSQL)
	case "trb.orm.query.explain":
		return g.ormQueryTerminal(call, arguments, goORMExplain)
	case "trb.orm.query.preload":
		return g.ormPreload(call, arguments)
	case "trb.orm.association.query.belongs_to", "trb.orm.association.query.has_many", "trb.orm.association.query.has_one":
		return g.ormAssociationQuery(call)
	case "trb.orm.association.value.belongs_to", "trb.orm.association.value.has_many", "trb.orm.association.value.has_one", "trb.orm.association.load.belongs_to", "trb.orm.association.load.has_many", "trb.orm.association.load.has_one":
		return g.ormLoadAssociation(call, false)
	case "trb.orm.association.reload.belongs_to", "trb.orm.association.reload.has_many", "trb.orm.association.reload.has_one":
		return g.ormLoadAssociation(call, true)
	case "trb.orm.association.loaded.belongs_to", "trb.orm.association.loaded.has_many", "trb.orm.association.loaded.has_one":
		return g.ormAssociationLoaded(call)
	case "trb.std.strings.length":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.RuneCountInString(" + arguments[0] + ")"
	case "trb.std.strings.empty":
		return arguments[0] + " == \"\""
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		g.requireImport("strings", "")
		g.requireImport("unicode", "stdunicode")
		function := "TrimFunc"
		if name == "trb.std.strings.lstrip" {
			function = "TrimLeftFunc"
		} else if name == "trb.std.strings.rstrip" {
			function = "TrimRightFunc"
		}
		return "strings." + function + "(" + arguments[0] + ", stdunicode.IsSpace)"
	case "trb.std.strings.uppercase":
		g.requireImport("strings", "")
		return "strings.ToUpper(" + arguments[0] + ")"
	case "trb.std.strings.lowercase":
		g.requireImport("strings", "")
		return "strings.ToLower(" + arguments[0] + ")"
	case "trb.std.strings.starts_with":
		g.requireImport("strings", "")
		return "strings.HasPrefix(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.ends_with":
		g.requireImport("strings", "")
		return "strings.HasSuffix(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.split":
		g.requireImport("strings", "")
		return g.arrayReference("func(value, separator string) []string { if separator == \"\" { panic(\"String split separator is empty\") }; return strings.Split(value, separator) }(" + arguments[0] + ", " + arguments[1] + ")")
	case "trb.std.strings.contains":
		g.requireImport("strings", "")
		return "strings.Contains(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.replace_all":
		g.requireImport("strings", "")
		return "func(value, pattern, replacement string) string { if pattern == \"\" { panic(\"String replacement pattern is empty\") }; return strings.ReplaceAll(value, pattern, replacement) }(" + arguments[0] + ", " + arguments[1] + ", " + arguments[2] + ")"
	case "trb.std.strings.codepoints":
		g.requireImport("unicode/utf8", "utf8")
		return g.arrayReference("func(value string) []int { result := make([]int, 0, utf8.RuneCountInString(value)); for _, codepoint := range value { result = append(result, int(codepoint)) }; return result }(" + arguments[0] + ")")
	case "trb.std.strings.characters":
		g.requireImport("unicode/utf8", "utf8")
		return g.arrayReference("func(value string) []string { result := make([]string, 0, utf8.RuneCountInString(value)); for _, character := range value { result = append(result, string(character)) }; return result }(" + arguments[0] + ")")
	case "trb.std.strings.reverse":
		g.requireImport("slices", "")
		return "func(value string) string { characters := []rune(value); slices.Reverse(characters); return string(characters) }(" + arguments[0] + ")"
	case "trb.std.strings.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "func(value []rune, requested int) " + resultType + " { index := requested; if index < 0 { index += len(value) }; if index < 0 || index >= len(value) { return " + indexLookupError("requested", "len(value)", "String index is out of bounds") + " }; return " + filesystemOK("string(value[index])") + " }([]rune(" + arguments[0] + "), " + arguments[1] + ")"
	case "trb.std.strings.slice", "trb.std.strings.try_slice":
		safe := name == "trb.std.strings.try_slice"
		returnType := "string"
		invalid := "panic(\"String slice range is out of bounds\")"
		success := "return string(value[start:stop])"
		if safe {
			returnType, _, _ = filesystemResultType()
			invalid = "return " + sliceRangeError("start", "end", "exclusive", "len(value)", "String slice range is out of bounds")
			success = "return " + filesystemOK("string(value[start:stop])")
		}
		return "func(value []rune, bounds [3]int) " + returnType + " { start, end, exclusive := bounds[0], bounds[1], bounds[2] == 1; valid := start >= 0 && end >= 0 && start <= end && (exclusive && end <= len(value) || !exclusive && end < len(value)); if !valid { " + invalid + " }; stop := end; if !exclusive { stop++ }; " + success + " }([]rune(" + arguments[0] + "), " + arguments[1] + ")"
	case "trb.std.strings.index", "trb.std.strings.rindex":
		reverse := name == "trb.std.strings.rindex"
		return "func(value, substring string) *int { characters, needle := []rune(value), []rune(substring); if len(needle) == 0 { index := 0; if " + strconv.FormatBool(reverse) + " { index = len(characters) }; return &index }; if len(needle) > len(characters) { return nil }; start, stop, step := 0, len(characters)-len(needle), 1; if " + strconv.FormatBool(reverse) + " { start, stop, step = stop, 0, -1 }; for index := start; ; index += step { matched := true; for offset := range needle { if characters[index+offset] != needle[offset] { matched = false; break } }; if matched { result := index; return &result }; if index == stop { break } }; return nil }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.unicode.version":
		return unicodeCall("version") + "()"
	case "trb.std.unicode.valid_scalar":
		return unicodeCall("valid_scalar") + "(" + arguments[0] + ")"
	case "trb.std.unicode.letter":
		return unicodeCall("letter") + "(" + arguments[0] + ")"
	case "trb.std.unicode.digit":
		return unicodeCall("digit") + "(" + arguments[0] + ")"
	case "trb.std.unicode.uppercase":
		return unicodeCall("uppercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.lowercase":
		return unicodeCall("lowercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.whitespace":
		return unicodeCall("whitespace") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_start":
		return unicodeCall("identifier_start") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_part":
		return unicodeCall("identifier_part") + "(" + arguments[0] + ")"
	case "trb.std.unicode.from_codepoint":
		return unicodeCall("from_codepoint") + "(" + arguments[0] + ")"
	case "trb.std.bytes.from_string":
		return "[]byte(" + arguments[0] + ")"
	case "trb.std.bytes.to_string":
		return g.utf8WithReplacement(arguments[0])
	case "trb.std.bytes.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.bytes.at":
		return "func(value []byte, index int) int { if index < 0 || index >= len(value) { panic(\"Bytes index is out of bounds\") }; return int(value[index]) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.bytes.concat":
		return "append(append([]byte{}, " + arguments[0] + "...), " + arguments[1] + "...)"
	case "trb.std.bytes.valid_utf8":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.Valid(" + arguments[0] + ")"
	case "trb.std.encoding.hex.encode":
		g.requireImport("encoding/hex", "stdhex")
		return "stdhex.EncodeToString(" + arguments[0] + ")"
	case "trb.std.encoding.hex.decode":
		g.requireImport("encoding/hex", "stdhex")
		resultType, _, _ := filesystemResultType()
		invalid := "character < '0' || character > '9' && character < 'A' || character > 'F' && character < 'a' || character > 'f'"
		return "func() " + resultType + " { input := " + arguments[0] + "; length := 0; for _, character := range input { if " + invalid + " { return " + hexDecodeError("InvalidCharacter", "input", "length", "invalid hexadecimal character") + " }; length++ }; if length%2 != 0 { return " + hexDecodeError("OddLength", "input", "length", "hex input has odd length") + " }; value, _ := stdhex.DecodeString(input); return " + filesystemOK("value") + " }()"
	case "trb.std.encoding.base64.encode":
		g.requireImport("encoding/base64", "stdbase64")
		return "stdbase64.StdEncoding.EncodeToString(" + arguments[0] + ")"
	case "trb.std.encoding.base64.url_encode":
		g.requireImport("encoding/base64", "stdbase64")
		return "stdbase64.RawURLEncoding.EncodeToString(" + arguments[0] + ")"
	case "trb.std.encoding.base64.decode":
		g.requireImport("encoding/base64", "stdbase64")
		resultType, _, _ := filesystemResultType()
		invalid := "(character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '+' && character != '/'"
		return "func() " + resultType + " { input := " + arguments[0] + "; characters := []rune(input); if len(characters)%4 != 0 { return " + base64DecodeError("InvalidLength", "input", "len(characters)", "base64 input length must be a multiple of 4") + " }; padding := 0; for index, character := range characters { if character == '=' { padding++; if index < len(characters)-2 || padding > 2 { return " + base64DecodeError("InvalidPadding", "input", "index", "invalid base64 padding") + " }; continue }; if padding > 0 { return " + base64DecodeError("InvalidPadding", "input", "index", "invalid base64 padding") + " }; if " + invalid + " { return " + base64DecodeError("InvalidCharacter", "input", "index", "invalid base64 character") + " } }; value, err := stdbase64.StdEncoding.Strict().DecodeString(input); if err != nil { return " + base64DecodeError("NonCanonical", "input", "len(characters)-padding-1", "non-canonical base64 encoding") + " }; return " + filesystemOK("value") + " }()"
	case "trb.std.encoding.base64.url_decode":
		g.requireImport("encoding/base64", "stdbase64")
		resultType, _, _ := filesystemResultType()
		invalid := "(character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_'"
		return "func() " + resultType + " { input := " + arguments[0] + "; characters := []rune(input); if len(characters)%4 == 1 { return " + base64DecodeError("InvalidLength", "input", "len(characters)", "base64url input has invalid length") + " }; for index, character := range characters { if character == '=' { return " + base64DecodeError("InvalidPadding", "input", "index", "base64url input must not contain padding") + " }; if " + invalid + " { return " + base64DecodeError("InvalidCharacter", "input", "index", "invalid base64url character") + " } }; value, err := stdbase64.RawURLEncoding.Strict().DecodeString(input); if err != nil { return " + base64DecodeError("NonCanonical", "input", "len(characters)-1", "non-canonical base64url encoding") + " }; return " + filesystemOK("value") + " }()"
	case "trb.std.hash.md5":
		g.requireImport("crypto/md5", "stdmd5")
		return "func(value []byte) []byte { digest := stdmd5.Sum(value); return digest[:] }(" + arguments[0] + ")"
	case "trb.std.hash.sha1":
		g.requireImport("crypto/sha1", "stdsha1")
		return "func(value []byte) []byte { digest := stdsha1.Sum(value); return digest[:] }(" + arguments[0] + ")"
	case "trb.std.hash.sha256":
		g.requireImport("crypto/sha256", "stdsha256")
		return "func(value []byte) []byte { digest := stdsha256.Sum256(value); return digest[:] }(" + arguments[0] + ")"
	case "trb.std.hash.sha512":
		g.requireImport("crypto/sha512", "stdsha512")
		return "func(value []byte) []byte { digest := stdsha512.Sum512(value); return digest[:] }(" + arguments[0] + ")"
	case "trb.std.hmac.sha256":
		g.requireImport("crypto/hmac", "stdhmac")
		g.requireImport("crypto/sha256", "stdsha256")
		return "func(key []byte, message []byte) []byte { digest := stdhmac.New(stdsha256.New, key); _, _ = digest.Write(message); return digest.Sum(nil) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hmac.sha512":
		g.requireImport("crypto/hmac", "stdhmac")
		g.requireImport("crypto/sha512", "stdsha512")
		return "func(key []byte, message []byte) []byte { digest := stdhmac.New(stdsha512.New, key); _, _ = digest.Write(message); return digest.Sum(nil) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hmac.equal", "trb.std.secure_compare.equal":
		g.requireImport("crypto/hmac", "stdhmac")
		return "stdhmac.Equal(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.random.float":
		g.requireImport("math/rand/v2", "stdrand")
		return "stdrand.Float64()"
	case "trb.std.random.integer":
		g.requireImport("math/rand/v2", "stdrand")
		return "func(upper int) int { if upper <= 0 { panic(\"random.integer upper bound must be greater than zero\") }; return stdrand.IntN(upper) }(" + arguments[0] + ")"
	case "trb.std.secure_random.bytes":
		g.requireImport("crypto/rand", "stdcryptorand")
		return "func(length int) []byte { if length < 0 || length > 65536 { panic(\"secure_random.bytes length must be between 0 and 65536\") }; value := make([]byte, length); stdcryptorand.Read(value); return value }(" + arguments[0] + ")"
	case "trb.std.string_builder.new":
		g.requireImport("strings", "")
		return "&strings.Builder{}"
	case "trb.std.string_builder.from_string":
		g.requireImport("strings", "")
		return "func(value string) *strings.Builder { builder := &strings.Builder{}; builder.WriteString(value); return builder }(" + arguments[0] + ")"
	case "trb.std.string_builder.append":
		return arguments[0] + ".WriteString(" + arguments[1] + ")"
	case "trb.std.string_builder.append_codepoint":
		return "func(builder *strings.Builder, value int) { if value < 0 || value > 0x10ffff || value >= 0xd800 && value <= 0xdfff { panic(\"invalid Unicode code point\") }; builder.WriteRune(rune(value)) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.string_builder.length":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.RuneCountInString(" + arguments[0] + ".String())"
	case "trb.std.string_builder.empty":
		return arguments[0] + ".Len() == 0"
	case "trb.std.string_builder.to_string":
		return arguments[0] + ".String()"
	case "trb.std.string_builder.clear":
		return arguments[0] + ".Reset()"
	case "trb.std.arrays.length":
		return "len(" + g.arrayValues(arguments[0]) + ")"
	case "trb.std.arrays.empty":
		return "len(" + g.arrayValues(arguments[0]) + ") == 0"
	case "trb.std.arrays.try_fetch":
		resultType, _, _ := filesystemResultType()
		valuesType := call.Arguments[0].Value.ExprType()
		if member, ok := call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Kind == types.Array {
			valuesType = member.Receiver.ExprType()
		}
		receiverType := g.goType(valuesType)
		return "func(receiver " + receiverType + ", requested int) " + resultType + " { values := " + g.arrayValues("receiver") + "; index := requested; if index < 0 { index += len(values) }; if index < 0 || index >= len(values) { return " + indexLookupError("requested", "len(values)", "Array index is out of bounds") + " }; return " + filesystemOK("values[index]") + " }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.slice", "trb.std.arrays.try_slice":
		g.requireImport("slices", "")
		safe := name == "trb.std.arrays.try_slice"
		returnType := g.goType(call.ExprType())
		invalid := "panic(\"Array slice range is out of bounds\")"
		success := "return " + g.arrayReference("slices.Clone(values[start:stop])")
		if safe {
			returnType, _, _ = filesystemResultType()
			invalid = "return " + sliceRangeError("start", "end", "exclusive", "len(values)", "Array slice range is out of bounds")
			success = "return " + filesystemOK(g.arrayReference("slices.Clone(values[start:stop])"))
		}
		valuesType := call.Arguments[0].Value.ExprType()
		if member, ok := call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Kind == types.Array {
			valuesType = member.Receiver.ExprType()
		}
		receiverType := g.goType(valuesType)
		return "func(receiver " + receiverType + ", bounds [3]int) " + returnType + " { values := " + g.arrayValues("receiver") + "; start, end, exclusive := bounds[0], bounds[1], bounds[2] == 1; valid := start >= 0 && end >= 0 && start <= end && (exclusive && end <= len(values) || !exclusive && end < len(values)); if !valid { " + invalid + " }; stop := end; if !exclusive { stop++ }; " + success + " }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.first":
		return "func() " + g.goType(call.ExprType()) + " { values := " + g.arrayValues(arguments[0]) + "; if len(values) == 0 { panic(\"Array is empty\") }; return values[0] }()"
	case "trb.std.arrays.last":
		return "func() " + g.goType(call.ExprType()) + " { values := " + g.arrayValues(arguments[0]) + "; if len(values) == 0 { panic(\"Array is empty\") }; return values[len(values)-1] }()"
	case "trb.std.arrays.copy":
		g.requireImport("slices", "")
		return g.arrayReference("slices.Clone(" + g.arrayValues(arguments[0]) + ")")
	case "trb.std.arrays.contains":
		g.requireImport("slices", "")
		return "slices.Contains(" + g.arrayValues(arguments[0]) + ", " + arguments[1] + ")"
	case "trb.std.arrays.index":
		valuesType := call.Arguments[0].Value.ExprType()
		if member, ok := call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Kind == types.Array {
			valuesType = member.Receiver.ExprType()
		}
		valueType := valuesType.Args[0]
		return "func(values " + g.goArraySliceType(valuesType) + ", target " + g.goType(valueType) + ") *int { for index, value := range values { if value == target { result := index; return &result } }; return nil }(" + g.arrayValues(arguments[0]) + ", " + arguments[1] + ")"
	case "trb.std.arrays.count":
		return "func() int { values := " + g.arrayValues(arguments[0]) + "; target := " + arguments[1] + "; count := 0; for _, value := range values { if value == target { count++ } }; return count }()"
	case "trb.std.arrays.uniq":
		g.requireImport("slices", "")
		return "func() " + g.goType(call.ExprType()) + " { result := " + g.goArraySliceType(call.ExprType()) + "{}; for _, value := range " + g.arrayValues(arguments[0]) + " { if !slices.Contains(result, value) { result = append(result, value) } }; return " + g.arrayReference("result") + " }()"
	case "trb.std.arrays.concat":
		g.requireImport("slices", "")
		return g.arrayReference("append(slices.Clone(" + g.arrayValues(arguments[0]) + "), " + g.arrayValues(arguments[1]) + "...)")
	case "trb.std.arrays.join":
		g.requireImport("strings", "")
		return "strings.Join(" + g.arrayValues(arguments[0]) + ", " + arguments[1] + ")"
	case "trb.std.arrays.pop":
		return "func(values *[]" + g.goType(call.ExprType()) + ") " + g.goType(call.ExprType()) + " { items := *values; if len(items) == 0 { panic(\"Array is empty\") }; index := len(items) - 1; value := items[index]; *values = items[:index]; return value }(" + arguments[0] + ")"
	case "trb.std.arrays.shift":
		return "func(values *[]" + g.goType(call.ExprType()) + ") " + g.goType(call.ExprType()) + " { items := *values; if len(items) == 0 { panic(\"Array is empty\") }; value := items[0]; *values = items[1:]; return value }(" + arguments[0] + ")"
	case "trb.std.arrays.push":
		valueType := call.Arguments[len(call.Arguments)-1].Value.ExprType()
		return "func(values *[]" + g.goType(valueType) + ", value " + g.goType(valueType) + ") { *values = append(*values, value) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.unshift":
		valueType := call.Arguments[len(call.Arguments)-1].Value.ExprType()
		return "func(values *[]" + g.goType(valueType) + ", value " + g.goType(valueType) + ") { items := *values; items = append(items, value); copy(items[1:], items[:len(items)-1]); items[0] = value; *values = items }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.reverse":
		g.requireImport("slices", "")
		return "func() " + g.goType(call.ExprType()) + " { values := slices.Clone(" + g.arrayValues(arguments[0]) + "); slices.Reverse(values); return " + g.arrayReference("values") + " }()"
	case "trb.std.arrays.sort", "trb.std.arrays.sort_descending":
		g.requireImport("slices", "")
		valuesType := call.ExprType()
		elementType := types.Type{Kind: types.Any, Name: "Any"}
		if len(valuesType.Args) == 1 {
			elementType = valuesType.Args[0]
		}
		comparison := g.portableSortComparison("left", "right", elementType, name == "trb.std.arrays.sort_descending")
		return "func() " + g.goType(valuesType) + " { values := slices.Clone(" + g.arrayValues(arguments[0]) + "); slices.SortStableFunc(values, func(left, right " + g.goType(elementType) + ") int { return " + comparison + " }); return " + g.arrayReference("values") + " }()"
	case "trb.std.ranges.to_array":
		return g.arrayReference("func(bounds [3]int) []int { start, end, exclusive := bounds[0], bounds[1], bounds[2] == 1; values := []int{}; for current := start; current < end; current++ { values = append(values, current) }; if !exclusive && start <= end { values = append(values, end) }; return values }(" + arguments[0] + ")")
	case "trb.std.hashes.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.hashes.empty":
		return "len(" + arguments[0] + ") == 0"
	case "trb.std.hashes.fetch":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; return value }()"
	case "trb.std.hashes.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { return " + keyLookupError("key", "Hash key is missing") + " }; return " + filesystemOK("value") + " }()"
	case "trb.std.hashes.contains_key":
		return "func() bool { values := " + arguments[0] + "; key := " + arguments[1] + "; _, ok := values[key]; return ok }()"
	case "trb.std.hashes.keys":
		g.requireImport("maps", "")
		g.requireImport("slices", "")
		return g.arrayReference("slices.Collect(maps.Keys(" + arguments[0] + "))")
	case "trb.std.hashes.values":
		g.requireImport("maps", "")
		g.requireImport("slices", "")
		return g.arrayReference("slices.Collect(maps.Values(" + arguments[0] + "))")
	case "trb.std.hashes.copy":
		g.requireImport("maps", "")
		return "maps.Clone(" + arguments[0] + ")"
	case "trb.std.hashes.delete":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; delete(values, key); return value }()"
	case "trb.std.hashes.merge":
		g.requireImport("maps", "")
		return "func() " + g.goType(call.ExprType()) + " { values := maps.Clone(" + arguments[0] + "); maps.Copy(values, " + arguments[1] + "); return values }()"
	case "trb.std.hashes.update":
		g.requireImport("maps", "")
		return "maps.Copy(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		g.requireImport("strconv", "")
		return "strconv.Itoa(" + arguments[0] + ")"
	case "trb.std.numbers.integer_to_float":
		return "float64(" + arguments[0] + ")"
	case "trb.std.numbers.integer_absolute":
		return "func(value int) int { if value < 0 { return -value }; return value }(" + arguments[0] + ")"
	case "trb.std.numbers.integer_min":
		return "min(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.integer_max":
		return "max(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.integer_clamp":
		return "func(value, minimum, maximum int) int { if minimum > maximum { panic(\"clamp minimum exceeds maximum\") }; return min(max(value, minimum), maximum) }(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.numbers.integer_zero":
		return arguments[0] + " == 0"
	case "trb.std.numbers.integer_positive":
		return arguments[0] + " > 0"
	case "trb.std.numbers.integer_negative":
		return arguments[0] + " < 0"
	case "trb.std.numbers.integer_even":
		return arguments[0] + "%2 == 0"
	case "trb.std.numbers.integer_odd":
		return arguments[0] + "%2 != 0"
	case "trb.std.numbers.float_to_string":
		return g.portableFloatString(arguments[0])
	case "trb.std.numbers.float_to_integer":
		return g.portableFloatInteger(arguments[0], "Trunc")
	case "trb.std.numbers.float_floor":
		return g.portableFloatInteger(arguments[0], "Floor")
	case "trb.std.numbers.float_ceil":
		return g.portableFloatInteger(arguments[0], "Ceil")
	case "trb.std.numbers.float_round":
		return g.portableFloatInteger(arguments[0], "Round")
	case "trb.std.numbers.float_absolute":
		g.requireImport("math", "")
		return "math.Abs(" + arguments[0] + ")"
	case "trb.std.numbers.float_finite":
		g.requireImport("math", "")
		return "func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }(" + arguments[0] + ")"
	case "trb.std.numbers.float_infinite":
		g.requireImport("math", "")
		return "math.IsInf(" + arguments[0] + ", 0)"
	case "trb.std.numbers.float_nan":
		g.requireImport("math", "")
		return "math.IsNaN(" + arguments[0] + ")"
	case "trb.std.numbers.parse_integer":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		return "func() int { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?[0-9]+$`, input); if !valid { panic(\"invalid Integer\") }; value, err := strconv.ParseInt(input, 10, 64); if err != nil || value < -9007199254740991 || value > 9007199254740991 { panic(\"Integer is outside the portable range\") }; return int(value) }()"
	case "trb.std.numbers.try_parse_integer":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?[0-9]+$`, input); if !valid { return " + numberParseError("InvalidFormat", "input", "invalid Integer") + " }; value, err := strconv.ParseInt(input, 10, 64); if err != nil || value < -9007199254740991 || value > 9007199254740991 { return " + numberParseError("OutOfRange", "input", "Integer is outside the portable range") + " }; return " + filesystemOK("int(value)") + " }()"
	case "trb.std.numbers.parse_float":
		g.requireImport("math", "")
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		return "func() float64 { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`, input); if !valid { panic(\"invalid Float\") }; value, err := strconv.ParseFloat(input, 64); if math.IsInf(value, 0) || (err != nil && value != 0) { panic(\"Float is outside the portable range\") }; return value }()"
	case "trb.std.numbers.try_parse_float":
		g.requireImport("math", "")
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`, input); if !valid { return " + numberParseError("InvalidFormat", "input", "invalid Float") + " }; value, err := strconv.ParseFloat(input, 64); if math.IsInf(value, 0) || (err != nil && value != 0) { return " + numberParseError("OutOfRange", "input", "Float is outside the portable range") + " }; return " + filesystemOK("value") + " }()"
	case "trb.std.math.sqrt":
		g.requireImport("math", "")
		return "math.Sqrt(" + arguments[0] + ")"
	case "trb.std.math.exp":
		g.requireImport("math", "")
		return "math.Exp(" + arguments[0] + ")"
	case "trb.std.math.log":
		g.requireImport("math", "")
		return "math.Log(" + arguments[0] + ")"
	case "trb.std.math.log2":
		g.requireImport("math", "")
		return "math.Log2(" + arguments[0] + ")"
	case "trb.std.math.log10":
		g.requireImport("math", "")
		return "math.Log10(" + arguments[0] + ")"
	case "trb.std.booleans.to_string":
		g.requireImport("strconv", "")
		return "strconv.FormatBool(" + arguments[0] + ")"
	case "trb.platform.go.context.background":
		g.requireImport("context", "trbcontext")
		return "trbcontext.Background()"
	case "trb.platform.go.context.todo":
		g.requireImport("context", "trbcontext")
		return "trbcontext.TODO()"
	case "trb.platform.go.http.router":
		g.requireImport("net/http", "http")
		return "http.NewServeMux()"
	case "trb.platform.go.http.handle":
		return arguments[0] + ".HandleFunc(" + arguments[1] + ", " + arguments[2] + ")"
	case "trb.platform.go.http.path":
		return arguments[0] + ".PathValue(" + arguments[1] + ")"
	case "trb.platform.go.http.decode":
		g.requireImport("encoding/json", "json")
		g.requireImport("net/http", "http")
		return "func() bool { if err := json.NewDecoder(" + arguments[1] + ".Body).Decode(&" + arguments[2] + "); err != nil { http.Error(" + arguments[0] + ", err.Error(), http.StatusBadRequest); return false }; return true }()"
	case "trb.platform.go.http.json":
		g.requireImport("encoding/json", "json")
		return "func() { " + arguments[0] + ".Header().Set(\"Content-Type\", \"application/json\"); " + arguments[0] + ".WriteHeader(" + arguments[1] + "); if err := json.NewEncoder(" + arguments[0] + ").Encode(" + arguments[2] + "); err != nil { panic(err) } }()"
	case "trb.platform.go.http.error":
		g.requireImport("net/http", "http")
		return "http.Error(" + strings.Join(arguments, ", ") + ")"
	case "trb.platform.go.http.cors":
		g.requireImport("net/http", "http")
		return "http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { response.Header().Set(\"Access-Control-Allow-Origin\", " + arguments[1] + "); response.Header().Set(\"Access-Control-Allow-Headers\", \"Content-Type\"); response.Header().Set(\"Access-Control-Allow-Methods\", \"GET, POST, PATCH, OPTIONS\"); if request.Method == http.MethodOptions { response.WriteHeader(http.StatusNoContent); return }; " + arguments[0] + ".ServeHTTP(response, request) })"
	case "trb.platform.go.http.serve":
		g.requireImport("net/http", "http")
		g.requireImport("log", "")
		return "func() { if err := http.ListenAndServe(" + arguments[0] + ", " + arguments[1] + "); err != nil { log.Fatal(err) } }()"
	case "trb.platform.go.gorm.open_sqlite":
		g.requireImport("gorm.io/driver/sqlite", "sqlite")
		g.requireImport("gorm.io/gorm", "gorm")
		g.requireImport("os", "")
		return "func() *gorm.DB { path := os.Getenv(\"TRB_DATABASE\"); if path == \"\" { path = " + arguments[0] + " }; database, err := gorm.Open(sqlite.Open(path), &gorm.Config{}); if err != nil { panic(err) }; return database }()"
	case "trb.platform.go.gorm.find_all":
		return g.gormRead(call, arguments, "find_all")
	case "trb.platform.go.gorm.where":
		return g.gormRead(call, arguments, "where")
	case "trb.platform.go.gorm.raw":
		return g.gormRead(call, arguments, "raw")
	case "trb.platform.go.gorm.first":
		return g.gormRead(call, arguments, "first")
	case "trb.platform.go.gorm.create":
		return g.gormWrite(call, arguments, "Create")
	case "trb.platform.go.gorm.save":
		return g.gormWrite(call, arguments, "Save")
	case "trb.platform.go.gorm.exec":
		args := ""
		if len(arguments) > 2 {
			args = ", " + strings.Join(arguments[2:], ", ")
		}
		return "func() { if err := " + arguments[0] + ".Exec(" + arguments[1] + args + ").Error; err != nil { panic(err) } }()"
	default:
		return "nil"
	}
}

func (g *generator) utf8WithReplacement(value string) string {
	g.utf8Replacement = true
	g.requireImport("unicode/utf8", "utf8")
	return g.utf8ReplacementRuntimeName() + "(" + value + ")"
}

func (g *generator) utf8ReplacementRuntimeSupport() {
	name := g.utf8ReplacementRuntimeName()
	g.line("func " + name + "(input []byte) string {")
	g.indent++
	g.line("if utf8.Valid(input) { return string(input) }")
	g.line("output := make([]rune, 0, len(input))")
	g.line("continuation := func(value byte) bool { return value >= 0x80 && value <= 0xbf }")
	g.line("for len(input) > 0 {")
	g.indent++
	g.line("character, size := utf8.DecodeRune(input)")
	g.line("if character != utf8.RuneError || size > 1 { output = append(output, character); input = input[size:]; continue }")
	g.line("maximalSubpart := 1")
	g.line("if len(input) > 1 {")
	g.indent++
	g.line("first, second := input[0], input[1]")
	g.line("validSecond := false")
	g.line("switch { case first == 0xe0: validSecond = second >= 0xa0 && second <= 0xbf; case first >= 0xe1 && first <= 0xec: validSecond = continuation(second); case first == 0xed: validSecond = second >= 0x80 && second <= 0x9f; case first >= 0xee && first <= 0xef: validSecond = continuation(second); case first == 0xf0: validSecond = second >= 0x90 && second <= 0xbf; case first >= 0xf1 && first <= 0xf3: validSecond = continuation(second); case first == 0xf4: validSecond = second >= 0x80 && second <= 0x8f }")
	g.line("if validSecond { maximalSubpart = 2; if first >= 0xf0 && len(input) > 2 && continuation(input[2]) { maximalSubpart = 3 } }")
	g.indent--
	g.line("}")
	g.line("output = append(output, utf8.RuneError)")
	g.line("input = input[maximalSubpart:]")
	g.indent--
	g.line("}")
	g.line("return string(output)")
	g.indent--
	g.line("}")
}

func (g *generator) utf8ReplacementRuntimeName() string {
	return "trbDecodeUTF8_" + naming.PrivateSuffix("utf8-replacement:"+g.modulePath)
}

func (g *generator) webContextWith(call *ir.Call, arguments []string) string {
	if len(arguments) != 3 {
		return "nil"
	}
	return "func() " + g.goType(call.ExprType()) + " { contextValue := " + arguments[0] + "; contextKey := " + arguments[1] + "; contextState := map[any]any{}; if existing, ok := contextValue.TrbInternalContextState.(map[any]any); ok { for key, value := range existing { contextState[key] = value } }; contextState[contextKey] = " + arguments[2] + "; result := contextValue.WithRequest(contextValue.TrbFieldRequest); result.TrbInternalContextState = contextState; return result }()"
}

func (g *generator) webContextWithRequest(call *ir.Call, arguments []string) string {
	if len(arguments) != 2 {
		return "nil"
	}
	return "func() " + g.goType(call.ExprType()) + " { contextValue := " + arguments[0] + "; result := contextValue.WithRequest(" + arguments[1] + "); result.TrbInternalContextState = contextValue.TrbInternalContextState; return result }()"
}

func (g *generator) webContextFetch(call *ir.Call, arguments []string) string {
	if len(arguments) != 2 || len(call.ExprType().Args) != 2 {
		return "nil"
	}
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	resultType := g.goType(call.ExprType())
	valueType := g.goType(call.ExprType().Args[0])
	errorType := g.goType(call.ExprType().Args[1])
	okResult := resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "]"
	errResult := resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "]"
	return "func() " + resultType + " { contextValue := " + arguments[0] + "; contextKey := " + arguments[1] + "; if contextState, ok := contextValue.TrbInternalContextState.(map[any]any); ok { if raw, found := contextState[contextKey]; found { value, valid := raw.(" + valueType + "); if !valid { panic(\"ContextKey value has an incompatible runtime type\") }; return " + okResult + "(value) } }; return " + errResult + "(" + errorType + "{Key: contextKey.TrbFieldName}) }()"
}

func (g *generator) portableArrayString(value string, typ types.Type) string {
	g.requireImport("strings", "")
	elementType := types.Type{Kind: types.Any, Name: "Any"}
	if len(typ.Args) > 0 {
		elementType = typ.Args[0]
	}
	element := "fmt.Sprint(item)"
	switch elementType.Kind {
	case types.String:
		g.requireImport("strconv", "")
		element = "strconv.Quote(item)"
	case types.Int:
		g.requireImport("strconv", "")
		element = "strconv.Itoa(item)"
	case types.Float:
		element = g.portableFloatString("item")
	case types.Bool:
		g.requireImport("strconv", "")
		element = "strconv.FormatBool(item)"
	case types.Array:
		element = g.portableArrayString("item", elementType)
	default:
		g.requireImport("fmt", "")
	}
	return "func(values " + g.goArraySliceType(typ) + ") string { parts := make([]string, len(values)); for index, item := range values { parts[index] = " + element + " }; return \"[\" + strings.Join(parts, \", \") + \"]\" }(" + g.arrayValues(value) + ")"
}

func (g *generator) webLogger(call *ir.Call, arguments []string) string {
	g.requireImport("encoding/json", "json")
	g.requireImport("fmt", "")
	g.requireImport("os", "")
	g.requireImport("time", "")
	options := ""
	if len(arguments) > 2 {
		g.requireImport("slices", "")
		options = "loggerOptions := " + arguments[2] + "; excluded = slices.Contains(" + g.arrayValues("loggerOptions.ExcludePaths") + ", loggerContext.TrbFieldRequest.TrbFieldPath); useStderr = loggerOptions.Stderr; "
	}
	return "func() (response " + g.goType(call.ExprType()) + ") { loggerContext := " + arguments[0] + "; loggerNextHandler := " + arguments[1] + "; excluded := false; useStderr := false; " + options + "if excluded { return loggerNextHandler.Call(__trbScope, loggerContext) }; started := time.Now(); status := 500; defer func() { level := \"info\"; if status >= 500 { level = \"error\" }; entry := map[string]any{\"timestamp\": time.Now().UTC().Format(time.RFC3339Nano), \"level\": level, \"event\": \"http_request\", \"method\": loggerContext.TrbFieldRequest.TrbFieldMethod.ToS(), \"path\": loggerContext.TrbFieldRequest.TrbFieldPath, \"status\": status, \"duration_ms\": float64(time.Since(started).Nanoseconds()) / 1e6}; encoded, _ := json.Marshal(entry); output := os.Stdout; if useStderr { output = os.Stderr }; fmt.Fprintln(output, string(encoded)) }(); response = loggerNextHandler.Call(__trbScope, loggerContext); status = response.TrbFieldStatus; return response }()"
}

func (g *generator) webTimeout(call *ir.Call, arguments []string) string {
	g.requireImport("context", "trbcontext")
	g.requireImport("time", "time")
	responseType := g.goType(call.ExprType())
	return "func() " + responseType + " { timeoutContext, cancel := trbcontext.WithTimeout(__trbScope, time.Duration(" + arguments[2] + ")*time.Millisecond); defer cancel(); type timeoutOutcome struct { response " + responseType + "; panicValue any }; outcome := make(chan timeoutOutcome, 1); go func() { result := timeoutOutcome{}; defer func() { result.panicValue = recover(); outcome <- result }(); result.response = " + arguments[1] + ".Call(timeoutContext, " + arguments[0] + ") }(); select { case result := <-outcome: if timeoutContext.Err() != nil { return " + arguments[3] + " }; if result.panicValue != nil { panic(result.panicValue) }; return result.response; case <-timeoutContext.Done(): return " + arguments[3] + " } }()"
}

func (g *generator) webGzip(value string) string {
	g.requireImport("bytes", "")
	g.requireImport("compress/gzip", "gzip")
	return "func(value []byte) []byte { var buffer bytes.Buffer; writer := gzip.NewWriter(&buffer); if _, err := writer.Write(value); err != nil { panic(\"trb/web gzip compression failed: \" + err.Error()) }; if err := writer.Close(); err != nil { panic(\"trb/web gzip compression failed: \" + err.Error()) }; return buffer.Bytes() }(" + value + ")"
}

func (g *generator) webRequestJSON(call *ir.Call, request string) string {
	if call.Codec == nil || len(call.ExprType().Args) != 2 {
		return "nil"
	}
	g.requireImport("strings", "")
	g.requireImport("unicode/utf8", "utf8")
	if g.typeAliases["JSON::Error"] == "" {
		g.typeAliases["JSON::Error"] = "__trb_json"
		g.requireImport(pathpkg.Join(g.goModule, "trb/std/json"), "__trb_json")
	}
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	webAlias := "web"
	if reference := expressionReference(call.Callee); reference != nil {
		if alias := g.referenceAlias(reference); alias != "" {
			webAlias = alias
		}
	}
	valueType := g.goCodecType(call.Codec)
	requestErrorType := webAlias + ".RequestError"
	resultType := resultAlias + ".Result[" + valueType + ", " + requestErrorType + "]"
	errResult := func(value string) string {
		return resultAlias + ".NewResultErr[" + valueType + ", " + requestErrorType + "](" + value + ")"
	}
	okResult := resultAlias + ".NewResultOk[" + valueType + ", " + requestErrorType + "]"
	innerCall := *call
	innerCall.ExprBase.Type = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{call.ExprType().Args[0], types.FromName("JSON::Error")}}
	decoded := g.jsonDecode(&innerCall, "string(requestValue.TrbFieldBody.Bytes())")
	missing := webAlias + "." + goConstantIdentifier("RequestError", "MissingContentType")
	duplicate := webAlias + "." + goConstantIdentifier("RequestError", "DuplicateContentType")
	unsupported := webAlias + ".New" + goIdentifier("RequestError", true) + goIdentifier("UnsupportedContentType", true) + "(contentTypes[0])"
	invalidUTF8 := webAlias + "." + goConstantIdentifier("RequestError", "InvalidUtf8")
	invalidJSON := webAlias + ".New" + goIdentifier("RequestError", true) + goIdentifier("InvalidJson", true) + "(decoded.ErrError)"
	return "func() " + resultType + " { requestValue := " + request + "; contentTypes := []string{}; for _, header := range " + g.arrayValues("requestValue.TrbFieldHeaders.Entries()") + " { if strings.EqualFold(header.Name, \"content-type\") { contentTypes = append(contentTypes, header.Value) } }; if len(contentTypes) == 0 { return " + errResult(missing) + " }; if len(contentTypes) != 1 { return " + errResult(duplicate) + " }; mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentTypes[0], \";\", 2)[0])); if mediaType != \"application/json\" && !(strings.HasPrefix(mediaType, \"application/\") && strings.HasSuffix(mediaType, \"+json\")) { return " + errResult(unsupported) + " }; if !utf8.Valid(requestValue.TrbFieldBody.Bytes()) { return " + errResult(invalidUTF8) + " }; decoded := " + decoded + "; if decoded.Kind == " + resultAlias + ".ResultErrTag { return " + errResult(invalidJSON) + " }; return " + okResult + "(decoded.OkValue) }()"
}

func (g *generator) webParameterBinding(call *ir.Call, receiver, source string) string {
	if call.Codec == nil || call.Codec.Kind != "record" || len(call.ExprType().Args) != 2 {
		return "nil"
	}
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	webAlias := "web"
	if reference := expressionReference(call.Callee); reference != nil {
		if alias := g.referenceAlias(reference); alias != "" {
			webAlias = alias
		}
	}
	valueType := g.goCodecType(call.Codec)
	errorType := webAlias + ".ParameterError"
	resultType := resultAlias + ".Result[" + valueType + ", " + errorType + "]"
	okResult := resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "]"
	errResult := func(value string) string {
		return resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "](" + value + ")"
	}
	sourceName := "Path"
	if source == "query" {
		sourceName = "Query"
	}
	sourceValue := webAlias + "." + goConstantIdentifier("ParameterSource", sourceName)
	missing := func(name string) string {
		return errResult(webAlias + ".NewParameterErrorMissing(map[string]any{\"source\": " + sourceValue + ", \"name\": " + strconv.Quote(name) + "})")
	}
	duplicate := func(name string) string {
		return errResult(webAlias + ".NewParameterErrorDuplicate(map[string]any{\"source\": " + sourceValue + ", \"name\": " + strconv.Quote(name) + "})")
	}
	invalid := func(name, value, expected string) string {
		return errResult(webAlias + ".NewParameterErrorInvalid(map[string]any{\"source\": " + sourceValue + ", \"name\": " + strconv.Quote(name) + ", \"value\": " + value + ", \"expected\": " + strconv.Quote(expected) + "})")
	}

	var body strings.Builder
	body.WriteString("parameterValues := map[string][]string{}; ")
	if source == "query" {
		body.WriteString("queryResult := parameterReceiver.QueryParameters(); if queryResult.Kind == " + resultAlias + ".ResultErrTag { return " + errResult(webAlias+".NewParameterErrorMalformedQuery(queryResult.ErrError)") + " }; for _, parameter := range " + g.arrayValues("queryResult.OkValue") + " { parameterValues[parameter.Name] = append(parameterValues[parameter.Name], parameter.Value) }; ")
	} else {
		for _, field := range call.Codec.Fields {
			body.WriteString("parameterValues[" + strconv.Quote(field.Name) + "] = []string{parameterReceiver.PathValue(" + strconv.Quote(field.Name) + ")}; ")
		}
	}
	constructor := make([]string, 0, len(call.Codec.Fields))
	for index, field := range call.Codec.Fields {
		variable := "field" + strconv.Itoa(index)
		values := "values" + strconv.Itoa(index)
		fieldType := g.goCodecType(field.Schema)
		body.WriteString(values + " := parameterValues[" + strconv.Quote(field.Name) + "]; var " + variable + " " + fieldType + "; ")
		if field.Schema.Kind == "array" {
			if field.Schema.Type.Nullable {
				body.WriteString("if len(" + values + ") > 0 { parsedValues := make(" + fieldType[1:] + ", len(" + values + ")); ")
			} else {
				body.WriteString("parsedValues := make(" + fieldType[1:] + ", len(" + values + ")); ")
			}
			parser := g.goWebParameterParser(field.Schema.Element)
			body.WriteString("for valueIndex, rawValue := range " + values + " { parsedValue, valid := " + parser + "(rawValue); if !valid { return " + invalid(field.Name, "rawValue", parameterExpected(field.Schema.Element)) + " }; parsedValues[valueIndex] = parsedValue }; ")
			body.WriteString(variable + " = " + g.arrayReference("parsedValues"))
			if field.Schema.Type.Nullable {
				body.WriteString(" }; ")
			} else {
				body.WriteString("; ")
			}
		} else {
			body.WriteString("if len(" + values + ") == 0 { ")
			if field.Schema.Type.Nullable {
				body.WriteString(variable + " = nil")
			} else {
				body.WriteString("return " + missing(field.Name))
			}
			body.WriteString(" } else if len(" + values + ") > 1 { return " + duplicate(field.Name) + " } else { rawValue := " + values + "[0]; ")
			nonnull := *field.Schema
			nonnull.Type.Nullable = false
			parser := g.goWebParameterParser(&nonnull)
			body.WriteString("parsedValue, valid := " + parser + "(rawValue); if !valid { return " + invalid(field.Name, "rawValue", parameterExpected(&nonnull)) + " }; ")
			if field.Schema.Type.Nullable && !isTimeCodec(field.Schema.Kind) {
				body.WriteString(variable + " = &parsedValue")
			} else {
				body.WriteString(variable + " = parsedValue")
			}
			body.WriteString(" }; ")
		}
		constructor = append(constructor, goIdentifier(field.Name, true)+": "+variable)
	}
	body.WriteString("return " + okResult + "(" + valueType + "{" + strings.Join(constructor, ", ") + "})")
	return "func() " + resultType + " { parameterReceiver := " + receiver + "; " + body.String() + " }()"
}

func parameterExpected(schema *ir.CodecSchema) string {
	if schema == nil {
		return "value"
	}
	typ := schema.Type
	typ.Nullable = false
	return typ.String()
}

func (g *generator) goWebParameterParser(schema *ir.CodecSchema) string {
	valueType := g.goCodecType(schema)
	body := "return value, true"
	switch schema.Kind {
	case "string":
		body = "return value, true"
	case "boolean":
		body = "if value == \"true\" { return true, true }; if value == \"false\" { return false, true }; return false, false"
	case "integer":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		body = "valid, _ := regexp.MatchString(`^[+-]?[0-9]+$`, value); if !valid { return 0, false }; parsed, err := strconv.ParseInt(value, 10, 64); if err != nil || parsed < -9007199254740991 || parsed > 9007199254740991 { return 0, false }; return int(parsed), true"
	case "float":
		g.requireImport("math", "")
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		body = "valid, _ := regexp.MatchString(`^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`, value); if !valid { return 0, false }; parsed, err := strconv.ParseFloat(value, 64); if math.IsInf(parsed, 0) || (err != nil && parsed != 0) { return 0, false }; return parsed, true"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		method := goTimeCodecOwner(schema, g) + timeCodecParseMethod(schema.Kind)
		body = "parsed := " + method + "(value); if parsed.Kind != 0 { var zero " + valueType + "; return zero, false }; return parsed.OkValue, true"
	case "raw_enum":
		prefix := ""
		if schema.Reference != nil && schema.Reference.Package != "" && schema.Reference.Package != g.modulePath {
			alias := schema.Reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(schema.Reference.Package))
			}
			prefix = goImportAlias(alias) + "."
		}
		branches := make([]string, 0, len(schema.RawValues))
		for _, item := range schema.RawValues {
			comparison := strconv.Quote(item.Raw)
			if schema.RawType.Kind == types.String {
				raw, err := strconv.Unquote(item.Raw)
				if err == nil {
					comparison = strconv.Quote(raw)
				}
			}
			branches = append(branches, "case "+comparison+": return "+prefix+goConstantIdentifier(schema.Type.Name, item.Member)+", true")
		}
		body = "switch value { " + strings.Join(branches, "; ") + " }; var zero " + valueType + "; return zero, false"
	}
	return "func(value string) (" + valueType + ", bool) { " + body + " }"
}

func (g *generator) webJSON(call *ir.Call, arguments []string) string {
	if call.Codec == nil || len(arguments) == 0 {
		return "nil"
	}
	status := "200"
	if len(arguments) > 1 {
		status = arguments[1]
	}
	jsonAlias := g.typeAliases["JSON::Error"]
	if jsonAlias == "" {
		jsonAlias = "json"
	}
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	webAlias := "web"
	if reference := expressionReference(call.Callee); reference != nil {
		if alias := g.referenceAlias(reference); alias != "" {
			webAlias = alias
		}
	}
	builder := &goJSONCodecBuilder{generator: g, jsonAlias: jsonAlias, errorType: jsonAlias + ".JSONError"}
	g.requireImport(pathpkg.Join(g.goModule, "trb/http"), "__trb_http")
	encoder := builder.encoder(call.Codec)
	encoded := jsonAlias + ".Stringify(" + encoder + "(" + arguments[0] + "))"
	responseType := "*" + webAlias + ".Response"
	headers := "__trb_http.NewHeaders(" + g.arrayReference("[]__trb_http.Header{{Name: \"content-type\", Value: \"application/json; charset=utf-8\"}}") + ")"
	return "func() " + responseType + " { " + builder.source.String() + " encoded := " + encoded + "; if encoded.Kind == " + resultAlias + ".ResultErrTag { return " + webAlias + ".NewResponse(map[string]any{\"status\": 500, \"headers\": " + headers + ", \"body\": __trb_http.NewBody([]byte(\"{\\\"error\\\":\\\"internal_server_error\\\"}\"))}) }; return " + webAlias + ".NewResponse(map[string]any{\"status\": " + status + ", \"headers\": " + headers + ", \"body\": __trb_http.NewBody([]byte(encoded.OkValue))}) }()"
}
