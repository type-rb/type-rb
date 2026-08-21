#!/usr/bin/env bash

set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
extension_root="$repository_root/editors/vscode"
qa_root="${TYPERB_LOCAL_QA_ROOT:-$extension_root/.vscode-test/local-qa}"
marker_text="TypeRB local VS Code QA v1"

usage() {
	cat <<'EOF'
Usage:
  ./scripts/vscode-local-qa.sh open WORKSPACE
  ./scripts/vscode-local-qa.sh test
  ./scripts/vscode-local-qa.sh regular WORKSPACE
  ./scripts/vscode-local-qa.sh reset

Commands:
  open     Build the current trb and VSIX, install them into an isolated
           VS Code profile, and open WORKSPACE in a new window.
  test     Run the extension unit tests and Stable Extension Host suite.
  regular  Open WORKSPACE with the regular VS Code profile and extensions.
  reset    Delete only the managed isolated profile and its local artifacts.

Environment:
  TYPERB_CODE                 VS Code CLI command or absolute path.
  TYPERB_LOCAL_QA_ROOT        Override the isolated state directory.
  TYPERB_LOCAL_QA_FORCE_NPM_CI=1
                              Reinstall extension dependencies before use.
EOF
}

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

resolve_command() {
	local value=$1
	local label=$2
	if [[ "$value" == */* ]]; then
		[[ -x "$value" ]] || fail "$label is not executable: $value"
		printf '%s\n' "$value"
		return
	fi
	command -v "$value" 2>/dev/null || fail "$label was not found: $value"
}

resolve_code() {
	if [[ -n "${TYPERB_CODE:-}" ]]; then
		resolve_command "$TYPERB_CODE" "VS Code CLI"
		return
	fi
	if command -v code >/dev/null 2>&1; then
		command -v code
		return
	fi
	local macos_code="/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"
	[[ -x "$macos_code" ]] || fail "VS Code CLI was not found; set TYPERB_CODE or install the 'code' shell command"
	printf '%s\n' "$macos_code"
}

resolve_workspace() {
	local target=$1
	[[ -e "$target" ]] || fail "workspace does not exist: $target"
	if [[ -d "$target" ]]; then
		(cd "$target" && pwd -P)
	else
		local parent
		parent="$(cd "$(dirname "$target")" && pwd -P)"
		printf '%s/%s\n' "$parent" "$(basename "$target")"
	fi
}

canonicalize_existing_qa_root() {
	[[ -d "$qa_root" ]] || fail "isolated QA path is not a directory: $qa_root"
	qa_root="$(cd "$qa_root" && pwd -P)"
	case "$qa_root" in
		""|/|"$repository_root"|"$extension_root")
			fail "refusing to use unsafe isolated QA root: $qa_root"
			;;
	esac
}

ensure_qa_root() {
	if [[ -e "$qa_root" && ! -d "$qa_root" ]]; then
		fail "isolated QA path is not a directory: $qa_root"
	fi
	if [[ ! -e "$qa_root" ]]; then
		mkdir -p "$qa_root"
		printf '%s\n' "$marker_text" > "$qa_root/.type-rb-local-qa"
	fi
	canonicalize_existing_qa_root
	local marker="$qa_root/.type-rb-local-qa"
	if [[ ! -f "$marker" || "$(<"$marker")" != "$marker_text" ]]; then
		fail "refusing to manage unrecognized directory: $qa_root"
	fi
}

install_dependencies() {
	local npm_bin=$1
	local installed_lock="$extension_root/node_modules/.package-lock.json"
	if [[ "${TYPERB_LOCAL_QA_FORCE_NPM_CI:-0}" == 1 || ! -x "$extension_root/node_modules/.bin/esbuild" || ! -x "$extension_root/node_modules/.bin/vsce" || ! -f "$installed_lock" || "$extension_root/package-lock.json" -nt "$installed_lock" ]]; then
		"$npm_bin" ci --prefix "$extension_root"
	fi
}

write_settings() {
	local node_bin=$1
	local settings_path=$2
	local trb_path=$3
	mkdir -p "$(dirname "$settings_path")"
	"$node_bin" - "$settings_path" "$trb_path" <<'NODE'
const fs = require("node:fs");
const [settingsPath, trbPath] = process.argv.slice(2);
const settings = {
	"typerb.server.path": trbPath,
	"extensions.autoCheckUpdates": false,
	"extensions.autoUpdate": false,
	"security.workspace.trust.enabled": false,
	"window.title": "[TypeRB Local QA] ${activeEditorShort}${separator}${rootName}",
	"workbench.startupEditor": "none",
	"[trb]": {
		"editor.defaultFormatter": "type-rb.typerb",
		"editor.formatOnSave": true,
	},
};
fs.writeFileSync(settingsPath, `${JSON.stringify(settings, null, "\t")}\n`);
NODE
}

open_isolated() {
	[[ $# -eq 1 ]] || { usage >&2; exit 2; }
	local workspace
	workspace="$(resolve_workspace "$1")"
	ensure_qa_root

	local code_bin go_bin npm_bin node_bin vsce_bin
	code_bin="$(resolve_code)"
	go_bin="$(resolve_command "${TYPERB_GO:-go}" "Go command")"
	npm_bin="$(resolve_command "${TYPERB_NPM:-npm}" "npm command")"
	node_bin="$(resolve_command "${TYPERB_NODE:-node}" "Node.js command")"
	install_dependencies "$npm_bin"
	vsce_bin="$(resolve_command "${TYPERB_VSCE:-$extension_root/node_modules/.bin/vsce}" "vsce command")"

	local binary_dir="$qa_root/bin"
	local trb_path="$binary_dir/trb"
	local vsix_path="$qa_root/typerb.vsix"
	local user_data_dir="$qa_root/user-data"
	local extensions_dir="$qa_root/extensions"
	mkdir -p "$binary_dir" "$user_data_dir" "$extensions_dir"

	(
		cd "$repository_root"
		GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/type-rb-go-cache}" \
			"$go_bin" build -o "$trb_path" ./cmd/trb
	)
	"$npm_bin" run build:production --prefix "$extension_root"
	(
		cd "$extension_root"
		"$vsce_bin" package --out "$vsix_path"
	)
	[[ -x "$trb_path" ]] || fail "local trb was not built: $trb_path"
	[[ -s "$vsix_path" ]] || fail "local VSIX was not built: $vsix_path"

	write_settings "$node_bin" "$user_data_dir/User/settings.json" "$trb_path"
	"$code_bin" \
		--user-data-dir "$user_data_dir" \
		--extensions-dir "$extensions_dir" \
		--install-extension "$vsix_path" \
		--force

	local installed
	installed="$("$code_bin" --user-data-dir "$user_data_dir" --extensions-dir "$extensions_dir" --list-extensions --show-versions)"
	grep -Eq '^type-rb\.typerb@' <<<"$installed" || fail "the isolated TypeRB extension installation could not be verified"

	"$code_bin" \
		--new-window \
		--user-data-dir "$user_data_dir" \
		--extensions-dir "$extensions_dir" \
		"$workspace"

	printf 'Opened TypeRB local QA in an isolated VS Code window.\n'
	printf '  workspace: %s\n' "$workspace"
	printf '  trb:       %s\n' "$trb_path"
	printf '  profile:   %s\n' "$qa_root"
	printf 'Close that window and run `%s regular %s` to use regular VS Code.\n' "$0" "$workspace"
}

run_tests() {
	[[ $# -eq 0 ]] || { usage >&2; exit 2; }
	local npm_bin
	npm_bin="$(resolve_command "${TYPERB_NPM:-npm}" "npm command")"
	install_dependencies "$npm_bin"
	"$npm_bin" test --prefix "$extension_root"
	"$npm_bin" run test:integration --prefix "$extension_root"
}

open_regular() {
	[[ $# -eq 1 ]] || { usage >&2; exit 2; }
	local workspace code_bin
	workspace="$(resolve_workspace "$1")"
	code_bin="$(resolve_code)"
	"$code_bin" --new-window "$workspace"
	printf 'Opened the regular VS Code profile; the isolated QA profile was not used.\n'
}

reset_isolated() {
	[[ $# -eq 0 ]] || { usage >&2; exit 2; }
	if [[ ! -e "$qa_root" ]]; then
		printf 'No isolated TypeRB QA state exists at %s\n' "$qa_root"
		return
	fi
	canonicalize_existing_qa_root
	local marker="$qa_root/.type-rb-local-qa"
	if [[ ! -f "$marker" || "$(<"$marker")" != "$marker_text" ]]; then
		fail "refusing to remove unrecognized directory: $qa_root"
	fi
	rm -rf -- "$qa_root"
	printf 'Removed isolated TypeRB QA state at %s\n' "$qa_root"
}

command_name="${1:-}"
if [[ $# -gt 0 ]]; then
	shift
fi
case "$command_name" in
	open)
		open_isolated "$@"
		;;
	test)
		run_tests "$@"
		;;
	regular)
		open_regular "$@"
		;;
	reset)
		reset_isolated "$@"
		;;
	-h|--help|help)
		usage
		;;
	*)
		usage >&2
		exit 2
		;;
esac
