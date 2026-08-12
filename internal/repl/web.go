package repl

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) webRequestJSON(receiver Value, resultType types.Type, schema *ir.CodecSchema) (Value, error) {
	if schema == nil {
		return Value{}, errors.New("Request#json() requires a checked codec")
	}
	request, ok := receiver.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("Request#json() requires Request")
	}
	requestError := func(variant string, fields map[string]Value) (Value, error) {
		resultDefinition, resultOK := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
		errorDefinition, errorOK := e.definitions[symbolKey("trb/web/index", "RequestError")].(*enumDefinition)
		if !resultOK || !errorOK {
			return Value{}, errors.New("Request#json() requires trb/web and trb/std/result")
		}
		errorValue := Value{Type: types.FromName("RequestError"), Data: &enumValue{Definition: errorDefinition, Name: variant, Payload: fields}}
		return Value{Type: resultType, Data: &enumValue{Definition: resultDefinition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
	}
	headers := request.Fields["@headers"]
	values, err := e.callMember(headers, "values", request.Definition.Module)
	if err != nil {
		return Value{}, err
	}
	contentTypes, err := e.call(values.Data.(*callable), []evaluatedArgument{{Value: Value{Type: types.FromName("String"), Data: "content-type"}}})
	if err != nil {
		return Value{}, err
	}
	items := contentTypes.Data.(*arrayValue).Items
	if len(items) == 0 {
		return requestError("MissingContentType", nil)
	}
	if len(items) != 1 {
		return requestError("DuplicateContentType", nil)
	}
	contentType := items[0].Data.(string)
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if mediaType != "application/json" && !(strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")) {
		return requestError("UnsupportedContentType", map[string]Value{"value": {Type: types.FromName("String"), Data: contentType}})
	}
	body, ok := request.Fields["@body"].Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("Request body is invalid")
	}
	data, ok := body.Fields["@_bytes"].Data.(bytesValue)
	if !ok {
		return Value{}, errors.New("Request body bytes are invalid")
	}
	if !utf8.Valid(data) {
		return requestError("InvalidUtf8", nil)
	}
	jsonResult := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{schema.Type, types.FromName("JsonError")}}
	decoded, err := e.decodeJSONCodec(jsonResult, string(data), schema)
	if err != nil {
		return Value{}, err
	}
	variant := decoded.Data.(*enumValue)
	if variant.Name == "Err" {
		return requestError("InvalidJson", map[string]Value{"error": variant.Payload["error"]})
	}
	return e.resultOK(resultType, variant.Payload["value"])
}

func (e *Evaluator) webParameterBinding(receiver Value, resultType types.Type, schema *ir.CodecSchema, source string) (Value, error) {
	if schema == nil || schema.Kind != "record" {
		return Value{}, errors.New("web parameter binding requires a checked record schema")
	}
	values := map[string][]string{}
	if source == "query" {
		object, ok := receiver.Data.(*objectInstance)
		if !ok {
			return Value{}, errors.New("Request#query() requires Request")
		}
		method := classMethod(object.Definition, "query_parameters", false)
		if method == nil {
			return Value{}, errors.New("Request#query() requires Request#query_parameters()")
		}
		parsed, err := e.call(&callable{Method: method, Receiver: receiver, Module: object.Definition.Module}, nil)
		if err != nil {
			return Value{}, err
		}
		result, ok := parsed.Data.(*enumValue)
		if !ok {
			return Value{}, errors.New("Request#query_parameters() returned an invalid value")
		}
		if result.Name == "Err" {
			return e.webParameterResultErr(resultType, "MalformedQuery", map[string]Value{"error": result.Payload["error"]})
		}
		array, ok := result.Payload["value"].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("Request#query_parameters() returned invalid parameters")
		}
		for _, item := range array.Items {
			parameter, ok := item.Data.(*recordInstance)
			if !ok {
				return Value{}, errors.New("Request#query_parameters() returned an invalid parameter")
			}
			name, nameOK := parameter.Fields["name"].Data.(string)
			value, valueOK := parameter.Fields["value"].Data.(string)
			if !nameOK || !valueOK {
				return Value{}, errors.New("Request#query_parameters() returned a non-string parameter")
			}
			values[name] = append(values[name], value)
		}
	} else {
		object, ok := receiver.Data.(*objectInstance)
		if !ok {
			return Value{}, errors.New("Context#params() requires Context")
		}
		method := classMethod(object.Definition, "path_value", false)
		if method == nil {
			return Value{}, errors.New("Context#params() requires Context#path_value()")
		}
		for _, field := range schema.Fields {
			value, err := e.call(&callable{Method: method, Receiver: receiver, Module: object.Definition.Module}, []evaluatedArgument{{Value: Value{Type: types.FromName("String"), Data: field.Name}}})
			if err != nil {
				return Value{}, err
			}
			raw, ok := value.Data.(string)
			if !ok {
				return Value{}, fmt.Errorf("path parameter %s is not a String", field.Name)
			}
			values[field.Name] = []string{raw}
		}
	}

	recordDefinition, ok := e.definitions[symbolKey(schema.Module, schema.Type.Name)].(*recordDefinition)
	if !ok {
		return Value{}, fmt.Errorf("record %s is not loaded", schema.Type.Name)
	}
	fields := map[string]Value{}
	for _, field := range schema.Fields {
		rawValues := values[field.Name]
		if field.Schema.Kind == "array" {
			if len(rawValues) == 0 && field.Schema.Type.Nullable {
				fields[field.Name] = Value{Type: field.Schema.Type}
				continue
			}
			items := make([]Value, 0, len(rawValues))
			for _, raw := range rawValues {
				item, valid, err := e.webParameterValue(field.Schema.Element, raw)
				if err != nil {
					return Value{}, err
				}
				if !valid {
					return e.webInvalidParameter(resultType, source, field.Name, raw, webParameterExpected(field.Schema.Element))
				}
				items = append(items, item)
			}
			fields[field.Name] = Value{Type: field.Schema.Type, Data: &arrayValue{Items: items}}
			continue
		}
		if len(rawValues) == 0 {
			if field.Schema.Type.Nullable {
				fields[field.Name] = Value{Type: field.Schema.Type}
				continue
			}
			return e.webNamedParameterError(resultType, "Missing", source, field.Name)
		}
		if len(rawValues) > 1 {
			return e.webNamedParameterError(resultType, "Duplicate", source, field.Name)
		}
		nonnull := *field.Schema
		nonnull.Type.Nullable = false
		value, valid, err := e.webParameterValue(&nonnull, rawValues[0])
		if err != nil {
			return Value{}, err
		}
		if !valid {
			return e.webInvalidParameter(resultType, source, field.Name, rawValues[0], webParameterExpected(&nonnull))
		}
		value.Type = field.Schema.Type
		fields[field.Name] = value
	}
	record := Value{Type: schema.Type, Data: &recordInstance{Definition: recordDefinition, Fields: fields}}
	return e.resultOK(resultType, record)
}

