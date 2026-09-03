#!/usr/bin/env bash

set -euo pipefail

readonly sentinel='__WATERFALL_BENCHMARK__'
readonly fixture_digest='c17499d65a3d9d75290dfb5e327aae2bb5fe5bc1bc82b74f40e0e6fa9d0d365b'
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

contains_literal() {
  local literal="$1"
  local path="$2"
  local status=0

  if [[ ! -e "${path}" ]]; then
    fail "artifact does not exist: ${path}"
  fi

  LC_ALL=C grep -aR -F -q -- "${literal}" "${path}" || status=$?
  case "${status}" in
    0) return 0 ;;
    1) return 1 ;;
    *) fail "could not scan ${path}" ;;
  esac
}

require_absent() {
  local literal="$1"
  local path="$2"
  if contains_literal "${literal}" "${path}"; then
    fail "${literal} found in ${path}"
  fi
}

require_present() {
  local literal="$1"
  local path="$2"
  if ! contains_literal "${literal}" "${path}"; then
    fail "${literal} missing from ${path}"
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

require_absent "${sentinel}" "${temporary_dir}/otel-desktop-viewer"
require_absent "${sentinel}" "${temporary_dir}/otel-desktop-viewer-tagged"
require_present "${sentinel}" "${temporary_dir}/waterfallbench"
require_absent "${fixture_digest}" "${temporary_dir}/otel-desktop-viewer"
require_absent "${fixture_digest}" "${temporary_dir}/otel-desktop-viewer-tagged"
require_present "${fixture_digest}" "${temporary_dir}/waterfallbench"

npm --prefix "${frontend}" run build -- --outDir "${temporary_dir}/frontend-production"
npm --prefix "${frontend}" run build:benchmark -- --outDir "${temporary_dir}/frontend-benchmark"

require_absent "${sentinel}" "${temporary_dir}/frontend-production"
require_absent "${sentinel}" "${repo_root}/desktopexporter/internal/server/static"
require_present "${sentinel}" "${temporary_dir}/frontend-benchmark"
require_absent "${fixture_digest}" "${temporary_dir}/frontend-production"
require_absent "${fixture_digest}" "${repo_root}/desktopexporter/internal/server/static"

if ! git -C "${repo_root}" check-ignore -q desktopexporter/internal/frontend/dist-benchmark/example; then
  fail 'dist-benchmark is not ignored'
fi

if [[ -n "$(git -C "${repo_root}" ls-files 'desktopexporter/internal/frontend/dist-benchmark/**')" ]]; then
  fail 'dist-benchmark contains tracked files'
fi

if [[ -e "${repo_root}/desktopexporter/internal/server/static/benchmark" ]]; then
  fail 'benchmark frontend exists under production embedded assets'
fi

printf 'Production artifacts exclude benchmark code and fixture inputs; benchmark artifacts contain both.\n'
