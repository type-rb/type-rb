#!/bin/sh

set -eu

usage() {
	cat >&2 <<'EOF'
usage: benchmarksgame-verify.sh --native-compiler PATH --reference-trb PATH
       --qbe PATH --cc PATH --target PROFILE --workspace PATH
EOF
	exit 64
}

fail() {
	printf 'benchmarksgame-verify: %s\n' "$1" >&2
	exit 1
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

native_compiler=
reference_trb=
qbe=
cc=
target=
workspace=

while test "$#" -gt 0; do
	case "$1" in
	--native-compiler)
		test -z "$native_compiler" && test "$#" -ge 2 || usage
		native_compiler=$2
		shift 2
		;;
	--reference-trb)
		test -z "$reference_trb" && test "$#" -ge 2 || usage
		reference_trb=$2
		shift 2
		;;
	--qbe)
		test -z "$qbe" && test "$#" -ge 2 || usage
		qbe=$2
		shift 2
		;;
	--cc)
		test -z "$cc" && test "$#" -ge 2 || usage
		cc=$2
		shift 2
		;;
	--target)
		test -z "$target" && test "$#" -ge 2 || usage
		target=$2
		shift 2
		;;
	--workspace)
		test -z "$workspace" && test "$#" -ge 2 || usage
		workspace=$2
		shift 2
		;;
	*) usage ;;
	esac
done

test -n "$native_compiler" && test -n "$reference_trb" || usage
test -n "$qbe" && test -n "$cc" && test -n "$target" && test -n "$workspace" || usage
test -x "$native_compiler" || fail "Native compiler is not executable"
test -x "$reference_trb" || fail "reference compiler is not executable"
test -x "$qbe" || fail "QBE is not executable"
test -x "$cc" || fail "C compiler is not executable"
test ! -e "$workspace" || fail "workspace already exists"

case "$target" in
darwin-arm64-v0 | linux-arm64-v0) ;;
*) usage ;;
esac

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
benchmark_root=$repository_root/benchmarks/benchmarksgame

mkdir -p "$workspace"
summary=$workspace/summary.tsv
printf 'case\tinput\tsource_sha256\texpected_sha256\treference_size\tnative_size\n' > "$summary"

verify_case() {
	case_name=$1
	input=$2
	expected_name=$3
	case_root=$benchmark_root/$case_name
	source=$case_root/src/main.trb
	config=$case_root/trbconfig.jsonc
	expected=$case_root/expected/$expected_name.txt
	case_workspace=$workspace/$case_name
	reference_program=$case_workspace/reference
	native_program=$case_workspace/native

	test -f "$source" || fail "missing source for $case_name"
	test -f "$config" || fail "missing configuration for $case_name"
	test -f "$expected" || fail "missing expected output for $case_name input $input"
	mkdir -p "$case_workspace"

	(
		cd "$case_root"
		"$reference_trb" check --config trbconfig.jsonc > "$case_workspace/reference-check.stdout" 2> "$case_workspace/reference-check.stderr"
		"$reference_trb" build --compile --config trbconfig.jsonc --outfile "$reference_program" > "$case_workspace/reference-build.stdout" 2> "$case_workspace/reference-build.stderr"
	)
	"$native_compiler" check "$source" > "$case_workspace/native-check.stdout" 2> "$case_workspace/native-check.stderr"
	"$native_compiler" build "$source" \
		--output "$native_program" \
		--qbe "$qbe" \
		--cc "$cc" \
		--target "$target" \
		> "$case_workspace/native-build.stdout" \
		2> "$case_workspace/native-build.stderr"

	set +e
	"$reference_program" "$input" > "$case_workspace/reference.stdout" 2> "$case_workspace/reference.stderr"
	reference_status=$?
	"$native_program" "$input" > "$case_workspace/native.stdout" 2> "$case_workspace/native.stderr"
	native_status=$?
	set -e

	test "$reference_status" -eq 0 || fail "$case_name reference run failed with status $reference_status"
	test "$native_status" -eq 0 || fail "$case_name Native run failed with status $native_status"
	test ! -s "$case_workspace/reference.stderr" || fail "$case_name reference run wrote stderr"
	test ! -s "$case_workspace/native.stderr" || fail "$case_name Native run wrote stderr"
	cmp "$expected" "$case_workspace/reference.stdout" >/dev/null || fail "$case_name reference output differs"
	cmp "$expected" "$case_workspace/native.stdout" >/dev/null || fail "$case_name Native output differs"
	cmp "$case_workspace/reference.stdout" "$case_workspace/native.stdout" >/dev/null || fail "$case_name backend outputs differ"

	printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$case_name" \
		"$input" \
		"$(sha256 "$source")" \
		"$(sha256 "$expected")" \
		"$(file_size "$reference_program")" \
		"$(file_size "$native_program")" \
		>> "$summary"
}

verify_case fannkuch-redux 7 7
verify_case n-body 1000 1000
verify_case spectral-norm 100 100

if find "$benchmark_root" "$workspace" -name '*.trbn.*' -print | grep . >/dev/null 2>&1; then
	fail "Native build left intermediate files"
fi

{
	printf 'type_rb_version=%s\n' "$("$reference_trb" --version)"
	printf 'type_rb_revision=%s\n' "$(tr -d '\n' < "$repository_root/TYPE_RB_REVISION")"
	printf 'native_compiler_sha256=%s\n' "$(sha256 "$native_compiler")"
	printf 'qbe_sha256=%s\n' "$(sha256 "$qbe")"
	printf 'cc=%s\n' "$($cc --version 2>&1 | awk 'NR == 1 { print; exit }')"
	printf 'target=%s\n' "$target"
} > "$workspace/toolchain.txt"

printf 'benchmarksgame-verify: all portable backend-pair cases passed\n'
