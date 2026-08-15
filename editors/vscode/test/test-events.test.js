"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const { decodeTestEvent } = require("../test-events");

test("decodes compiler-owned test events", () => {
	assert.deepEqual(
		decodeTestEvent('{"type":"test_failed","name":"Math / adds","test_file":"/src/math_test.trb","file":"/src/math.trb","line":4,"column":2,"message":"expected 2"}'),
		{ type: "test_failed", name: "Math / adds", test_file: "/src/math_test.trb", file: "/src/math.trb", line: 4, column: 2, message: "expected 2" }
	);
	assert.deepEqual(decodeTestEvent('{"type":"test_summary","total":3,"failed":1}'), {
		type: "test_summary",
		total: 3,
		failed: 1,
	});
	assert.equal(decodeTestEvent("not JSON"), undefined);
	assert.equal(decodeTestEvent('{"type":"unknown"}'), undefined);
});
