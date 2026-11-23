#!/bin/sh

set -eu

usage() {
	cat >&2 <<'EOF'
usage: prepare-runtime.sh smoke|formal --native-compiler PATH
       --reference-trb PATH --qbe PATH --cc PATH --target PROFILE
       --source-archive PATH --cxx PATH --go PATH --rustc PATH
       --javac PATH --java PATH --unzip PATH --workspace PATH
       --evidence PATH --catalog PATH [--case all|CASE]
EOF
	exit 64
}

fail() {
	printf 'benchmarksgame-prepare-runtime: %s\n' "$1" >&2
	exit 1
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

file_size() {
	wc -c < "$1" | tr -d ' '
}

test "$#" -gt 0 || usage
mode=$1
shift
case "$mode" in
smoke | formal) ;;
*) usage ;;
esac

native_compiler=
reference_trb=
qbe=
cc=
target=
source_archive=
cxx=
go_tool=
rustc=
javac=
java=
unzip_tool=
workspace=
evidence=
catalog=
selected_case=all
selected_case_set=false

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
	--source-archive)
		test -z "$source_archive" && test "$#" -ge 2 || usage
		source_archive=$2
		shift 2
		;;
	--cxx)
		test -z "$cxx" && test "$#" -ge 2 || usage
		cxx=$2
		shift 2
		;;
	--go)
		test -z "$go_tool" && test "$#" -ge 2 || usage
		go_tool=$2
		shift 2
		;;
	--rustc)
		test -z "$rustc" && test "$#" -ge 2 || usage
		rustc=$2
		shift 2
		;;
	--javac)
		test -z "$javac" && test "$#" -ge 2 || usage
		javac=$2
		shift 2
		;;
	--java)
		test -z "$java" && test "$#" -ge 2 || usage
		java=$2
		shift 2
		;;
	--unzip)
		test -z "$unzip_tool" && test "$#" -ge 2 || usage
		unzip_tool=$2
		shift 2
		;;
	--workspace)
		test -z "$workspace" && test "$#" -ge 2 || usage
		workspace=$2
		shift 2
		;;
	--evidence)
		test -z "$evidence" && test "$#" -ge 2 || usage
		evidence=$2
		shift 2
		;;
	--catalog)
		test -z "$catalog" && test "$#" -ge 2 || usage
		catalog=$2
		shift 2
		;;
	--case)
		test "$selected_case_set" = false && test "$#" -ge 2 || usage
		selected_case=$2
		selected_case_set=true
		shift 2
		;;
	*) usage ;;
	esac
done

test -n "$native_compiler" && test -n "$reference_trb" || usage
test -n "$qbe" && test -n "$cc" && test -n "$target" || usage
test -n "$source_archive" && test -n "$cxx" && test -n "$go_tool" || usage
test -n "$rustc" && test -n "$javac" && test -n "$java" && test -n "$unzip_tool" || usage
test -n "$workspace" && test -n "$evidence" && test -n "$catalog" || usage
for executable in "$native_compiler" "$reference_trb" "$qbe" "$cc" "$cxx" "$go_tool" "$rustc" "$javac" "$java" "$unzip_tool"; do
	test -x "$executable" || fail "required tool is not executable: $executable"
done
test -f "$source_archive" || fail "source archive does not exist"
test ! -e "$workspace" || fail "workspace already exists"
test ! -e "$evidence" || fail "evidence path already exists"
test ! -e "$catalog" || fail "catalog already exists"
case "$selected_case" in
all | fannkuch-redux | n-body | spectral-norm) ;;
*) usage ;;
esac
case "$target" in
darwin-arm64-v0 | linux-arm64-v0) ;;
*) usage ;;
esac
if test "$mode" = formal; then
	test "$(uname -s)" = Linux || fail "formal preparation requires Linux"
	case "$(uname -m)" in
	aarch64 | arm64) ;;
	*) fail "formal preparation requires Linux arm64" ;;
	esac
	test "$target" = linux-arm64-v0 || fail "formal preparation requires linux-arm64-v0"
fi
test "$(command -v go)" = "$go_tool" || fail "Go command does not match PATH"

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/../.." && pwd)
mkdir -p "$workspace" "$evidence" "$(dirname -- "$catalog")"
backend_workspace=$workspace/backend-pair
context_workspace=$workspace/context

/bin/sh "$repository_root/tools/benchmarksgame-verify.sh" \
	--native-compiler "$native_compiler" \
	--reference-trb "$reference_trb" \
	--qbe "$qbe" \
	--cc "$cc" \
	--target "$target" \
	--workspace "$backend_workspace" \
	> "$evidence/backend-pair.stdout" 2> "$evidence/backend-pair.stderr"

/bin/sh "$repository_root/tools/benchmarksgame-context-verify.sh" \
	--source-archive "$source_archive" \
	--workspace "$context_workspace" \
	--cc "$cc" \
	--cxx "$cxx" \
	--go "$go_tool" \
	--rustc "$rustc" \
	--javac "$javac" \
	--java "$java" \
	--unzip "$unzip_tool" \
	> "$evidence/context.stdout" 2> "$evidence/context.stderr"

