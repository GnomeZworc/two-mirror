set -u
API=http://127.0.0.1:8080
IMAGE_URL=https://cloud.debian.org/images/cloud/bookworm/latest
IMAGE_FILE=debian-12-genericcloud-amd64.qcow2
TWO_IMAGE=/var/tmp/deb-two.qcow2
VM_KEY=/root/.ssh/lab-vm
VOLUMES=/var/lib/two/volumes

check () {
    local desc="${1}" out
    shift
    if out=$("$@" 2>&1); then
        echo "RÉUSSI: ${desc}"
    else
        echo "ÉCHOUÉ: ${desc}${out:+ — $(printf '%s' "${out}" | tail -n 1)}"
    fi
}

info () { echo "INFO: ${1}"; }

api_post () {
    curl -fsS -o /dev/null -X POST -H 'Content-Type: application/json' -d "${2}" "${API}/${1}"
}

api_delete () {
    curl -fsS -o /dev/null -X DELETE "${API}/${1}"
}

api_state () {
    curl -fsS "${API}/${1}" 2>/dev/null | jq -r .state
}

wait_state () {
    local path="${1}" want="${2}" i s
    for i in $(seq 1 90); do
        s=$(api_state "${path}")
        [ "${s}" = "${want}" ] && return 0
        [ "${want}" = deleted ] && [ -z "${s}" ] && return 0
        [ "${s}" = error ] && { echo "${path}: error" >&2; return 1; }
        sleep 2
    done
    echo "${path}: ${s:-absent}, ${want} attendu" >&2
    return 1
}

vpc_create () {
    api_state "vpcs/${1}" | grep -qx running && return 0
    api_post vpcs "{\"name\":\"${1}\",\"cidr\":\"${2}\"}" && wait_state "vpcs/${1}" running
}

subnet_create () {
    local name="${1}" vpc="${2}" vni="${3}" gw="${4}" cidr="${5}" extra="${6:-}"
    api_state "subnets/${name}" | grep -qx running && return 0
    api_post subnets "{\"name\":\"${name}\",\"vpc\":\"${vpc}\",\"mode\":\"vxlan\",\"vxlan_id\":${vni},\"iface_type\":\"vms\",\"interface_ip\":\"${gw}\",\"cidr\":\"${cidr}\"${extra}}" \
        && wait_state "subnets/${name}" running
}

two_image () {
    [ -f "${TWO_IMAGE}" ] && return 0
    local work
    work=$(mktemp -d)
    curl -fsSL -o "${work}/${IMAGE_FILE}" "${IMAGE_URL}/${IMAGE_FILE}" || return 1
    curl -fsSL "${IMAGE_URL}/SHA512SUMS" | grep " ${IMAGE_FILE}\$" > "${work}/sums" || return 1
    (cd "${work}" && sha512sum -c sums >/dev/null) || { echo "image: somme SHA-512 incorrecte" >&2; return 1; }
    command -v qemu-nbd >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends qemu-utils >/dev/null
    modprobe nbd max_part=8
    qemu-nbd -c /dev/nbd0 "${work}/${IMAGE_FILE}" || return 1
    for i in $(seq 1 20); do [ -b /dev/nbd0p1 ] && break; sleep 0.5; done
    mkdir -p /mnt/two-image
    mount /dev/nbd0p1 /mnt/two-image || { qemu-nbd -d /dev/nbd0 >/dev/null; return 1; }
    printf '%s\n' 'datasource_list: [ NoCloud ]' 'datasource:' '  NoCloud:' "    seedfrom: 'http://169.254.169.254:80/'" '    timeout: 5' '    max_wait: 10' \
        > /mnt/two-image/etc/cloud/cloud.cfg.d/99_metadata.cfg
    umount /mnt/two-image
    qemu-nbd -d /dev/nbd0 >/dev/null
    mv "${work}/${IMAGE_FILE}" "${TWO_IMAGE}"
    rm -rf "${work}"
}

vm_key () {
    [ -f "${VM_KEY}" ] || ssh-keygen -q -t ed25519 -N '' -C lab-vm -f "${VM_KEY}"
    cat "${VM_KEY}.pub"
}

vm_create () {
    local name="${1}" subnet="${2}" ip="${3}" key="${4:-}" meta="{}"
    [ -n "${key}" ] && meta="{\"sshkey\":\"${key}\"}"
    mkdir -p "${VOLUMES}"
    cp "${TWO_IMAGE}" "${VOLUMES}/${name}.qcow2" || return 1
    api_post vms "{\"name\":\"${name}\",\"memory\":1024,\"cpus\":1,\"metadata\":${meta},\"interfaces\":[{\"subnet\":\"${subnet}\",\"ip\":\"${ip}\",\"primary\":true}],\"storage\":[{\"path\":\"${VOLUMES}/${name}.qcow2\",\"dev\":\"vda\"}]}" \
        && wait_state "vms/${name}" running
}

vm_delete () {
    api_delete "vms/${1}" && wait_state "vms/${1}" deleted
}

vm_ssh () {
    local vpc="${1}" ip="${2}"
    shift 2
    ip netns exec "${vpc}" ssh -i "${VM_KEY}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -o LogLevel=ERROR -o ConnectTimeout=3 -o BatchMode=yes -n "syonad@${ip}" "$@"
}

vm_wait () {
    local i
    for i in $(seq 1 40); do
        vm_ssh "${1}" "${2}" true 2>/dev/null && return 0
        sleep 5
    done
    echo "${2}: SSH injoignable" >&2
    return 1
}

vm_fails () {
    local out
    out=$(vm_ssh "${1}" "${2}" "${3} >/dev/null 2>&1; echo rc=\$?") || { echo "SSH vers ${2} en échec"; return 1; }
    case "${out}" in
        rc=0) echo "la commande a réussi dans ${2}"; return 1 ;;
        rc=*) return 0 ;;
        *) echo "sortie inattendue : ${out}"; return 1 ;;
    esac
}

vm_grep () {
    vm_ssh "${1}" "${2}" "${3} 2>&1" | grep -q -- "${4}"
}

has_addr () {
    vm_ssh "${1}" "${2}" 'ip -4 -o addr show dev ens3' | grep -q " ${2}/"
}

route_via () {
    vm_ssh "${1}" "${2}" "ip -4 route show ${3}" | grep -q "via ${4} "
}

dhcp_hosts () {
    python3 - "/run/two/dhcp/${1}.sock" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sys.argv[1])
s.sendall(b'{"verb":"get-state"}\n')
buf = b""
while not buf.endswith(b"\n"):
    c = s.recv(65536)
    if not c:
        break
    buf += c
state = json.loads(buf).get("state") or {}
print(" ".join(sorted(h["mac"].lower() + "=" + h["ip"] for h in state.get("hosts") or [])))
PY
}
