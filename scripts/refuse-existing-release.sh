#!/usr/bin/env bash
# Refuse to publish over a tag that already has a release, including a draft.
#
# Three outcomes, not two. The original inline guard discarded stderr and
# tested only gh's exit status, which made a FAILED query indistinguishable
# from "no release exists" — so it reported success while checking nothing
# whenever gh could not resolve the repository (MADR 0007 F1). An undiagnosed
# failure must refuse, never proceed.
#
# Usage: refuse-existing-release.sh <tag>
# Exit:  0 the tag has no release and publication may proceed
#        1 the release exists, or its existence could not be determined
set -euo pipefail

# GH is the tool this guard queries with. It is an explicit seam rather than a
# bare `gh` on PATH so the tests can substitute a stub deterministically: a
# PATH-prefix stub is defeated by any shell startup file that re-orders PATH,
# which silently made an earlier version of these tests exercise the real gh.
GH="${GH:-gh}"

if [ "$#" -ne 1 ] || [ -z "${1:-}" ]; then
	echo "usage: refuse-existing-release.sh <tag>" >&2
	exit 1
fi
tag="$1"

if out=$("$GH" release view "$tag" 2>&1); then
	echo "refuse-existing-release: release $tag already exists (including drafts)" >&2
	exit 1
fi

case "$out" in
*"release not found"* | *"Not Found"* | *"no release found"*)
	exit 0
	;;
*)
	echo "refuse-existing-release: could not determine whether $tag already exists" >&2
	echo "$out" >&2
	exit 1
	;;
esac
