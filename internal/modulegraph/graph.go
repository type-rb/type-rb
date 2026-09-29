// Package modulegraph provides deterministic source-module graph traversal.
package modulegraph

// Component groups mutually reachable modules. Modules retain DFS discovery
// order; components are returned dependencies first. Singleton self imports
// are cyclic too.
type Component struct {
	Modules []string
	Cyclic  bool
}

func Components(roots []string, dependencies func(string) []string) []Component {
	indices, low := map[string]int{}, map[string]int{}
	active := map[string]bool{}
	var stack []string
	var result []Component
	next := 0
	var visit func(string)
	visit = func(module string) {
		next++
		indices[module], low[module] = next, next
		stack = append(stack, module)
		active[module] = true
		self := false
		for _, dependency := range dependencies(module) {
			self = self || dependency == module
			if indices[dependency] == 0 {
				visit(dependency)
				low[module] = min(low[module], low[dependency])
			} else if active[dependency] {
				low[module] = min(low[module], indices[dependency])
			}
		}
		if low[module] != indices[module] {
			return
		}
		component := Component{Cyclic: self}
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			active[last] = false
			component.Modules = append(component.Modules, last)
			if last == module {
				break
			}
		}
		// Tarjan pops in reverse discovery order.
		for left, right := 0, len(component.Modules)-1; left < right; left, right = left+1, right-1 {
			component.Modules[left], component.Modules[right] = component.Modules[right], component.Modules[left]
		}
		component.Cyclic = component.Cyclic || len(component.Modules) > 1
		result = append(result, component)
	}
	for _, root := range roots {
		if indices[root] == 0 {
			visit(root)
		}
	}
	return result
}
