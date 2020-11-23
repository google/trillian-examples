#!/bin/bash
set -e
. $(go list -f '{{.Dir}}' github.com/google/trillian)/integration/functions.sh
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/ft_functions.sh

ft_prep_test

function cleanup {
banner "Cleaning up"
ft_stop_test
TO_KILL=()
}

trap cleanup EXIT

DEVICE_STATE=$(mktemp -d /tmp/dummy-XXXXX)
UPDATE_FILE=$(mktemp /tmp/update-XXXXX.json)


# Cleanup for the Trillian components
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})

# Cleanup for the personality
TO_DELETE="${TO_DELETE} ${FT_CAS_DB} ${DEVICE_STATE} ${UPDATE_FILE} ${FT_MONITOR_LOG}"
TO_KILL+=(${FT_SERVER_PID})
TO_KILL+=(${FT_MONITOR_PID})

echo "Running test(s)"
pushd "${INTEGRATION_DIR}"

banner "Logging initial firmware"
go run ../cmd/publisher/ --binary_path ../testdata/firmware/dummy_device/example.wasm  --revision 1 --logtostderr -v 2 --output_path=${UPDATE_FILE}

banner "Force flashing device (init)"
go run ../cmd/flash_tool/ --logtostderr --update_file=${UPDATE_FILE} --dummy_storage_dir=${DEVICE_STATE} --force --v 2

banner "Booting device with initial firmware"
go run ../cmd/emulator/dummy/ --logtostderr --dummy_storage_dir=${DEVICE_STATE}

banner "Logging update firmware"
go run ../cmd/publisher/ --binary_path ../testdata/firmware/dummy_device/example.wasm  --revision 2 --logtostderr -v 2 --output_path=${UPDATE_FILE}

banner "Force flashing device (update)"
go run ../cmd/flash_tool/ --logtostderr --update_file=${UPDATE_FILE} --dummy_storage_dir=${DEVICE_STATE} --v 2

banner "Booting updated device"
go run ../cmd/emulator/dummy/ --logtostderr --dummy_storage_dir=${DEVICE_STATE}

# Give the monitor a chance to see some things
sleep 10

banner "Monitor saw (${FT_MONITOR_LOG})"
cat ${FT_MONITOR_LOG}

banner "DONE"

popd

exit $RESULT

