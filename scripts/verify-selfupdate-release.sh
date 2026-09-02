#!/bin/sh
# Verify a staged self-update release directory against the canonical
# product/platform/extra matrix. Performs no publication.
set -eu

usage() {
	echo "usage: verify-selfupdate-release.sh --dir DIR --products JSON --platforms JSON --extras JSON [--bridge true|false] [--tag TAG] [--repository OWNER/REPO]" >&2
	exit 2
}

DIR=""
PRODUCTS_JSON=""
PLATFORMS_JSON=""
EXTRAS_JSON="[]"
BRIDGE="false"
TAG=""
REPOSITORY=""

while [ $# -gt 0 ]; do
	case "$1" in
	--dir)
		DIR="${2:-}"
		shift 2
		;;
	--products)
		PRODUCTS_JSON="${2:-}"
		shift 2
		;;
	--platforms)
		PLATFORMS_JSON="${2:-}"
		shift 2
		;;
	--extras)
		EXTRAS_JSON="${2:-}"
		shift 2
		;;
	--bridge)
		BRIDGE="${2:-false}"
		shift 2
		;;
	--tag)
		TAG="${2:-}"
		shift 2
		;;
	--repository)
		REPOSITORY="${2:-}"
		shift 2
		;;
	*)
		usage
		;;
	esac
done

[ -n "$DIR" ] && [ -n "$PRODUCTS_JSON" ] && [ -n "$PLATFORMS_JSON" ] || usage
[ -d "$DIR" ] || {
	echo "verify-selfupdate-release: staging directory $DIR is missing" >&2
	exit 1
}

python3 - "$DIR" "$PRODUCTS_JSON" "$PLATFORMS_JSON" "$EXTRAS_JSON" "$BRIDGE" "$TAG" "$REPOSITORY" <<'PY'
import hashlib, json, os, re, sys

dirpath, products_raw, platforms_raw, extras_raw, bridge, tag, repo = sys.argv[1:8]
tag_re = re.compile(r"^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")
hex_re = re.compile(r"^[0-9a-fA-F]{64}$")
os_arch_re = re.compile(r"^[a-z0-9][a-z0-9_]*$")
product_re = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")

def fail(msg):
    print("verify-selfupdate-release: " + msg, file=sys.stderr)
    sys.exit(1)

try:
    products = json.loads(products_raw)
    platforms = json.loads(platforms_raw)
    extras = json.loads(extras_raw)
except json.JSONDecodeError as e:
    fail("invalid JSON: %s" % e)

if not isinstance(products, list) or not products:
    fail("products-json must be a non-empty JSON array")
if not isinstance(platforms, list) or not platforms:
    fail("platforms-json must be a non-empty JSON array")
if not isinstance(extras, list):
    fail("extra-assets-json must be a JSON array")

seen_products = set()
for p in products:
    if not isinstance(p, str) or not product_re.match(p):
        fail("invalid product %r" % p)
    if p in seen_products:
        fail("duplicate product %s" % p)
    seen_products.add(p)

seen_plats = set()
for plat in platforms:
    if not isinstance(plat, dict) or set(plat.keys()) != {"os", "arch"}:
        fail("platform objects must have only os and arch")
    osname, arch = plat["os"], plat["arch"]
    if not isinstance(osname, str) or not os_arch_re.match(osname):
        fail("invalid platform os %r" % osname)
    if not isinstance(arch, str) or not os_arch_re.match(arch):
        fail("invalid platform arch %r" % arch)
    key = (osname, arch)
    if key in seen_plats:
        fail("duplicate platform %s/%s" % key)
    seen_plats.add(key)

seen_extras = set()
for extra in extras:
    if not isinstance(extra, str) or extra in ("", ".", "..") or "/" in extra or "\\" in extra:
        fail("invalid extra asset %r" % extra)
    if extra in seen_extras or extra == "SHA256SUMS" or extra.startswith("SHA256SUMS-"):
        fail("invalid or duplicate extra asset %r" % extra)
    seen_extras.add(extra)

