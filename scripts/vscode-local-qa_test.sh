#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repository_root/scripts/vscode-local-qa.sh"
temporary="$(mktemp -d)"
temporary="$(cd "$temporary" && pwd -P)"
trap 'rm -rf "$temporary"' EXIT

fake_bin="$temporary/bin"
qa_root="$temporary/qa"
workspace="$temporary/workspace"
command_log="$temporary/commands.log"
mkdir -p "$fake_bin" "$workspace"

cat > "$fake_bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'go %s\n' "$*" >> "$TYPERB_QA_TEST_LOG"
output=""
while [[ $# -gt 0 ]]; do
	if [[ "$1" == -o ]]; then
		output=$2
		shift 2
		continue
	fi
	shift
done
mkdir -p "$(dirname "$output")"
printf '#!/bin/sh\nprintf "trb local QA test\\n"\n' > "$output"
chmod +x "$output"
EOF

cat > "$fake_bin/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'npm %s\n' "$*" >> "$TYPERB_QA_TEST_LOG"
EOF

cat > "$fake_bin/vsce" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'vsce %s\n' "$*" >> "$TYPERB_QA_TEST_LOG"
output=""
while [[ $# -gt 0 ]]; do
	if [[ "$1" == --out ]]; then
		output=$2
		shift 2
		continue
	fi
	shift
done
mkdir -p "$(dirname "$output")"
printf 'local QA VSIX\n' > "$output"
EOF

cat > "$fake_bin/code" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'code %s\n' "$*" >> "$TYPERB_QA_TEST_LOG"
for argument in "$@"; do
	if [[ "$argument" == --list-extensions ]]; then
		printf 'type-rb.typerb@0.0.0-local\n'
		exit 0
	fi
done
EOF

chmod +x "$fake_bin/go" "$fake_bin/npm" "$fake_bin/vsce" "$fake_bin/code"

export TYPERB_QA_TEST_LOG="$command_log"
export TYPERB_CODE="$fake_bin/code"
export TYPERB_GO="$fake_bin/go"
export TYPERB_NPM="$fake_bin/npm"
export TYPERB_VSCE="$fake_bin/vsce"
export TYPERB_LOCAL_QA_ROOT="$qa_root"
export TYPERB_LOCAL_QA_FORCE_NPM_CI=1

"$script" open "$workspace"

test -x "$qa_root/bin/trb"
test -s "$qa_root/typerb.vsix"
test "$(cat "$qa_root/.type-rb-local-qa")" = "TypeRB local VS Code QA v1"
grep -Fq "\"typerb.server.path\": \"$qa_root/bin/trb\"" "$qa_root/user-data/User/settings.json"
grep -Fq '"editor.formatOnSave": true' "$qa_root/user-data/User/settings.json"
grep -Fq 'npm ci --prefix' "$command_log"
grep -Fq 'go build -o' "$command_log"
grep -Fq 'npm run build:production --prefix' "$command_log"
grep -Fq "vsce package --out $qa_root/typerb.vsix" "$command_log"
grep -Fq -- "--user-data-dir $qa_root/user-data --extensions-dir $qa_root/extensions --install-extension $qa_root/typerb.vsix --force" "$command_log"
grep -Fq -- "--new-window --user-data-dir $qa_root/user-data --extensions-dir $qa_root/extensions $workspace" "$command_log"

before_regular="$(wc -l < "$command_log")"
"$script" regular "$workspace"
regular_command="$(tail -n +$((before_regular + 1)) "$command_log")"
grep -Fq "code --new-window $workspace" <<<"$regular_command"
if grep -Fq -- '--user-data-dir' <<<"$regular_command"; then
	echo "regular VS Code unexpectedly used the isolated user data" >&2
	exit 1
fi

"$script" test
grep -Fq 'npm test --prefix' "$command_log"
grep -Fq 'npm run test:integration --prefix' "$command_log"

"$script" reset
test ! -e "$qa_root"

unmanaged="$temporary/unmanaged"
mkdir -p "$unmanaged"
printf 'keep\n' > "$unmanaged/file"
if TYPERB_LOCAL_QA_ROOT="$unmanaged" "$script" reset >/dev/null 2>&1; then
	echo "reset accepted an unmanaged directory" >&2
	exit 1
fi
test -f "$unmanaged/file"
