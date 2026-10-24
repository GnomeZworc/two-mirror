# fonctionnalité: perte d'un switch de la passerelle VRRP — bascule vers l'autre en moins de 5 s, sessions EVPN jamais coupées ; au retour, un seul maître (préemption désactivée : celui qui l'est est relevé), plus long silence mesuré
# hôtes:
# tickets: #54, #69, #80

. "${LAB_DIR}/lib/net.sh"

vrrp_state () {
    on "${1}" <<'NODE' | sed -n 's/^VRRP: //p'
echo "VRRP: $(vtysh -c 'show vrrp' | awk '/Status \(v4\)/ {print $3}')"
NODE
}

vrrp_priority () {
    on "${1}" <<'NODE' | sed -n 's/^PRIO: //p'
p=$(vtysh -c 'show running-config' | awk '$1 == "vrrp" && $3 == "priority" {print $4; exit}')
echo "PRIO: ${p:-100}"
NODE
}

hv1_now () {
    on hv1 <<'NODE' | sed -n 's/^NOW: //p'
echo "NOW: $(date +%s.%N)"
NODE
}

on hv1 <<'NODE'
for rr in 10.255.255.1 10.255.255.2; do
    check "prérequis : session EVPN de hv1 vers ${rr} établie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
    vtysh -c "show bgp neighbors ${rr}" | grep -o 'dropped [0-9]*' > "/tmp/t72.${rr}"
done
check "prérequis : hv1 joint la loopback de rr1 par la passerelle" ping -c 2 -W 2 10.255.255.1
rm -f /tmp/t72.log
nohup ping -D -i 0.2 10.255.255.1 > /tmp/t72.log 2>&1 &
sleep 5
NODE

SW1=$(vrrp_state sw1)
SW2=$(vrrp_state sw2)
P1=$(vrrp_priority sw1)
P2=$(vrrp_priority sw2)
MASTER="" BACKUP=""
if [[ "${SW1} ${SW2}" == "Master Backup" ]]; then
    MASTER=sw1 BACKUP=sw2
elif [[ "${SW1} ${SW2}" == "Backup Master" ]]; then
    MASTER=sw2 BACKUP=sw1
fi

on hv1 "SW1=${SW1}" "SW2=${SW2}" "P1=${P1}" "P2=${P2}" "MASTER=${MASTER}" <<'NODE'
info "état VRRP : sw1 ${SW1} (priorité ${P1}), sw2 ${SW2} (priorité ${P2})"
check "priorités VRRP lues sur sw1 et sw2" sh -c "[ -n \"\$1\" ] && [ -n \"\$2\" ]" _ "${P1}" "${P2}"
check "un seul maître VRRP pour la passerelle, l'autre en attente" sh -c "[ \"\$1 \$2\" = 'Master Backup' ] || [ \"\$1 \$2\" = 'Backup Master' ]" _ "${SW1}" "${SW2}"
NODE

if [[ -z "${MASTER}" ]]; then
    on hv1 <<'NODE'
pkill -f 'ping -D -i 0.2 10.255.255.1'
rm -f /tmp/t72.log /tmp/t72.10.255.255.1 /tmp/t72.10.255.255.2
check "état VRRP lisible avec un seul maître : aucune panne simulée" false
NODE
    return 1
fi
on hv1 "MASTER=${MASTER}" <<'NODE'
info "panne simulée sur ${MASTER} : interface underlay coupée"
NODE

on "${MASTER}" "NODE_NAME=${MASTER}" <<'NODE'
check "${NODE_NAME} : coupure de l'interface underlay" ip link set dev underlay down
NODE
sleep 20

on hv1 "MASTER=${MASTER}" "BACKUP_STATE=$(vrrp_state "${BACKUP}")" "BACKUP=${BACKUP}" <<'NODE'
check "${MASTER} coupé : ${BACKUP} est devenu maître" test "${BACKUP_STATE}" = Master
for rr in 10.255.255.1 10.255.255.2; do
    check "${MASTER} coupé : session EVPN vers ${rr} toujours établie, sans coupure" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established' && [ \"\$(vtysh -c 'show bgp neighbors ${rr}' | grep -o 'dropped [0-9]*')\" = \"\$(cat /tmp/t72.${rr})\" ]"
