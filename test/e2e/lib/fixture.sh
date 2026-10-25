STATE_DIR=/var/tmp/e2e
SCRATCH_DIR=/tmp

owned () {
    grep -E "^(vp-|sn-)?t${T}"
}

api_names () {
    local out
    out=$(curl -fsS "${API}/${1}") || { echo "API injoignable (${1})" >&2; return 1; }
    jq -r '.[] | select(.state != "deleted") | .name' <<<"${out}"
}

delete_owned () {
    local kind="${1}" names name rc=0
    names=$(api_names "${kind}") || return 1
    for name in $(owned <<<"${names}"); do
        api_delete "${kind}/${name}" && wait_state "${kind}/${name}" deleted || { echo "${kind}/${name} : suppression en échec" >&2; rc=1; }
    done
    return "${rc}"
}

scratch_netns () {
    ip netns list | awk '{print $1}' | grep -E "^t${T}-"
}

scratch_files () {
    find "${SCRATCH_DIR}" "${STATE_DIR}" -maxdepth 1 -type f \( -name "t${T}-*" -o -name "t${T}.*" \) ! -name "t${T}.avant"
}

clean_scratch () {
    local ns f
    for ns in $(scratch_netns); do
        ip netns del "${ns}" || return 1
    done
    for f in $(scratch_files); do
        rm -f "${f}" || return 1
    done
}

teardown_owned () {
    delete_owned vms && delete_owned subnets && delete_owned vpcs || return 1
    rm -f "${VOLUMES}/t${T}"-*
    clean_scratch || return 1
    local left="" kind names
    for kind in vms subnets vpcs; do
        names=$(api_names "${kind}") || return 1
        left="${left}$(owned <<<"${names}" | sed "s|^|${kind}/|" | tr '\n' ' ')"
    done
    [ -z "${left}" ] || { echo "encore présents : ${left}"; return 1; }
}

host_state () {
    echo "## netns"; ip netns list | awk '{print $1}' | sort
    echo "## liens"; ip -o link show | awk -F': ' '{print $2}' | cut -d@ -f1 | sort
    echo "## adresses"; ip -o -4 addr show | awk '{print $2, $4}' | sort
    echo "## routes"; ip -4 route show table all | grep -v '^local \|^broadcast ' | sort
    echo "## nftables bridge"; nft list ruleset bridge
    echo "## iptables"; iptables-save | grep -v '^#' | sed 's/ \[[0-9]*:[0-9]*\]$//'
    echo "## frr"; vtysh -c 'show running-config' | grep -v '^Building\|^Current configuration\|^!'
    echo "## frr.conf"; sha256sum /etc/frr/frr.conf | cut -d' ' -f1
    echo "## fichiers"; find /run/two /etc/dnsmasq.d "${VOLUMES}" -mindepth 1 ! -type d 2>/dev/null | sort
    echo "## unités"; systemctl list-units --all --plain --no-legend 'two-vm-*' | awk '{print $1}'
    echo "## processus"; pgrep -a dnsmasq | cut -d' ' -f2- | sort; echo "qemu: $(pgrep -c qemu-system)"
}

prepare_test () {
    mkdir -p "${STATE_DIR}"
    curl -fsS -o /dev/null "${API}/vpcs" || echo "ÉCHOUÉ: ${HOST} : API de l'agent injoignable avant le test"
    if [ -n "$(for kind in vms subnets vpcs; do api_names "${kind}" | owned; done)$(scratch_netns)$(scratch_files)" ]; then
        info "${HOST} : ressources, netns ou fichiers t${T} d'un passage précédent, suppression"
        check "${HOST} : nettoyage des ressources t${T} restantes" teardown_owned
    fi
    host_state > "${STATE_DIR}/t${T}.avant"
}

state_unchanged () {
    local i diff
    for i in $(seq 1 15); do
        diff=$(diff "${STATE_DIR}/t${T}.avant" <(host_state)) && return 0
        sleep 2
    done
    printf '%s\n' "${diff}"
    echo "$(grep -c '^[<>]' <<<"${diff}") différence(s), dont : $(grep -m 1 '^[<>]' <<<"${diff}")"
    return 1
}

state_section () {
    awk -v s="## ${2}" '/^## / {p = ($0 == s); next} p' "${1}"
}

section_unchanged () {
    local now d
    now=$(mktemp)
    host_state > "${now}"
    d=$(diff <(state_section "${1}" "${2}") <(state_section "${now}" "${2}"))
    rm -f "${now}"
    [ -z "${d}" ] && return 0
    echo "${2} en cause : $(grep '^[<>]' <<<"${d}" | sed 's/^< /-/; s/^> /+/' | tr '\n' ' ')"
    return 1
}

finish_test () {
    check "${HOST} : suppression des ressources t${T}" teardown_owned
    check "${HOST} : hôte revenu à l'état d'avant le test (assert_clean)" state_unchanged
}
