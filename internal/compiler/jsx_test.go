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
		Source: []byte(`import { ReactNode } from trb/platform/typescript/react

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
		`return <article className={"card"} data-selected={props.selected}>`,
		`<h2>{props.title}</h2>`,
		`<Card title={"TypeRB"} selected />`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXWithTypedFunctionCallbacks(t *testing.T) {
	source := SourceUnit{
		Filename:   "button.trb",
		ModulePath: "app/button",
		Source: []byte(`import { ReactEvent, ReactNode, input_value, prevent_default } from trb/platform/typescript/react

record ButtonProps
	on_click: (ReactEvent) -> Void
end

def Button(props: ButtonProps): ReactNode
	return <button onClick={props.on_click}>Save</button>
end

def Page(): ReactNode
	handle_click := fn(event: ReactEvent)
		prevent_default(event)
		puts(input_value(event))
		return
	end
	return <Button on_click={handle_click} />
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		"on_click: (arg0: React.SyntheticEvent) => void;",
		"const handle_click: (arg0: React.SyntheticEvent) => void = (event: React.SyntheticEvent): void =>",
		"event.preventDefault();",
		"<Button on_click={handle_click} />",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated callback TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXWithTypedState(t *testing.T) {
	source := SourceUnit{
		Filename:   "counter.trb",
		ModulePath: "app/counter",
		Source: []byte(`import { ReactEvent, ReactNode, use_state } from trb/platform/typescript/react

def Counter(): ReactNode
	count := use_state(0)
	increment := fn(_event: ReactEvent)
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
		"function useTrbState<T>(initial: T): Readonly<{ value: T; set: (value: T) => void }>",
		"const count: Readonly<{ value: number; set: (value: number) => void }> = useTrbState<number>(0);",
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

func TestCompileTypeScriptJSXWithBrowserRouting(t *testing.T) {
	source := SourceUnit{
		Filename:   "router.trb",
		ModulePath: "app/router",
		Source: []byte(`import { ReactNode } from trb/platform/typescript/react
import { browser_router, link_to, route, route_param, use_navigate } from trb/platform/typescript/react/router

def Home(): ReactNode
	return <main>{link_to("/todos/42", <span>Open todo</span>)}</main>
end

def TodoPage(): ReactNode
	id := route_param("id")
	navigate := use_navigate()
	go_home := fn()
		navigate("/")
		return
	end
	return <main><p>Todo {id}</p><button onClick={go_home}>Home</button></main>
end

def App(): ReactNode
	return browser_router([
		route("/", <Home />),
		route("/todos/:id", <TodoPage />),
	])
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`import { BrowserRouter, Link, Route, Routes, useNavigate, useParams } from "react-router-dom";`,
		"function renderTrbBrowserRouter(routes:",
		`React.createElement(Link, { to: "/todos/42" }, <span>Open todo</span>)`,
		`(useParams<Record<string, string | undefined>>()["id"] ?? null)`,
		`{ path: "/todos/:id", element: <TodoPage /> }`,
		"renderTrbBrowserRouter([",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated routed TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXWithTypedFormState(t *testing.T) {
	source := SourceUnit{
		Filename:   "form.trb",
		ModulePath: "app/form",
		Source: []byte(`import { ReactEvent, ReactNode, input_value, prevent_default } from trb/platform/typescript/react
import { use_form } from trb/platform/typescript/react/form

record TodoDraft
	title: String
end

record TodoErrors
	title: String?
end

def TodoEditor(): ReactNode
	form := use_form(TodoDraft.new(title: ""), TodoErrors.new(title: nil))
	update_title := fn(event: ReactEvent)
		form.set_value(TodoDraft.new(title: input_value(event)))
		return
	end
	submit := fn(event: ReactEvent)
		prevent_default(event)
		if form.value.title.empty?()
			form.set_errors(TodoErrors.new(title: "Title is required"))
		else
			form.clear_errors()
		end
		return
	end
	return <form onSubmit={submit}>
		<input value={form.value.title} onChange={update_title} />
		<p>{form.errors.title}</p>
		<button type="submit">Save</button>
	</form>
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifacts[0].Output)
	for _, expected := range []string{
		`import { useState as useTrbFormState } from "react";`,
		"function useTrbForm<T, E>(initial: T, emptyErrors: E)",
		"const form: Readonly<{ value: TodoDraft; errors: TodoErrors; dirty: boolean; submitting: boolean;",
		`form.set_value(({title: (event.currentTarget as HTMLInputElement).value} satisfies TodoDraft))`,
		`form.set_errors(({title: "Title is required"} satisfies TodoErrors))`,
		"form.clear_errors()",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated form TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptJSXChecksFormStateTypes(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { use_form } from trb/platform/typescript/react/form

record Draft
	title: String
end

record Errors
	title: String?
end

def Editor(): ReactNode
	form := use_form(Draft.new(title: ""), Errors.new(title: nil))
	form.set_errors(Draft.new(title: "wrong"))
	return <p>{form.value.title}</p>
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "form.trb", ModulePath: "app/form", Source: source}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "argument 1 to set_errors() has type Draft, expected Errors") {
		t.Fatalf("expected typed form error diagnostic, got %v", err)
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

func TestCompileTypeScriptBrowserJSONQueryIsTypedAndSuspending(t *testing.T) {
	source := SourceUnit{
		Filename:   "query.trb",
		ModulePath: "app/query",
		Source: []byte(`import { FetchError, delete_json, get_json, patch_json, post_json, put_json } from trb/platform/typescript/browser

record Todo
	id: Integer
	title: String
end

def load_todos(): Array<Todo> fails FetchError
	return get_json<Array<Todo>>("/api/todos")
end

def save_todo(todo: Todo): Todo fails FetchError
	return put_json<Todo>("/api/todos/1", todo)
end

def create_todo(todo: Todo): Todo fails FetchError
	return post_json<Todo>("/api/todos", todo)
end

def patch_todo(todo: Todo): Todo fails FetchError
	return patch_json<Todo>("/api/todos/1", todo)
end

def delete_todo(): Todo fails FetchError
	return delete_json<Todo>("/api/todos/1")
end
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == source.Filename {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		"async function load_todos(): Promise<Result<Array<Todo>, FetchError>>",
		"await fetch(\"/api/todos\")",
		`method: "PUT"`,
		`method: "POST"`,
		`method: "PATCH"`,
		`method: "DELETE"`,
		`fetch("/api/todos/1", { method: "DELETE" })`,
		`"Content-Type": "application/json"`,
		"FetchError.InvalidJson",
		"Result.Ok<Array<Todo>, FetchError>",
		"async function save_todo(todo: Todo): Promise<Result<Todo, FetchError>>",
		"async function create_todo(todo: Todo): Promise<Result<Todo, FetchError>>",
		"async function patch_todo(todo: Todo): Promise<Result<Todo, FetchError>>",
		"async function delete_todo(): Promise<Result<Todo, FetchError>>",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated browser query is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptBrowserJSONChecksRequestValue(t *testing.T) {
	source := []byte(`import { FetchError, post_json } from trb/platform/typescript/browser

record Todo
	id: Integer
end

def create_todo(): Todo fails FetchError
	return post_json<Todo>("/api/todos", "not a todo")
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "query.trb", ModulePath: "app/query", Source: source}}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err == nil || !strings.Contains(err.Error(), "argument 2 to post_json() has type String, expected Todo") {
		t.Fatalf("expected typed browser request diagnostic, got %v", err)
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

func TestCompileRejectsJSXOutsideTypeScript(t *testing.T) {
	source := []byte("def view(): Any\n\treturn <div />\nend\n")
	_, err := Compile("view.trb", source, "go")
	if err == nil || !strings.Contains(err.Error(), "JSX is only available in mode typescript") {
		t.Fatalf("expected TypeScript-only JSX diagnostic, got %v", err)
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
