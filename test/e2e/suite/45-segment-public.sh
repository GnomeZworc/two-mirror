# fonctionnalité: segment public de l'hôte — vxlan-public (VNI 1) posé par le bootstrap sur br-public, VNI annoncé en EVPN et vu par les bordures, gateway anycast des bordures installée vers une seule bordure, gateway et destination de test joignables sans doublon, L2 entre hyperviseurs, veth des VPC sur le bridge public, watchdog
# hôtes: hv1 hv2 hv3
# tickets: #87

for host in hv1 hv2 hv3; do
case "${host}" in
    hv1) pub=240.0.0.11 ;;
    hv2) pub=240.0.0.12 ;;
    hv3) pub=240.0.0.13 ;;
esac
on "${host}" "HOST=${host}" "PUB=${pub}" <<'NODE'
NS=t45-pub
GW_MAC=02:01:00:00:00:01
vxlan_info () { ip -j -d link show dev vxlan-public | jq -r '.[0] | "\(.linkinfo.info_data.id) \(.linkinfo.info_data.local) \(.linkinfo.info_data.learning) \(.master) \(.operstate)"'; }
remote_vtep () { vtysh -c 'show evpn vni 1' | grep -qwF "${1}"; }
pings_clean () {
    local out
    out=$(ip netns exec "${NS}" ping -n -c 5 -i 0.2 -W 1 "${1}" 2>&1) || { echo "${out}" | tail -2; return 1; }
    ! grep -q DUP <<<"${out}" || { echo "${out}" | grep -m1 DUP; return 1; }
}
check "${HOST} : vxlan-public, VNI 1, VTEP $(vtep_host), sans apprentissage, esclave de br-public, actif ($(vxlan_info))" test "$(vxlan_info)" = "1 $(vtep_host) false br-public UNKNOWN"
check "${HOST} : VNI 1 de type L2 dans FRR" sh -c "vtysh -c 'show evpn vni 1' | grep -q 'Type: L2'"
for bd in 192.168.14.21 192.168.14.22; do
    check "${HOST} : bordure ${bd} parmi les VTEP distants du VNI 1" remote_vtep "${bd}"
done
check "${HOST} : MAC de la gateway ${GW_MAC} installée vers une seule bordure" test "$(bridge fdb show dev vxlan-public | grep -c "^${GW_MAC} .*dst 192\.168\.14\.2[12] ")" -eq 1
ip netns add "${NS}"
ip link add t45-pub-h type veth peer name t45-pub-n netns "${NS}"
ip link set t45-pub-h master br-public up
ip -n "${NS}" link set t45-pub-n up
ip -n "${NS}" addr add "${PUB}/24" dev t45-pub-n
ip -n "${NS}" route add default via 240.0.0.1
check "${HOST} : netns de test ${NS} sur br-public, ${PUB}/24" sh -c "ip -n ${NS} -4 -o addr show dev t45-pub-n | grep -q ' ${PUB}/24 '"
check "${HOST} : gateway des bordures 240.0.0.1 joignable, sans doublon" pings_clean 240.0.0.1
check "${HOST} : destination de test 240.255.0.1 joignable par la gateway, sans doublon" pings_clean 240.255.0.1
check "${HOST} : ARP de la gateway résolu vers ${GW_MAC}" sh -c "ip -n ${NS} neigh show 240.0.0.1 | grep -q 'lladdr ${GW_MAC}'"
NODE
done

on bd1 <<'NODE'
for hv in 192.168.14.11 192.168.14.12 192.168.14.13; do
    check "bd1 : hyperviseur ${hv} parmi les VTEP distants du VNI 1" sh -c "vtysh -c 'show evpn vni 1' | grep -qwF ${hv}"
done
NODE

on hv1 <<'NODE'
NS=t45-pub
pings_clean () {
    local out
    out=$(ip netns exec "${NS}" ping -n -c 5 -i 0.2 -W 1 "${1}" 2>&1) || { echo "${out}" | tail -2; return 1; }
    ! grep -q DUP <<<"${out}" || { echo "${out}" | grep -m1 DUP; return 1; }
}
for peer in 240.0.0.12 240.0.0.13; do
    check "hv1 : ${peer} (netns de test d'un autre hyperviseur) joignable en L2 sur le VNI 1" pings_clean "${peer}"
done
check "VPC vp-t45 sur hv1" vpc_create vp-t45 10.45.0.0/16
check "vp-t45 : vp-t45-e esclave du bridge public br-public (interfaces.internet par défaut)" sh -c "ip -o link show vp-t45-e | grep -q 'master br-public'"
ip link set vxlan-public nomaster
check "watchdog : br-public sans port VXLAN signalé" wait_watchdog "no vxlan port: the public segment is unreachable from this host"
check "sans vxlan-public, la gateway des bordures ne répond plus (témoin)" sh -c "! ip netns exec ${NS} ping -n -c 2 -W 1 240.0.0.1 >/dev/null 2>&1"
check "vxlan-public remis sur br-public" ip link set vxlan-public master br-public
check "vxlan-public remis : la gateway des bordures répond de nouveau" sh -c "for i in 1 2 3 4 5 6 7 8 9 10; do ip netns exec ${NS} ping -n -c 1 -W 1 240.0.0.1 >/dev/null 2>&1 && exit 0; sleep 2; done; exit 1"
check "vxlan-public remis : sans doublon" pings_clean 240.0.0.1
NODE
