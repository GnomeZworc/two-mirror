# fonctionnalité: redémarrage de l'agent avec des VM en marche — rien ne disparaît ni n'est recréé, trafic routé sans perte, DHCP et métadonnées servis, base BGP réappliquée à l'identique, ressources toujours gérées
# hôtes: hv1 hv2
# tickets: #41, #46

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t61, l3vni 6100" vpc_create vp-t61 10.61.0.0/16 6100
check "hv1 : subnet sn-t61a (VNI 6111)" subnet_create sn-t61a vp-t61 6111 10.61.1.0/24
check "hv1 : VM t61-a 10.61.1.10" vm_create t61-a sn-t61a 10.61.1.10 "${KEY}"
check "hv1 : VM t61-d 10.61.1.11, supprimée après le redémarrage" vm_create t61-d sn-t61a 10.61.1.11 "${KEY}"
NODE
on hv2 <<'NODE'
check "hv2 : VPC vp-t61, l3vni 6100" vpc_create vp-t61 10.61.0.0/16 6100
check "hv2 : subnet sn-t61b (VNI 6112)" subnet_create sn-t61b vp-t61 6112 10.61.2.0/24
check "hv2 : VM t61-b 10.61.2.10" vm_create t61-b sn-t61b 10.61.2.10
check "hv2 : t61-b répond depuis son netns" netns_ping_wait vp-t61 10.61.2.10
NODE

on hv1 <<'NODE'
api_snapshot () {
    for kind in vpcs subnets vms; do
        curl -fsS "${API}/${kind}" | jq -r --arg k "${kind}" '.[] | "\($k) \(.name) \(.state)"'
    done | sort
}
dropped () { vtysh -c "show bgp neighbors ${1}" | grep -o 'dropped [0-9]*'; }
same_as () { local d; d=$(diff "${1}" <("${2}")) && return 0; printf '%s\n' "${d}" | tail -n 3; return 1; }
mkdir -p "${STATE_DIR}"
check "t61-a joignable" vm_wait vp-t61 10.61.1.10
check "t61-d joignable" vm_wait vp-t61 10.61.1.11
check "hv1 : vrf-t61 apprend 10.61.2.10/32 via le VTEP de hv2" wait_vrf_route vp-t61 10.61.2.10 192.168.14.12
for i in $(seq 1 24); do vm_ssh vp-t61 10.61.1.10 'ping -c 1 -W 2 10.61.2.10' >/dev/null 2>&1 && break; sleep 5; done
check "prérequis : t61-a (hv1) joint t61-b (hv2) par le L3VNI" vm_ssh vp-t61 10.61.1.10 'ping -n -c 3 -W 2 10.61.2.10'
QEMU=$(pgrep -f "${VOLUMES}/t61-a.qcow2" | sort | tr '\n' ' ')
QEMU_D=$(pgrep -f "${VOLUMES}/t61-d.qcow2" | sort | tr '\n' ' ')
check "prérequis : processus QEMU de t61-a et t61-d relevés (${QEMU:-?} / ${QEMU_D:-?})" test -n "${QEMU}" -a -n "${QEMU_D}"
host_state > "${STATE_DIR}/t61.restart"
api_snapshot > "${STATE_DIR}/t61.api"
for rr in 10.255.255.1 10.255.255.2; do dropped "${rr}" > "${STATE_DIR}/t61.${rr}"; done
vm_ssh vp-t61 10.61.1.10 'rm -f /tmp/t61.log; nohup ping -D -i 0.2 10.61.2.10 > /tmp/t61.log 2>&1 &'
sleep 5
START=$(date +%s)
check "hv1 : redémarrage de l'agent" systemctl restart agent
for i in $(seq 1 30); do curl -fsS -o /dev/null "${API}/vpcs" 2>/dev/null && break; sleep 1; done
info "API de nouveau joignable $(( $(date +%s) - START )) s après la demande de redémarrage"
check "après redémarrage : agent actif et API joignable" sh -c "systemctl is-active --quiet agent && curl -fsS -o /dev/null '${API}/vpcs'"
check "après redémarrage : base BGP réappliquée au démarrage" sh -c "journalctl -u agent --since '@${START}' --no-pager -o cat | grep -qF 'bgp base configuration applied'"
sleep 20
check "après redémarrage : même processus QEMU pour t61-a (ni arrêtée ni recréée)" test "$(pgrep -f "${VOLUMES}/t61-a.qcow2" | sort | tr '\n' ' ')" = "${QEMU}"
check "après redémarrage : même processus QEMU pour t61-d" test "$(pgrep -f "${VOLUMES}/t61-d.qcow2" | sort | tr '\n' ' ')" = "${QEMU_D}"
check "après redémarrage : mêmes fichiers sous /run/two, /etc/dnsmasq.d et les volumes (sinon, fichiers en cause)" section_unchanged "${STATE_DIR}/t61.restart" fichiers
check "après redémarrage : hôte inchangé (netns, liens, taps, adresses, routes, ebtables, iptables, FRR, frr.conf, fichiers, unités)" same_as "${STATE_DIR}/t61.restart" host_state
check "après redémarrage : l'API rend les mêmes ressources dans le même état" same_as "${STATE_DIR}/t61.api" api_snapshot
for rr in 10.255.255.1 10.255.255.2; do
    check "après redémarrage : session EVPN vers ${rr} établie, jamais coupée" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established' && [ \"\$(vtysh -c 'show bgp neighbors ${rr}' | grep -o 'dropped [0-9]*')\" = \"\$(cat '${STATE_DIR}/t61.${rr}')\" ]"
