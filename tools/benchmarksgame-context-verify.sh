#!/bin/sh

set -eu

LC_ALL=C
export LC_ALL

archive_sha256=aabcf6726cdc14f0f45b99e5daba48584f94bbb48883fd3711a1d040474d1cb4

usage() {
	cat >&2 <<'EOF'
usage: benchmarksgame-context-verify.sh --source-archive PATH --workspace PATH
       --cc PATH --cxx PATH --go PATH --rustc PATH --javac PATH --java PATH
       --unzip PATH
EOF
	exit 64
}

fail() {
	printf 'benchmarksgame-context-verify: %s\n' "$1" >&2
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

directory_size() {
	find "$1" -type f -name '*.class' -exec wc -c {} \; | awk '{ total += $1 } END { print total + 0 }'
}

first_line() {
	"$@" 2>&1 | awk 'NR == 1 { print; exit }'
}

source_archive=
workspace=
cc=
cxx=
go_tool=
rustc=
javac=
java=
unzip_tool=

while test "$#" -gt 0; do
	case "$1" in
	--source-archive)
		test -z "$source_archive" && test "$#" -ge 2 || usage
		source_archive=$2
		shift 2
		;;
	--workspace)
		test -z "$workspace" && test "$#" -ge 2 || usage
		workspace=$2
		shift 2
		;;
	--cc)
		test -z "$cc" && test "$#" -ge 2 || usage
		cc=$2
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
	*) usage ;;
	esac
done

test -n "$source_archive" && test -n "$workspace" || usage
test -n "$cc" && test -n "$cxx" && test -n "$go_tool" || usage
test -n "$rustc" && test -n "$javac" && test -n "$java" || usage
test -n "$unzip_tool" || usage
test -f "$source_archive" || fail "source archive does not exist"
test -x "$cc" || fail "C compiler is not executable"
test -x "$cxx" || fail "C++ compiler is not executable"
test -x "$go_tool" || fail "Go compiler is not executable"
test -x "$rustc" || fail "Rust compiler is not executable"
test -x "$javac" || fail "Java compiler is not executable"
test -x "$java" || fail "Java runtime is not executable"
test -x "$unzip_tool" || fail "unzip tool is not executable"
test ! -e "$workspace" || fail "workspace already exists"

actual_archive_sha256=$(sha256 "$source_archive")
test "$actual_archive_sha256" = "$archive_sha256" || fail "source archive SHA-256 differs"

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH= cd -- "$script_directory/.." && pwd)
benchmark_root=$repository_root/benchmarks/benchmarksgame
manifest=$benchmark_root/context-sources.tsv
tab=$(printf '\t')
expected_header=$(printf 'case\tlanguage\tarchive_path\tsource_sha256\tinput\texpected\tentry')
actual_header=$(sed -n '1p' "$manifest")
test "$actual_header" = "$expected_header" || fail "context source manifest header differs"
manifest_entries=$(awk 'NR > 1 && NF > 0 { count += 1 } END { print count + 0 }' "$manifest")
test "$manifest_entries" -eq 15 || fail "context source manifest must contain 15 programs"

mkdir -p "$workspace"
summary=$workspace/summary.tsv
commands=$workspace/commands.tsv
printf 'case\tlanguage\tarchive_path\tsource_sha256\texpected_sha256\tartifact_bytes\n' > "$summary"
printf 'case\tlanguage\tphase\tcommand\n' > "$commands"

