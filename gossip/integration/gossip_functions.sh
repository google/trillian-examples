# Functions for setting up Gossip personalities for integration tests
# Requires github.com/google/trillian/integration/functions.sh

declare -a HUB_SERVER_PIDS
HUB_SERVERS=
HUB_CFG=
PROMETHEUS_CFGDIR=

# gossip_prep_test prepares a set of running processes for a Gossip test.
# Parameters:
#   - number of log servers to run
#   - number of log signers to run
#   - number of Gossip Hub personality instances to run
#   - Gossip Hub template config file (optional: if not present, a new config is created)
# Populates:
#  - HUB_SERVERS         : list of HTTP addresses (comma separated)
#  - HUB_SERVER_1        : first HTTP address
#  - HUB_METRICS_SERVERS : list of HTTP addresses (comma separated) serving metrics
#  - HUB_SERVER_PIDS     : bash array of Gossip Hub server pids
#  - HUB_CFG             : Hub config file
#  - SRC_PRIV_KEYS       : list of source private key files (space separated), if new config created
#  - SRC_PRIV_KEY_LIST   : list of source private key files (comma separated), if new config created
# in addition to the variables populated by Trillian's log_prep_test.
# If etcd and Prometheus are configured, it also populates:
#  - ETCDISCOVER_PID   : pid of etcd service watcher
#  - PROMETHEUS_PID    : pid of local Prometheus server
#  - PROMETHEUS_CFGDIR : Prometheus configuration directory
gossip_prep_test() {
  # Default to one of everything.
  local log_server_count=${1:-1}
  local log_signer_count=${2:-1}
  local hub_server_count=${3:-1}
  local hub_cfg_template=${4}

  echo "Launching core Trillian log components"
  log_prep_test "${log_server_count}" "${log_signer_count}"

  echo "Building Hub personality code"
  go build ${GOFLAGS} github.com/google/trillian-examples/gossip/hub/hub_server

  echo "Provisioning logs for Gossip Hub"
  gossip_provision "${RPC_SERVER_1}" "${hub_cfg_template}"

  echo "Launching Gossip Hub personalities"
  for ((i=0; i < hub_server_count; i++)); do
    local port=$(pick_unused_port)
    HUB_SERVERS="${HUB_SERVERS},localhost:${port}"
    local metrics_port=$(pick_unused_port ${port})
    HUB_METRICS_SERVERS="${HUB_METRICS_SERVERS},localhost:${metrics_port}"
    if [[ $i -eq 0 ]]; then
      HUB_SERVER_1="localhost:${port}"
    fi

    echo "Starting Gossip Hub server on localhost:${port}, metrics on localhost:${metrics_port}"
    ./hub_server ${ETCD_OPTS} --hub_config="${HUB_CFG}" --log_rpc_server="${RPC_SERVERS}" --http_endpoint="localhost:${port}" --metrics_endpoint="localhost:${metrics_port}" &
    pid=$!
    HUB_SERVER_PIDS+=(${pid})
    wait_for_server_startup ${port}
  done
  HUB_SERVERS="${HUB_SERVERS:1}"
  HUB_METRICS_SERVERS="${HUB_METRICS_SERVERS:1}"

  if [[ ! -z "${ETCD_OPTS}" ]]; then
    echo "Registered HTTP endpoints"
    ETCDCTL_API=3 etcdctl get gossip-hub-http/ --prefix
    ETCDCTL_API=3 etcdctl get gossip-hub-metrics-http/ --prefix
  fi

  if [[ -x "${PROMETHEUS_DIR}/prometheus" ]]; then
    if [[ ! -z "${ETCD_OPTS}" ]]; then
        PROMETHEUS_CFGDIR="$(mktemp -d ${TMPDIR}/gossip-prometheus-XXXXXX)"
        local prom_cfg="${PROMETHEUS_CFGDIR}/config.yaml"
        local etcdiscovered="${PROMETHEUS_CFGDIR}/trillian.json"
        sed "s!@ETCDISCOVERED@!${etcdiscovered}!" ${GOPATH}/src/github.com/google/trillian-examples/gossip/integration/prometheus.yml > "${prom_cfg}"
        echo "Prometheus configuration in ${prom_cfg}:"
        cat ${prom_cfg}

        echo "Building etcdiscover"
        go build github.com/google/trillian/monitoring/prometheus/etcdiscover

        echo "Launching etcd service monitor updating ${etcdiscovered}"
        ./etcdiscover ${ETCD_OPTS} --etcd_services=gossip-hub-metrics-http,trillian-logserver-http,trillian-logsigner-http -target=${etcdiscovered} --logtostderr &
        ETCDISCOVER_PID=$!
        echo "Launching Prometheus (default location localhost:9090)"
        ${PROMETHEUS_DIR}/prometheus --config.file=${prom_cfg} &
        PROMETHEUS_PID=$!
    fi
  fi
}

