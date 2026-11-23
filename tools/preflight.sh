#!/bin/sh
# Run the pull request quick checks that need only the pinned reference
# compiler, cheapest first, and stop at the first failure. The CI planner
# selects documentation-only checks when no code or CLI input changed.
#
# usage: tools/preflight.sh /path/to/pinned/trb [--full]
#
# Build the pinned reference once from a clean checkout at TYPE_RB_REVISION:
#   python3 tools/build-reference.py /path/to/type-rb "$TMPDIR/trb"
# TYPE_RB_CHECKOUT points to a reference Git checkout containing TYPE_RB_REVISION
# for the authoritative official-source comparison (defaults to .type-rb).
# --full adds the reference language fixtures and every root and compiler
# unit. Set TYPE_RB_CHECKOUT to that checkout to also verify the reference AST,
# and TYPE_RB_NATIVE_QBE to a QBE binary for QBE-backed compiler units.
set -eu

reference=${1:?usage: tools/preflight.sh /path/to/pinned/trb [--full]}
full=${2:-}
if [ "$#" -gt 2 ] || { [ -n "$full" ] && [ "$full" != "--full" ]; }; then
	printf 'usage: tools/preflight.sh /path/to/pinned/trb [--full]\n' >&2
	exit 2
fi
case "$reference" in /*) ;; *) reference="$PWD/$reference" ;; esac
test -x "$reference" || { printf 'preflight: %s is not an executable reference compiler\n' "$reference" >&2; exit 2; }
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

log=$(mktemp "${TMPDIR:-/tmp}/native-preflight.XXXXXX")
trap 'rm -f "$log" "$log.json"' 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# Each step's output is shown only when it fails; tests may print expected errors.
step() {
	name=$1
	shift
	started=$(date +%s)
	if ! "$@" > "$log" 2>&1; then
		cat "$log" >&2
		printf 'preflight: failed: %s\n' "$name" >&2
		exit 1
	fi
	printf 'ok  %-40s %ss\n' "$name" "$(( $(date +%s) - started ))"
}

base=$(git merge-base HEAD origin/main 2>/dev/null || git rev-parse HEAD)
changed=$( { git diff --name-only "$base"; git ls-files --others --exclude-standard; } | sort -u)
plan=$(printf '%s\n' "$changed" | node --input-type=module -e '
	import { classify } from "./tools/ci-plan.mjs";
	const paths = (await new Promise(resolve => { let data = ""; process.stdin.on("data", chunk => data += chunk); process.stdin.on("end", () => resolve(data)); }))
		.split("\n").filter(Boolean);
	const plan = classify(paths, false, "tiered");
	console.log(plan.code || plan.cli ? "code" : "documentation");
')
printf 'preflight: %s change set against %s\n' "$plan" "$(git rev-parse --short "$base")"

step "whitespace" git diff --check "$base"
step "generated coverage views" sh -c '
	python3 tools/native-language-coverage.py --check-feature-table docs/native-language-feature-inventory.md &&
	python3 tools/native-language-coverage.py --check-pages-data docs/capabilities/ordinary-language.js &&
	python3 tools/dependency-pages.py --check'
if [ "$plan" = documentation ]; then
	printf 'preflight: documentation checks passed\n'
	exit 0
fi

step "QBE dependency identity" python3 tools/qbe_toolchain_test.py
step "Float decimal constants" python3 tools/float-decimal-powers.py
step "authored QBE branch widths" python3 tools/native-runtime-branch-test.py --check-source
step "compiler source staging" python3 tools/compiler_sources_test.py
step "bootstrap snapshot input and catalog bounds" python3 tools/bootstrap_snapshot_size_test.py
step "compiler ownership and migration" sh -c 'python3 tools/compiler_layout_test.py && python3 tools/migrate_compiler_layout_test.py && python3 tools/compiler_layout.py'
step "Linux prerequisite controls" python3 tools/ci-linux-prerequisites-test.py
step "CI planner" node --test tools/ci-plan-test.mjs tools/ci-run-suites-test.mjs tools/recovery-workspace-test.mjs
step "recovery import boundaries" sh -c '
	python3 tools/recovery_layout_sync_test.py &&
	python3 tools/recovery_layout_sync.py --check'
step "reference identity" sh -c '
	python3 -m unittest tools/compatibility_manifest_test.py &&
	python3 tools/compatibility_manifest.py --reference-trb "$1"' preflight "$reference"
step "official source controls" python3 tools/official-sources-test.py
step "pinned official source bytes" python3 tools/official-sources.py \
	--reference-checkout "${TYPE_RB_CHECKOUT:-$root/.type-rb}" --check
step "Unicode data" sh -c 'python3 tools/unicode-identifier-data.py --check && python3 tools/unicode-case-data.py --check && python3 tools/json-diagnostic-data.py --check'
step "formatting" "$reference" fmt --check recovery compiler corpus tools benchmarks
step "recovery types" "$reference" check --config recovery/trbconfig.jsonc
step "compiler types" "$reference" check --config compiler/trbconfig.jsonc
step "CLI and core closure" sh tools/check-native-cli.sh "$reference"
step "reference formatter observations" python3 tools/native-format-test.py --reference-only --reference "$reference"
step "reference language server editing" python3 tools/native-lsp-test.py "$reference"
step "bootstrap snapshot v4 closure" sh tools/check-bootstrap-snapshot.sh "$reference"
step "browser client reference contracts" python3 tools/native-web-client-test.py "$reference" --reference --browser-check --browser-runtime "$(command -v node)"
step "HTTP server reference contracts" python3 tools/native-web-network-test.py "$reference" --reference
step "HTTP REPL reference contract" python3 tools/native-web-network-repl-test.py "$reference" --reference
step "gzip response reference contracts" python3 tools/native-web-compression-test.py "$reference" --reference --receipt "$log.json"
step "digest and HMAC reference contracts" python3 tools/native-crypto-test.py "$reference" --reference --receipt "$log.json"
step "OIDC reference contracts" python3 tools/native-oidc-test.py "$reference" --reference --tls --transport --cli-paths --receipt "$log.json"
step "application time service reference contracts" python3 tools/native-time-service-test.py --reference "$reference" --report "$log.json"
step "filesystem reference contracts" python3 tools/native-filesystem-test.py "$reference" --reference --report "$log.json"
step "filesystem reference rejection" python3 tools/native-filesystem-check-test.py "$reference" --reference --report "$log.json"
step "test manifest classification" python3 tools/native-test-runner-harness-test.py
step "release scenario evidence controls" python3 tools/release-scenarios-test.py
step "project test reference contracts" python3 tools/native-test-runner-test.py --reference "$reference" --report "$log.json"
step "test value and origin reference contracts" python3 tools/native-test-values-test.py --reference "$reference" --report "$log.json"
rm -f "$log.json"
if [ -f tools/project-scenarios.py ]; then
	# Scenarios run with an isolated HOME; share the Go build cache as CI does.
	if [ -z "${GOCACHE:-}" ] && command -v go > /dev/null 2>&1; then
		GOCACHE=$(go env GOCACHE)
		export GOCACHE
	fi
	step "project scenarios against the reference" sh -c '
		python3 tools/project-scenarios-test.py &&
		python3 tools/project-scenarios.py --reference "$1" --current-reference --jobs 4 > /dev/null' preflight "$reference"
fi

if [ "$full" = --full ]; then
	if [ -n "${TYPE_RB_CHECKOUT:-}" ]; then
		step "language fixtures against the reference" sh -c '
			python3 tools/native-language-coverage.py --reference "$1" --reference-ast "$2/internal/ast/ast.go" --jobs 4 > /dev/null' \
			preflight "$reference" "$TYPE_RB_CHECKOUT"
	else
		step "language fixtures against the reference" sh -c '
			python3 tools/native-language-coverage.py --reference "$1" --jobs 4 > /dev/null' preflight "$reference"
	fi
	step "recovery units" env TYPE_RB_NATIVE_ROOT="$root" "$reference" test --config recovery/trbconfig.jsonc
	step "compiler units" env TYPE_RB_NATIVE_ROOT="$root" "$reference" test --config compiler/trbconfig.jsonc
fi
printf 'preflight: passed\n'
