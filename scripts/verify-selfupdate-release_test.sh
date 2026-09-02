#!/bin/sh
# Offline fixtures for scripts/verify-selfupdate-release.sh. Performs no publication.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="$ROOT/scripts/verify-selfupdate-release.sh"
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

pass() {
	echo "ok - $1"
}

fail() {
	echo "not ok - $1" >&2
	exit 1
}

PRODUCTS='["demo"]'
PLATFORMS='[{"os":"linux","arch":"amd64"},{"os":"windows","arch":"amd64"}]'
EXTRAS='["install.sh"]'

digest_of() {
	python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$1"
}

make_valid() {
	d="$1"
	mkdir -p "$d"
	printf 'linux-body' >"$d/demo-linux-amd64"
	printf 'win-body' >"$d/demo-windows-amd64.exe"
	printf 'installer' >"$d/install.sh"
	linux=$(digest_of "$d/demo-linux-amd64")
	win=$(digest_of "$d/demo-windows-amd64.exe")
	printf '%s  demo-linux-amd64\n%s  demo-windows-amd64.exe\n' "$linux" "$win" >"$d/SHA256SUMS"
}

run_ok() {
	label="$1"
	shift
	if "$SCRIPT" "$@"; then
		pass "$label"
	else
		fail "$label"
	fi
}

run_fail() {
	label="$1"
	shift
	if "$SCRIPT" "$@" >/dev/null 2>&1; then
		fail "$label (expected failure)"
	else
		pass "$label"
	fi
}

VALID="$WORKDIR/valid"
make_valid "$VALID"
run_ok "valid artifact" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

MISSING="$WORKDIR/missing"
cp -R "$VALID" "$MISSING"
rm -f "$MISSING/demo-linux-amd64"
run_fail "missing binary" \
	--dir "$MISSING" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

DUP="$WORKDIR/dup"
cp -R "$VALID" "$DUP"
linux=$(digest_of "$DUP/demo-linux-amd64")
printf '%s  demo-linux-amd64\n%s  demo-linux-amd64\n' "$linux" "$linux" >"$DUP/SHA256SUMS"
run_fail "duplicate checksum entry" \
	--dir "$DUP" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

EXTRA="$WORKDIR/extra"
cp -R "$VALID" "$EXTRA"
printf 'nope' >"$EXTRA/unexpected"
run_fail "undeclared extra file" \
	--dir "$EXTRA" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

MALFORMED="$WORKDIR/malformed"
cp -R "$VALID" "$MALFORMED"
printf 'not-a-digest  demo-linux-amd64\n' >"$MALFORMED/SHA256SUMS"
run_fail "malformed SHA256SUMS" \
	--dir "$MALFORMED" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

BRIDGE="$WORKDIR/bridge"
make_valid "$BRIDGE"
cp "$BRIDGE/demo-linux-amd64" "$BRIDGE/demo-linux-amd64-0.16.0"
cp "$BRIDGE/demo-windows-amd64.exe" "$BRIDGE/demo-windows-amd64-0.16.0.exe"
linux=$(digest_of "$BRIDGE/demo-linux-amd64-0.16.0")
win=$(digest_of "$BRIDGE/demo-windows-amd64-0.16.0.exe")
printf '%s  demo-linux-amd64-0.16.0\n%s  demo-windows-amd64-0.16.0.exe\n' "$linux" "$win" >"$BRIDGE/SHA256SUMS-0.16.0"
run_ok "bridge artifact" \
	--dir "$BRIDGE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--bridge true --tag v0.16.0 --repository maccavelli/magic-cli-remote

run_fail "bridge rejected for other repo" \
	--dir "$BRIDGE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--bridge true --tag v0.16.0 --repository maccavelli/other

run_fail "bridge rejected for other tag" \
	--dir "$BRIDGE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--bridge true --tag v0.17.0 --repository maccavelli/magic-cli-remote

BADBRIDGE="$WORKDIR/badbridge"
cp -R "$BRIDGE" "$BADBRIDGE"
printf 'mutated' >"$BADBRIDGE/demo-linux-amd64-0.16.0"
run_fail "bridge binary not identical" \
	--dir "$BADBRIDGE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--bridge true --tag v0.16.0 --repository maccavelli/magic-cli-remote

echo "verify-selfupdate-release_test: all fixtures passed"
