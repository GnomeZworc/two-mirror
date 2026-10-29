# fonctionnalité: passerelle NAT d'un VPC — macvlan ng-<id> sur le br-public du netns, MAC 02:04:00 + l3vni, SNAT vers l'IP de sortie, route par défaut en type 5 importée par les autres hôtes, l'unreachable default jamais annoncée, rien n'entre sans connexion établie, bascule vers un autre hôte par mobilité MAC, refus 409, suppression propre, watchdog
# hôtes: hv1 hv2 hv3
# tickets: #42

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t46, l3vni 4600" vpc_create vp-t46 10.46.0.0/16 4600
check "hv1 : subnet sn-t46a (VNI 4611)" subnet_create sn-t46a vp-t46 4611 10.46.1.0/24
check "hv1 : VM t46-a" vm_create t46-a sn-t46a 10.46.1.10 "${KEY}"
check "t46-a joignable" vm_wait vp-t46 10.46.1.10
check "hv1 : route par défaut du netns vp-t46 vers la VRF (169.254.0.1)" sh -c "ip -n vp-t46 -4 route show default | grep -q '^default via 169.254.0.1 dev l3-t46-i'"
check "sans passerelle, t46-a ne joint pas 240.255.0.1 : refusé par vrf-t46 (169.254.0.1)" vm_grep vp-t46 10.46.1.10 'ping -n -c 2 -W 2 240.255.0.1' 'From 169.254.0.1 .*Unreachable'
NODE

on hv2 <<'NODE'
KEY=$(vm_key)
check "hv2 : VPC vp-t46, l3vni 4600" vpc_create vp-t46 10.46.0.0/16 4600
check "hv2 : subnet sn-t46b (VNI 4612)" subnet_create sn-t46b vp-t46 4612 10.46.2.0/24
check "hv2 : VM t46-b" vm_create t46-b sn-t46b 10.46.2.10 "${KEY}"
check "t46-b joignable" vm_wait vp-t46 10.46.2.10
NODE

on hv3 <<'NODE'
NS=t46-pub
ip netns add "${NS}"
ip link add t46-pub-h type veth peer name t46-pub-n netns "${NS}"
ip link set t46-pub-h master br-public up
ip -n "${NS}" link set t46-pub-n up
ip -n "${NS}" addr add 240.0.0.146/24 dev t46-pub-n
check "hv3 : netns de test ${NS} sur br-public, 240.0.0.146/24" sh -c "ip -n ${NS} -4 -o addr show dev t46-pub-n | grep -q ' 240.0.0.146/24 '"
check "hv3 : témoin, ${NS} joint la gateway des bordures 240.0.0.1" ip netns exec "${NS}" ping -n -c 3 -W 1 240.0.0.1
NODE

on hv1 <<'NODE'
MAC=02:04:00:00:11:f8
check "hv1 : passerelle ng-t46 de vp-t46 (240.0.0.46/24, gateway 240.0.0.1)" api_post nat-gateways '{"name":"ng-t46","vpc":"vp-t46","ip":"240.0.0.46/24","gateway":"240.0.0.1"}'
check "hv1 : ng-t46 running" wait_state nat-gateways/ng-t46 running
check "hv1 : seconde passerelle pour vp-t46 refusée (409)" expect_code 409 api_post_code nat-gateways '{"name":"ng-t46x","vpc":"vp-t46","ip":"240.0.0.45/24","gateway":"240.0.0.1"}'
check "hv1 : macvlan ng-t46 sur le br-public du netns, MAC ${MAC} (l3vni 4600)" sh -c "ip -n vp-t46 -d -o link show ng-t46 | grep -q 'ng-t46@br-public:.*link/ether ${MAC} .*macvlan mode bridge'"
check "hv1 : entrée FDB locale ${MAC} sur le br-public du netns" sh -c "ip netns exec vp-t46 bridge fdb show br br-public | grep -q '^${MAC} dev br-public master br-public permanent'"
check "hv1 : ng-t46 porte 240.0.0.46/24" sh -c "ip -n vp-t46 -4 -o addr show dev ng-t46 | grep -q ' 240.0.0.46/24 '"
check "hv1 : arp_ignore=1, arp_announce=2, arp_notify=1 sur ng-t46" sh -c "[ \"\$(ip netns exec vp-t46 sysctl -n net.ipv4.conf.ng-t46.arp_ignore net.ipv4.conf.ng-t46.arp_announce net.ipv4.conf.ng-t46.arp_notify | tr '\n' ' ')\" = '1 2 1 ' ]"
check "hv1 : route par défaut du netns vers les bordures par ng-t46" sh -c "ip -n vp-t46 -4 route show default | grep -q '^default via 240.0.0.1 dev ng-t46'"
check "hv1 : route du VPC toujours vers la VRF" sh -c "ip -n vp-t46 -4 route show 10.46.0.0/16 | grep -q 'via 169.254.0.1 dev l3-t46-i'"
check "hv1 : table 4600 : route par défaut vers le netns (169.254.0.2)" sh -c "ip -4 route show table 4600 default | grep -q '^default via 169.254.0.2 dev l3-t46-o'"
check "hv1 : table 4600 : unreachable default toujours là" sh -c "ip -4 route show table 4600 default | grep -q '^unreachable default metric 4278198272'"
check "hv1 : SNAT vers 240.0.0.46 en sortie par ng-t46" sh -c "ip netns exec vp-t46 nft list chain ip two snat-ng-t46 | grep -q 'oifname \"ng-t46\" snat to 240.0.0.46'"
for chain in fwd in; do
    check "hv1 : ${chain}-ng-t46 n'accepte par ng-t46 que les connexions établies" sh -c "ip netns exec vp-t46 nft list chain ip two ${chain}-ng-t46 | grep -q 'iifname \"ng-t46\" ct state established,related accept' && ip netns exec vp-t46 nft list chain ip two ${chain}-ng-t46 | grep -q 'iifname \"ng-t46\" drop'"
