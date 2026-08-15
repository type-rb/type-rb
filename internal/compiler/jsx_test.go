package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

func TestCompileTypeScriptJSXToStructuredTSX(t *testing.T) {
	source := SourceUnit{
		Filename:   "card.trb",
		ModulePath: "app/card",
		Source: []byte(`import { ReactNode, mount } from trb/platform/typescript/react

record CardProps
	title: String
	selected: Boolean
end

def Card(props: CardProps): ReactNode
	return <article className="card" data-selected={props.selected}>
		<h2>{props.title}</h2>
	</article>
end

def Page(): ReactNode
	return <>
		<Card title="TypeRB" selected />
	</>
end

def main()
	mount(<Page />, "root")
	return
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	var artifact *Artifact
	for _, candidate := range artifacts {
		if candidate.Filename == source.Filename {
			artifact = candidate
			break
		}
	}
	if artifact == nil {
		t.Fatal("application artifact was not generated")
	}
	if !artifact.IR.UsesJSX {
		t.Fatal("typed IR did not retain JSX usage")
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		`import * as React from "react";`,
		`import { createRoot } from "react-dom/client";`,
		`return <article className={"card"} data-selected={props.selected}>`,
		`<h2>{props.title}</h2>`,
		`<Card title={"TypeRB"} selected />`,
		`createRoot(document.getElementById("root")!).render(<Page />)`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXWithTypedState(t *testing.T) {
	source := SourceUnit{
		Filename:   "counter.trb",
		ModulePath: "app/counter",
		Source: []byte(`import { MouseEvent, ReactNode, use_state } from trb/platform/typescript/react

def Counter(): ReactNode
	count := use_state(0)
	increment := fn(_event: MouseEvent)
		count.set(count.value + 1)
		return
	end
	return <button onClick={increment}>Count: {count.value}</button>
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"type __TrbReactState<T> = Readonly<{ value: T; set: (value: T) => void }>;",
		"function useTrbState<T>(initial: T): __TrbReactState<T>",
		"const count: __TrbReactState<number> = useTrbState(0);",
		"count.set(count.value + 1);",
		"<button onClick={increment}>Count: {count.value}</button>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated stateful TSX is missing %q:\n%s", expected, output)
		}
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
	}
	context := languageservice.BuildContext(programs, source.ModulePath)
	completionSource := `import { ReactNode, use_state } from trb/platform/typescript/react
def Counter(): ReactNode
	count := use_state(0)
	count.`
	items := languageservice.Complete(languageservice.CompletionRequest{
		Source: completionSource, Cursor: len(completionSource), Mode: "typescript", Context: context,
	})
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Label] = true
	}
	if !seen["value"] || !seen["set"] {
		t.Fatalf("React state completion is missing value/set: %#v", items)
	}
}

