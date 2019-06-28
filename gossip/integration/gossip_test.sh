#!/bin/bash
set -e
. $(go list -f '{{.Dir}}' github.com/google/trillian)/integration/functions.sh
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/gossip_functions.sh

# Default to one of everything.
LOG_SERVER_COUNT=${1:-1}
LOG_SIGNER_COUNT=${2:-1}
HUB_SERVER_COUNT=${3:-1}

gossip_prep_test "${LOG_SERVER_COUNT}" "${LOG_SIGNER_COUNT}" "${HUB_SERVER_COUNT}"

# Cleanup for the Trillian components
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR} ${PROMETHEUS_CFGDIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})
TO_KILL+=(${PROMETHEUS_PID})
TO_KILL+=(${ETCDISCOVER_PID})

# Cleanup for the personality
TO_DELETE="${TO_DELETE} ${HUB_CFG} ${SRC_PRIV_KEYS}"
TO_KILL+=(${HUB_SERVER_PIDS[@]})

echo "Running test(s)"
pushd "${INTEGRATION_DIR}"
set +e
go test -v -run ".*LiveGossip.*" --timeout=5m ./ --hub_servers=${HUB_SERVERS} --hub_config=${HUB_CFG} --src_priv_keys=${SRC_PRIV_KEY_LIST}
RESULT=$?
set -e
popd

gossip_stop_test
TO_KILL=()

exit $RESULT
