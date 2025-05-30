#!/bin/bash
#
# Presubmit checks for Trillian examples.
#
# Checks for lint errors, spelling, licensing, correct builds / tests and so on.
# Flags may be specified to allow suppressing of checks or automatic fixes, try
# `scripts/presubmit.sh --help` for details.
#
# Globals:
#   GO_TEST_TIMEOUT: timeout for 'go test'. Optional (defaults to 5m).
set -eu

check_pkg() {
  local cmd="$1"
  local pkg="$2"
  check_cmd "$cmd" "try running 'go get -u $pkg'"
}

check_cmd() {
  local cmd="$1"
  local msg="$2"
  if ! type -p "${cmd}" > /dev/null; then
    echo "${cmd} not found, ${msg}"
    return 1
  fi
}

usage() {
  echo "$0 [--coverage] [--fix] [--no-build] [--no-linters] [--no-generate] [--no-actions] [--no-docker] [--no-serverless-wasm] [--cloud-build]"
}

main() {
  local coverage=0
  local fix=0
  local flag_cloud_build=0
  local run_build=1
  local build_actions=1
  local build_docker=1
  local run_lint=1
  local run_generate=1
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --coverage)
        coverage=1
        ;;
      --fix)
        fix=1
        ;;
      --help)
        usage
        exit 0
        ;;
      --no-build)
        run_build=0
        ;;
      --no-linters)
        run_lint=0
        ;;
      --no-generate)
        run_generate=0
        ;;
      --no-actions)
        build_actions=0
        ;;
      --no-docker)
        build_docker=0
        ;;
      --cloud-build)
        flag_cloud_build=1
        ;;
      *)
        usage
        exit 1
        ;;
    esac
    shift 1
  done

  cd "$(dirname "$0")"  # at scripts/
  cd ..  # at top level

  go_srcs="$(find . -name '*.go' | \
    grep -v vendor/ | \
    grep -v registers/ | \
    grep -v mock_ | \
    grep -v .pb.go | \
    grep -v .pb.gw.go | \
    grep -v _string.go | \
    tr '\n' ' ')"

  if [[ "$fix" -eq 1 ]]; then
    check_pkg goimports golang.org/x/tools/cmd/goimports || exit 1

    echo 'running gofmt'
    gofmt -s -w ${go_srcs}
    echo 'running goimports'
    goimports -w ${go_srcs}
  fi

  if [[ "${flag_cloud_build}" -eq 1 ]]; then
      export GO_TEST_DOCKER_INTEGRATION="cloudbuild"
  fi

  if [[ "${run_build}" -eq 1 ]]; then
    echo 'running go build'
    go build ./...

    local coverflags=""
    if [[ ${coverage} -eq 1 ]]; then
        coverflags="-covermode=atomic -coverprofile=coverage.txt"
    fi

    echo "running go test"
    go test \
        -short \
        -timeout=${GO_TEST_TIMEOUT:-5m} \
        ${coverflags} \
        ./...
  fi

  if [[ "${build_docker}" -eq 1 ]]; then
    echo "Building non-serverless-action dockerfiles ===================="
    for i in $(find . -name Dockerfile ); do
      echo "Building ${i} ------------------------------------------------"
      docker build -f "${i}" .
    done
  fi

  if [[ "${run_lint}" -eq 1 ]]; then
    check_cmd golangci-lint \
      'have you installed github.com/golangci/golangci-lint?' || exit 1

    echo 'running golangci-lint'
    golangci-lint run --timeout=10m
    echo 'checking license headers'
    ./scripts/check_license.sh ${go_srcs}
  fi

  if [[ "${run_generate}" -eq 1 ]]; then
    check_cmd protoc 'have you installed protoc?'

    echo 'running go generate'
    go generate -run="protoc" ./...
  fi
}

main "$@"
