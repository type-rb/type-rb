const elements = {
	editor: document.querySelector("#editor"),
	highlight: document.querySelector("#highlight code"),
	highlightBox: document.querySelector("#highlight"),
	lineNumbers: document.querySelector("#line-numbers"),
	mode: document.querySelector("#mode"),
	version: document.querySelector("#version"),
	pageName: document.querySelector("#page-name"),
	run: document.querySelector("#run-button"),
	format: document.querySelector("#format-button"),
	transpile: document.querySelector("#transpile-button"),
	reset: document.querySelector("#reset-button"),
	status: document.querySelector("#run-status"),
	terminal: document.querySelector("#terminal-output"),
	empty: document.querySelector("#empty-output"),
	diagnostics: document.querySelector("#diagnostics"),
	target: document.querySelector("#target-output code"),
	executionTime: document.querySelector("#execution-time"),
	outputMode: document.querySelector("#output-mode"),
	cursor: document.querySelector("#cursor-position"),
	dirty: document.querySelector("#dirty-indicator"),
	toast: document.querySelector("#toast"),
	eyebrow: document.querySelector("#eyebrow"),
	title: document.querySelector("#page-title"),
	description: document.querySelector("#page-description"),
	introNote: document.querySelector("#intro-note"),
	lessonList: document.querySelector("#lesson-list"),
	tourProgress: document.querySelector("#tour-progress"),
	progressBar: document.querySelector("#progress-bar"),
	lessonHint: document.querySelector("#lesson-hint"),
	previous: document.querySelector("#previous-lesson"),
	next: document.querySelector("#next-lesson"),
};

const playSource = `name := "TypeRB"
numbers := [1, 2, 3, 4]

doubled := numbers.map do |number|
	number * 2
end

puts("Hello, " + name + "!")
puts(doubled)
`;

const state = {
	page: location.pathname.split("/").filter(Boolean).at(-1) === "tour" ? "tour" : "play",
	mode: "go",
	lessons: [],
	lessonIndex: -1,
	completed: new Set(readJSON("trb.tour.v2.completed", [])),
	baseline: "",
	busy: false,
};

const pageTitles = {
	play: "TypeRB Playground",
	tour: "A Tour of TypeRB",
};

let runtime = null;
let tourURL = new URL("../tour.json", document.baseURI);

const keywords = new Set([
	"and", "break", "case", "class", "def", "do", "else", "elsif", "end", "enum", "fn",
	"false", "if", "implements", "import", "module", "mut", "next", "nil", "not",
	"or", "readonly", "record", "return", "self", "then", "true", "when", "while",
]);
const builtInTypes = new Set([
	"Any", "Array", "Boolean", "Bytes", "Float", "Hash", "Integer", "Range", "Result",
	"String", "StringBuilder", "Unit",
]);