done
check "après redémarrage : vrf-t61 exporte toujours t61-a 10.61.1.10/32" vrf_route_via vp-t61 10.61.1.10 169.254.0.2
check "après redémarrage : vrf-t61 apprend toujours 10.61.2.10/32 via hv2" vrf_route_via vp-t61 10.61.2.10 192.168.14.12
check "après redémarrage : bail de t61-a renouvelé (DHCP servi)" lease_renewed vp-t61 10.61.1.10
check "après redémarrage : métadonnées de t61-a servies (instance-id t61-a)" vm_grep vp-t61 10.61.1.10 'curl -fsS http://169.254.169.254/meta-data' 't61-a'
check "après redémarrage : aucune incohérence signalée par le watchdog" watchdog_quiet "$(( $(date +%s) - START ))"
vm_ssh vp-t61 10.61.1.10 'pkill -x ping' || true
sleep 1
stats=$(vm_ssh vp-t61 10.61.1.10 "awk -F'icmp_seq=' '/bytes from/ {split(\$2, a, \" \"); n++; if (a[1] > max) max = a[1]} END {print n+0, max+0}' /tmp/t61.log")
received=${stats% *}
sent=${stats#* }
gap=$(vm_ssh vp-t61 10.61.1.10 "LC_ALL=C awk -F'[][]' '/bytes from/ {if (prev != \"\" && \$2 - prev > max) max = \$2 - prev; prev = \$2} END {printf \"%d\", max * 10}' /tmp/t61.log")
info "trafic routé t61-a → t61-b pendant le redémarrage : ${received} réponses pour ${sent} requêtes ; plus long silence : $(( ${gap:-0} / 10 )),$(( ${gap:-0} % 10 )) s"
check "trafic routé pendant le redémarrage : au moins 100 requêtes émises (témoin)" test "${sent:-0}" -ge 100
check "trafic routé pendant le redémarrage : au plus 1 % de perte" test $(( (${sent:-0} - ${received:-0}) * 100 )) -le "${sent:-0}"
check "après redémarrage : suppression de t61-d acceptée et menée à terme" vm_delete t61-d
check "après redémarrage : processus QEMU de t61-d arrêté" sh -c "! pgrep -f '${VOLUMES}/t61-d.qcow2' >/dev/null"
check "après redémarrage : unité two-vm-t61-d.scope retirée" sh -c "! systemctl list-units --all --plain --no-legend 'two-vm-t61-d.scope' | grep -q ."
check "après redémarrage : VM t61-c créée sur le même subnet" vm_create t61-c sn-t61a 10.61.1.12 "$(vm_key)"
check "t61-c joignable" vm_wait vp-t61 10.61.1.12
check "t61-c (créée après le redémarrage) joint t61-b (hv2)" vm_ssh vp-t61 10.61.1.12 'ping -n -c 3 -W 2 10.61.2.10'
NODE
