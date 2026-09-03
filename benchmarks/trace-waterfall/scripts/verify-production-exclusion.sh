#!/usr/bin/env bash

set -euo pipefail

readonly sentinel='__WATERFALL_BENCHMARK__'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly script_dir
repo_root="$(cd "${script_dir}/../../.." && pwd)"
readonly repo_root
readonly frontend="${repo_root}/desktopexporter/internal/frontend"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/waterfallbench.XXXXXX")"
readonly temporary_dir

cleanup() {
  rm -rf "${temporary_dir}"
}
trap cleanup EXIT

fail() {
  printf 'production exclusion check failed: %s\n' "$1" >&2
  exit 1
}

contains_sentinel() {
  local path="$1"
  local status=0

  if [[ ! -e "${path}" ]]; then
    fail "artifact does not exist: ${path}"
  fi

  LC_ALL=C grep -aR -F -q -- "${sentinel}" "${path}" || status=$?
  case "${status}" in
    0) return 0 ;;
    1) return 1 ;;
    *) fail "could not scan ${path}" ;;
  esac
}

require_absent() {
  if contains_sentinel "$1"; then
    fail "sentinel found in $1"
  fi
}

require_present() {
  if ! contains_sentinel "$1"; then
    fail "positive-control sentinel missing from $1"
  fi
}

normal_packages="$(cd "${repo_root}" && go list ./...)"
if [[ "${normal_packages}" == *'/desktopexporter/internal/cmd/waterfallbench'* ]]; then
  fail 'tagged Go command appears in the normal package graph'
fi

tagged_packages="$(cd "${repo_root}" && go list -tags=waterfallbench ./...)"
if [[ "${tagged_packages}" != *'/desktopexporter/internal/cmd/waterfallbench'* ]]; then
  fail 'tagged Go command is absent from the benchmark package graph'
fi

(
  cd "${repo_root}"
  go build -trimpath -ldflags='-s -w' -o "${temporary_dir}/otel-desktop-viewer" .
  go build -trimpath -tags=waterfallbench -ldflags='-s -w' -o "${temporary_dir}/otel-desktop-viewer-tagged" .
  go build -trimpath -tags=waterfallbench -ldflags='-s -w' -o "${temporary_dir}/waterfallbench" ./desktopexporter/internal/cmd/waterfallbench
)

require_absent "${temporary_dir}/otel-desktop-viewer"
require_absent "${temporary_dir}/otel-desktop-viewer-tagged"
require_present "${temporary_dir}/waterfallbench"

npm --prefix "${frontend}" run build -- --outDir "${temporary_dir}/frontend-production"
npm --prefix "${frontend}" run build:benchmark -- --outDir "${temporary_dir}/frontend-benchmark"

require_absent "${temporary_dir}/frontend-production"
require_absent "${repo_root}/desktopexporter/internal/server/static"
require_present "${temporary_dir}/frontend-benchmark"

if ! git -C "${repo_root}" check-ignore -q desktopexporter/internal/frontend/dist-benchmark/example; then
  fail 'dist-benchmark is not ignored'
fi

if [[ -n "$(git -C "${repo_root}" ls-files 'desktopexporter/internal/frontend/dist-benchmark/**')" ]]; then
  fail 'dist-benchmark contains tracked files'
fi

if [[ -e "${repo_root}/desktopexporter/internal/server/static/benchmark" ]]; then
  fail 'benchmark frontend exists under production embedded assets'
fi

printf 'Production artifacts exclude %s; benchmark artifacts contain it.\n' "${sentinel}"
