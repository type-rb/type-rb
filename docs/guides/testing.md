# Testing TypeRB applications

TypeRB provides a portable test API in `trb/std/test`. The same test source
runs through the Go, Ruby, and TypeScript backends.

Colocate unit tests with the source they exercise below the configured
`sourceDir`. For `calculator.trb`, use `calculator_test.trb` in the same directory:

```text
calculator.trb
calculator_test.trb
```

The matching filename is the default: `<filename>.trb` and
`<filename>_test.trb`. A large unit suite may use additional concern-specific
files beside the same source, such as `calculator_overflow_test.trb`.

Choose placement by the behavior the test guarantees. A test of one module's
contract stays beside that module even if constructing its input uses several
other modules or an external process. Tests of a complete feature across several
components may live in a feature directory such as
`src/tests/orders/checkout_test.trb`. System tests belong with the subsystem
whose end-to-end contract they verify. Every TypeRB test file still uses the
`_test.trb` suffix, and `trb test` discovers it recursively below `sourceDir`.

Extract reused fixtures and execution helpers into ordinary helper modules
rather than importing one test file from another. A project may group shared
helpers in `testing/`; keep test-case registration in test files and application
code independent of test helpers. The names `tests/` and `testing/` are project
conventions, not reserved directories: a helper without the `_test.trb` suffix
is an ordinary module under the language rules.

Keep unit tests beside their source when reorganizing a large directory. Move
both together by responsibility rather than relocating unit tests only to reduce
the directory's file count. Add tests for observable contracts and meaningful
boundaries; one test file per source is a placement convention, not a requirement
to create a test for every implementation file.

For example, `calculator.trb` can define the function under test:

```trb
def add(left: Integer, right: Integer): Integer
	return left + right
end
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
trb test -t "Calculator / negative values$"
```

Pass files or directories positionally to select their tests. Directories are
searched recursively, and multiple paths form a union:

```sh
trb test src/calculator_test.trb
trb test src/domain src/application/orders_test.trb
trb test src/domain/*_test.trb
```

The last example relies on shell expansion; `trb test` does not interpret a
quoted glob. Only selected test files are compiled during a focused run, while
the complete set of production files remains available.
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
- `expect_ok(actual_result)`
- `expect_err(actual_result)`

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

Recoverable operations return ordinary Result values. Use `try` only when a
helper function itself returns a compatible Result, and use `catch` when a
test can recover with a value. Result values already support structural
equality, so an exact result can be asserted directly. Use `expect_ok()` or
`expect_err()` when the payload is the subject of later assertions:

```trb
expect(load_user("missing")).to_equal(UserResult::Err(UserError::NotFound("missing")))

user := expect_ok(load_user("ada"))
expect(user.name).to_equal("Ada")

expect(expect_err(load_user("invalid"))).to_equal(UserError::Invalid("invalid"))
```

`expect_ok()` returns the `Ok` value and fails the current test if it receives
`Err`. `expect_err()` returns the `Err` value and fails if it receives `Ok`.
Both failures point to the helper call. An exhaustive `case` remains available
when the test needs branch-specific control flow; these helpers do not add an
implicit Result boundary.

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
