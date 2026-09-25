#!/usr/bin/env bash
# Regenerate examples/diff-demo/diff.xml.
#
# Builds a throwaway git repository in a temp directory, commits two revisions
# apart, then runs `ctxpack diff` on the range between them. The scratch repo
# is discarded when the script exits, so nothing it creates needs committing.
#
# Usage: make.sh [ctxpack-binary]
set -euo pipefail

CTX=${1:-ctxpack}
OUT=examples/diff-demo/diff.xml

base=$(mktemp -d)
work="$base/diffdemo"
mkdir "$work"
trap 'rm -rf "$base"' EXIT

cd "$work"
git init -q
git config user.email demo@example.invalid
git config user.name demo
git config core.autocrlf false

# Revision one: a small package, plus the file revision two deletes.
mkdir -p src
cat > src/app.go <<'EOF'
package app

// Run starts the server.
func Run() {
	println("listening")
}
EOF
cat > src/legacy.go <<'EOF'
package app

// Legacy is kept for old callers; revision two removes it.
func Legacy() {}
EOF
git add -A
git commit -qm "initial"

# Revision two: one modification, one addition and one deletion.
cat >> src/app.go <<'EOF'

// Serve handles a single request.
func Serve(req string) string {
	return "ok: " + req
}
EOF
cat > src/util.go <<'EOF'
package app

// TitleCase uppercases the first letter of s.
func TitleCase(s string) string {
	if s == "" {
		return s
	}
	return string([]rune(s)[0]) + s[1:]
}
EOF
git rm -q src/legacy.go
git add -A
git commit -qm "add util, retire legacy"

# The range has two commits, so HEAD~1..HEAD is well defined.
cd - >/dev/null
mkdir -p "$(dirname "$OUT")"
"$CTX" diff "$work" --format xml --ref 'HEAD~1..HEAD' -o "$OUT"
