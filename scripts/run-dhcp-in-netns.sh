#!/bin/bash
set -e

# Expects one argument: netns_bridge (e.g. vpc-00003_br-00002 or vpc1_br0)
# The netns is only needed here, to enter it. The server is handed its bridge
# and its two file paths, and knows nothing of the namespace it runs in.
arg="$1"
NETNS="${arg%%_*}"
BRIDGE="${arg#*_}"
RUN_DIR="/run/two/dhcp"

if [[ "${NETNS}" == "${arg}" || -z "${NETNS}" || -z "${BRIDGE}" ]]
then
    echo "instance ${arg} is not <netns>_<bridge>" >&2
    exit 1
fi

echo "start dhcp ${arg}"

exec ip netns exec "${NETNS}" \
  /opt/two/bin/dhcp \
    -conf /etc/two/agent.yml \
    -interface "${BRIDGE}" \
    -state "${RUN_DIR}/${arg}.state" \
    -socket "${RUN_DIR}/${arg}.sock"
