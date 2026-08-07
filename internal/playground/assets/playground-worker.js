"use strict";

const pending = [];
let ready = false;

self.trbPlaygroundReady = () => {
	ready = true;
	self.postMessage({ type: "ready" });
	while (pending.length) invoke(pending.shift());
};

self.onmessage = (event) => {
	if (!ready) {
		pending.push(event.data);
		return;
	}
	invoke(event.data);
};

function invoke(message) {
	try {
		const serialized = self.trbPlaygroundInvoke(message.operation, message.source, message.mode);
		self.postMessage({ type: "result", id: message.id, payload: JSON.parse(serialized) });
	} catch (error) {
		self.postMessage({ type: "error", id: message.id, message: error instanceof Error ? error.message : String(error) });
	}
}

async function start() {
	importScripts(new URL("wasm_exec.js", self.location.href).href);
	const go = new Go();
	const wasmURL = new URL("trb.wasm", self.location.href);
	let instance;
	if (WebAssembly.instantiateStreaming) {
		try {
			({ instance } = await WebAssembly.instantiateStreaming(fetch(wasmURL), go.importObject));
		} catch {
			const response = await fetch(wasmURL);
			({ instance } = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject));
		}
	} else {
		const response = await fetch(wasmURL);
		({ instance } = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject));
	}
	go.run(instance).catch((error) => {
		self.postMessage({ type: "fatal", message: error instanceof Error ? error.message : String(error) });
	});
}

start().catch((error) => {
	self.postMessage({ type: "fatal", message: error instanceof Error ? error.message : String(error) });
});
