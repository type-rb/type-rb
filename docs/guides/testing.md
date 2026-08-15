# Testing TypeRB applications

TypeRB provides a portable test API in `trb/std/test`. The same test source
runs through the Go, Ruby, and TypeScript backends.

Place tests beside the source they exercise and name them `*_test.trb`:

```text
src/
  calculator.trb
  calculator_test.trb
```

A test file uses named imports for the explicit suite, case, and assertion
helpers:

```trb
import { add } from calculator
import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(add(1, 2)).to_equal(3)
	end

	describe("negative values") do
		test("preserves the sign") do
			expect(add(0, -2)).to_equal(-2)
		end
	end
end
```

Run every project test with:

```sh
trb test
```

Select a suite or case by its full display name:

```sh
trb test --filter "Calculator / negative values"
```

Use `--file src/calculator_test.trb` to select declarations from one file.
Editors combine the file and full name so identically named suites in different
modules remain independent.

Failures return a nonzero process status and point to the original `.trb`
assertion. `--reporter json` emits one JSON object per line for editors and
other tools.

The initial assertion surface is:

- `expect(actual).to_equal(expected)`
- `expect(actual).to_not_equal(expected)`
- `expect(actual).to_be_true()`
- `expect(actual).to_be_false()`
- `expect(actual).to_be_nil()`

Equality assertions compare portable values structurally across backends,
including Arrays, Hashes, records, and value-carrying enums.

`describe()` contains nested `describe()` and `test()` declarations. A
`test()` body is ordinary TypeRB code, so shared setup is an explicit helper
function and data-driven tests use an Array with `each`:

```trb
describe("Parser") do
	test("accepts valid integers") do
		["0", "12", "-7"].each do |source|
			expect(source.to_i()).to_not_equal(999)
		end
	end
end
```

Fallible operations retain the language's ordinary `fails` and `attempt`
rules. Use `attempt` when the error value itself is the subject of the
assertion.

There is no implicit setup inheritance, dedicated table-test syntax, fixture
registry, or mock-generation magic. Tests use ordinary constructors,
interfaces, and explicit fake implementations. Higher-level testing packages
can build on this portable core without changing the runner protocol.

The VS Code extension discovers the same suite hierarchy through `trb lsp`,
shows it in Test Explorer, and supports running a project, suite, or individual
case. Go projects can also debug those selections with ordinary `.trb`
breakpoints, stepping, and variable inspection through Delve. Test CodeLens
actions use that same native Testing API and debugger integration.

TypeScript process tests currently require the Bun or Node runtime. Browser
test execution is staged separately because it needs a browser host rather
than the process runner used by `trb test`.
