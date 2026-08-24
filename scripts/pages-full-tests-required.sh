#!/usr/bin/env bash

set -euo pipefail

saw_path=false
while IFS= read -r changed_path; do
	if [[ -z "$changed_path" ]]; then
		continue
	fi
	saw_path=true
	case "$changed_path" in
		README.md | llms.txt | docs/* | cmd/trb-site/* | internal/site/* | internal/playground/assets/* | scripts/build-site.sh | syntaxes/* | tools/site/* | tools/textmate/*)
			;;
		*)
			printf 'true\n'
			exit 0
			;;
	esac
done

if [[ "$saw_path" == false ]]; then
	printf 'true\n'
else
	printf 'false\n'
fi
