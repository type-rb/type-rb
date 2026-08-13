#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT

cat > "$temporary/CHANGELOG.md" <<'EOF'
# Changelog

## 0.2.4 - 2026-08-13

### Fixed

- Immediate jobs are ready without waiting for the next poll.

## 0.2.3 - 2026-08-12

- Earlier release.
EOF

"$root/scripts/extract-release-notes.sh" 0.2.4 "$temporary/CHANGELOG.md" "$temporary/notes.md"
grep -Fqx -- "- Immediate jobs are ready without waiting for the next poll." "$temporary/notes.md"
if grep -Fq -- "Earlier release" "$temporary/notes.md"; then
	echo "release notes included the following release" >&2
	exit 1
fi

if "$root/scripts/extract-release-notes.sh" 0.2.5 "$temporary/CHANGELOG.md" "$temporary/missing.md" >/dev/null 2>&1; then
	echo "missing release section was accepted" >&2
	exit 1
fi

cat > "$temporary/empty.md" <<'EOF'
# Changelog

## 0.2.4 - 2026-08-13

## 0.2.3 - 2026-08-12

- Earlier release.
EOF
if "$root/scripts/extract-release-notes.sh" 0.2.4 "$temporary/empty.md" "$temporary/empty-notes.md" >/dev/null 2>&1; then
	echo "empty release section was accepted" >&2
	exit 1
fi

cat > "$temporary/invalid-date.md" <<'EOF'
# Changelog

## 0.2.4 - August 13, 2026

- Invalid date.
EOF
if "$root/scripts/extract-release-notes.sh" 0.2.4 "$temporary/invalid-date.md" "$temporary/invalid-date-notes.md" >/dev/null 2>&1; then
	echo "invalid release date was accepted" >&2
	exit 1
fi
