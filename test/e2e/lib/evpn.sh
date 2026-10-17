vrf_nexthops () {
    ip -4 route show vrf "vrf-${1#vp-}" "${2}" | grep -o 'via [0-9.]*' | cut -d' ' -f2 | sort -u | tr '\n' ' ' | sed 's/ $//'
}

wait_vrf_nexthops () {
    local i
    for i in $(seq 1 30); do
        [ "$(vrf_nexthops "${1}" "${2}")" = "${3}" ] && return 0
        sleep 2
    done
    echo "vrf-${1#vp-} ${2} : next-hops « $(vrf_nexthops "${1}" "${2}") », « ${3} » attendu" >&2
    return 1
}

vrf_route_via () {
    ip -4 route show vrf "vrf-${1#vp-}" "${2}" | grep -q "via ${3} "
}

wait_vrf_route () {
    local i
    for i in $(seq 1 30); do
        vrf_route_via "$@" && return 0
        sleep 2
    done
    echo "vrf-${1#vp-} : pas de route ${2} via ${3}" >&2
    return 1
}

vrf_members () {
    ip -o link show master "vrf-${1#vp-}" | awk -F': ' '{print $2}' | cut -d@ -f1 | sort | tr '\n' ' ' | sed 's/ $//'
}

l3vni_ready () {
    local out
    out=$(vtysh -c "show evpn vni ${1}") &&
        grep -q 'Type: L3' <<<"${out}" &&
        grep -q 'State: Up' <<<"${out}" &&
        grep -qi "Router MAC: ${2}" <<<"${out}"
}

wait_l3vni () {
    local i
    for i in $(seq 1 15); do
        l3vni_ready "$@" && return 0
        sleep 2
    done
    echo "VNI ${1} : pas de type L3 actif avec la RMAC ${2}" >&2
    return 1
}

vtep_host () {
    ip -4 route get 10.255.255.1 | grep -o 'src [0-9.]*' | cut -d' ' -f2
}

bgp_paths () {
    vtysh -c "show bgp vrf vrf-${1#vp-} ipv4 unicast ${2} json" 2>/dev/null | jq -r '(.paths // []) | length'
}

bgp_best () {
    vtysh -c "show bgp vrf vrf-${1#vp-} ipv4 unicast ${2} json" 2>/dev/null |
        jq -r '(.paths // [])[] | select(.bestpath.overall == true) | "\(.nexthops[0].ip) (\(.bestpath.selectionReason // "seul chemin"))"'
}

watchdog_reported () {
    journalctl -u agent --since "-${2:-120}s" --no-pager -o cat | grep -F 'watchdog: inconsistency detected' | grep -qF -- "${1}"
}

wait_watchdog () {
    local i
    for i in $(seq 1 30); do
        watchdog_reported "$@" && return 0
        sleep 2
    done
    echo "watchdog : « ${1} » non signalé" >&2
    return 1
}

watchdog_alerts () {
    journalctl -u agent --since "-${1:-300}s" --no-pager -o cat | grep -F 'watchdog: inconsistency detected'
}

watchdog_quiet () {
    local out
    out=$(watchdog_alerts "${1:-300}" | { if [ -n "${2:-}" ]; then grep -vF -- "${2}"; else cat; fi; }) || return 0
    printf '%s\n' "${out}"
    return 1
}

watchdog_quiet_about () {
    local out
    out=$(watchdog_alerts "${1}" | grep -F -- "${2}") || return 0
    printf '%s\n' "${out}"
    return 1
}

evpn_mac_remote () {
    vtysh -c "show evpn mac vni ${1} json" 2>/dev/null |
        jq -e --arg m "$(tr 'A-F' 'a-f' <<<"${2}")" --arg v "${3}" '(.macs // {})[$m] | .type == "remote" and .remoteVtep == $v' >/dev/null
}

evpn_mac_json_shape () {
    local out
    out=$(vtysh -c "show evpn mac vni ${1} json" 2>/dev/null)
    jq -e '[(.macs // {})[] | select(.type == "remote")] | length > 0 and all(has("remoteVtep"))' >/dev/null <<<"${out}" && return 0
    echo "VNI ${1} : champs d'une MAC distante : $(jq -c '[(.macs // {})[] | select(.type == "remote")][0] // {} | keys' <<<"${out}" 2>/dev/null)"
    return 1
}

wait_evpn_mac () {
    local i
    for i in $(seq 1 30); do
        evpn_mac_remote "$@" && return 0
        sleep 2
    done
    echo "VNI ${1} : ${2} non apprise en type 2 via ${3}" >&2
    return 1
}

wait_bgp_paths () {
    local i n
    for i in $(seq 1 15); do
        n=$(bgp_paths "${1}" "${2}")
        [ "${n}" = "${3}" ] && return 0
        sleep 2
    done
    echo "vrf-${1#vp-} ${2} : ${n:-?} chemin(s) BGP, ${3} attendu(s)" >&2
    return 1
}

watchdog_l3_quiet () {
    local out
    out=$(journalctl -u agent --since "-${1:-60}s" --no-pager -o cat | grep -F 'watchdog: inconsistency detected' | grep -E '"kind":"bgp"|l3vni [0-9]|interface (vrf|brl3|vxl3|l3)-') || return 0
    printf '%s\n' "${out}"
    return 1
}
