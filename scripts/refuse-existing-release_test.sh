#!/usr/bin/env bash
# Offline tests for refuse-existing-release.sh, driven by a stubbed gh.
#
# The stub is injected through the GH variable, not a PATH prefix: this
# environment's shell startup files re-order PATH, which silently made an
# earlier version of these tests run the REAL gh and pass for the wrong reason.
#
# The third case is the one that matters. Before MADR 0007 the guard could not
# distinguish a FAILED query from "no release exists", so it proceeded whenever
# gh could not resolve the repository — reporting success while checking
# nothing. A guard that cannot fail is not a guard.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GUARD="${GUARD:-$ROOT/scripts/refuse-existing-release.sh}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0

check() { # name want got
	if [ "$2" = "$3" ]; then
		echo "  ok   $1"
		PASS=$((PASS + 1))
	else
		echo "  FAIL $1: want exit $2, got $3"
		FAIL=$((FAIL + 1))
	fi
}

# stub_gh <exit-code> <output>
stub_gh() {
	mkdir -p "$WORK/bin"
	cat >"$WORK/bin/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "$2" >&2
exit $1
STUB
	chmod +x "$WORK/bin/gh"
}

run_guard() {
	set +e
	GH="$WORK/bin/gh" "$GUARD" v1.2.3 >/dev/null 2>&1
	rc=$?
	set -e
	echo "$rc"
}

# 1. The release exists: gh succeeds. Must refuse.
stub_gh 0 "some release json"
check "existing release is refused" 1 "$(run_guard)"

# 2. The release is genuinely absent. Must proceed.
stub_gh 1 "release not found"
check "absent release proceeds" 0 "$(run_guard)"

# 2b. GitHub's other absent wording.
stub_gh 1 "HTTP 404: Not Found (https://api.github.com/repos/o/r/releases/tags/v1.2.3)"
check "404 Not Found proceeds" 0 "$(run_guard)"

# 3. THE REGRESSION CASE. The query itself failed for an unrelated reason.
#    This is exactly what happened in runs 33690128008 and 33692726660.
#    Must refuse, not proceed.
stub_gh 1 "failed to run git: fatal: not a git repository (or any of the parent directories): .git"
check "undiagnosed gh failure is refused" 1 "$(run_guard)"

# 4. Another undiagnosed failure shape: no credentials.
stub_gh 1 "gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN environment variable"
check "missing credentials is refused" 1 "$(run_guard)"

# 5. Usage errors.
set +e
GH="$WORK/bin/gh" "$GUARD" >/dev/null 2>&1
check "no argument is rejected" 1 "$?"
set -e

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