func (e *Evaluator) webParameterValue(schema *ir.CodecSchema, raw string) (Value, bool, error) {
	value := Value{Type: schema.Type}
	switch schema.Kind {
	case "string":
		value.Data = raw
		return value, true, nil
	case "boolean":
		if raw == "true" {
			value.Data = true
			return value, true, nil
		}
		if raw == "false" {
			value.Data = false
			return value, true, nil
		}
		return value, false, nil
	case "integer":
		parsed, message := parsePortableInteger(raw)
		value.Data = parsed
		return value, message == "", nil
	case "float":
		parsed, message := parsePortableFloat(raw)
		value.Data = parsed
		return value, message == "", nil
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		parsed, err := e.decodeTimeJSONCodec(schema.Kind, raw)
		if err != nil {
			return value, false, nil
		}
		parsed.Type = schema.Type
		return parsed, true, nil
	case "raw_enum":
		definition, ok := e.definitions[symbolKey(schema.Module, schema.Type.Name)].(*enumDefinition)
		if !ok {
			return value, false, fmt.Errorf("enum %s is not loaded", schema.Type.Name)
		}
		var rawValue Value
		if schema.RawType.Kind == types.Int {
			parsed, message := parsePortableInteger(raw)
			if message != "" {
				return value, false, nil
			}
			rawValue = Value{Type: types.FromName("Integer"), Data: parsed}
		} else {
			rawValue = Value{Type: types.FromName("String"), Data: raw}
		}
		for _, candidate := range schema.RawValues {
			member := definition.Members[candidate.Member]
			if member == nil || member.RawValue == nil {
				continue
			}
			declared, err := e.expression(member.RawValue, definition.Module, e.global)
			if err != nil {
				return value, false, err
			}
			if equal(rawValue, declared) {
				value.Data = &enumValue{Definition: definition, Name: candidate.Member, Payload: map[string]Value{}}
				return value, true, nil
			}
		}
		return value, false, nil
	}
	return value, false, nil
}

func (e *Evaluator) webNamedParameterError(resultType types.Type, variant, source, name string) (Value, error) {
	sourceValue, err := e.webParameterSource(source)
	if err != nil {
		return Value{}, err
	}
	return e.webParameterResultErr(resultType, variant, map[string]Value{
		"source": sourceValue,
		"name":   {Type: types.FromName("String"), Data: name},
	})
}

func (e *Evaluator) webInvalidParameter(resultType types.Type, source, name, value, expected string) (Value, error) {
	sourceValue, err := e.webParameterSource(source)
	if err != nil {
		return Value{}, err
	}
	return e.webParameterResultErr(resultType, "Invalid", map[string]Value{
		"source":   sourceValue,
		"name":     {Type: types.FromName("String"), Data: name},
		"value":    {Type: types.FromName("String"), Data: value},
		"expected": {Type: types.FromName("String"), Data: expected},
	})
}

func (e *Evaluator) webParameterSource(source string) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/web/index", "ParameterSource")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("web parameter binding requires trb/web")
	}
	name := strings.ToUpper(source[:1]) + source[1:]
	return Value{Type: types.FromName("ParameterSource"), Data: &enumValue{Definition: definition, Name: name, Payload: map[string]Value{}}}, nil
}

func (e *Evaluator) webParameterResultErr(resultType types.Type, variant string, fields map[string]Value) (Value, error) {
	resultDefinition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("web parameter binding requires trb/std/result")
	}
	errorDefinition, ok := e.definitions[symbolKey("trb/web/index", "ParameterError")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("web parameter binding requires trb/web")
	}
	errorValue := Value{Type: types.FromName("ParameterError"), Data: &enumValue{Definition: errorDefinition, Name: variant, Payload: fields}}
	return Value{Type: resultType, Data: &enumValue{Definition: resultDefinition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
}

func webParameterExpected(schema *ir.CodecSchema) string {
	if schema == nil {
		return "value"
	}
	typ := schema.Type
	typ.Nullable = false
	return typ.String()
}
