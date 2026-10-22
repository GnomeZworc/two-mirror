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
    printf '%s\n' '[Match]' 'Name=en*' '' '[Network]' 'DHCP=ipv4' \
        > /mnt/two-image/etc/systemd/network/90-all-en.network
    printf '%s\n' \
        'SUBSYSTEM=="block", ENV{DEVTYPE}=="disk", ENV{ID_SERIAL_SHORT}=="xvd[a-z]", SYMLINK+="$env{ID_SERIAL_SHORT}"' \
        'SUBSYSTEM=="block", ENV{DEVTYPE}=="partition", ENV{ID_SERIAL_SHORT}=="xvd[a-z]", SYMLINK+="$env{ID_SERIAL_SHORT}%n"' \
        > /mnt/two-image/etc/udev/rules.d/90-two-xvd.rules
    umount /mnt/two-image
    qemu-nbd -d /dev/nbd0 >/dev/null
    mv "${work}/${IMAGE_FILE}" "${TWO_IMAGE}"
    rm -rf "${work}"
}

vm_key () {
    [ -f "${VM_KEY}" ] || ssh-keygen -q -t ed25519 -N '' -C lab-vm -f "${VM_KEY}"
    cat "${VM_KEY}.pub"
}

mac_for_ip () {
    local IFS=.
    set -- ${1}
    printf '52:54:00:%02x:%02x:%02x' "${2}" "${3}" "${4}"
}

vm_create () {
    local name="${1}" subnet="${2}" ip="${3}" key="${4:-}" meta="{}"
    api_state "vms/${name}" | grep -qx running && return 0
    [ -n "${key}" ] && meta="{\"sshkey\":\"${key}\"}"
    mkdir -p "${VOLUMES}"
    cp "${TWO_IMAGE}" "${VOLUMES}/${name}.qcow2" || return 1
    api_post vms "{\"name\":\"${name}\",\"memory\":1024,\"cpus\":1,\"metadata\":${meta},\"interfaces\":[{\"subnet\":\"${subnet}\",\"ip\":\"${ip}\",\"mac\":\"$(mac_for_ip "${ip}")\",\"primary\":true}],\"storage\":[{\"path\":\"${VOLUMES}/${name}.qcow2\",\"dev\":\"vda\"}]}" \
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

ping_rate () {
    vm_ssh "${1}" "${2}" "ping -q -n -c ${4:-50} -i 0.2 -W 1 ${3}" 2>/dev/null | grep -o '[0-9]* received' | cut -d' ' -f1
}

vm_body () {
    local name="${1}" subnet="${2}" ip="${3}" disk="${4:-${VOLUMES}/${1}.qcow2}" meta="${5:-{\}}" dev="${6:-vda}"
    printf '{"name":"%s","memory":1024,"cpus":1,"metadata":%s,"interfaces":[{"subnet":"%s","ip":"%s","mac":"%s","primary":true}],"storage":[{"path":"%s","dev":"%s"}]}' \
        "${name}" "${meta}" "${subnet}" "${ip}" "$(mac_for_ip "${ip}")" "${disk}" "${dev}"
}

vm_json () {
    local name="${1}" nics='[]' disks='[]' meta='{}' uefi=false primary=true sn rest ip mac
    shift
    while [ $# -gt 0 ]; do
        case "${1}" in
            --nic)
                sn=${2%%=*}; rest=${2#*=}; ip=${rest%%=*}; mac=$(mac_for_ip "${ip}")
                [ "${rest}" != "${ip}" ] && mac=${rest#*=}
                nics=$(jq -c --arg s "${sn}" --arg ip "${ip}" --arg m "${mac}" --argjson p "${primary}" '. + [{subnet: $s, ip: $ip, mac: $m, primary: $p}]' <<<"${nics}"); primary=false; shift 2 ;;
            --disk) disks=$(jq -c --arg d "${2%%=*}" --arg p "${2#*=}" '. + [{path: $p, dev: $d}]' <<<"${disks}"); shift 2 ;;
            --sshkey) meta=$(jq -c --arg v "${2}" '. + {sshkey: $v}' <<<"${meta}"); shift 2 ;;
            --password) meta=$(jq -c --arg v "${2}" '. + {password: $v}' <<<"${meta}"); shift 2 ;;
            --user-data) meta=$(jq -c --arg v "${2}" '. + {user_data: $v}' <<<"${meta}"); shift 2 ;;
            --uefi) uefi=true; shift ;;
            *) echo "vm_json : option inconnue ${1}" >&2; return 1 ;;
        esac
    done
    jq -nc --arg n "${name}" --argjson nics "${nics}" --argjson disks "${disks}" --argjson meta "${meta}" --argjson uefi "${uefi}" \
        '{name: $n, memory: 1024, cpus: 1, metadata: $meta, interfaces: $nics, storage: $disks, uefi: $uefi}'
}

vm_launch () {
    local name="${1}" body
    shift
    api_state "vms/${name}" | grep -qx running && return 0
    body=$(vm_json "${name}" --disk "vda=${VOLUMES}/${name}.qcow2" "$@") || return 1
    mkdir -p "${VOLUMES}"
    cp "${TWO_IMAGE}" "${VOLUMES}/${name}.qcow2" || return 1
    api_post vms "${body}" && wait_state "vms/${name}" running
}

vm_has_ip () {
    vm_ssh "${1}" "${2}" 'ip -4 -o addr show' | grep -q " ${3}/"
}

vm_lease () {
    vm_ssh "${1}" "${2}" "sudo cat /run/systemd/netif/leases/\$(cat /sys/class/net/${4:-ens3}/ifindex)" | sed -n "s/^${3}=//p"
}

vm_meta () {
    vm_ssh "${1}" "${2}" "curl -fsS http://169.254.169.254/${3}"
}

vm_boot_id () {
    vm_ssh "${1}" "${2}" 'cat /proc/sys/kernel/random/boot_id'
}

wait_reboot () {
    local i id
    for i in $(seq 1 60); do
        id=$(vm_boot_id "${1}" "${2}" 2>/dev/null)
        [ -n "${id}" ] && [ "${id}" != "${3}" ] && return 0
        sleep 5
    done
    echo "${2} : pas de redémarrage constaté (boot_id ${id:-illisible})" >&2
    return 1
}

dhcp_acks () {
    journalctl --since "@${1}" --no-pager -o cat -u 'dhcp@*' -u 'dnsmasq@*' | grep -cE "\"type\":5,\"ip\":\"${2}\"|DHCPACK\([^)]*\) ${2} "
}

lease_renewed () {
    local t0 i
    t0=$(date +%s)
    sleep 1
    vm_ssh "${1}" "${2}" "sudo networkctl renew ${3:-ens3}" || return 1
    for i in $(seq 1 15); do
        [ "$(dhcp_acks "${t0}" "${2}")" -gt 0 ] && return 0
        sleep 2
    done
    echo "${2} : aucun DHCPACK du serveur depuis la demande de renouvellement" >&2
    return 1
}

ssh_denied () {
    local i out
    for i in $(seq 1 40); do
        out=$(vm_ssh "${1}" "${2}" true 2>&1) && { echo "${2} : SSH accepté"; return 1; }
        grep -q 'Permission denied' <<<"${out}" && return 0
        sleep 5
    done
    echo "${2} : ni accepté ni refusé (${out:-aucune réponse})" >&2
    return 1
}

vm_neigh_mac () {
    vm_ssh "${1}" "${2}" "ip neigh show ${3}" | grep -o 'lladdr [0-9a-f:]*' | cut -d' ' -f2
}
