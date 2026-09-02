#!/usr/bin/env bash
# Assert every step of the reusable release workflow that calls gh against a
# repository also sets GH_REPO.
#
# The job never checks the calling repository out at top level, so gh has no
# local repository to infer from. A step that omits GH_REPO either fails
# outright or, worse, degrades silently — that is how a guard shipped which
# reported success while checking nothing (MADR 0007).
#
# --help probes are exempt: they resolve no repository.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOW="${1:-$ROOT/.github/workflows/publish-selfupdate-release.yml}"

[ -f "$WORKFLOW" ] || {
	echo "check-workflow-gh-repo: no such workflow: $WORKFLOW" >&2
	exit 1
}

missing=$(awk '
	/^      - name:/ { step = $0; has_repo = 0; next }   # a step NAME can contain
	                                                     # "gh release"; never
	                                                     # treat it as a command
	/GH_REPO:/       { has_repo = 1 }
	/^ *#/           { next }                            # comments are not calls
	/gh (release|api) / {
		if ($0 ~ /--help/) next
		if (!has_repo) printf "  %s\n      %s\n", step, $0
	}
' "$WORKFLOW")

if [ -n "$missing" ]; then
	echo "check-workflow-gh-repo: gh steps missing GH_REPO:" >&2
	echo "$missing" >&2
	exit 1
fi

echo "check-workflow-gh-repo: ok — every repository-scoped gh step sets GH_REPO"
