# Functions for setting up FT personalities for integration tests
# Requires github.com/google/trillian/integration/functions.sh

declare -a FT_SERVER_PIDS
FT_SERVERS=

# ft_prep_test prepares a set of running processes for a FT test.
# Parameters:
#  none
# Populates:
#  - FT_SERVER         : personality HTTP address
#  - FT_SERVER_PID     : FT personality pid
#  - FT_MONITOR_PID    : FT monitor pid
#  - FT_MONITOR_LOG    : FT monitor log file
#  - FT_CAS_DB         : FT CAS datbase file
# in addition to the variables populated by Trillian's log_prep_test.
ft_prep_test() {
  echo "Launching core Trillian log components"
  log_prep_test

  echo "building personality code"
  go build ${goflags} github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality

  echo "Provisioning logs for FT "
  ft_provision "${RPC_SERVER_1}"

  echo "Launching FT personality"
  local port=$(pick_unused_port)
  FT_SERVER="localhost:${port}"

  echo "Starting FT server on localhost:${port}, metrics on localhost:${metrics_port}"
  ./ft_personality  --trillian="${RPC_SERVERS}" --listen="localhost:${port}" --tree_id=${tree_id} --cas_db_file=${ft_cas} &
  pid=$!
  FT_SERVER_PID=${pid}
  FT_CAS_DB=${ft_cas}
  wait_for_server_startup ${port}

  echo "building personality code"
  go build ${goflags} github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_monitor

  echo "Starting FT monitor"
  FT_MONITOR_LOG=$(mktemp /tmp/ft-monitor-log-XXXXX)
  ./ft_monitor  --ftlog="http://localhost:${port}" --poll_interval=200ms -v 1 --logtostderr > ${FT_MONITOR_LOG} 2>&1 &
  pid=$!
  FT_MONITOR_PID=${pid}
}

# fr_provision provisions everything an FT personality needs to run.
ft_provision() {
  local admin_server="$1"
  local ft_cfg_template="$2"

  echo 'Building createtree'
  go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/

  echo 'Provisioning FT Logs'
  ft_provision_tree ${admin_server} ${FT_CFG}

  ft_cas=$(mktemp ${TMPDIR}/ft-cas-XXXXXX)
}

# ft_provision_tree provisions trees for the logs in a specified config file.
# Parameters:
#   - location of admin server instance
#   - the config file to be provisioned for
ft_provision_tree() {
  local admin_server="$1"

  tree_id=$(./createtree \
    --admin_server="${admin_server}" \
    --private_key_format=PrivateKey \
    --pem_key_path=$(go list -f '{{.Dir}}' github.com/google/certificate-transparency-go)/trillian/testdata/log-rpc-server.privkey.pem \
    --pem_key_password=towel \
    --signature_algorithm=ECDSA)
  echo "Created tree ${tree_id}"
}


# ft_stop_test closes the running processes for a FT test.
# Assumes the following variables are set, in addition to those needed by logStopTest:
#  - FT_SERVER_PIDS  : bash array of FT server pids
ft_stop_test() {
  local pids
  echo "Stopping FT server (pids ${FT_SERVER_PID})"
  pids+=" ${FT_SERVER_PID}"
  kill_pid ${pids}
  log_stop_test
}

banner() {
  echo "---[$1]----------------------------------------------"
}
