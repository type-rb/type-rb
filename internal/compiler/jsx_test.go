package compiler

import (
	"strings"
	"testing"
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
