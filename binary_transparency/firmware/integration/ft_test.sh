#!/bin/bash
set -e
set -x
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"

TESTFLAGS="-v --logtostderr"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --coverage)
    TESTFLAGS+=" -covermode=atomic -coverprofile=coverage.txt -coverpkg ../..."
      ;;

    *)
      usage
      exit 1
      ;;
  esac
  shift 1
done

# Trillian must already be running
[ -z ${TRILLIAN_LOG_RPC+x} ] && TRILLIAN_LOG_RPC="localhost:8090"

cd ${INTEGRATION_DIR}
go test . --trillian=${TRILLIAN_LOG_RPC} ${TESTFLAGS}