printf 'case\tcandidate\tcommand\targ1\targ2\targ3\targ4\texpected\n' > "$catalog"
for case_name in fannkuch-redux n-body spectral-norm; do
	if test "$selected_case" != all && test "$selected_case" != "$case_name"; then
		continue
	fi
	case "$case_name:$mode" in
	fannkuch-redux:smoke) input=7; expected_name=7 ;;
	fannkuch-redux:formal) input=12; expected_name=12 ;;
	n-body:smoke) input=1000; expected_name=1000 ;;
	n-body:formal) input=50000000; expected_name=50000000 ;;
	spectral-norm:smoke) input=100; expected_name=100 ;;
	spectral-norm:formal) input=5500; expected_name=5500 ;;
	esac
	expected=$repository_root/benchmarks/benchmarksgame/$case_name/expected/$expected_name.txt
	printf '%s\ttyperb-native\t%s\t%s\t-\t-\t-\t%s\n' \
		"$case_name" "$backend_workspace/$case_name/native" "$input" "$expected" >> "$catalog"
	printf '%s\ttyperb-go\t%s\t%s\t-\t-\t-\t%s\n' \
		"$case_name" "$backend_workspace/$case_name/reference" "$input" "$expected" >> "$catalog"
	for language in c cpp go rust; do
		printf '%s\t%s\t%s\t%s\t-\t-\t-\t%s\n' \
			"$case_name" "$language" "$context_workspace/programs/$case_name/$language/program" "$input" "$expected" \
			>> "$catalog"
	done
	case "$case_name" in
	fannkuch-redux) java_class=fannkuchredux ;;
	n-body) java_class=nbody ;;
	spectral-norm) java_class=spectralnorm ;;
	esac
	printf '%s\tjava\t%s\t-cp\t%s\t%s\t%s\t%s\n' \
		"$case_name" "$java" "$context_workspace/programs/$case_name/java/classes" "$java_class" "$input" "$expected" \
		>> "$catalog"
done

tab=$(printf '\t')
printf 'case\tcandidate\tstatus\tstderr_empty\tstdout_exact\n' > "$evidence/catalog-correctness.tsv"
correctness_failures=0
while IFS="$tab" read -r case_name candidate command arg1 arg2 arg3 arg4 expected; do
	if test "$case_name" = case; then continue; fi
	case_evidence=$evidence/catalog-correctness/$case_name/$candidate
	mkdir -p "$case_evidence"
	set -- "$command"
	for argument in "$arg1" "$arg2" "$arg3" "$arg4"; do
		if test "$argument" != -; then set -- "$@" "$argument"; fi
	done
	set +e
	"$@" > "$case_evidence/stdout" 2> "$case_evidence/stderr"
	status=$?
	set -e
	stderr_empty=true
	stdout_exact=true
	if test -s "$case_evidence/stderr"; then stderr_empty=false; correctness_failures=$((correctness_failures + 1)); fi
	if ! cmp "$expected" "$case_evidence/stdout" >/dev/null; then stdout_exact=false; correctness_failures=$((correctness_failures + 1)); fi
	if test "$status" -ne 0; then correctness_failures=$((correctness_failures + 1)); fi
	printf '%s\t%s\t%s\t%s\t%s\n' "$case_name" "$candidate" "$status" "$stderr_empty" "$stdout_exact" \
		>> "$evidence/catalog-correctness.tsv"
done < "$catalog"

printf 'case\tcandidate\tartifact\tbytes\tsha256\n' > "$evidence/artifacts.tsv"
while IFS="$tab" read -r case_name candidate command arg1 arg2 arg3 arg4 expected; do
	if test "$case_name" = case; then continue; fi
	if test "$candidate" = java; then
		class_directory=$arg2
		find "$class_directory" -type f -name '*.class' | LC_ALL=C sort | while IFS= read -r class_file; do
			printf '%s\t%s\t%s\t%s\t%s\n' \
				"$case_name" "$candidate" "$class_file" "$(file_size "$class_file")" "$(sha256 "$class_file")"
		done >> "$evidence/artifacts.tsv"
	else
		printf '%s\t%s\t%s\t%s\t%s\n' \
			"$case_name" "$candidate" "$command" "$(file_size "$command")" "$(sha256 "$command")" \
			>> "$evidence/artifacts.tsv"
	fi
done < "$catalog"

{
	printf 'mode=%s\n' "$mode"
	printf 'selected_case=%s\n' "$selected_case"
	printf 'target=%s\n' "$target"
	printf 'repository_revision=%s\n' "$(git -C "$repository_root" rev-parse HEAD)"
	printf 'type_rb_revision=%s\n' "$(tr -d '\n' < "$repository_root/TYPE_RB_REVISION")"
	printf 'source_archive_sha256=%s\n' "$(sha256 "$source_archive")"
	printf 'catalog_sha256=%s\n' "$(sha256 "$catalog")"
	printf 'native_compiler_sha256=%s\n' "$(sha256 "$native_compiler")"
	printf 'reference_trb=%s\n' "$("$reference_trb" --version)"
	printf 'cc=%s\n' "$($cc --version 2>&1 | awk 'NR == 1 { print; exit }')"
	printf 'cxx=%s\n' "$($cxx --version 2>&1 | awk 'NR == 1 { print; exit }')"
	printf 'go=%s\n' "$($go_tool version)"
	printf 'rustc=%s\n' "$($rustc --version)"
	printf 'javac=%s\n' "$($javac -version 2>&1)"
	printf 'java=%s\n' "$($java -version 2>&1 | awk 'NR == 1 { print; exit }')"
} > "$evidence/environment.txt"

cp "$backend_workspace/summary.tsv" "$evidence/backend-pair-summary.tsv"
cp "$backend_workspace/toolchain.txt" "$evidence/backend-pair-toolchain.txt"
cp "$context_workspace/summary.tsv" "$evidence/context-summary.tsv"
cp "$context_workspace/commands.tsv" "$evidence/context-commands.tsv"
cp "$context_workspace/toolchain.txt" "$evidence/context-toolchain.txt"

test "$correctness_failures" -eq 0 || fail "prepared runtime correctness failed"
printf 'benchmarksgame-prepare-runtime: %s catalog passed\n' "$mode"
