#!/usr/bin/env bash
# Tag desktopexporter/VERSION and VERSION on the same commit so go install and
# GoReleaser both work. Run when main is ready to ship.
set -euo pipefail

version="${1:-${VERSION:-}}"
if [[ -z "$version" ]]; then
	echo "Usage: release-go-modules.sh v0.3.0" >&2
	echo "   or: make release-go-modules VERSION=v0.3.0" >&2
	exit 1
fi

if [[ "$version" != v* ]]; then
	version="v${version}"
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
	echo "VERSION must look like v0.3.0 or v0.3.0-alpha.1 (got: ${version})" >&2
	exit 1
fi

root_tag="$version"
sub_tag="desktopexporter/${version}"
module="github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"

if [[ -n "$(git status --porcelain)" ]]; then
	echo "Commit or stash changes before releasing." >&2
	exit 1
fi

if git rev-parse "$sub_tag" >/dev/null 2>&1; then
	echo "Tag ${sub_tag} already exists." >&2
	exit 1
fi
if git rev-parse "$root_tag" >/dev/null 2>&1; then
	echo "Tag ${root_tag} already exists." >&2
	exit 1
fi

echo "→ tag ${sub_tag} (temporary, for go get)"
git tag "$sub_tag"

echo "→ bump root require to ${module}@${version}"
export GOPROXY=direct
go get "${module}@${version}"
go work sync
go mod tidy
( cd desktopexporter && go mod tidy )

if [[ -n "$(git status --porcelain)" ]]; then
	git add go.mod go.sum go.work go.work.sum desktopexporter/go.mod desktopexporter/go.sum 2>/dev/null || true
	git add go.mod go.sum
	git commit -m "chore: bump desktopexporter to ${version}"
	echo "→ move ${sub_tag} to bump commit"
	git tag -f "$sub_tag"
fi

echo "→ tag ${root_tag}"
git tag "$root_tag"

sha="$(git rev-parse --short HEAD)"
echo ""
echo "Done. Both tags point at ${sha}:"
echo "  ${sub_tag}"
echo "  ${root_tag}"
echo ""
echo "Push to publish:"
echo "  git push origin ${sub_tag} ${root_tag}"
