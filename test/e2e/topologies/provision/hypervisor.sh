#!/bin/sh
set -eu
RELEASE=0.2.0
GIT_SERVER=https://git.g3e.fr/
REPO=syonad/two
n=0
until curl -fsS -o /dev/null "${GIT_SERVER}"; do
    n=$((n + 1))
    [ "$n" -lt 30 ] || exit 1
    sleep 10
done
install -D -m 0640 "${LAB_FILES}/agent.yml" /etc/two/agent.yml
if [ -e /sys/class/net/lan ]; then
    addr=$(ip -4 -o addr show dev lan | awk '{print $4}' | head -n 1)
    ip link show br-lan >/dev/null 2>&1 || ip link add name br-lan type bridge stp_state 0
    ip link set dev br-lan mtu "$(cat /sys/class/net/lan/mtu)"
    ip link set dev lan master br-lan
    ip link set dev br-lan up
    if [ -n "${addr}" ]; then
        ip addr del "${addr}" dev lan
        ip addr replace "${addr}" dev br-lan
    fi
fi
mkdir -p /opt/two/scripts
cd /opt/two/scripts
for script in deploy.sh bootstrap_kvm.sh; do
    curl -fsS -o "${script}" "${GIT_SERVER}${REPO}/raw/tag/${RELEASE}/scripts/${script}"
done
chmod 0755 deploy.sh bootstrap_kvm.sh
bash ./deploy.sh --noup_script -i -u "${LAB_UPLINK}" -t "${RELEASE}"
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends nftables
for binary in "${LAB_FILES}"/bin/*; do
    [ -f "${binary}" ] || continue
    name=$(basename "${binary}")
    install -D -m 0755 "${binary}" "/opt/two/local/bin/${name}"
    ln -sfn "/opt/two/local/bin/${name}" "/opt/two/bin/${name}"
    echo "lab: ${name} compilé en local remplace celui de la release ${RELEASE}"
done
