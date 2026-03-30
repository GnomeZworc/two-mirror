#!/bin/bash
set -e

# Expects one argument: netns_bridge (e.g. vpc-00003_br-00002 or vpc1_br0)
arg="$1"
NETNS="${arg%%_*}"
BRIDGE="${arg#*_}"

echo "start dnsmasq ${NETNS} ${BRIDGE}"

exec ip netns exec "${NETNS}" \
  dnsmasq \
    --no-daemon \
    --interface="${BRIDGE}" \
    --bind-interfaces \
    --pid-file="/run/dnsmasq-$arg.pid" \
    --conf-file="/etc/dnsmasq.d/$arg.conf" \
    --no-hosts \
    --no-resolv \
    --log-facility="/var/log/dnsmasq-$arg.log" \
    --no-daemon -p0
