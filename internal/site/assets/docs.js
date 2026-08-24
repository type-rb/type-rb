(() => {
	const resetDelay = 1800;

	async function copyText(value) {
		if (window.isSecureContext && navigator.clipboard?.writeText) {
			await navigator.clipboard.writeText(value);
			return;
		}

		const textarea = document.createElement("textarea");
		textarea.value = value;
		textarea.setAttribute("readonly", "");
		textarea.style.position = "fixed";
		textarea.style.opacity = "0";
		document.body.append(textarea);
		let copied;
		try {
			textarea.select();
			copied = document.execCommand("copy");
		} finally {
			textarea.remove();
		}
		if (!copied) throw new Error("copy command was rejected");
	}

	function setButtonState(button, state) {
		const labels = {
			copy: "Copy",
			copied: "Copied",
			failed: "Failed"
		};
		button.dataset.state = state;
		button.textContent = labels[state];
		button.setAttribute("aria-label", state === "copy" ? "Copy code" : labels[state]);
	}

	for (const pre of document.querySelectorAll(".markdown-body pre")) {
		const code = pre.firstElementChild;
		if (!code || code.tagName !== "CODE") continue;

		const wrapper = document.createElement("div");
		wrapper.className = "code-block";
		pre.before(wrapper);
		wrapper.append(pre);

		const button = document.createElement("button");
		button.className = "code-copy-button";
		button.type = "button";
		button.setAttribute("aria-live", "polite");
		setButtonState(button, "copy");
		wrapper.append(button);

		let resetTimer;
		button.addEventListener("click", async () => {
			window.clearTimeout(resetTimer);
			button.disabled = true;
			try {
				await copyText(code.textContent);
				setButtonState(button, "copied");
			} catch {
				setButtonState(button, "failed");
			} finally {
				button.disabled = false;
				resetTimer = window.setTimeout(() => setButtonState(button, "copy"), resetDelay);
			}
		});
	}
})();
