# fonctionnalité: perte d'un route reflector — l'autre garde la session EVPN, le VTEP distant reste connu, le trafic L2 entre hôtes passe sans perte mesurable, retour du RR arrêté
# hôtes: hv1 hv2
# tickets: #54

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t71" vpc_create vp-t71 10.71.0.0/16
check "hv1 : subnet sn-t71 (VNI 7101)" subnet_create sn-t71 vp-t71 7101 10.71.1.0/24
check "hv1 : VM t71-p" vm_create t71-p sn-t71 10.71.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
check "hv2 : VPC vp-t71" vpc_create vp-t71 10.71.0.0/16
check "hv2 : subnet sn-t71 (VNI 7101)" subnet_create sn-t71 vp-t71 7101 10.71.1.0/24
check "hv2 : VM t71-q" vm_create t71-q sn-t71 10.71.1.11
check "hv2 : t71-q répond depuis son netns" netns_ping_wait vp-t71 10.71.1.11
NODE

on hv1 <<'NODE'
check "prérequis : t71-p joignable" vm_wait vp-t71 10.71.1.10
for rr in 10.255.255.1 10.255.255.2; do
    check "session EVPN de hv1 vers ${rr} établie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
done
for i in $(seq 1 24); do vm_ssh vp-t71 10.71.1.10 'ping -c 1 -W 2 10.71.1.11' >/dev/null 2>&1 && break; sleep 5; done
check "prérequis : t71-p joint t71-q" vm_ssh vp-t71 10.71.1.10 'ping -c 2 -W 2 10.71.1.11'
vm_ssh vp-t71 10.71.1.10 'rm -f /tmp/t71.log; nohup ping -D -i 0.2 10.71.1.11 > /tmp/t71.log 2>&1 &'
sleep 5
NODE

for pair in rr1:10.255.255.1:10.255.255.2 rr2:10.255.255.2:10.255.255.1; do
IFS=: read -r node lost kept <<<"${pair}"
on "${node}" "NODE_NAME=${node}" <<'NODE'
check "${NODE_NAME} : arrêt de FRR" systemctl stop frr
NODE
sleep 30
on hv1 "NODE_NAME=${node}" "LOST=${lost}" "KEPT=${kept}" <<'NODE'
recent () { vm_ssh vp-t71 10.71.1.10 "now=\$(date +%s); awk -F'[][]' -v now=\$now '/bytes from/ && \$2 > now - ${1}' /tmp/t71.log | wc -l"; }
check "${NODE_NAME} arrêté : session vers ${LOST} tombée" sh -c "! vtysh -c 'show bgp neighbors ${LOST}' | grep -q 'BGP state = Established'"
check "${NODE_NAME} arrêté : session vers ${KEPT} toujours établie" sh -c "vtysh -c 'show bgp neighbors ${KEPT}' | grep -q 'BGP state = Established'"
check "${NODE_NAME} arrêté : VTEP distant toujours connu" sh -c "vtysh -c 'show evpn vni 7101' | grep -q 192.168.14.12"
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
vm_ssh vp-t71 10.71.1.10 'pkill -x ping' || true
sleep 1
stats=$(vm_ssh vp-t71 10.71.1.10 "awk -F'icmp_seq=' '/bytes from/ {split(\$2, a, \" \"); n++; if (a[1] > max) max = a[1]} END {print n+0, max+0}' /tmp/t71.log")
received=${stats% *}
sent=${stats#* }
info "pendant tout le scénario : ${received:-0} réponses pour ${sent:-0} requêtes, $(( ${sent:-0} - ${received:-0} )) perdues"
check "trafic pendant tout le scénario : au moins 300 requêtes émises (témoin)" test "${sent:-0}" -ge 300
check "aucune perte mesurable sur l'ensemble du scénario (au plus 1 %)" test $(( (${sent:-0} - ${received:-0}) * 100 )) -le "${sent:-0}"
NODE