function escapeHTML(value) {
	return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

function highlightTypeRB(source) {
	const pattern = /#[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|@[A-Za-z_][A-Za-z0-9_]*|\b(?:\d+(?:\.\d+)?)\b|::|:=|\.\.\.|\.\.|=>|==|!=|<=|>=|&&|\|\||\*\*|[+\-*\/%<>=!?:|&]|\b[A-Za-z_][A-Za-z0-9_]*[!?]?\b/g;
	let result = "";
	let cursor = 0;
	for (const match of source.matchAll(pattern)) {
		const index = match.index ?? 0;
		result += escapeHTML(source.slice(cursor, index));
		const token = match[0];
		let kind = "";
		if (token.startsWith("#")) kind = "comment";
		else if (token.startsWith('"') || token.startsWith("'")) kind = "string";
		else if (/^\d/.test(token)) kind = "number";
		else if (token.startsWith("@")) kind = "ivar";
		else if (keywords.has(token)) kind = "keyword";
		else if (builtInTypes.has(token) || /^[A-Z][a-z]/.test(token)) kind = "type";
		else if (/^[A-Z][A-Z0-9_]*$/.test(token)) kind = "constant";
		else if (/^[A-Za-z_]/.test(token) && /^\s*\(/.test(source.slice(index + token.length))) kind = "function";
		else if (/^[A-Za-z_]/.test(token) && source.slice(Math.max(0, index - 2), index) === "::") kind = "symbol";
		else if (!/^[A-Za-z_]/.test(token)) kind = "operator";
		result += kind ? `<span class="tok-${kind}">${escapeHTML(token)}</span>` : escapeHTML(token);
		cursor = index + token.length;
	}
	result += escapeHTML(source.slice(cursor));
	return result;
}

function updateEditor() {
	const source = elements.editor.value;
	elements.highlight.innerHTML = highlightTypeRB(source) + (source.endsWith("\n") ? " " : "");
	const count = Math.max(1, source.split("\n").length);
	elements.lineNumbers.textContent = Array.from({ length: count }, (_, index) => String(index + 1)).join("\n");
	elements.dirty.classList.toggle("visible", source !== state.baseline);
	updateCursor();
}

function updateCursor() {
	const before = elements.editor.value.slice(0, elements.editor.selectionStart);
	const lines = before.split("\n");
	elements.cursor.textContent = `Ln ${lines.length}, Col ${lines.at(-1).length + 1}`;
}

function syncScroll() {
	elements.highlightBox.scrollTop = elements.editor.scrollTop;
	elements.highlightBox.scrollLeft = elements.editor.scrollLeft;
	elements.lineNumbers.scrollTop = elements.editor.scrollTop;
}

function setSource(source, baseline = source) {
	elements.editor.value = source;
	state.baseline = baseline;
	updateEditor();
	elements.editor.scrollTop = 0;
	elements.editor.scrollLeft = 0;
	syncScroll();
}

function setStatus(kind, label) {
	elements.status.className = `run-status ${kind}`;
	elements.status.querySelector("strong").textContent = label;
}

function setBusy(busy, label = "Working") {
	state.busy = busy;
	for (const button of [elements.run, elements.format, elements.transpile, elements.reset]) {
		button.disabled = busy;
	}
	if (busy) setStatus("running", label);
}

function selectTab(name) {
	for (const tab of document.querySelectorAll(".tab")) {
		const active = tab.dataset.tab === name;
		tab.classList.toggle("active", active);
		tab.setAttribute("aria-selected", String(active));
	}
	document.querySelector("#result-view").classList.toggle("hidden", name !== "result");
	document.querySelector("#target-view").classList.toggle("hidden", name !== "target");
}

function showResult(payload) {
	elements.empty.classList.add("hidden");
	elements.diagnostics.classList.add("hidden");
	elements.terminal.classList.remove("hidden");
	elements.terminal.textContent = payload.output || "(completed with no output)\n";
	if (payload.generated) elements.target.textContent = payload.generated;
	elements.executionTime.textContent = `${payload.durationMs ?? 0} ms`;
	setStatus("success", "Passed");
	selectTab("result");
}

function showDiagnostics(payload) {
	elements.empty.classList.add("hidden");
	elements.terminal.classList.add("hidden");
	elements.diagnostics.classList.remove("hidden");
	elements.diagnostics.replaceChildren();
	for (const item of payload.diagnostics || [{ severity: "error", message: "Unknown compiler error" }]) {
		const card = document.createElement("button");
		card.type = "button";
		card.className = "diagnostic";
		const location = document.createElement("span");
		location.className = "diagnostic-location";
		location.textContent = item.line ? `main.trb:${item.line}:${item.column || 1} · compile ${item.severity}` : `runtime ${item.severity}`;
		const message = document.createElement("span");
		message.className = "diagnostic-message";
		message.textContent = item.message;
		card.append(location, message);
		const frame = diagnosticSourceFrame(item);
		if (frame) {
			const source = document.createElement("pre");
			source.className = "diagnostic-source";
			source.textContent = frame;
			card.append(source);
		}
		if (item.line) {
			card.addEventListener("click", () => focusLocation(item.line, item.column || 1));
		}
		elements.diagnostics.append(card);
	}
	if (payload.generated) elements.target.textContent = payload.generated;
	elements.executionTime.textContent = `${payload.durationMs ?? 0} ms`;
	setStatus("error", "Error");
	selectTab("result");
}

function diagnosticSourceFrame(item) {
	if (!item.line) return "";
	const sourceLine = elements.editor.value.split("\n")[item.line - 1];
	if (sourceLine === undefined) return "";
	const column = Math.max(1, item.column || 1);
	const displayLine = sourceLine.replaceAll("\t", "    ");
	const displayPrefix = sourceLine.slice(0, column - 1).replaceAll("\t", "    ");
	let markerLength = 1;
	if (item.endLine === item.line && item.endColumn > column) {
		markerLength = Math.max(1, sourceLine.slice(column - 1, item.endColumn - 1).replaceAll("\t", "    ").length);
	}
	const lineNumber = String(item.line);
	return `${lineNumber} │ ${displayLine}\n${" ".repeat(lineNumber.length)} │ ${" ".repeat(displayPrefix.length)}${"^".repeat(markerLength)}`;
}

function focusLocation(line, column) {
	const lines = elements.editor.value.split("\n");
	let position = 0;
	for (let index = 0; index < Math.max(0, line - 1); index++) position += lines[index].length + 1;
	position += Math.max(0, column - 1);
	elements.editor.focus();
	elements.editor.setSelectionRange(position, position);
	updateCursor();
}

class HTTPRuntime {
	constructor(baseURL) {
		this.baseURL = baseURL;
	}

	async invoke(operation, source, mode) {
		const response = await fetch(new URL(operation, this.baseURL), {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ source, mode }),
		});
		const payload = await response.json();
		if (!response.ok) throw new Error(payload.error || `Request failed with ${response.status}`);
		return payload;
	}
}