done
S=$(mktemp -d)
sniff_start "${S}/public" vp-t46-e 12 icmp-from 240.0.0.46
sniff_start "${S}/private" vp-t46-e 12 icmp-from 10.46.1.10
for i in $(seq 1 10); do vm_ssh vp-t46 10.46.1.10 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t46-a (hôte de la passerelle) joint 240.255.0.1 derrière les bordures" vm_ssh vp-t46 10.46.1.10 'ping -n -c 3 -W 2 240.255.0.1'
wait
info "ICMP sortis du netns par vp-t46-e : depuis 240.0.0.46 $(cat "${S}/public"), depuis 10.46.1.10 $(cat "${S}/private")"
check "témoin : l'ICMP de t46-a sort du netns, source 240.0.0.46" test "$(cat "${S}/public")" -gt 0
check "aucun ICMP ne sort du netns avec l'adresse privée 10.46.1.10" test "$(cat "${S}/private")" -eq 0
rm -r "${S}"
NODE

on rr1 <<'NODE'
check "rr1 : RT-5 0.0.0.0/0 de hv1 (RD 192.168.14.11:4600) reçue et réfléchie" sh -c "vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.11:4600 type prefix' | grep -q '\[0.0.0.0\]'"
check "rr1 : aucune RT-5 0.0.0.0/0 de hv2 (RD 192.168.14.12:4600) : son unreachable default n'est pas annoncée" sh -c "! vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.12:4600 type prefix' | grep -q '\[0.0.0.0\]'"
NODE

on hv2 <<'NODE'
check "hv2 : vrf-t46 apprend la route par défaut via le VTEP de hv1" wait_vrf_route vp-t46 default 192.168.14.11
check "hv2 : aucune interface ng-t46 sur un hôte sans passerelle" sh -c "! ip -n vp-t46 link show ng-t46 >/dev/null 2>&1"
check "hv2 : route par défaut du netns vers la VRF" sh -c "ip -n vp-t46 -4 route show default | grep -q '^default via 169.254.0.1 dev l3-t46-i'"
for i in $(seq 1 10); do vm_ssh vp-t46 10.46.2.10 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t46-b (hv2) joint 240.255.0.1 par la passerelle de hv1" vm_ssh vp-t46 10.46.2.10 'ping -n -c 3 -W 2 240.255.0.1'
NODE

on hv3 <<'NODE'
NS=t46-pub
MAC=02:04:00:00:11:f8
check "hv3 : ${MAC} apprise en type 2 sur le VNI 1 via hv1" wait_evpn_mac 1 "${MAC}" 192.168.14.11
check "hv3 : ${NS} ne joint pas 240.0.0.46 (entrée neuve refusée)" sh -c "! ip netns exec ${NS} ping -n -c 3 -W 1 240.0.0.46 >/dev/null 2>&1"
check "hv3 : 240.0.0.46 résolue vers ${MAC} (l'ARP passe, seul l'IP est filtré)" sh -c "ip -n ${NS} neigh show 240.0.0.46 | grep -q 'lladdr ${MAC}'"
ip -n "${NS}" route add 10.46.0.0/16 via 240.0.0.46
check "hv3 : ${NS} ne joint pas t46-a à travers la passerelle (transit neuf refusé)" sh -c "! ip netns exec ${NS} ping -n -c 3 -W 1 10.46.1.10 >/dev/null 2>&1"
NODE

