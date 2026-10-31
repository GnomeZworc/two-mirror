#!/bin/sh
set -eu
PUBLIC_VNI=1
GIT_SERVER=https://git.g3e.fr/
REPO=syonad/two
ASSETS="${LAB_FILES}/bin"
if [ -f "${ASSETS}/SHA256SUMS" ] && [ -f "${ASSETS}/VERSION" ] && [ -f "${LAB_FILES}/scripts/deploy.sh" ] && [ -f "${LAB_FILES}/scripts/bootstrap_kvm.sh" ]; then
    MODE=tree
    VERSION=$(cat "${ASSETS}/VERSION")
elif [ -f "${ASSETS}/RELEASE" ]; then
    MODE=release
    VERSION=$(cat "${ASSETS}/RELEASE")
else
    echo "lab: ni arbre courant (SHA256SUMS, VERSION, scripts) ni release (RELEASE) dans ${LAB_FILES} : lancer test/e2e/build.sh" >&2
    exit 1
fi
[ -n "${VERSION}" ] || { echo "lab: version vide (${MODE})" >&2; exit 1; }
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
case "${MODE}" in
    tree)
        install -m 0755 "${LAB_FILES}/scripts/deploy.sh" "${LAB_FILES}/scripts/bootstrap_kvm.sh" .
        echo "lab: arbre courant ${VERSION}, déployé depuis ${ASSETS} sans téléchargement"
        bash ./deploy.sh --noup_script -i -u "${LAB_UPLINK}" -N "${PUBLIC_VNI}" -a "${ASSETS}" -t "${VERSION}"
        ;;
    release)
        for script in deploy.sh bootstrap_kvm.sh; do
            curl -fsS -o "${script}" "${GIT_SERVER}${REPO}/raw/tag/${VERSION}/scripts/${script}"
        done
        chmod 0755 deploy.sh bootstrap_kvm.sh
        echo "lab: release ${VERSION}, déployée telle que publiée"
        bash ./deploy.sh --noup_script -i -u "${LAB_UPLINK}" -t "${VERSION}"
        ;;
esac
