//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/type-rb/type-rb/internal/playground"
)

var invokeFunction js.Func

func main() {
	invokeFunction = js.FuncOf(invoke)
	js.Global().Set("trbPlaygroundInvoke", invokeFunction)
	js.Global().Call("trbPlaygroundReady")
	select {}
}

func invoke(_ js.Value, arguments []js.Value) any {
	if len(arguments) != 3 {
		return marshal(playground.Response{OK: false, Diagnostics: []playground.Diagnostic{{
			Severity: "error", Message: "browser runtime requires operation, source, and mode",
		}}})
	}
	operation := arguments[0].String()
	source := arguments[1].String()
	mode := arguments[2].String()
	if !playground.ValidMode(mode) {
		return marshal(playground.Response{OK: false, Diagnostics: []playground.Diagnostic{{
			Severity: "error", Message: fmt.Sprintf("unsupported mode %q", mode),
		}}})
	}

	var result playground.Response
	switch operation {
	case "run":
		result = playground.Run(context.Background(), source, mode)
	case "transpile":
		result = playground.Transpile(source, mode)
	case "format":
		result = playground.Format(source)
	default:
		result = playground.Response{OK: false, Diagnostics: []playground.Diagnostic{{
			Severity: "error", Message: fmt.Sprintf("unsupported browser operation %q", operation),
		}}}
	}
	return marshal(result)
}

func marshal(value playground.Response) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `{"ok":false,"diagnostics":[{"severity":"error","message":"browser runtime response could not be encoded"}],"durationMs":0}`
	}
	return string(data)
}
