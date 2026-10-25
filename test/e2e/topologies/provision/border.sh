#!/bin/sh
set -eu
VNI=66001
GATEWAY=240.0.0.1/24
GATEWAY_MAC=02:01:00:01:01:d1
DESTINATION=240.255.0.1/32
VTEP=$(ip -4 -o addr show dev "${LAB_UPLINK}" scope global | awk 'NR == 1 {sub("/.*", "", $4); print $4}')
[ -n "${VTEP}" ] || { echo "border: no IPv4 on ${LAB_UPLINK}" >&2; exit 1; }
ip link add br-public type bridge
ip link set br-public address "${GATEWAY_MAC}"
ip link add "vxlan-${VNI}" type vxlan id "${VNI}" local "${VTEP}" dstport 4789 nolearning
ip link set "vxlan-${VNI}" mtu 1500 master br-public
ip link set "vxlan-${VNI}" up
ip link set br-public up
ip addr add "${GATEWAY}" dev br-public
ip link add internet type dummy
ip addr add "${DESTINATION}" dev internet
ip link set internet up
