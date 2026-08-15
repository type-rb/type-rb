"use strict";

function decodeTestEvent(line) {
	const value = line.trim();
	if (value === "") {
		return undefined;
	}
	let event;
	try {
		event = JSON.parse(value);
	} catch {
		return undefined;
	}
	if (event === null || typeof event !== "object" || typeof event.type !== "string") {
		return undefined;
	}
	if (!["test_started", "test_passed", "test_failed", "test_summary"].includes(event.type)) {
		return undefined;
	}
	return event;
}

module.exports = { decodeTestEvent };