class WasmRuntime {
	constructor(workerURL) {
		this.workerURL = workerURL;
		this.nextID = 1;
		this.pending = new Map();
		this.start();
	}

	start() {
		this.worker = new Worker(this.workerURL);
		this.ready = new Promise((resolve, reject) => {
			this.resolveReady = resolve;
			this.rejectReady = reject;
		});
		this.worker.addEventListener("message", (event) => this.handleMessage(event.data));
		this.worker.addEventListener("error", (event) => {
			this.fail(new Error(event.message || "The browser compiler worker failed"));
		});
	}

	handleMessage(message) {
		if (message.type === "ready") {
			this.resolveReady();
			return;
		}
		if (message.type === "fatal") {
			this.fail(new Error(message.message || "The browser compiler could not start"));
			return;
		}
		const request = this.pending.get(message.id);
		if (!request) return;
		this.pending.delete(message.id);
		clearTimeout(request.timer);
		if (message.type === "result") request.resolve(message.payload);
		else request.reject(new Error(message.message || "The browser compiler failed"));
	}

	fail(error) {
		this.rejectReady(error);
		for (const request of this.pending.values()) {
			clearTimeout(request.timer);
			request.reject(error);
		}
		this.pending.clear();
		this.worker?.terminate();
		this.worker = null;
	}

	restart(error) {
		this.fail(error);
		this.start();
	}

	async invoke(operation, source, mode) {
		if (!this.worker) this.start();
		await this.ready;
		const id = this.nextID++;
		return new Promise((resolve, reject) => {
			const limit = operation === "run" ? 4500 : 15000;
			const timer = setTimeout(() => {
				const error = new Error(`${operation} exceeded the browser execution limit`);
				this.restart(error);
			}, limit);
			this.pending.set(id, { resolve, reject, timer });
			this.worker.postMessage({ id, operation, source, mode });
		});
	}
}

async function requestRuntime(operation) {
	if (!runtime) throw new Error("The compiler runtime is not ready");
	return runtime.invoke(operation, elements.editor.value, state.mode);
}

async function run() {
	if (state.busy) return;
	setBusy(true, "Running");
	try {
		const payload = await requestRuntime("run");
		if (payload.ok) {
			showResult(payload);
			if (state.page === "tour") completeCurrentLesson();
		} else {
			showDiagnostics(payload);
		}
	} catch (error) {
		showDiagnostics({ diagnostics: [{ severity: "error", message: error.message }], durationMs: 0 });
	} finally {
		setBusy(false);
	}
}

async function transpile() {
	if (state.busy) return;
	setBusy(true, "Compiling");
	try {
		const payload = await requestRuntime("transpile");
		if (payload.ok) {
			elements.target.textContent = payload.generated || "// No generated source.";
			elements.executionTime.textContent = `${payload.durationMs ?? 0} ms`;
			setStatus("success", "Compiled");
			selectTab("target");
		} else {
			showDiagnostics(payload);
		}
	} catch (error) {
		showDiagnostics({ diagnostics: [{ severity: "error", message: error.message }], durationMs: 0 });
	} finally {
		setBusy(false);
	}
}

