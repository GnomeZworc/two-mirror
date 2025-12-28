#!/bin/bash
set -e

# Expects one argument: netns_bridge (e.g. vpc-00003_br-00002 or vpc1_br0)
arg="$1"
NETNS="${arg%%_*}"
ip_port="${arg#*_}"
IP="${ip_port%%-*}"
PORT="${ip_port#*-}"

echo "start metadata ${NETNS} "

exec ip netns exec "${NETNS}" \
  /usr/bin/metadata \
    -file "/opt/metadata/${arg}.conf" \
    -interface "${IP}" \
    -port "${PORT}"