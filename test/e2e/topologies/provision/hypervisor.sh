#!/bin/sh
set -eu
RELEASE=0.2.0rc008
GIT_SERVER=https://git.g3e.fr/
REPO=syonad/two
n=0
until curl -fsS -o /dev/null "${GIT_SERVER}"; do
    n=$((n + 1))
    [ "$n" -lt 30 ] || exit 1
    sleep 10
done
install -D -m 0640 "${LAB_FILES}/agent.yml" /etc/two/agent.yml
mkdir -p /opt/two/scripts
cd /opt/two/scripts
for script in deploy.sh bootstrap_kvm.sh; do
    curl -fsS -o "${script}" "${GIT_SERVER}${REPO}/raw/tag/${RELEASE}/scripts/${script}"
done
chmod 0755 deploy.sh bootstrap_kvm.sh
bash ./deploy.sh --noup_script -i -u "${LAB_UPLINK}" -t "${RELEASE}"
for binary in "${LAB_FILES}"/bin/*; do
    [ -f "${binary}" ] || continue
    name=$(basename "${binary}")
    install -D -m 0755 "${binary}" "/opt/two/local/bin/${name}"
    ln -sfn "/opt/two/local/bin/${name}" "/opt/two/bin/${name}"
    echo "lab: ${name} compilé en local remplace celui de la release ${RELEASE}"
done
