package checker

// A failed or interrupted interactive submission may have mutated retained
// values. At its successful-source boundary, discard earlier flow assumptions
// without pretending that the failed statements completed or replaying them.
func (c *Checker) invalidateInteractiveFlow(sc *scope, offset int) {
	if !c.interactiveTopLevel || sc.parent != nil {
		return
	}
	for c.interactiveFlowResetIndex < len(c.interactiveFlowResets) &&
		c.interactiveFlowResets[c.interactiveFlowResetIndex] <= offset {
		for name, value := range sc.values {
			if value.declared.Kind != "" {
				value.typ = value.declared
				sc.values[name] = value
			}
		}
		clear(sc.nullableMembers)
		c.interactiveFlowResetIndex++
	}
}