async function formatSource() {
	if (state.busy) return;
	setBusy(true, "Formatting");
	try {
		const payload = await requestRuntime("format");
		if (payload.ok) {
			elements.editor.value = payload.formatted;
			updateEditor();
			persistSource();
			setStatus("success", "Formatted");
			elements.executionTime.textContent = `${payload.durationMs ?? 0} ms`;
			showToast("Source formatted");
		} else {
			showDiagnostics(payload);
		}
	} catch (error) {
		showDiagnostics({ diagnostics: [{ severity: "error", message: error.message }], durationMs: 0 });
	} finally {
		setBusy(false);
	}
}

function resetSource() {
	setSource(state.baseline, state.baseline);
	persistSource();
	clearOutput();
	showToast("Source reset");
}

function clearOutput() {
	elements.empty.classList.remove("hidden");
	elements.terminal.classList.add("hidden");
	elements.diagnostics.classList.add("hidden");
	elements.target.textContent = "// Generated target source appears here.";
	elements.executionTime.textContent = "Not run yet";
	setStatus("ready", "Ready");
}

function modeLabel(mode) {
	return mode === "typescript" ? "TypeScript" : mode[0].toUpperCase() + mode.slice(1);
}

function setMode(mode) {
	state.mode = mode;
	elements.mode.value = mode;
	elements.outputMode.textContent = `${modeLabel(mode)} output`;
	localStorage.setItem("trb.play.mode", mode);
	if (!elements.target.textContent.startsWith("// Generated")) transpile();
}

function insertText(text, selectionOffset = text.length) {
	const start = elements.editor.selectionStart;
	const end = elements.editor.selectionEnd;
	elements.editor.setRangeText(text, start, end, "end");
	const next = start + selectionOffset;
	elements.editor.setSelectionRange(next, next);
	updateEditor();
}

function handleEditorKeydown(event) {
	if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
		event.preventDefault();
		run();
		return;
	}
	if (event.key === "Tab") {
		event.preventDefault();
		insertText("\t");
		return;
	}
	if (event.key === "Enter") {
		const start = elements.editor.selectionStart;
		const before = elements.editor.value.slice(0, start);
		const currentLine = before.slice(before.lastIndexOf("\n") + 1);
		const indent = currentLine.match(/^\s*/)?.[0] || "";
		const opensBlock = /\b(?:do|def|class|module|record|enum|case|if|elsif|else|while)\b[^#]*$/.test(currentLine.trim());
		event.preventDefault();
		insertText(`\n${indent}${opensBlock ? "\t" : ""}`);
	}
}

function showToast(message) {
	elements.toast.textContent = message;
	elements.toast.classList.add("visible");
	clearTimeout(showToast.timer);
	showToast.timer = setTimeout(() => elements.toast.classList.remove("visible"), 1600);
}

function readJSON(key, fallback) {
	try {
		return JSON.parse(localStorage.getItem(key)) ?? fallback;
	} catch {
		return fallback;
	}
}

function persistSource() {
	if (state.page === "play") {
		localStorage.setItem("trb.play.source", elements.editor.value);
		return;
	}
	const lesson = state.lessons[state.lessonIndex];
	if (lesson) localStorage.setItem(lessonStorageKey(lesson.id), elements.editor.value);
}

function lessonStorageKey(id) {
	return `trb.tour.v2.source.${id}`;
}

function renderLessonList() {
	elements.lessonList.replaceChildren();
	let chapter = "";
	state.lessons.forEach((lesson, index) => {
		if (lesson.chapter !== chapter) {
			chapter = lesson.chapter;
			const heading = document.createElement("li");
			heading.className = "lesson-chapter";
			heading.textContent = chapter;
			elements.lessonList.append(heading);
		}
		const item = document.createElement("li");
		const button = document.createElement("button");
		button.type = "button";
		button.className = "lesson-button";
		button.classList.toggle("complete", state.completed.has(lesson.id));
		button.dataset.index = String(index);
		button.innerHTML = `<span class="lesson-number">${state.completed.has(lesson.id) ? "✓" : String(index + 1).padStart(2, "0")}</span><span>${escapeHTML(lesson.title)}</span>`;
		button.addEventListener("click", () => selectLesson(index));
		item.append(button);
		elements.lessonList.append(item);
	});
	updateTourProgress();
}