{
	IFS= read -r header
	while IFS="$tab" read -r case_name language archive_path source_sha256 input expected_name entry; do
		test -n "$case_name" || continue
		case_workspace=$workspace/programs/$case_name/$language
		source_directory=$case_workspace/source
		class_directory=$case_workspace/classes
		program=$case_workspace/program
		expected=$benchmark_root/$case_name/expected/$expected_name.txt
		mkdir -p "$source_directory"
		test -f "$expected" || fail "missing expected output for $case_name input $input"

		case "$language" in
		java) source=$source_directory/$entry.java ;;
		*) source=$source_directory/$(basename "$archive_path") ;;
		esac
		"$unzip_tool" -p "$source_archive" "$archive_path" > "$source"
		actual_source_sha256=$(sha256 "$source")
		test "$actual_source_sha256" = "$source_sha256" || fail "$archive_path SHA-256 differs"

		case "$language" in
		c)
			printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
				"$cc -O3 -std=gnu11 -x c $source -lm -o $program" >> "$commands"
			"$cc" -O3 -std=gnu11 -x c "$source" -lm -o "$program" \
				> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
				fail "$case_name $language build failed; see $case_workspace/build.stderr"
			;;
		cpp)
			if test "$case_name" = n-body; then
				printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
					"$cxx -O3 -std=gnu++17 -include cstdlib -x c++ $source -lm -o $program" >> "$commands"
				"$cxx" -O3 -std=gnu++17 -include cstdlib -x c++ "$source" -lm -o "$program" \
					> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
					fail "$case_name $language build failed; see $case_workspace/build.stderr"
			else
				printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
					"$cxx -O3 -std=gnu++17 -x c++ $source -lm -o $program" >> "$commands"
				"$cxx" -O3 -std=gnu++17 -x c++ "$source" -lm -o "$program" \
					> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
					fail "$case_name $language build failed; see $case_workspace/build.stderr"
			fi
			;;
		go)
			printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
				"$go_tool build -trimpath -o $program $source" >> "$commands"
			"$go_tool" build -trimpath -o "$program" "$source" \
				> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
				fail "$case_name $language build failed; see $case_workspace/build.stderr"
			;;
		rust)
			printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
				"$rustc --crate-name $entry -C opt-level=3 $source -o $program" >> "$commands"
			"$rustc" --crate-name "$entry" -C opt-level=3 "$source" -o "$program" \
				> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
				fail "$case_name $language build failed; see $case_workspace/build.stderr"
			;;
		java)
			mkdir -p "$class_directory"
			printf '%s\t%s\tbuild\t%s\n' "$case_name" "$language" \
				"$javac -d $class_directory $source" >> "$commands"
			"$javac" -d "$class_directory" "$source" \
				> "$case_workspace/build.stdout" 2> "$case_workspace/build.stderr" || \
				fail "$case_name $language build failed; see $case_workspace/build.stderr"
			;;
		*) fail "unknown language $language" ;;
		esac

		set +e
		if test "$language" = java; then
			printf '%s\t%s\trun\t%s\n' "$case_name" "$language" \
				"$java -cp $class_directory $entry $input" >> "$commands"
			"$java" -cp "$class_directory" "$entry" "$input" \
				> "$case_workspace/program.stdout" 2> "$case_workspace/program.stderr"
			status=$?
			artifact_bytes=$(directory_size "$class_directory")
		else
			printf '%s\t%s\trun\t%s\n' "$case_name" "$language" \
				"$program $input" >> "$commands"
			"$program" "$input" > "$case_workspace/program.stdout" 2> "$case_workspace/program.stderr"
			status=$?
			artifact_bytes=$(file_size "$program")
		fi
		set -e

		test "$status" -eq 0 || fail "$case_name $language run failed with status $status"
		test ! -s "$case_workspace/program.stderr" || fail "$case_name $language run wrote stderr"
		cmp "$expected" "$case_workspace/program.stdout" >/dev/null || fail "$case_name $language output differs"
		printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
			"$case_name" \
			"$language" \
			"$archive_path" \
			"$actual_source_sha256" \
			"$(sha256 "$expected")" \
			"$artifact_bytes" \
			>> "$summary"
	done
} < "$manifest"

{
	printf 'archive_sha256=%s\n' "$actual_archive_sha256"
	printf 'manifest_sha256=%s\n' "$(sha256 "$manifest")"
	printf 'cc=%s\n' "$(first_line "$cc" --version)"
	printf 'cxx=%s\n' "$(first_line "$cxx" --version)"
	printf 'go=%s\n' "$(first_line "$go_tool" version)"
	printf 'rustc=%s\n' "$(first_line "$rustc" --version)"
	printf 'javac=%s\n' "$(first_line "$javac" -version)"
	printf 'java=%s\n' "$(first_line "$java" -version)"
	printf 'unzip=%s\n' "$(first_line "$unzip_tool" -v)"
} > "$workspace/toolchain.txt"

printf 'benchmarksgame-context-verify: all 15 pinned context programs passed\n'
