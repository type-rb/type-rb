(() => {
	const root = document.querySelector("[data-capability-catalog]");
	if (!root) return;

	const state = { query: "", status: "all", scope: "all" };
	const areas = [...root.querySelectorAll("[data-capability-area]")];
	const statusButtons = [...root.querySelectorAll("[data-capability-status]")];
	const scopeButtons = [...root.querySelectorAll("[data-capability-scope]")];
	const visibleCount = root.querySelector("[data-capability-visible]");
	const empty = root.querySelector("[data-capability-empty]");

	function matches(item) {
		const statusMatches = state.status === "all" || item.dataset.status === state.status;
		const scopes = item.dataset.scopes.split(" ");
		const scopeMatches = state.scope === "all" || scopes.includes(state.scope);
		const queryMatches = !state.query || item.dataset.search.includes(state.query);
		return statusMatches && scopeMatches && queryMatches;
	}

	function render() {
		let shown = 0;
		let visibleAreas = 0;
		for (const area of areas) {
			const items = [...area.querySelectorAll("[data-capability-item]")];
			let areaShown = 0;
			let available = 0;
			for (const item of items) {
				const visible = matches(item);
				item.hidden = !visible;
				if (!visible) continue;
				areaShown += 1;
				shown += 1;
				if (item.dataset.status === "available") available += 1;
			}
			area.hidden = areaShown === 0;
			if (areaShown === 0) continue;
			visibleAreas += 1;
			area.querySelector("[data-capability-area-icon]").textContent = available === areaShown ? "✓" : String(available);
			area.querySelector("[data-capability-area-meta]").textContent = `${available} / ${areaShown} available`;
		}

		for (const button of statusButtons) {
			button.setAttribute("aria-pressed", String(button.dataset.capabilityStatus === state.status));
		}
		for (const button of scopeButtons) {
			button.setAttribute("aria-pressed", String(button.dataset.capabilityScope === state.scope));
		}
		visibleCount.textContent = `${shown} shown`;
		empty.hidden = visibleAreas !== 0;
	}

	for (const button of statusButtons) {
		button.addEventListener("click", () => {
			state.status = button.dataset.capabilityStatus;
			render();
		});
	}
	for (const button of scopeButtons) {
		button.addEventListener("click", () => {
			const selected = button.dataset.capabilityScope;
			state.scope = state.scope === selected ? "all" : selected;
			render();
		});
	}

	root.querySelector("[data-capability-search]").addEventListener("input", (event) => {
		state.query = event.target.value.trim().toLocaleLowerCase("en");
		render();
	});
	root.querySelector("[data-capability-expand]").addEventListener("click", () => {
		root.querySelectorAll("details:not([hidden])").forEach((details) => { details.open = true; });
	});
	root.querySelector("[data-capability-collapse]").addEventListener("click", () => {
		root.querySelectorAll("details").forEach((details) => { details.open = false; });
	});
	root.querySelectorAll(".capability-evidence").forEach((link) => {
		link.addEventListener("click", (event) => event.stopPropagation());
	});

	render();
})();