# gossip_provision generates a Gossip Hub configuration file and provisions the trees for it.
# Parameters:
#   - location of admin server instance
#   - Gossip Hub template config file (optional: if not present, a new config is created)
# Populates:
#   - HUB_CFG : configuration file for Gossip test
#  - SRC_PRIV_KEYS       : list of source log private key files (space separated), if new config created
#  - SRC_PRIV_KEY_LIST   : list of source log private key files (comma separated), if new config created
gossip_provision() {
  local admin_server="$1"
  local hub_cfg_template="$2"

  HUB_CFG=$(mktemp ${TMPDIR}/gossip-XXXXXX)

  if [[ "${hub_cfg_template}" == "" ]]; then
    echo "Creating Gossip Hub config from scratch"
    go build ${GOFLAGS} github.com/google/trillian-examples/gossip/integration/make_hub_cfg
    ./make_hub_cfg --hubs=2 --logs=3 --hub_config=${HUB_CFG} --log_priv_key_prefix="${TMPDIR}/src-log-priv-key"
    SRC_PRIV_KEYS="${TMPDIR}/src-log-priv-key-00.pem ${TMPDIR}/src-log-priv-key-01.pem ${TMPDIR}/src-log-priv-key-02.pem"
    SRC_PRIV_KEY_LIST="${TMPDIR}/src-log-priv-key-00.pem,${TMPDIR}/src-log-priv-key-01.pem,${TMPDIR}/src-log-priv-key-02.pem"
  else
    cp ${hub_cfg_template} ${HUB_CFG}
  fi

  echo 'Building createtree'
  go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/

  echo 'Provisioning Gossip Logs'
  gossip_provision_cfg ${admin_server} ${HUB_CFG}

  echo "Gossip Hub Configuration in ${HUB_CFG}:"
  cat "${HUB_CFG}"
  echo
}

# gossip_provision_cfg provisions trees for the logs in a specified config file.
# Parameters:
#   - location of admin server instance
#   - the config file to be provisioned for
gossip_provision_cfg() {
  local admin_server="$1"
  local cfg="$2"

  num_logs=$(grep -c '999999' ${cfg})
  for i in $(seq ${num_logs}); do
    tree_id=$(./createtree \
      --admin_server="${admin_server}" \
      --private_key_format=PrivateKey \
      --pem_key_path=${GOPATH}/src/github.com/google/certificate-transparency-go/trillian/testdata/log-rpc-server.privkey.pem \
      --pem_key_password=towel \
      --signature_algorithm=ECDSA)
    echo "Created tree ${tree_id}"
    # Need suffix for sed -i to cope with both GNU and non-GNU (e.g. OS X) sed.
    sed -i'.bak' "1,/999999/s/999999/${tree_id}/" "${cfg}"
    rm -f "${cfg}.bak"
  done
}


# gossip_stop_test closes the running processes for a Gossip test.
# Assumes the following variables are set, in addition to those needed by logStopTest:
#  - HUB_SERVER_PIDS  : bash array of Gossip Hub server pids
gossip_stop_test() {
  local pids
  if [[ "${PROMETHEUS_PID}" != "" ]]; then
    pids+=" ${PROMETHEUS_PID}"
  fi
  if [[ "${ETCDISCOVER_PID}" != "" ]]; then
    pids+=" ${ETCDISCOVER_PID}"
  fi
  echo "Stopping Gossip Hub server (pids ${HUB_SERVER_PIDS[@]})"
  pids+=" ${HUB_SERVER_PIDS[@]}"
  kill_pid ${pids}
  log_stop_test
}
