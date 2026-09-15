package compiler

import (
	"strings"
	"testing"
)

func TestRecursiveEnumPayloadsExecuteAcrossBackends(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{"direct", `enum Chain
End
Link(value: Integer, tail: Chain)
end
def total(chain: Chain): Integer
case chain
when Chain::End
return 0
when Chain::Link(value, tail)
return value + total(tail)
end
end
def main()
chain := Chain::Link(1, Chain::Link(2, Chain::End))
puts(total(chain))
puts(total(chain))
end
`, "3\n3"},
		{"generic enum", `enum Chain<T>
End(value: T)
Link(value: T, tail: Chain<T>)
end
def total(chain: Chain<Integer>): Integer
case chain
when Chain::End(value)
return value
when Chain::Link(value, tail)
return value + total(tail)
end
end
def main()
puts(total(Chain<Integer>::Link(2, Chain<Integer>::End(3))))
end
`, "5"},
		{"record cycle", `enum Tree
Node(value: Node)
Leaf
end
record Node
value: String
tail: Tree
end
def label(tree: Tree): String
return case tree
when Tree::Node(node)
node.value + label(node.tail)
when Tree::Leaf
"!"
end
end
def main()
puts(label(Tree::Node(Node.new(value: "held", tail: Tree::Leaf))))
end
`, "held!"},
		{"generic wrapper and named payload", `record Box<T>
value: T
end
enum Chain
End
Link(*, value: Integer, tail: Box<Box<Chain>>)
end
def total(chain: Chain): Integer
return case chain
when Chain::End
0
when Chain::Link(tail: wrapped, value: number)
number + total(wrapped.value.value)
end
end
def main()
puts(total(Chain::Link(tail: Box<Box<Chain>>.new(value: Box<Chain>.new(value: Chain::End)), value: 7)))
end
`, "7"},
	}
	for _, test := range tests {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				runEffectSource(t, mode, "main.trb", []byte(test.source), test.want)
			})
		}
	}
}

func TestImportedRecursiveEnumAliasExecutes(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/model/tree.trb", ModulePath: "model/tree", Package: "model", Source: []byte(`enum Tree
Leaf
Pair(left: Tree, right: Tree)
end
`)},
		{Filename: "/project/main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { Tree as Node } from model/tree
def leaves(value: Node): Integer
case value
when Node::Leaf
return 1
when Node::Pair(left, right)
return leaves(left) + leaves(right)
end
end
def main()
puts(leaves(Node::Pair(Node::Leaf, Node::Pair(Node::Leaf, Node::Leaf))))
end
`)},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject(units, Options{Mode: mode, GoModule: "example.com/recursive-enums", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/recursive-enums")); got != "3" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