done
n=$(LC_ALL=C awk -F'[][]' -v now="$(date +%s)" '/bytes from/ && $2 > now - 10' /tmp/t72.log | wc -l)
info "${MASTER} coupé depuis 20 s : ${n} réponses de rr1 dans les 10 dernières secondes (50 si tout passe)"
check "${MASTER} coupé : le trafic vers rr1 passe par ${BACKUP} (au moins 45 réponses sur 50)" test "${n}" -ge 45
NODE

T_UP=$(hv1_now)
on "${MASTER}" "NODE_NAME=${MASTER}" <<'NODE'
check "${NODE_NAME} : rétablissement de l'interface underlay" ip link set dev underlay up
for i in $(seq 1 30); do
    vtysh -c 'show bgp neighbors 169.254.0.3' | grep -q 'BGP state = Established' && vtysh -c 'show bgp neighbors 169.254.0.4' | grep -q 'BGP state = Established' && break
    sleep 2
done
for rr in 169.254.0.3 169.254.0.4; do
    check "${NODE_NAME} rétabli : session eBGP vers ${rr} rétablie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
done
NODE
sleep 10

if [[ "${MASTER}" == sw1 ]]; then CUT_PRIO=${P1}; else CUT_PRIO=${P2}; fi
HIGHEST=$(vrrp_expected_master "sw1:${P1}:192.168.14.2" "sw2:${P2}:192.168.14.3")

on hv1 "MASTER=${MASTER}" "CUT_PRIO=${CUT_PRIO}" "HIGHEST=${HIGHEST}" "T_UP=${T_UP}" \
    "SW1=$(vrrp_state sw1)" "SW2=$(vrrp_state sw2)" <<'NODE'
NOW_MASTER=""
[ "${SW1}" = Master ] && NOW_MASTER=sw1
[ "${SW2}" = Master ] && NOW_MASTER=sw2
info "${MASTER} rétabli (priorité ${CUT_PRIO}) : sw1 ${SW1}, sw2 ${SW2} ; maître ${NOW_MASTER:-aucun}, le plus prioritaire est ${HIGHEST} (préemption désactivée)"
check "${MASTER} rétabli : un seul maître VRRP, l'autre en attente" sh -c "[ \"\$1 \$2\" = 'Master Backup' ] || [ \"\$1 \$2\" = 'Backup Master' ]" _ "${SW1}" "${SW2}"
pkill -f 'ping -D -i 0.2 10.255.255.1' || true
sleep 1
for rr in 10.255.255.1 10.255.255.2; do
    check "fin du scénario : session EVPN vers ${rr} jamais coupée" sh -c "[ \"\$(vtysh -c 'show bgp neighbors ${rr}' | grep -o 'dropped [0-9]*')\" = \"\$(cat /tmp/t72.${rr})\" ]"
done
first=$(log_gap /tmp/t72.log 0 "${T_UP}")
second=$(log_gap /tmp/t72.log "${T_UP}" 0)
stats=$(awk -F'icmp_seq=' '/bytes from/ {split($2, a, " "); n++; if (a[1] > max) max = a[1]} END {print n+0, max+0}' /tmp/t72.log)
received=${stats% *}
sent=${stats#* }
after=$(LC_ALL=C awk -F'[][]' -v from="${T_UP}" '/bytes from/ && $2 >= from' /tmp/t72.log | wc -l)
info "pendant tout le scénario : ${received} réponses pour ${sent} requêtes, $(( sent - received )) perdues ; première bascule : silence de $(( first / 10 )),$(( first % 10 )) s ; après le retour de ${MASTER} : ${after} réponses, silence de $(( second / 10 )),$(( second % 10 )) s"
check "première bascule de la passerelle en moins de 5 s" test "${first}" -lt 50
check "retour de ${MASTER} : plus long silence de moins de 5 s, qu'il redevienne maître ou non" test "${second}" -lt 50
rm -f /tmp/t72.log /tmp/t72.10.255.255.1 /tmp/t72.10.255.255.2
NODE
