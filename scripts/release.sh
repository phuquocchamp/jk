#!/bin/sh
# Create the next release tag locally. It never pushes: it prints the push
# command, because pushing a v* tag publishes a release.
#
#   scripts/release.sh patch|minor|major [--pre rc] [--dry-run]
#
# Guards: on the release branch (main), clean tree, HEAD == origin/main after
# fetching, and the tag does not exist locally or on origin. The version comes
# from the latest vX.Y.Z tag (v0.0.0 when there is none).
set -eu

branch=${RELEASE_BRANCH:-main}
remote=${RELEASE_REMOTE:-origin}
bump=""
pre=""
dry=0

usage() {
	echo "usage: $0 patch|minor|major [--pre LABEL] [--dry-run]" >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case $1 in
	patch | minor | major) bump=$1 ;;
	--pre)
		[ $# -ge 2 ] || usage
		pre=$2
		shift
		;;
	--dry-run) dry=1 ;;
	-h | --help) usage ;;
	*) usage ;;
	esac
	shift
done
[ -n "$bump" ] || usage
case $pre in
"" | [a-z]*) ;;
*)
	echo "release: --pre must start with a lowercase letter (e.g. rc)" >&2
	exit 2
	;;
esac

fail() {
	echo "release: $*" >&2
	exit 1
}

current=$(git rev-parse --abbrev-ref HEAD)
[ "$current" = "$branch" ] || fail "on branch $current; releases are tagged on $branch"
[ -z "$(git status --porcelain)" ] || fail "the working tree is not clean"

git fetch --quiet --tags "$remote" "$branch"
[ "$(git rev-parse HEAD)" = "$(git rev-parse "$remote/$branch")" ] ||
	fail "HEAD is not $remote/$branch; pull or push first"

# Latest stable tag vMAJOR.MINOR.PATCH.
last=$(git tag -l 'v[0-9]*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sed 's/^v//' |
	sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1 || true)
if [ -n "$last" ]; then last="v$last"; else last=v0.0.0; fi
ver=${last#v}
major=${ver%%.*}
rest=${ver#*.}
minor=${rest%%.*}
patch=${rest#*.}

case $bump in
major) next="v$((major + 1)).0.0" ;;
minor) next="v$major.$((minor + 1)).0" ;;
patch) next="v$major.$minor.$((patch + 1))" ;;
esac

if [ -n "$pre" ]; then
	n=1
	while git rev-parse -q --verify "refs/tags/$next-$pre.$n" >/dev/null; do
		n=$((n + 1))
	done
	next="$next-$pre.$n"
fi

git rev-parse -q --verify "refs/tags/$next" >/dev/null && fail "tag $next already exists"
if git ls-remote --exit-code --tags "$remote" "refs/tags/$next" >/dev/null 2>&1; then
	fail "tag $next already exists on $remote"
fi

echo "last tag: $last"
echo "next tag: $next ($bump${pre:+, pre-release $pre}) on $(git rev-parse --short HEAD)"
if [ "$dry" -eq 1 ]; then
	echo "dry run: no tag created"
	exit 0
fi
git tag -a "$next" -m "jk $next"
echo "created $next. Publish it with:"
echo "  git push $remote $next"
