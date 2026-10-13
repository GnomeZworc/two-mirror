vrrp_state () {
    on "${1}" <<'NODE' | sed -n 's/^VRRP: //p'
echo "VRRP: $(vtysh -c 'show vrrp' | awk '/Status \(v4\)/ {print $3}')"
NODE
}

on hv1 <<'NODE'
for rr in 10.255.255.1 10.255.255.2; do
    check "prérequis : session EVPN de hv1 vers ${rr} établie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
    vtysh -c "show bgp neighbors ${rr}" | grep -o 'dropped [0-9]*' > "/tmp/s10.${rr}"
done
check "prérequis : hv1 joint la loopback de rr1 par la passerelle" ping -c 2 -W 2 10.255.255.1
rm -f /tmp/s10.log
nohup ping -D -i 0.2 10.255.255.1 > /tmp/s10.log 2>&1 &
sleep 5
NODE

SW1=$(vrrp_state sw1)
SW2=$(vrrp_state sw2)
if [[ "${SW1}" == Master ]]; then
    MASTER=sw1 BACKUP=sw2
else
    MASTER=sw2 BACKUP=sw1
fi

on hv1 "SW1=${SW1}" "SW2=${SW2}" "MASTER=${MASTER}" <<'NODE'
info "état VRRP : sw1 ${SW1}, sw2 ${SW2}"
check "un seul maître VRRP pour la passerelle, l'autre en attente" sh -c "[ \"\$1 \$2\" = 'Master Backup' ] || [ \"\$1 \$2\" = 'Backup Master' ]" _ "${SW1}" "${SW2}"
info "panne simulée sur ${MASTER} : interface underlay coupée"
NODE

on "${MASTER}" "NODE_NAME=${MASTER}" <<'NODE'
check "${NODE_NAME} : coupure de l'interface underlay" ip link set dev underlay down
NODE
sleep 20

on hv1 "MASTER=${MASTER}" "BACKUP_STATE=$(vrrp_state "${BACKUP}")" "BACKUP=${BACKUP}" <<'NODE'
check "${MASTER} coupé : ${BACKUP} est devenu maître" test "${BACKUP_STATE}" = Master
for rr in 10.255.255.1 10.255.255.2; do
    check "${MASTER} coupé : session EVPN vers ${rr} toujours établie, sans coupure" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established' && [ \"\$(vtysh -c 'show bgp neighbors ${rr}' | grep -o 'dropped [0-9]*')\" = \"\$(cat /tmp/s10.${rr})\" ]"
done
n=$(LC_ALL=C awk -F'[][]' -v now="$(date +%s)" '/bytes from/ && $2 > now - 10' /tmp/s10.log | wc -l)
info "${MASTER} coupé depuis 20 s : ${n} réponses de rr1 dans les 10 dernières secondes (50 si tout passe)"
check "${MASTER} coupé : le trafic vers rr1 passe par ${BACKUP} (au moins 45 réponses sur 50)" test "${n}" -ge 45
NODE

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

on hv1 "MASTER=${MASTER}" "MASTER_STATE=$(vrrp_state "${MASTER}")" "BACKUP=${BACKUP}" "BACKUP_STATE=$(vrrp_state "${BACKUP}")" <<'NODE'
check "${MASTER} rétabli : il reste en attente, sans reprendre la passerelle" test "${MASTER_STATE}" = Backup
check "${MASTER} rétabli : ${BACKUP} reste maître" test "${BACKUP_STATE}" = Master
pkill -f 'ping -D -i 0.2 10.255.255.1' || true
sleep 1
for rr in 10.255.255.1 10.255.255.2; do
    check "fin du scénario : session EVPN vers ${rr} jamais coupée" sh -c "[ \"\$(vtysh -c 'show bgp neighbors ${rr}' | grep -o 'dropped [0-9]*')\" = \"\$(cat /tmp/s10.${rr})\" ]"
done
gap=$(LC_ALL=C awk -F'[][]' '/bytes from/ {if (prev != "" && $2 - prev > max) max = $2 - prev; prev = $2} END {printf "%d", max * 10}' /tmp/s10.log)
stats=$(awk -F'icmp_seq=' '/bytes from/ {split($2, a, " "); n++; if (a[1] > max) max = a[1]} END {print n, max}' /tmp/s10.log)
received=${stats% *}
sent=${stats#* }
info "pendant tout le scénario : ${received} réponses pour ${sent} requêtes, $(( sent - received )) perdues ; plus long silence : $(( gap / 10 )),$(( gap % 10 )) s"
check "bascule de la passerelle en moins de 5 s" test "${gap}" -lt 50
NODE
