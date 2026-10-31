#!/bin/sh
# Builds a repository full of awkward changes so you can look at how drif
# renders them. Every file here is something that has broken a diff viewer
# before: renames, binaries, mode-only changes, symlinks, spaces, non-ASCII,
# CRLF, an empty file, a deleted file and an untracked binary.
#
#   scripts/demo.sh [dir]     # default: /tmp/drif-demo
#   cd /tmp/drif-demo && drif
set -eu

dir=${1:-/tmp/drif-demo}
rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"

git init -q -b main
git config user.email demo@example.com
git config user.name demo

mkdir -p src "docs with space" vendor
cat > src/main.go <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
EOF
cat > src/render.go <<'EOF'
package main

// Render draws one line.
func Render(in string) string {
	out := "[" + in + "]"
	if out == "" {
		return "empty"
	}
	return out
}
EOF
printf 'one\ntwo\nthree\n' > src/delete-me.sh
printf '#!/bin/sh\necho hi\n' > src/script.sh
printf 'a\n' > "docs with space/notes.md"
printf 'naive\n' > "docs with space/naive.txt"
printf 'crlf\r\nlines\r\n' > src/crlf.txt
printf '' > src/empty.txt
ln -s main.go src/link.go
printf '\211PNG\r\n\032\n\000\000\000\rIHDR' > src/logo.png
printf 'lock contents\n' > vendor/vendor.lock
printf 'package main\n\nfunc helper() {}\n' > src/helper.go
git add -A
git commit -qm "initial"

# --- changes -------------------------------------------------------------
git mv src/helper.go src/renamed-helper.go
sed -i 's/func helper() {}/func helper() { \/\/ renamed and edited }/' src/renamed-helper.go

sed -i 's/fmt.Println("hello")/fmt.Println("hello, world")/' src/main.go

cat > src/render.go <<'EOF'
package main

import "strings"

// Render draws one line, padded.
func Render(in string) string {
	out := strings.TrimSpace("[" + in + "]")
	if out == "" {
		return "empty"
	}
	return out
}
EOF

git rm -q src/delete-me.sh
chmod +x src/script.sh
ln -sfn render.go src/link.go
printf 'naïve café\n' > "docs with space/naïve café.txt"
printf 'a\nb\n' > "docs with space/notes.md"
sed -i 's/$/\r/' src/crlf.txt 2>/dev/null || true
printf '\211PNG\r\n\032\n\000\000\000\rIHDR\000\000' > src/logo.png
printf 'new\n' > src/empty.txt

# a large untracked file and a large modified one, to exercise the size limits
awk 'BEGIN { for (i = 0; i < 4000; i++) print "line " i }' > src/big.txt
printf 'untracked\n' > src/untracked.txt
printf '\000\001\002\003' > src/untracked.bin

echo "built $dir"
echo
git status --short
echo
echo "now run:  cd $dir && drif"
