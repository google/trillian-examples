#!/bin/bash
# Kill all gossip related processes.
killall $@ hub_server
killall $@ gosmin
killall $@ goshawk
if [[ -x "${ETCD_DIR}/etcd" ]]; then
  killall $@ etcd
  if [[ -x "${PROMETHEUS_DIR}/prometheus" ]]; then
    killall $@ etcdiscover
    killall $@ prometheus
  fi
fi
