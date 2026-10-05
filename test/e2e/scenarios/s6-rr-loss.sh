on hv1 <<'NODE'
check "prérequis : VM s4-p de S4 en marche" test "$(api_state vms/s4-p)" = running
check "session EVPN de hv1 établie avant l'arrêt" sh -c "vtysh -c 'show bgp neighbors 10.255.255.1' | grep -q 'BGP state = Established'"
check "prérequis : s4-p joint s4-q" vm_ssh vp-s4 10.240.1.10 'ping -c 2 -W 2 10.240.1.11'
vm_ssh vp-s4 10.240.1.10 'rm -f /tmp/s6.log; nohup ping -D -i 0.2 10.240.1.11 > /tmp/s6.log 2>&1 &'
sleep 5
NODE
on rr1 <<'NODE'
date +%s > /tmp/s6.stop
check "arrêt de FRR sur le route reflector" systemctl stop frr
NODE
sleep 30
on hv1 <<'NODE'
recent () { vm_ssh vp-s4 10.240.1.10 "now=\$(date +%s); awk -F'[][]' -v now=\$now '/bytes from/ && \$2 > now - ${1}' /tmp/s6.log | wc -l"; }
check "session EVPN de hv1 tombée" sh -c "! vtysh -c 'show bgp neighbors 10.255.255.1' | grep -q 'BGP state = Established'"
info "30 s après l'arrêt du route reflector : $(recent 10) réponses dans les 10 dernières secondes (50 si le trafic passe intégralement)"
info "VTEP distant encore connu : $(vtysh -c 'show evpn vni 2401' | grep -c 192.168.14.12) ; entrée d'inondation : $(bridge fdb show dev vxlan-2401 | grep -c '00:00:00:00:00:00 dst')"
sleep 60
info "90 s après l'arrêt : $(recent 10) réponses dans les 10 dernières secondes"
NODE
on rr1 <<'NODE'
check "redémarrage de FRR sur le route reflector" systemctl start frr
date +%s > /tmp/s6.start
NODE
on hv1 <<'NODE'
recent () { vm_ssh vp-s4 10.240.1.10 "now=\$(date +%s); awk -F'[][]' -v now=\$now '/bytes from/ && \$2 > now - ${1}' /tmp/s6.log | wc -l"; }
start=$(date +%s)
for i in $(seq 1 90); do
    vtysh -c "show evpn vni 2401" | grep -q 192.168.14.12 && [ "$(recent 3)" -gt 0 ] && break
    sleep 2
done
info "retour du VTEP distant et du trafic $(( $(date +%s) - start )) s après le redémarrage de FRR"
check "après le retour du route reflector : VTEP distant réappris" sh -c "vtysh -c 'show evpn vni 2401' | grep -q 192.168.14.12"
check "après le retour du route reflector : le trafic passe" test "$(recent 5)" -gt 0
vm_ssh vp-s4 10.240.1.10 'pkill -x ping' || true
NODE