function selectLesson(index) {
	if (!state.lessons.length) return;
	if (state.lessonIndex >= 0 && state.lessons[state.lessonIndex]) {
		localStorage.setItem(lessonStorageKey(state.lessons[state.lessonIndex].id), elements.editor.value);
	}
	state.lessonIndex = Math.max(0, Math.min(index, state.lessons.length - 1));
	const lesson = state.lessons[state.lessonIndex];
	const saved = localStorage.getItem(lessonStorageKey(lesson.id));
	setSource(saved ?? lesson.source, lesson.source);
	elements.eyebrow.textContent = lesson.eyebrow;
	elements.title.textContent = lesson.title;
	elements.description.textContent = lesson.description;
	elements.lessonHint.textContent = lesson.hint;
	elements.introNote.innerHTML = `<span class="pulse-dot"></span><span>Expected: ${escapeHTML(lesson.expected.trim().replaceAll("\n", " · "))}</span>`;
	elements.previous.disabled = state.lessonIndex === 0;
	elements.next.textContent = state.lessonIndex === state.lessons.length - 1 ? "Finish tour ✓" : "Next lesson →";
	for (const button of elements.lessonList.querySelectorAll(".lesson-button")) {
		button.classList.toggle("active", Number(button.dataset.index) === state.lessonIndex);
	}
	location.hash = lesson.id;
	clearOutput();
}

function completeCurrentLesson() {
	const lesson = state.lessons[state.lessonIndex];
	if (!lesson) return;
	state.completed.add(lesson.id);
	localStorage.setItem("trb.tour.v2.completed", JSON.stringify([...state.completed]));
	renderLessonList();
	for (const button of elements.lessonList.querySelectorAll(".lesson-button")) {
		button.classList.toggle("active", Number(button.dataset.index) === state.lessonIndex);
	}
	showToast("Lesson complete");
}

function updateTourProgress() {
	const completed = state.lessons.filter((lesson) => state.completed.has(lesson.id)).length;
	elements.tourProgress.textContent = `${completed} / ${state.lessons.length}`;
	elements.progressBar.style.width = `${state.lessons.length ? (completed / state.lessons.length) * 100 : 0}%`;
}

async function initializeTour() {
	document.body.classList.add("tour-page");
	const response = await fetch(tourURL);
	state.lessons = await response.json();
	renderLessonList();
	const requested = location.hash.slice(1);
	const index = state.lessons.findIndex((lesson) => lesson.id === requested);
	selectLesson(index >= 0 ? index : 0);
}

async function initialize() {
	const pageTitle = pageTitles[state.page];
	document.title = pageTitle;
	elements.pageName.textContent = pageTitle;
	for (const link of document.querySelectorAll("[data-page-link]")) {
		const active = link.dataset.pageLink === state.page;
		link.classList.toggle("active", active);
		if (active) link.setAttribute("aria-current", "page");
		else link.removeAttribute("aria-current");
	}
	const response = await fetch(new URL("../runtime.json", document.baseURI));
	const config = await response.json();
	if (config.transport === "wasm") {
		runtime = new WasmRuntime(new URL("../assets/playground-worker.js", document.baseURI));
	} else {
		runtime = new HTTPRuntime(new URL("../api/", document.baseURI));
		tourURL = new URL("../api/tour", document.baseURI);
	}
	elements.version.textContent = config.version || "local";
	const savedMode = localStorage.getItem("trb.play.mode");
	setMode(config.modes.includes(savedMode) ? savedMode : config.mode);
	if (state.page === "tour") {
		await initializeTour();
	} else {
		const saved = localStorage.getItem("trb.play.source");
		setSource(saved ?? playSource, playSource);
	}
}

elements.editor.addEventListener("input", () => {
	updateEditor();
	persistSource();
});
elements.editor.addEventListener("scroll", syncScroll);
elements.editor.addEventListener("click", updateCursor);
elements.editor.addEventListener("keyup", updateCursor);
elements.editor.addEventListener("keydown", handleEditorKeydown);
elements.run.addEventListener("click", run);
elements.transpile.addEventListener("click", transpile);
elements.format.addEventListener("click", formatSource);
elements.reset.addEventListener("click", resetSource);
elements.mode.addEventListener("change", () => setMode(elements.mode.value));
elements.previous.addEventListener("click", () => selectLesson(state.lessonIndex - 1));
elements.next.addEventListener("click", () => {
	if (state.lessonIndex < state.lessons.length - 1) selectLesson(state.lessonIndex + 1);
	else showToast("Tour complete — keep experimenting!");
});
for (const tab of document.querySelectorAll(".tab")) {
	tab.addEventListener("click", () => selectTab(tab.dataset.tab));
}

initialize().catch((error) => {
	showDiagnostics({ diagnostics: [{ severity: "error", message: error.message }], durationMs: 0 });
});