def asset_name(product, osname, arch):
    name = "%s-%s-%s" % (product, osname, arch)
    if osname == "windows":
        name += ".exe"
    return name

canonical = []
for product in products:
    for osname, arch in seen_plats:
        canonical.append(asset_name(product, osname, arch))

expected = set(canonical)
expected.add("SHA256SUMS")
expected.update(seen_extras)

bridge_on = bridge.lower() in ("1", "true", "yes")
compat = []
if bridge_on:
    if repo != "maccavelli/magic-cli-remote" or tag != "v0.16.0":
        fail("bridge-release is permitted only for maccavelli/magic-cli-remote tag v0.16.0")
    if not tag_re.match(tag):
        fail("tag %r is not a strict stable tag" % tag)
    for product in products:
        for osname, arch in seen_plats:
            name = "%s-%s-%s-0.16.0" % (product, osname, arch)
            if osname == "windows":
                name += ".exe"
            compat.append(name)
    expected.update(compat)
    expected.add("SHA256SUMS-0.16.0")
elif tag and not tag_re.match(tag):
    fail("tag %r is not a strict stable tag" % tag)

present = []
for name in os.listdir(dirpath):
    path = os.path.join(dirpath, name)
    if os.path.isdir(path):
        fail("unexpected directory %s in staging" % name)
    present.append(name)

present_set = set(present)
if present_set != expected:
    missing = sorted(expected - present_set)
    extra = sorted(present_set - expected)
    fail("file set mismatch missing=%s extra=%s" % (missing, extra))

def parse_sums(path):
    entries = {}
    with open(path, "r", encoding="utf-8", newline="") as f:
        data = f.read()
    if not data:
        fail("%s is empty" % os.path.basename(path))
    for i, raw in enumerate(data.splitlines(), 1):
        line = raw.rstrip("\r")
        stripped = line.strip()
        if stripped == "" or stripped.startswith("#"):
            continue
        fields = line.split()
        if len(fields) != 2:
            fail("%s line %d: want exactly two fields" % (os.path.basename(path), i))
        digest, name = fields
        if name.startswith("*"):
            name = name[1:]
        if not hex_re.match(digest):
            fail("%s line %d: malformed digest" % (os.path.basename(path), i))
        if name in ("", ".", "..") or "/" in name or "\\" in name:
            fail("%s line %d: filename is not a basename" % (os.path.basename(path), i))
        key = name
        if key in entries:
            fail("%s duplicate filename %s" % (os.path.basename(path), name))
        entries[key] = digest.lower()
    return entries

sums = parse_sums(os.path.join(dirpath, "SHA256SUMS"))
if set(sums) != set(canonical):
    fail("SHA256SUMS must contain exactly the canonical binaries")

def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

for name in canonical:
    path = os.path.join(dirpath, name)
    got = sha256_file(path)
    if got != sums[name]:
        fail("SHA256SUMS mismatch for %s" % name)

if bridge_on:
    bridge_sums = parse_sums(os.path.join(dirpath, "SHA256SUMS-0.16.0"))
    if set(bridge_sums) != set(compat):
        fail("SHA256SUMS-0.16.0 must contain exactly the compatibility binaries")
    for product in products:
        for osname, arch in seen_plats:
            canon = asset_name(product, osname, arch)
            alias = "%s-%s-%s-0.16.0" % (product, osname, arch)
            if osname == "windows":
                alias += ".exe"
            a = os.path.join(dirpath, canon)
            b = os.path.join(dirpath, alias)
            with open(a, "rb") as fa, open(b, "rb") as fb:
                if fa.read() != fb.read():
                    fail("%s is not byte-identical to %s" % (alias, canon))
            got = sha256_file(b)
            if got != bridge_sums[alias]:
                fail("SHA256SUMS-0.16.0 mismatch for %s" % alias)

print("verify-selfupdate-release: ok")
PY