func TestCompileTypeScriptReactComponentKeepsExecutionScopeInternal(t *testing.T) {
	source := SourceUnit{
		Filename:   "page.trb",
		ModulePath: "app/page",
		Source: []byte(`import { ReactNode } from trb/platform/typescript/react
import { HttpClient, RequestError, Response } from trb/platform/typescript/browser

API := HttpClient.new("https://api.example.test")

type Loader = () -> Response<String> fails RequestError

def make_loader(): Loader
	return fn(): Response<String> fails RequestError
		return API.request("/message").text()
	end
end

def Page(): ReactNode
	_loader := make_loader()
	return <p>Ready</p>
end

def Wrapper(): ReactNode
	return Page()
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"export function Page(): React.ReactNode {",
		"const __trbScope: AbortSignal | undefined = undefined;",
		"const _loader: () => Result<__trb_browser.Response<string>, __trb_browser.RequestError> | Promise<Result<__trb_browser.Response<string>, __trb_browser.RequestError>> = make_loader(__trbScope);",
		"return Page();",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated component is missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "function Page(__trbScope") || strings.Contains(output, "Page(__trbScope)") {
		t.Fatalf("generated component exposes its internal execution scope:\n%s", output)
	}
}

func TestCompileTypeScriptJSXChecksStateUpdates(t *testing.T) {
	source := []byte(`import { ReactNode, use_state } from trb/platform/typescript/react

def Counter(): ReactNode
	count := use_state(0)
	count.set("wrong")
	return <p>{count.value}</p>
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "counter.trb", ModulePath: "app/counter", Source: source}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "argument 1 to set() has type String, expected Integer") {
		t.Fatalf("expected typed state update diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptJSXChecksComponentProps(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react

record CardProps
	title: String
end

def Card(props: CardProps): ReactNode
	return <h2>{props.title}</h2>
end

def Page(): ReactNode
	return <Card missing="TypeRB" />
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "card.trb", ModulePath: "app/card", Source: source}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "has no prop missing") {
		t.Fatalf("expected unknown prop diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptJSXChecksImportedPropsAndImportsTSX(t *testing.T) {
	card := SourceUnit{
		Filename:   "card.trb",
		ModulePath: "app/ui/card",
		Source: []byte(`import { ReactNode } from trb/platform/typescript/react

record CardProps
	title: String
end

def Card(props: CardProps): ReactNode
	return <article>{props.title}</article>
end
`),
	}
	page := SourceUnit{
		Filename:   "page.trb",
		ModulePath: "app/page",
		Source: []byte(`import { Card } from app/ui/card
import { ReactNode } from trb/platform/typescript/react

def Page(): ReactNode
	return <Card title="TypeRB" />
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{card, page}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact.Filename == page.Filename && !strings.Contains(string(artifact.Output), `from "./ui/card.tsx"`) {
			t.Fatalf("generated page did not import TSX module:\n%s", artifact.Output)
		}
	}
	page.Source = []byte(`import { Card } from app/ui/card
import { ReactNode } from trb/platform/typescript/react

def Page(): ReactNode
	return <Card />
end
`)
	_, err = CompileProject([]SourceUnit{card, page}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "requires prop title") {
		t.Fatalf("expected imported component prop diagnostic, got %v", err)
	}
}

func TestCompileJSXRequiresAnExplicitPlatformProvider(t *testing.T) {
	source := []byte("def view(): Any\n\treturn <div />\nend\n")
	for _, mode := range []string{"go", "ruby", "typescript"} {
		_, err := Compile("view.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "JSX requires an imported JSX provider") {
			t.Fatalf("%s: expected JSX provider diagnostic, got %v", mode, err)
		}
	}
}

func TestReactJSXProviderIsTypeScriptOnly(t *testing.T) {
	source := []byte("import { ReactNode } from trb/platform/typescript/react\ndef view(): ReactNode\n\treturn <div />\nend\n")
	for _, mode := range []string{"go", "ruby"} {
		_, err := Compile("view.trb", source, mode)
		if err == nil || !strings.Contains(err.Error(), "does not support mode "+mode) {
			t.Fatalf("%s: expected React platform boundary diagnostic, got %v", mode, err)
		}
	}
}

func TestCompileTypeScriptJSXRejectsNonRenderableChildren(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react

record Payload
	value: String
end

def Page(): ReactNode
	return <div>{Payload.new(value: "not renderable")}</div>
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "page.trb", ModulePath: "app/page", Source: source}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "JSX child must be renderable, got Payload") {
		t.Fatalf("expected non-renderable child diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptJSXRendersImportedTransparentAliases(t *testing.T) {
	contracts := SourceUnit{
		Filename:   "contracts.trb",
		ModulePath: "contracts",
		Source: []byte(`type EmailAddress = String

record Contact
	email_address: EmailAddress
end
`),
	}
	page := SourceUnit{
		Filename:   "page.trb",
		ModulePath: "app/page",
		Source: []byte(`import { Contact } from contracts
import { ReactNode } from trb/platform/typescript/react

def ContactEmail(contact: Contact): ReactNode
	return <span>{contact.email_address}</span>
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{contracts, page}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact.Filename == page.Filename && !strings.Contains(string(artifact.Output), "{contact.email_address}") {
			t.Fatalf("generated TSX did not render the imported transparent alias:\n%s", artifact.Output)
		}
	}
}

func TestCompileTypeScriptJSXChecksPurposeSpecificReactEvents(t *testing.T) {
	source := []byte(`import {
	ChangeEvent,
	FormEvent,
	KeyboardEvent,
	MouseEvent,
	ReactNode,
} from trb/platform/typescript/react

def Form(): ReactNode
	on_change := fn(event: ChangeEvent)
		puts(event.currentTarget.value)
		return
	end
	on_submit := fn(event: FormEvent)
		event.preventDefault()
		puts(event.currentTarget.id)
		return
	end
	on_click := fn(event: MouseEvent)
		puts(event.button)
		return
	end
	on_key_down := fn(event: KeyboardEvent)
		puts(event.key)
		return
	end
	return <form id="todo-form" onSubmit={on_submit}>
		<input name="title" onChange={on_change} onKeyDown={on_key_down} />
		<button type="submit" onClick={on_click}>Save</button>
	</form>
end
`)
	artifact, err := Compile("events.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		"React.ChangeEvent<HTMLInputElement>",
		"React.FormEvent<HTMLFormElement>",
		"React.MouseEvent<HTMLElement>",
		"React.KeyboardEvent<HTMLElement>",
		"event.currentTarget.value",
		"event.preventDefault()",
		"onSubmit={on_submit}",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated event TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXRejectsWrongReactEventHandler(t *testing.T) {
	source := []byte(`import { ChangeEvent, ReactNode } from trb/platform/typescript/react

def Page(): ReactNode
	handle := fn(event: ChangeEvent)
		puts(event.currentTarget.value)
		return
	end
	return <button onClick={handle}>Save</button>
end
`)
	_, err := Compile("wrong_event.trb", source, "typescript")
	if err == nil || !strings.Contains(err.Error(), "JSX attribute onClick expects (MouseEvent) -> Void, got (ChangeEvent) -> Void") {
		t.Fatalf("expected purpose-specific event diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptDoesNotRewriteLocalReactEventNames(t *testing.T) {
	source := []byte(`class MouseEvent
end

def passthrough(event: MouseEvent): MouseEvent
	return event
end
`)
	artifact, err := Compile("local_event.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if strings.Contains(output, "React.MouseEvent") {
		t.Fatalf("local MouseEvent was rewritten as a React type:\n%s", output)
	}
	if !strings.Contains(output, "event: MouseEvent") {
		t.Fatalf("generated TypeScript is missing the local event type:\n%s", output)
	}
}