on hv3 <<'NODE'
check "hv3 : VPC vp-t46c, l3vni 4601, sans subnet" vpc_create vp-t46c 10.47.0.0/16 4601
check "hv3 : passerelle ng-t46c de vp-t46c" api_post nat-gateways '{"name":"ng-t46c","vpc":"vp-t46c","ip":"240.0.0.47/24","gateway":"240.0.0.1"}'
check "hv3 : ng-t46c running" wait_state nat-gateways/ng-t46c running
check "hv3 : suppression de vp-t46c refusée tant que sa passerelle existe (409)" expect_code 409 api_code DELETE vpcs/vp-t46c
check "hv3 : le refus nomme la passerelle" sh -c "curl -s -X DELETE ${API}/vpcs/vp-t46c | grep -q 'nat gateway .*ng-t46c.* must be deleted'"
check "hv3 : VPC vp-t46f sans l3vni" vpc_create vp-t46f 10.48.0.0/16
check "hv3 : passerelle refusée pour un VPC sans l3vni (409)" expect_code 409 api_post_code nat-gateways '{"name":"ng-t46f","vpc":"vp-t46f","ip":"240.0.0.48/24","gateway":"240.0.0.1"}'
check "hv3 : passerelle refusée pour un VPC absent de l'hôte (409)" expect_code 409 api_post_code nat-gateways '{"name":"ng-t46z","vpc":"vp-t46z","ip":"240.0.0.49/24","gateway":"240.0.0.1"}'
check "hv3 : ip sans longueur de préfixe refusée (400)" expect_code 400 api_post_code nat-gateways '{"name":"ng-t46y","vpc":"vp-t46c","ip":"240.0.0.50","gateway":"240.0.0.1"}'
check "hv3 : suppression de ng-t46c" api_delete nat-gateways/ng-t46c
check "hv3 : ng-t46c supprimée" wait_state nat-gateways/ng-t46c deleted
check "hv3 : vp-t46c supprimable une fois la passerelle partie" api_delete vpcs/vp-t46c
check "hv3 : vp-t46c supprimé" wait_state vpcs/vp-t46c deleted
NODE

on hv1 <<'NODE'
check "hv1 : watchdog muet sur la route par défaut de vrf-t46" watchdog_quiet_about 120 '0.0.0.0/0'
check "hv1 : bascule, suppression de ng-t46" api_delete nat-gateways/ng-t46
check "hv1 : ng-t46 supprimée" wait_state nat-gateways/ng-t46 deleted
check "hv1 : plus d'interface ng-t46" sh -c "! ip -n vp-t46 link show ng-t46 >/dev/null 2>&1"
check "hv1 : plus d'entrée FDB 02:04:00:00:11:f8 sur le br-public du netns" sh -c "! ip netns exec vp-t46 bridge fdb show br br-public | grep -q '^02:04:00:00:11:f8 '"
check "hv1 : plus de chaîne de la passerelle dans la table ip two du netns" sh -c "! ip netns exec vp-t46 nft list table ip two | grep -q 'ng-t46'"
check "hv1 : route par défaut du netns revenue vers la VRF" sh -c "ip -n vp-t46 -4 route show default | grep -q '^default via 169.254.0.1 dev l3-t46-i'"
check "hv1 : table 4600 : plus de route par défaut vers le netns" sh -c "! ip -4 route show table 4600 default | grep -q 'via 169.254.0.2'"
check "hv1 : table 4600 : l'unreachable default a survécu au retrait" sh -c "ip -4 route show table 4600 default | grep -q '^unreachable default metric 4278198272'"
check "sans passerelle, t46-a ne sort plus" vm_fails vp-t46 10.46.1.10 'ping -n -c 2 -W 2 240.255.0.1'
NODE

on hv2 <<'NODE'
check "hv2 : bascule, passerelle ng-t46 recréée sur hv2, même IP" api_post nat-gateways '{"name":"ng-t46","vpc":"vp-t46","ip":"240.0.0.46/24","gateway":"240.0.0.1"}'
check "hv2 : ng-t46 running" wait_state nat-gateways/ng-t46 running
check "hv2 : même MAC 02:04:00:00:11:f8 qu'avant la bascule" sh -c "ip -n vp-t46 -o link show ng-t46 | grep -q 'link/ether 02:04:00:00:11:f8 '"
NODE

on hv3 <<'NODE'
check "hv3 : 02:04:00:00:11:f8 a suivi la passerelle, apprise via hv2" wait_evpn_mac 1 02:04:00:00:11:f8 192.168.14.12
NODE

on hv2 <<'NODE'
for i in $(seq 1 15); do vm_ssh vp-t46 10.46.2.10 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t46-b (hôte de la passerelle désormais) joint 240.255.0.1" vm_ssh vp-t46 10.46.2.10 'ping -n -c 3 -W 2 240.255.0.1'
NODE

on hv1 <<'NODE'
check "hv1 : vrf-t46 apprend la route par défaut via le VTEP de hv2" wait_vrf_route vp-t46 default 192.168.14.12
START=$(date +%s)
for i in $(seq 1 30); do vm_ssh vp-t46 10.46.1.10 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
info "hv1 : sortie de t46-a rétablie $(( $(date +%s) - START )) s après l'import de la route par défaut de hv2"
check "t46-a (hv1) sort par la passerelle de hv2 après la bascule" vm_ssh vp-t46 10.46.1.10 'ping -n -c 3 -W 2 240.255.0.1'
NODE
