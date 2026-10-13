on hv1 <<'NODE'
check "prérequis : VM s4-p de S4 en marche" test "$(api_state vms/s4-p)" = running
for rr in 10.255.255.1 10.255.255.2; do
    check "session EVPN de hv1 vers ${rr} établie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
done
check "prérequis : s4-p joint s4-q" vm_ssh vp-s4 10.240.1.10 'ping -c 2 -W 2 10.240.1.11'
vm_ssh vp-s4 10.240.1.10 'rm -f /tmp/s6.log; nohup ping -D -i 0.2 10.240.1.11 > /tmp/s6.log 2>&1 &'
sleep 5
NODE

for pair in rr1:10.255.255.1:10.255.255.2 rr2:10.255.255.2:10.255.255.1; do
IFS=: read -r node lost kept <<<"${pair}"
on "${node}" "NODE_NAME=${node}" <<'NODE'
check "${NODE_NAME} : arrêt de FRR" systemctl stop frr
NODE
sleep 30
on hv1 "NODE_NAME=${node}" "LOST=${lost}" "KEPT=${kept}" <<'NODE'
recent () { vm_ssh vp-s4 10.240.1.10 "now=\$(date +%s); awk -F'[][]' -v now=\$now '/bytes from/ && \$2 > now - ${1}' /tmp/s6.log | wc -l"; }
check "${NODE_NAME} arrêté : session vers ${LOST} tombée" sh -c "! vtysh -c 'show bgp neighbors ${LOST}' | grep -q 'BGP state = Established'"
check "${NODE_NAME} arrêté : session vers ${KEPT} toujours établie" sh -c "vtysh -c 'show bgp neighbors ${KEPT}' | grep -q 'BGP state = Established'"
check "${NODE_NAME} arrêté : VTEP distant toujours connu" sh -c "vtysh -c 'show evpn vni 2401' | grep -q 192.168.14.12"
n=$(recent 10)
info "${NODE_NAME} arrêté depuis 30 s : ${n} réponses dans les 10 dernières secondes (50 si le trafic passe intégralement)"
check "${NODE_NAME} arrêté : le trafic passe (au moins 45 réponses sur 50)" test "${n}" -ge 45
NODE
on "${node}" "NODE_NAME=${node}" <<'NODE'
check "${NODE_NAME} : redémarrage de FRR" systemctl start frr
NODE
on hv1 "NODE_NAME=${node}" "LOST=${lost}" <<'NODE'
start=$(date +%s)
for i in $(seq 1 60); do
    vtysh -c "show bgp neighbors ${LOST}" | grep -q 'BGP state = Established' && break
    sleep 2
done
info "${NODE_NAME} : session vers ${LOST} rétablie $(( $(date +%s) - start )) s après le redémarrage de FRR"
check "${NODE_NAME} redémarré : session vers ${LOST} rétablie" sh -c "vtysh -c 'show bgp neighbors ${LOST}' | grep -q 'BGP state = Established'"
NODE
done

on hv1 <<'NODE'
vm_ssh vp-s4 10.240.1.10 'pkill -x ping' || true
sleep 1
stats=$(vm_ssh vp-s4 10.240.1.10 "awk -F'icmp_seq=' '/bytes from/ {split(\$2, a, \" \"); n++; if (a[1] > max) max = a[1]} END {print n, max}' /tmp/s6.log")
received=${stats% *}
sent=${stats#* }
info "pendant tout le scénario : ${received} réponses pour ${sent} requêtes, $(( sent - received )) perdues"
check "aucune perte mesurable sur l'ensemble du scénario (au plus 1 %)" test $(( (sent - received) * 100 )) -le "${sent}"
NODE
