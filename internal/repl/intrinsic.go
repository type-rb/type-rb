package repl

import (
	"bytes"
	stdhmac "crypto/hmac"
	stdmd5 "crypto/md5"
	stdcryptorand "crypto/rand"
	stdsha1 "crypto/sha1"
	stdsha256 "crypto/sha256"
	stdsha512 "crypto/sha512"
	stdbase64 "encoding/base64"
	stdhex "encoding/hex"
	"errors"
	"fmt"
	"math"
	stdrand "math/rand/v2"
	"os"
	"os/exec"
	pathpkg "path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) intrinsic(name string, arguments []evaluatedArgument, typ types.Type, codec *ir.CodecSchema) (Value, error) {
	return e.intrinsicCall(name, arguments, typ, codec, nil)
}

func (e *Evaluator) intrinsicCall(name string, arguments []evaluatedArgument, typ types.Type, codec *ir.CodecSchema, call *ir.Call) (Value, error) {
	if value, handled, err := e.runtimeCall(runtimeInvocation{
		Name: name, Arguments: arguments, Type: typ, Codec: codec, Call: call,
	}); handled {
		return value, err
	}
	if value, handled, err := e.timeIntrinsic(name, arguments, typ); handled {
		return value, err
	}
	values := func() []Value {
		result := make([]Value, len(arguments))
		for index, argument := range arguments {
			result[index] = argument.Value
		}
		return result
	}()
	if name == "trb.internal.runtime.fail" {
		if len(values) != 1 {
			return Value{}, errors.New("runtime.fail expects one argument")
		}
		message, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("runtime.fail expects String")
		}
		return Value{}, errors.New(message)
	}
	require := func(count int) error {
		if len(values) < count {
			return fmt.Errorf("intrinsic %s requires %d arguments", name, count)
		}
		return nil
	}
	switch name {
	case "trb.web.request_query":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return e.webParameterBinding(values[0], typ, codec, "query")
	case "trb.web.context_params":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return e.webParameterBinding(values[0], typ, codec, "path")
	case "trb.web.context_bind":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return e.webEndpointInput(values[0], typ, codec)
	case "trb.web.context_with":
		if err := require(3); err != nil {
			return Value{}, err
		}
		return e.webContextWith(values[0], values[1], values[2], typ)
	case "trb.web.context_with_request":
		if err := require(2); err != nil {
			return Value{}, err
		}
		return e.webContextWithRequest(values[0], values[1], typ)
	case "trb.web.context_fetch":
		if err := require(2); err != nil {
			return Value{}, err
		}
		return e.webContextFetch(values[0], values[1], typ)
	case "trb.web.request_json":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return e.webRequestJSON(values[0], typ, codec)
	case "trb.std.io.puts":
		if err := require(1); err != nil {
			return Value{}, err
		}
		if value, ok := values[0].Data.(float64); ok {
			fmt.Fprintln(e.stdout, portableFloatText(value))
			return Value{Type: typ}, nil
		}
		fmt.Fprintln(e.stdout, plain(values[0]))
		return Value{Type: typ}, nil
	case "trb.std.path.separator":
		return Value{Type: typ, Data: "/"}, nil
	case "trb.std.path.clean":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("path.clean expects String")
		}
		return Value{Type: typ, Data: pathpkg.Clean(value)}, nil
	case "trb.std.path.join":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(string)
		right, rightOK := values[1].Data.(string)
		if !leftOK || !rightOK {
			return Value{}, errors.New("path.join expects String values")
		}
		if left == "" {
			return Value{Type: typ, Data: pathpkg.Clean(right)}, nil
		}
		if right == "" {
			return Value{Type: typ, Data: pathpkg.Clean(left)}, nil
		}
		return Value{Type: typ, Data: pathpkg.Clean(left + "/" + right)}, nil
	case "trb.std.path.absolute":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		return Value{Type: typ, Data: ok && strings.HasPrefix(value, "/")}, nil
	case "trb.std.path.components":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("path.components expects String")
		}
		parts := strings.Split(pathpkg.Clean(value), "/")
		items := make([]Value, 0, len(parts))
		for _, part := range parts {
			if part != "" && part != "." {
				items = append(items, Value{Type: types.FromName("String"), Data: part})
			}
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.path.base", "trb.std.path.directory":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("path.base/directory expects String")
		}
		cleaned := pathpkg.Clean(value)
		if name == "trb.std.path.base" {
			return Value{Type: typ, Data: pathpkg.Base(cleaned)}, nil
		}
		return Value{Type: typ, Data: pathpkg.Dir(cleaned)}, nil
	case "trb.std.url.encode_component":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("url.encode_component expects String")
		}
		return Value{Type: typ, Data: encodeURLComponent(value)}, nil
	case "trb.std.url.decode_component":
		if err := require(1); err != nil {
			return Value{}, err
		}
		input, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("url.decode_component expects String")
		}
		value, kind, message := decodeURLComponent(input)
		if kind != "" {
			return e.percentDecodeResultErr(typ, kind, input, message)
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("String"), Data: value})
	case "trb.internal.filesystem.exists":
		if err := require(1); err != nil {
			return Value{}, err
		}
		path, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("filesystem.exists expects String")
		}
		_, err := os.Stat(path)
		if err == nil {
			return e.filesystemOK(typ, Value{Type: types.FromName("Boolean"), Data: true})
		}
		if os.IsNotExist(err) {
			return e.filesystemOK(typ, Value{Type: types.FromName("Boolean"), Data: false})
		}
		return e.filesystemErr(typ, "exists", path, err)
	case "trb.internal.filesystem.read_text", "trb.internal.filesystem.read_bytes":
		if err := require(1); err != nil {
			return Value{}, err
		}
		path, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("filesystem read expects String")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return e.filesystemErr(typ, strings.TrimPrefix(name, "trb.internal.filesystem."), path, err)
		}
		if name == "trb.internal.filesystem.read_text" {
			return e.filesystemOK(typ, Value{Type: types.FromName("String"), Data: strings.ToValidUTF8(string(data), "�")})
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("Bytes"), Data: bytesValue(append([]byte(nil), data...))})
	case "trb.internal.filesystem.write_text", "trb.internal.filesystem.write_bytes":
		if err := require(2); err != nil {
			return Value{}, err
		}
		path, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("filesystem write path expects String")
		}
		var data []byte
		if name == "trb.internal.filesystem.write_text" {
			value, stringOK := values[1].Data.(string)
			if !stringOK {
				return Value{}, errors.New("filesystem.write_text expects String")
			}
			data = []byte(value)
		} else {
			value, bytesOK := values[1].Data.(bytesValue)
			if !bytesOK {
				return Value{}, errors.New("filesystem.write_bytes expects Bytes")
			}
			data = []byte(value)
		}
		operation := strings.TrimPrefix(name, "trb.internal.filesystem.")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return e.filesystemErr(typ, operation, path, err)
		}
		unit, err := e.unitValue()
		if err != nil {
			return Value{}, err
		}
		return e.filesystemOK(typ, unit)
	case "trb.internal.filesystem.create_directory":
		if err := require(1); err != nil {
			return Value{}, err
		}
		path, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("filesystem.create_directory expects String")
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return e.filesystemErr(typ, "create_directory", path, err)
		}
		unit, err := e.unitValue()
		if err != nil {
			return Value{}, err
		}
		return e.filesystemOK(typ, unit)
	case "trb.internal.filesystem.list":
		if err := require(1); err != nil {
			return Value{}, err
		}
		path, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("filesystem.list expects String")
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return e.filesystemErr(typ, "list", path, err)
		}
		names := make([]string, len(entries))
		for index, entry := range entries {
			names[index] = entry.Name()
		}
		sort.Strings(names)
		items := make([]Value, len(names))
		for index, item := range names {
			items[index] = Value{Type: types.FromName("String"), Data: item}
		}
		arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
		return e.filesystemOK(typ, Value{Type: arrayType, Data: &arrayValue{Items: items}})
	case "trb.internal.process.arguments":
		arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
		return Value{Type: arrayType, Data: &arrayValue{}}, nil
	case "trb.internal.process.environment":
		if err := require(1); err != nil {
			return Value{}, err
		}
		name, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("process.environment expects String")
		}
		value, found := os.LookupEnv(name)
		if !found {
			return Value{Type: typ, Data: nil}, nil
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.internal.process.working_directory":
		directory, err := os.Getwd()
		if err != nil {
			return e.processErr(typ, "working_directory", "", err)
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("String"), Data: directory})
	case "trb.internal.process.run":
		if err := require(2); err != nil {
			return Value{}, err
		}
		commandName, commandOK := values[0].Data.(string)
		argumentValues, argumentsOK := values[1].Data.(*arrayValue)
		if !commandOK || !argumentsOK {
			return Value{}, errors.New("process.run expects a String command and Array<String> arguments")
		}
		commandArguments := make([]string, len(argumentValues.Items))
		for index, argument := range argumentValues.Items {
			value, ok := argument.Data.(string)
			if !ok {
				return Value{}, errors.New("process.run arguments must be String")
			}
			commandArguments[index] = value
		}
		command := exec.CommandContext(e.context, commandName, commandArguments...)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		runErr := command.Run()
		status := int64(0)
		if runErr != nil {
			var exitError *exec.ExitError
			if errors.As(runErr, &exitError) {
				status = int64(exitError.ExitCode())
			} else {
				return e.processErr(typ, "run", commandName, runErr)
			}
		}
		definition, ok := e.definitions[symbolKey("trb/std/process/index", "ProcessResult")].(*recordDefinition)
		if !ok {
			return Value{}, errors.New("process.run requires trb/std/process")
		}
		fields := map[string]Value{
			"status":  {Type: types.FromName("Integer"), Data: status},
			"stdout":  {Type: types.FromName("String"), Data: strings.ToValidUTF8(stdout.String(), "�")},
			"stderr":  {Type: types.FromName("String"), Data: strings.ToValidUTF8(stderr.String(), "�")},
			"success": {Type: types.FromName("Boolean"), Data: status == 0},
		}
		value := Value{Type: types.FromName("ProcessResult"), Data: &recordInstance{Definition: definition, Fields: fields}}
		return e.filesystemOK(typ, value)
	case "trb.internal.json.parse", "trb.internal.json.parse_jsonc":
		if err := require(1); err != nil {
			return Value{}, err
		}
		source, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("json.parse expects String")
		}
		if name == "trb.internal.json.parse_jsonc" {
			source = stripJSONC(source)
		}
		return e.parseJSON(typ, source)
	case "trb.internal.json.stringify":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return e.stringifyJSON(typ, values[0])
	case "trb.internal.json.decode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		source, ok := values[0].Data.(string)
		if !ok || codec == nil {
			return Value{}, errors.New("json.decode requires a checked codec and String source")
		}
		return e.decodeJSONCodec(typ, source, codec)
	case "trb.internal.json.encode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		if codec == nil {
			return Value{}, errors.New("json.encode requires a checked codec")
		}
		return e.encodeJSONCodec(typ, values[0], codec)
	case "trb.std.strings.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.length expects String")
		}
		return Value{Type: typ, Data: int64(utf8.RuneCountInString(value))}, nil
	case "trb.std.strings.empty":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.empty expects String")
		}
		return Value{Type: typ, Data: value == ""}, nil
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("String trimming expects String")
		}
		switch name {
		case "trb.std.strings.lstrip":
			value = strings.TrimLeftFunc(value, unicode.IsSpace)
		case "trb.std.strings.rstrip":
			value = strings.TrimRightFunc(value, unicode.IsSpace)
		default:
			value = strings.TrimFunc(value, unicode.IsSpace)
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.std.strings.uppercase", "trb.std.strings.lowercase":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("string intrinsic expects String")
		}
		if name == "trb.std.strings.uppercase" {
			value = strings.ToUpper(value)
		} else {
			value = strings.ToLower(value)
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.std.strings.starts_with", "trb.std.strings.ends_with":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		part, partOK := values[1].Data.(string)
		if !valueOK || !partOK {
			return Value{}, errors.New("strings.starts_with/ends_with expects String values")
		}
		if name == "trb.std.strings.starts_with" {
			return Value{Type: typ, Data: strings.HasPrefix(value, part)}, nil
		}
		return Value{Type: typ, Data: strings.HasSuffix(value, part)}, nil
	case "trb.std.strings.split":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		separator, separatorOK := values[1].Data.(string)
		if !valueOK || !separatorOK {
			return Value{}, errors.New("strings.split expects String values")
		}
		if separator == "" {
			return Value{}, errors.New("String split separator is empty")
		}
		parts := strings.Split(value, separator)
		items := make([]Value, len(parts))
		for index, part := range parts {
			items[index] = Value{Type: types.FromName("String"), Data: part}
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.strings.contains":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(string)
		right, rightOK := values[1].Data.(string)
		if !leftOK || !rightOK {
			return Value{}, errors.New("strings.contains expects String arguments")
		}
		return Value{Type: typ, Data: strings.Contains(left, right)}, nil
	case "trb.std.strings.replace_all":
		if err := require(3); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		pattern, patternOK := values[1].Data.(string)
		replacement, replacementOK := values[2].Data.(string)
		if !valueOK || !patternOK || !replacementOK {
			return Value{}, errors.New("strings.replace_all expects String arguments")
		}
		if pattern == "" {
			return Value{}, errors.New("String replacement pattern is empty")
		}
		return Value{Type: typ, Data: strings.ReplaceAll(value, pattern, replacement)}, nil
	case "trb.std.strings.codepoints":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.codepoints expects String")
		}
		items := make([]Value, 0, utf8.RuneCountInString(value))
		for _, codepoint := range value {
			items = append(items, Value{Type: types.FromName("Integer"), Data: int64(codepoint)})
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.strings.characters":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.characters expects String")
		}
		items := make([]Value, 0, utf8.RuneCountInString(value))
		for _, character := range value {
			items = append(items, Value{Type: types.FromName("String"), Data: string(character)})
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.strings.reverse":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.reverse expects String")
		}
		characters := []rune(value)
		slices.Reverse(characters)
		return Value{Type: typ, Data: string(characters)}, nil
	case "trb.std.strings.try_fetch":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		index, indexOK := values[1].Data.(int64)
		if !valueOK || !indexOK {
			return Value{}, errors.New("strings.try_fetch expects String and Integer")
		}
		characters := []rune(value)
		if index < 0 || index >= int64(len(characters)) {
			return e.indexLookupResultErr(typ, index, int64(len(characters)), "String index is out of bounds")
		}
		result := Value{Type: types.FromName("String"), Data: string(characters[index])}
		return e.filesystemOK(typ, result)
	case "trb.std.strings.slice", "trb.std.strings.try_slice":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		bounds, boundsOK := values[1].Data.(*rangeValue)
		if !valueOK || !boundsOK {
			return Value{}, errors.New("strings.slice expects String and Range<Integer>")
		}
		characters := []rune(value)
		stop, valid := sliceStop(bounds, int64(len(characters)))
		if !valid {
			if name == "trb.std.strings.try_slice" {
				return e.sliceRangeResultErr(typ, bounds, int64(len(characters)), "String slice range is out of bounds")
			}
			return Value{}, errors.New("String slice range is out of bounds")
		}
		result := Value{Type: types.FromName("String"), Data: string(characters[bounds.Start:stop])}
		if name == "trb.std.strings.try_slice" {
			return e.filesystemOK(typ, result)
		}
		return result, nil
	case "trb.std.strings.index", "trb.std.strings.rindex":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(string)
		substring, substringOK := values[1].Data.(string)
		if !valueOK || !substringOK {
			return Value{}, errors.New("strings.index expects String arguments")
		}
		characters, needle := []rune(value), []rune(substring)
		found := codepointIndex(characters, needle, name == "trb.std.strings.rindex")
		return Value{Type: typ, Data: found}, nil
	case "trb.std.unicode.version":
		return Value{Type: typ, Data: unicode.Version}, nil
	case "trb.std.unicode.valid_scalar":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(int64)
		return Value{Type: typ, Data: ok && validUnicodeScalar(value)}, nil
	case "trb.std.unicode.letter", "trb.std.unicode.digit", "trb.std.unicode.uppercase", "trb.std.unicode.lowercase", "trb.std.unicode.whitespace", "trb.std.unicode.identifier_start", "trb.std.unicode.identifier_part":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(int64)
		if !ok || !validUnicodeScalar(value) {
			return Value{Type: typ, Data: false}, nil
		}
		codepoint := rune(value)
		result := false
		switch name {
		case "trb.std.unicode.letter":
			result = unicode.Is(unicode.Letter, codepoint)
		case "trb.std.unicode.digit":
			result = unicode.Is(unicode.Digit, codepoint)
		case "trb.std.unicode.uppercase":
			result = unicode.Is(unicode.Upper, codepoint)
		case "trb.std.unicode.lowercase":
			result = unicode.Is(unicode.Lower, codepoint)
		case "trb.std.unicode.whitespace":
			result = unicode.Is(unicode.White_Space, codepoint)
		case "trb.std.unicode.identifier_start":
			result = value == 95 || value == 64 || unicode.Is(unicode.Letter, codepoint)
		case "trb.std.unicode.identifier_part":
			result = value == 95 || value == 64 || unicode.Is(unicode.Letter, codepoint) || unicode.Is(unicode.Digit, codepoint)
		}
		return Value{Type: typ, Data: result}, nil
	case "trb.std.unicode.from_codepoint":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(int64)
		if !ok || !validUnicodeScalar(value) {
			return Value{}, errors.New("invalid Unicode code point")
		}
		return Value{Type: typ, Data: string(rune(value))}, nil
	case "trb.std.bytes.from_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("bytes.from_string expects String")
		}
		return Value{Type: typ, Data: bytesValue([]byte(value))}, nil
	case "trb.std.bytes.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.to_string expects Bytes")
		}
		return Value{Type: typ, Data: strings.ToValidUTF8(string(value), "�")}, nil
	case "trb.std.bytes.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.length expects Bytes")
		}
		return Value{Type: typ, Data: int64(len(value))}, nil
	case "trb.std.bytes.at":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(bytesValue)
		index, indexOK := values[1].Data.(int64)
		if !valueOK || !indexOK {
			return Value{}, errors.New("bytes.at expects Bytes and Integer")
		}
		if index < 0 || index >= int64(len(value)) {
			return Value{}, errors.New("Bytes index is out of bounds")
		}
		return Value{Type: typ, Data: int64(value[index])}, nil
	case "trb.std.bytes.concat":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(bytesValue)
		right, rightOK := values[1].Data.(bytesValue)
		if !leftOK || !rightOK {
			return Value{}, errors.New("bytes.concat expects Bytes arguments")
		}
		result := append(bytesValue(nil), left...)
		result = append(result, right...)
		return Value{Type: typ, Data: result}, nil
	case "trb.std.bytes.valid_utf8":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.valid_utf8 expects Bytes")
		}
		return Value{Type: typ, Data: utf8.Valid(value)}, nil
	case "trb.std.encoding.hex.encode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("hex.encode expects Bytes")
		}
		return Value{Type: typ, Data: stdhex.EncodeToString(value)}, nil
	case "trb.std.encoding.hex.decode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		input, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("hex.decode expects String")
		}
		length := int64(0)
		for _, character := range input {
			if !hexadecimalCharacter(character) {
				return e.hexDecodeResultErr(typ, "InvalidCharacter", input, length, "invalid hexadecimal character")
			}
			length++
		}
		if length%2 != 0 {
			return e.hexDecodeResultErr(typ, "OddLength", input, length, "hex input has odd length")
		}
		value, err := stdhex.DecodeString(input)
		if err != nil {
			return Value{}, err
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("Bytes"), Data: bytesValue(value)})
	case "trb.std.encoding.base64.encode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("base64.encode expects Bytes")
		}
		return Value{Type: typ, Data: stdbase64.StdEncoding.EncodeToString(value)}, nil
	case "trb.std.encoding.base64.url_encode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("base64.url_encode expects Bytes")
		}
		return Value{Type: typ, Data: stdbase64.RawURLEncoding.EncodeToString(value)}, nil
	case "trb.std.encoding.base64.decode", "trb.std.encoding.base64.url_decode":
		if err := require(1); err != nil {
			return Value{}, err
		}
		input, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("base64 decode expects String")
		}
		urlSafe := name == "trb.std.encoding.base64.url_decode"
		value, kind, index, message := decodeBase64(input, urlSafe)
		if kind != "" {
			return e.base64DecodeResultErr(typ, kind, input, index, message)
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("Bytes"), Data: bytesValue(value)})
	case "trb.std.hash.md5":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("hash.md5 expects Bytes")
		}
		digest := stdmd5.Sum(value)
		return Value{Type: typ, Data: bytesValue(digest[:])}, nil
	case "trb.std.hash.sha1":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("hash.sha1 expects Bytes")
		}
		digest := stdsha1.Sum(value)
		return Value{Type: typ, Data: bytesValue(digest[:])}, nil
	case "trb.std.hash.sha256":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("hash.sha256 expects Bytes")
		}
		digest := stdsha256.Sum256(value)
		return Value{Type: typ, Data: bytesValue(digest[:])}, nil
	case "trb.std.hash.sha512":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("hash.sha512 expects Bytes")
		}
		digest := stdsha512.Sum512(value)
		return Value{Type: typ, Data: bytesValue(digest[:])}, nil
	case "trb.std.hmac.sha256", "trb.std.hmac.sha512":
		if err := require(2); err != nil {
			return Value{}, err
		}
		key, keyOK := values[0].Data.(bytesValue)
		message, messageOK := values[1].Data.(bytesValue)
		if !keyOK || !messageOK {
			return Value{}, errors.New("hmac digest expects Bytes arguments")
		}
		if name == "trb.std.hmac.sha256" {
			digest := stdhmac.New(stdsha256.New, key)
			_, _ = digest.Write(message)
			return Value{Type: typ, Data: bytesValue(digest.Sum(nil))}, nil
		}
		digest := stdhmac.New(stdsha512.New, key)
		_, _ = digest.Write(message)
		return Value{Type: typ, Data: bytesValue(digest.Sum(nil))}, nil
	case "trb.std.hmac.equal", "trb.std.secure_compare.equal":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(bytesValue)
		right, rightOK := values[1].Data.(bytesValue)
		if !leftOK || !rightOK {
			operation := "hmac.equal"
			if name == "trb.std.secure_compare.equal" {
				operation = "secure_compare.equal"
			}
			return Value{}, errors.New(operation + " expects Bytes arguments")
		}
		return Value{Type: typ, Data: stdhmac.Equal(left, right)}, nil
	case "trb.std.random.float":
		if err := require(0); err != nil {
			return Value{}, err
		}
		return Value{Type: typ, Data: stdrand.Float64()}, nil
	case "trb.std.random.integer":
		if err := require(1); err != nil {
			return Value{}, err
		}
		upper, ok := values[0].Data.(int64)
		if !ok {
			return Value{}, errors.New("random.integer expects Integer")
		}
		if upper <= 0 {
			return Value{}, errors.New("random.integer upper bound must be greater than zero")
		}
		return Value{Type: typ, Data: stdrand.Int64N(upper)}, nil
	case "trb.std.secure_random.bytes":
		if err := require(1); err != nil {
			return Value{}, err
		}
		length, ok := values[0].Data.(int64)
		if !ok {
			return Value{}, errors.New("secure_random.bytes expects Integer")
		}
		if length < 0 || length > 65536 {
			return Value{}, errors.New("secure_random.bytes length must be between 0 and 65536")
		}
		value := make([]byte, length)
		stdcryptorand.Read(value)
		return Value{Type: typ, Data: bytesValue(value)}, nil
	case "trb.std.string_builder.new":
		return Value{Type: typ, Data: &stringBuilderValue{}}, nil
	case "trb.std.string_builder.from_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("string_builder.from_string expects String")
		}
		builder := &stringBuilderValue{}
		builder.value.WriteString(value)
		return Value{Type: typ, Data: builder}, nil
	case "trb.std.string_builder.append":
		if err := require(2); err != nil {
			return Value{}, err
		}
		builder, builderOK := values[0].Data.(*stringBuilderValue)
		value, valueOK := values[1].Data.(string)
		if !builderOK || !valueOK {
			return Value{}, errors.New("string_builder.append expects StringBuilder and String")
		}
		builder.value.WriteString(value)
		return Value{Type: typ}, nil
	case "trb.std.string_builder.append_codepoint":
		if err := require(2); err != nil {
			return Value{}, err
		}
		builder, builderOK := values[0].Data.(*stringBuilderValue)
		value, valueOK := values[1].Data.(int64)
		if !builderOK || !valueOK {
			return Value{}, errors.New("string_builder.append_codepoint expects StringBuilder and Integer")
		}
		if value < 0 || value > utf8.MaxRune || value >= 0xd800 && value <= 0xdfff {
			return Value{}, errors.New("invalid Unicode code point")
		}
		builder.value.WriteRune(rune(value))
		return Value{Type: typ}, nil
	case "trb.std.string_builder.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		builder, ok := values[0].Data.(*stringBuilderValue)
		if !ok {
			return Value{}, errors.New("string_builder.length expects StringBuilder")
		}
		return Value{Type: typ, Data: int64(utf8.RuneCountInString(builder.value.String()))}, nil
	case "trb.std.string_builder.empty":
		if err := require(1); err != nil {
			return Value{}, err
		}
		builder, ok := values[0].Data.(*stringBuilderValue)
		if !ok {
			return Value{}, errors.New("string_builder.empty expects StringBuilder")
		}
		return Value{Type: typ, Data: builder.value.Len() == 0}, nil
	case "trb.std.string_builder.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		builder, ok := values[0].Data.(*stringBuilderValue)
		if !ok {
			return Value{}, errors.New("string_builder.to_string expects StringBuilder")
		}
		return Value{Type: typ, Data: builder.value.String()}, nil
	case "trb.std.string_builder.clear":
		if err := require(1); err != nil {
			return Value{}, err
		}
		builder, ok := values[0].Data.(*stringBuilderValue)
		if !ok {
			return Value{}, errors.New("string_builder.clear expects StringBuilder")
		}
		builder.value.Reset()
		return Value{Type: typ}, nil
	case "trb.std.arrays.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.length expects Array")
		}
		return Value{Type: typ, Data: int64(len(array.Items))}, nil
	case "trb.std.arrays.empty":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.empty expects Array")
		}
		return Value{Type: typ, Data: len(array.Items) == 0}, nil
	case "trb.std.arrays.try_fetch":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		index, integer := values[1].Data.(int64)
		if !ok || !integer {
			return Value{}, errors.New("arrays.try_fetch expects Array and Integer")
		}
		if index < 0 || index >= int64(len(array.Items)) {
			return e.indexLookupResultErr(typ, index, int64(len(array.Items)), "Array index is out of bounds")
		}
		result := array.Items[index]
		return e.filesystemOK(typ, result)
	case "trb.std.arrays.slice", "trb.std.arrays.try_slice":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, arrayOK := values[0].Data.(*arrayValue)
		bounds, boundsOK := values[1].Data.(*rangeValue)
		if !arrayOK || !boundsOK {
			return Value{}, errors.New("arrays.slice expects Array and Range<Integer>")
		}
		stop, valid := sliceStop(bounds, int64(len(array.Items)))
		if !valid {
			if name == "trb.std.arrays.try_slice" {
				return e.sliceRangeResultErr(typ, bounds, int64(len(array.Items)), "Array slice range is out of bounds")
			}
			return Value{}, errors.New("Array slice range is out of bounds")
		}
		items := append([]Value(nil), array.Items[bounds.Start:stop]...)
		resultType := typ
		if name == "trb.std.arrays.try_slice" && len(typ.Args) == 2 {
			resultType = typ.Args[0]
		}
		result := Value{Type: resultType, Data: &arrayValue{Items: items}}
		if name == "trb.std.arrays.try_slice" {
			return e.filesystemOK(typ, result)
		}
		return result, nil
	case "trb.std.arrays.first", "trb.std.arrays.last":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.first/last expects Array")
		}
		if len(array.Items) == 0 {
			return Value{}, errors.New("Array is empty")
		}
		index := 0
		if name == "trb.std.arrays.last" {
			index = len(array.Items) - 1
		}
		result := array.Items[index]
		result.Type = typ
		return result, nil
	case "trb.std.arrays.copy":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.copy expects Array")
		}
		items := append([]Value(nil), array.Items...)
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.arrays.contains", "trb.std.arrays.count":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.contains/count expects Array")
		}
		count := int64(0)
		for _, item := range array.Items {
			if equal(item, values[1]) {
				count++
			}
		}
		if name == "trb.std.arrays.contains" {
			return Value{Type: typ, Data: count > 0}, nil
		}
		return Value{Type: typ, Data: count}, nil
	case "trb.std.arrays.uniq":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.uniq expects Array")
		}
		items := make([]Value, 0, len(array.Items))
		for _, item := range array.Items {
			known := false
			for _, existing := range items {
				if equal(existing, item) {
					known = true
					break
				}
			}
			if !known {
				items = append(items, item)
			}
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.arrays.concat":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(*arrayValue)
		right, rightOK := values[1].Data.(*arrayValue)
		if !leftOK || !rightOK {
			return Value{}, errors.New("arrays.concat expects two Arrays")
		}
		items := make([]Value, 0, len(left.Items)+len(right.Items))
		items = append(items, left.Items...)
		items = append(items, right.Items...)
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.arrays.join":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		separator, separatorOK := values[1].Data.(string)
		if !ok || !separatorOK {
			return Value{}, errors.New("arrays.join expects Array<String> and String")
		}
		parts := make([]string, len(array.Items))
		for index, item := range array.Items {
			part, partOK := item.Data.(string)
			if !partOK {
				return Value{}, errors.New("arrays.join expects Array<String>")
			}
			parts[index] = part
		}
		return Value{Type: typ, Data: strings.Join(parts, separator)}, nil
	case "trb.std.arrays.pop":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.pop expects Array")
		}
		if len(array.Items) == 0 {
			return Value{}, errors.New("Array is empty")
		}
		index := len(array.Items) - 1
		result := array.Items[index]
		array.Items = array.Items[:index]
		result.Type = typ
		return result, nil
	case "trb.std.arrays.shift":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.shift expects Array")
		}
		if len(array.Items) == 0 {
			return Value{}, errors.New("Array is empty")
		}
		result := array.Items[0]
		array.Items = array.Items[1:]
		result.Type = typ
		return result, nil
	case "trb.std.arrays.push":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.push expects Array")
		}
		array.Items = append(array.Items, values[1])
		return Value{Type: typ}, nil
	case "trb.std.arrays.unshift":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.unshift expects Array")
		}
		array.Items = append(array.Items, Value{})
		copy(array.Items[1:], array.Items)
		array.Items[0] = values[1]
		return Value{Type: typ}, nil
	case "trb.std.arrays.reverse":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.reverse expects Array")
		}
		items := make([]Value, len(array.Items))
		for index := range array.Items {
			items[len(array.Items)-1-index] = array.Items[index]
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.arrays.sort", "trb.std.arrays.sort_descending":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.sort expects Array")
		}
		items := append([]Value(nil), array.Items...)
		descending := name == "trb.std.arrays.sort_descending"
		var compareErr error
		sort.SliceStable(items, func(left, right int) bool {
			compared, err := comparePortableValues(items[left], items[right])
			if err != nil {
				compareErr = err
				return false
			}
			if descending {
				return compared > 0
			}
			return compared < 0
		})
		if compareErr != nil {
			return Value{}, compareErr
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.hashes.length", "trb.std.hashes.empty":
		if err := require(1); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.length/empty expects Hash")
		}
		if name == "trb.std.hashes.empty" {
			return Value{Type: typ, Data: len(hash.Entries) == 0}, nil
		}
		return Value{Type: typ, Data: int64(len(hash.Entries))}, nil
	case "trb.std.hashes.fetch":
		if err := require(2); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.fetch expects Hash")
		}
		for _, entry := range hash.Entries {
			if equal(entry.Key, values[1]) {
				result := entry.Value
				result.Type = typ
				return result, nil
			}
		}
		return Value{}, errors.New("Hash key is missing")
	case "trb.std.hashes.try_fetch":
		if err := require(2); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.try_fetch expects Hash")
		}
		for _, entry := range hash.Entries {
			if equal(entry.Key, values[1]) {
				return e.filesystemOK(typ, entry.Value)
			}
		}
		return e.keyLookupResultErr(typ, values[1], "Hash key is missing")
	case "trb.std.hashes.contains_key":
		if err := require(2); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.contains_key expects Hash")
		}
		for _, entry := range hash.Entries {
			if equal(entry.Key, values[1]) {
				return Value{Type: typ, Data: true}, nil
			}
		}
		return Value{Type: typ, Data: false}, nil
	case "trb.std.hashes.keys", "trb.std.hashes.values":
		if err := require(1); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.keys/values expects Hash")
		}
		items := make([]Value, 0, len(hash.Entries))
		for _, entry := range hash.Entries {
			if name == "trb.std.hashes.keys" {
				items = append(items, entry.Key)
			} else {
				items = append(items, entry.Value)
			}
		}
		return Value{Type: typ, Data: &arrayValue{Items: items}}, nil
	case "trb.std.hashes.copy":
		if err := require(1); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.copy expects Hash")
		}
		entries := append([]hashEntry(nil), hash.Entries...)
		return Value{Type: typ, Data: &hashValue{Entries: entries}}, nil
	case "trb.std.hashes.delete":
		if err := require(2); err != nil {
			return Value{}, err
		}
		hash, ok := values[0].Data.(*hashValue)
		if !ok {
			return Value{}, errors.New("hashes.delete expects Hash")
		}
		for index, entry := range hash.Entries {
			if equal(entry.Key, values[1]) {
				result := entry.Value
				hash.Entries = append(hash.Entries[:index], hash.Entries[index+1:]...)
				result.Type = typ
				return result, nil
			}
		}
		return Value{}, errors.New("Hash key is missing")
	case "trb.std.hashes.merge", "trb.std.hashes.update":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(*hashValue)
		right, rightOK := values[1].Data.(*hashValue)
		if !leftOK || !rightOK {
			return Value{}, errors.New("hashes.merge/update expects Hash values")
		}
		target := left
		if name == "trb.std.hashes.merge" {
			target = &hashValue{Entries: append([]hashEntry(nil), left.Entries...)}
		}
		for _, incoming := range right.Entries {
			replaced := false
			for index, existing := range target.Entries {
				if equal(existing.Key, incoming.Key) {
					target.Entries[index].Value = incoming.Value
					replaced = true
					break
				}
			}
			if !replaced {
				target.Entries = append(target.Entries, incoming)
			}
		}
		if name == "trb.std.hashes.merge" {
			return Value{Type: typ, Data: target}, nil
		}
		return Value{Type: typ}, nil
	case "trb.std.numbers.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return Value{Type: typ, Data: plain(values[0])}, nil
	case "trb.std.numbers.integer_to_float":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(int64)
		if !ok {
			return Value{}, errors.New("numbers.to_float expects Integer")
		}
		return Value{Type: typ, Data: float64(value)}, nil
	case "trb.std.numbers.integer_min", "trb.std.numbers.integer_max":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(int64)
		right, rightOK := values[1].Data.(int64)
		if !leftOK || !rightOK {
			return Value{}, errors.New("numbers min/max expect Integer values")
		}
		if name == "trb.std.numbers.integer_min" && right < left || name == "trb.std.numbers.integer_max" && right > left {
			left = right
		}
		return Value{Type: typ, Data: left}, nil
	case "trb.std.numbers.integer_clamp":
		if err := require(3); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(int64)
		minimum, minimumOK := values[1].Data.(int64)
		maximum, maximumOK := values[2].Data.(int64)
		if !valueOK || !minimumOK || !maximumOK {
			return Value{}, errors.New("numbers.clamp expects Integer values")
		}
		if minimum > maximum {
			return Value{}, errors.New("clamp minimum exceeds maximum")
		}
		if value < minimum {
			value = minimum
		} else if value > maximum {
			value = maximum
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.std.numbers.integer_absolute",
		"trb.std.numbers.integer_zero",
		"trb.std.numbers.integer_positive",
		"trb.std.numbers.integer_negative",
		"trb.std.numbers.integer_even",
		"trb.std.numbers.integer_odd":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(int64)
		if !ok {
			return Value{}, errors.New("numbers Integer predicate expects Integer")
		}
		switch name {
		case "trb.std.numbers.integer_absolute":
			if value < 0 {
				value = -value
			}
			return Value{Type: typ, Data: value}, nil
		case "trb.std.numbers.integer_zero":
			return Value{Type: typ, Data: value == 0}, nil
		case "trb.std.numbers.integer_positive":
			return Value{Type: typ, Data: value > 0}, nil
		case "trb.std.numbers.integer_negative":
			return Value{Type: typ, Data: value < 0}, nil
		case "trb.std.numbers.integer_even":
			return Value{Type: typ, Data: value%2 == 0}, nil
		default:
			return Value{Type: typ, Data: value%2 != 0}, nil
		}
	case "trb.std.numbers.float_to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(float64)
		if !ok {
			return Value{}, errors.New("numbers.float_to_string expects Float")
		}
		return Value{Type: typ, Data: portableFloatText(value)}, nil
	case "trb.std.numbers.float_to_integer":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(float64)
		if !ok {
			return Value{}, errors.New("numbers.truncate expects Float")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Value{}, errors.New("Float cannot be converted to Integer")
		}
		if value < -9007199254740991 || value > 9007199254740991 {
			return Value{}, errors.New("Integer is outside the portable range")
		}
		return Value{Type: typ, Data: int64(math.Trunc(value))}, nil
	case "trb.std.numbers.float_floor", "trb.std.numbers.float_ceil", "trb.std.numbers.float_round":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(float64)
		if !ok {
			return Value{}, errors.New("numbers rounding expects Float")
		}
		switch name {
		case "trb.std.numbers.float_floor":
			value = math.Floor(value)
		case "trb.std.numbers.float_ceil":
			value = math.Ceil(value)
		default:
			value = math.Round(value)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Value{}, errors.New("Float cannot be converted to Integer")
		}
		if value < -9007199254740991 || value > 9007199254740991 {
			return Value{}, errors.New("Integer is outside the portable range")
		}
		return Value{Type: typ, Data: int64(value)}, nil
	case "trb.std.numbers.float_absolute",
		"trb.std.numbers.float_finite",
		"trb.std.numbers.float_infinite",
		"trb.std.numbers.float_nan":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(float64)
		if !ok {
			return Value{}, errors.New("numbers Float predicate expects Float")
		}
		switch name {
		case "trb.std.numbers.float_absolute":
			return Value{Type: typ, Data: math.Abs(value)}, nil
		case "trb.std.numbers.float_finite":
			return Value{Type: typ, Data: !math.IsNaN(value) && !math.IsInf(value, 0)}, nil
		case "trb.std.numbers.float_infinite":
			return Value{Type: typ, Data: math.IsInf(value, 0)}, nil
		default:
			return Value{Type: typ, Data: math.IsNaN(value)}, nil
		}
	case "trb.std.numbers.parse_integer":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("numbers.parse_integer expects String")
		}
		parsed, message := parsePortableInteger(value)
		if message != "" {
			return Value{}, errors.New(message)
		}
		return Value{Type: typ, Data: parsed}, nil
	case "trb.std.numbers.try_parse_integer":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("numbers.try_parse_integer expects String")
		}
		parsed, message := parsePortableInteger(value)
		if message != "" {
			kind := "InvalidFormat"
			if message == "Integer is outside the portable range" {
				kind = "OutOfRange"
			}
			return e.numberParseResultErr(typ, kind, value, message)
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("Integer"), Data: parsed})
	case "trb.std.numbers.parse_float":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("numbers.parse_float expects String")
		}
		parsed, message := parsePortableFloat(value)
		if message != "" {
			return Value{}, errors.New(message)
		}
		return Value{Type: typ, Data: parsed}, nil
	case "trb.std.numbers.try_parse_float":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("numbers.try_parse_float expects String")
		}
		parsed, message := parsePortableFloat(value)
		if message != "" {
			kind := "InvalidFormat"
			if message == "Float is outside the portable range" {
				kind = "OutOfRange"
			}
			return e.numberParseResultErr(typ, kind, value, message)
		}
		return e.filesystemOK(typ, Value{Type: types.FromName("Float"), Data: parsed})
	case "trb.std.math.sqrt", "trb.std.math.exp", "trb.std.math.log", "trb.std.math.log2", "trb.std.math.log10":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(float64)
		if !ok {
			return Value{}, errors.New("math function expects Float")
		}
		switch name {
		case "trb.std.math.sqrt":
			value = math.Sqrt(value)
		case "trb.std.math.exp":
			value = math.Exp(value)
		case "trb.std.math.log":
			value = math.Log(value)
		case "trb.std.math.log2":
			value = math.Log2(value)
		default:
			value = math.Log10(value)
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.std.booleans.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bool)
		if !ok {
			return Value{}, errors.New("booleans.to_string expects Boolean")
		}
		return Value{Type: typ, Data: strconv.FormatBool(value)}, nil
	case "trb.platform.typescript.node.argv":
		return Value{Type: typ, Data: &arrayValue{}}, nil
	case "trb.platform.go.context.background", "trb.platform.go.context.todo":
		return Value{Type: typ, Data: map[string]string{"context": strings.TrimPrefix(name, "trb.platform.go.context.")}}, nil
	default:
		return Value{}, fmt.Errorf("intrinsic %s is type-checked for mode %s but has no REPL runtime adapter", name, e.mode)
	}
}

func sliceStop(bounds *rangeValue, size int64) (int64, bool) {
	if bounds == nil || bounds.Start < 0 || bounds.End < 0 || bounds.Start > bounds.End {
		return 0, false
	}
	if bounds.Exclusive {
		return bounds.End, bounds.End <= size
	}
	return bounds.End + 1, bounds.End < size
}

func codepointIndex(characters, needle []rune, reverse bool) any {
	if len(needle) == 0 {
		if reverse {
			return int64(len(characters))
		}
		return int64(0)
	}
	if len(needle) > len(characters) {
		return nil
	}
	start, stop, step := 0, len(characters)-len(needle), 1
	if reverse {
		start, stop, step = stop, 0, -1
	}
	for index := start; ; index += step {
		if index >= 0 && index+len(needle) <= len(characters) {
			matched := true
			for offset := range needle {
				if characters[index+offset] != needle[offset] {
					matched = false
					break
				}
			}
			if matched {
				return int64(index)
			}
		}
		if index == stop {
			break
		}
	}
	return nil
}
