# fonctionnalité: IP flottante d'une interface de VM — macvlan fip-<hex> sur le br-public du netns, MAC 02:05 + IP publique, NAT 1:1 sans état, routage par source (sortie par l'IP flottante même dans un VPC qui a une passerelle NAT), entrée neuve depuis le segment public, métadonnées servies, affichage sur la VM, refus 409, détachement, déplacement vers un autre hôte par mobilité MAC
# hôtes: hv1 hv2 hv3
# tickets: #88

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t47, l3vni 4700" vpc_create vp-t47 10.47.0.0/16 4700
check "hv1 : subnet sn-t47a (VNI 4711)" subnet_create sn-t47a vp-t47 4711 10.47.1.0/24
check "hv1 : VM t47-a, interface nic-0" vm_create t47-a sn-t47a 10.47.1.10 "${KEY}"
check "hv1 : VM t47-c, sans IP flottante" vm_create t47-c sn-t47a 10.47.1.11 "${KEY}"
check "t47-a joignable" vm_wait vp-t47 10.47.1.10
check "t47-c joignable" vm_wait vp-t47 10.47.1.11
check "hv1 : passerelle NAT ng-t47 du VPC (240.0.0.57)" api_post nat-gateways '{"name":"ng-t47","vpc":"vp-t47","ip":"240.0.0.57/24","gateway":"240.0.0.1"}'
check "hv1 : ng-t47 running" wait_state nat-gateways/ng-t47 running
check "hv1 : IP flottante sur une interface inconnue refusée (409)" expect_code 409 api_post_code floating-ips '{"name":"fip-t47x","vm":"t47-a","interface":"nic-9","ip":"240.0.0.48/24","gateway":"240.0.0.1"}'
check "hv1 : IP flottante fip-t47 (240.0.0.47) sur nic-0 de t47-a" api_post floating-ips '{"name":"fip-t47","vm":"t47-a","interface":"nic-0","ip":"240.0.0.47/24","gateway":"240.0.0.1"}'
check "hv1 : fip-t47 running" wait_state floating-ips/fip-t47 running
check "hv1 : seconde IP flottante sur nic-0 refusée (409)" expect_code 409 api_post_code floating-ips '{"name":"fip-t47y","vm":"t47-a","interface":"nic-0","ip":"240.0.0.49/24","gateway":"240.0.0.1"}'
check "hv1 : GET de t47-a affiche fip-t47 sur nic-0" sh -c "curl -fsS ${API}/vms/t47-a | jq -e '.interfaces[0] | .name == \"nic-0\" and .floating_ip.name == \"fip-t47\" and .floating_ip.ip == \"240.0.0.47/24\"'"
check "hv1 : GET de t47-c sans floating_ip" sh -c "curl -fsS ${API}/vms/t47-c | jq -e '.interfaces[0] | has(\"floating_ip\") | not'"
check "hv1 : suppression de t47-a refusée tant que fip-t47 est attachée (409)" expect_code 409 api_code DELETE vms/t47-a
check "hv1 : macvlan fip-f000002f sur le br-public du netns, MAC 02:05:f0:00:00:2f" sh -c "ip -n vp-t47 -d -o link show fip-f000002f | grep -q 'fip-f000002f@br-public:.*link/ether 02:05:f0:00:00:2f .*macvlan mode bridge'"
check "hv1 : entrée FDB locale 02:05:f0:00:00:2f" sh -c "ip netns exec vp-t47 bridge fdb show br br-public | grep -q '^02:05:f0:00:00:2f dev br-public master br-public permanent'"
check "hv1 : 240.0.0.47 posée en /32" sh -c "ip -n vp-t47 -4 -o addr show dev fip-f000002f | grep -q ' 240.0.0.47/32 '"
check "hv1 : règle 100, trafic vers le VPC dans main" sh -c "ip -n vp-t47 rule show | grep -q '^100:.*from 10.47.1.10 to 10.47.0.0/16 lookup main'"
check "hv1 : règle 101 vers la table 170852618" sh -c "ip -n vp-t47 rule show | grep -q '^101:.*from 10.47.1.10 lookup 170852618'"
check "hv1 : table 170852618 : segment et route par défaut par fip-f000002f" sh -c "ip -n vp-t47 route show table 170852618 | grep -q '^default via 240.0.0.1 dev fip-f000002f' && ip -n vp-t47 route show table 170852618 | grep -q '^240.0.0.0/24 dev fip-f000002f'"
check "hv1 : main ne porte qu'une route du segment, celle de la passerelle" sh -c "[ \"\$(ip -n vp-t47 -4 route show 240.0.0.0/24 | grep -c .)\" = 1 ] && ip -n vp-t47 -4 route show 240.0.0.0/24 | grep -q ' dev ng-t47 '"
check "hv1 : traduction à l'entrée et à la sortie dans ip two" sh -c "ip netns exec vp-t47 nft list chain ip two pre-fip-f000002f | grep -q 'ip daddr 240.0.0.47 ip daddr set 10.47.1.10' && ip netns exec vp-t47 nft list chain ip two post-fip-f000002f | grep -q 'ip saddr 10.47.1.10 ip saddr set 240.0.0.47'"
S=$(mktemp -d)
sniff_start "${S}/fip" vp-t47-e 12 icmp-from 240.0.0.47
sniff_start "${S}/ng" vp-t47-e 12 icmp-from 240.0.0.57
sniff_start "${S}/private" vp-t47-e 12 icmp-from 10.47.1.10
for i in $(seq 1 10); do vm_ssh vp-t47 10.47.1.10 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t47-a joint 240.255.0.1 derrière les bordures" vm_ssh vp-t47 10.47.1.10 'ping -n -c 3 -W 2 240.255.0.1'
wait
info "ICMP sortis du netns pendant le ping de t47-a : source 240.0.0.47 $(cat "${S}/fip"), 240.0.0.57 $(cat "${S}/ng"), 10.47.1.10 $(cat "${S}/private")"
check "t47-a sort avec son IP flottante 240.0.0.47" test "$(cat "${S}/fip")" -gt 0
check "t47-a ne sort pas par la passerelle NAT du VPC" test "$(cat "${S}/ng")" -eq 0
check "aucun ICMP de t47-a ne sort avec son adresse privée" test "$(cat "${S}/private")" -eq 0
sniff_start "${S}/ng-c" vp-t47-e 12 icmp-from 240.0.0.57
vm_ssh vp-t47 10.47.1.11 'ping -n -c 3 -W 2 240.255.0.1' >/dev/null 2>&1
wait
check "témoin : t47-c, sans IP flottante, sort par la passerelle (240.0.0.57)" test "$(cat "${S}/ng-c")" -gt 0
rm -r "${S}"
check "t47-a joint t47-c dans le VPC" vm_ssh vp-t47 10.47.1.10 'ping -n -c 2 -W 2 10.47.1.11'
check "t47-a reçoit toujours ses métadonnées (instance-id t47-a)" sh -c 'case "$1" in *t47-a*) exit 0 ;; esac; echo "meta-data : $1"; exit 1' _ "$(vm_meta vp-t47 10.47.1.10 meta-data)"
NODE

on hv3 <<'NODE'
NS=t47-pub
ip netns add "${NS}"
ip link add t47-pub-h type veth peer name t47-pub-n netns "${NS}"
ip link set t47-pub-h master br-public up
ip -n "${NS}" link set t47-pub-n up
ip -n "${NS}" addr add 240.0.0.147/24 dev t47-pub-n
check "hv3 : témoin, ${NS} joint la gateway des bordures" ip netns exec "${NS}" ping -n -c 3 -W 1 240.0.0.1
check "hv3 : 02:05:f0:00:00:2f apprise en type 2 sur le VNI 1 via hv1" wait_evpn_mac 1 02:05:f0:00:00:2f 192.168.14.11
check "hv3 : ${NS} joint t47-a par son IP flottante (entrée neuve)" ip netns exec "${NS}" ping -n -c 3 -W 2 240.0.0.47
check "hv3 : 240.0.0.47 résolue vers 02:05:f0:00:00:2f" sh -c "ip -n ${NS} neigh show 240.0.0.47 | grep -q 'lladdr 02:05:f0:00:00:2f'"
NODE

on hv1 <<'NODE'
check "hv1 : détachement, suppression de fip-t47" api_delete floating-ips/fip-t47
check "hv1 : fip-t47 supprimée" wait_state floating-ips/fip-t47 deleted
check "hv1 : plus d'interface fip-f000002f" sh -c "! ip -n vp-t47 link show fip-f000002f >/dev/null 2>&1"
check "hv1 : plus de règle pour 10.47.1.10" sh -c "! ip -n vp-t47 rule show | grep -q 'from 10.47.1.10'"
check "hv1 : table 170852618 vide" test -z "$(ip -n vp-t47 route show table 170852618 2>/dev/null)"
check "hv1 : plus d'entrée FDB 02:05:f0:00:00:2f" sh -c "! ip netns exec vp-t47 bridge fdb show br br-public | grep -q '^02:05:f0:00:00:2f '"
check "hv1 : plus de chaîne de l'IP flottante dans ip two" sh -c "! ip netns exec vp-t47 nft list table ip two | grep -q 'fip-f000002f'"
check "hv1 : GET de t47-a sans floating_ip" sh -c "curl -fsS ${API}/vms/t47-a | jq -e '.interfaces[0] | has(\"floating_ip\") | not'"
check "détachée, t47-a sort par la passerelle NAT" vm_ssh vp-t47 10.47.1.10 'ping -n -c 3 -W 2 240.255.0.1'
check "hv1 : déplacement, suppression de t47-a" vm_delete t47-a
NODE

on hv3 <<'NODE'
check "hv3 : détachée, t47-pub ne joint plus 240.0.0.47" sh -c "! ip netns exec t47-pub ping -n -c 3 -W 1 240.0.0.47 >/dev/null 2>&1"
NODE

on hv2 <<'NODE'
KEY=$(vm_key)
check "hv2 : VPC vp-t47, l3vni 4700" vpc_create vp-t47 10.47.0.0/16 4700
check "hv2 : subnet sn-t47a (VNI 4711)" subnet_create sn-t47a vp-t47 4711 10.47.1.0/24
check "hv2 : déplacement, t47-a recréée sur hv2, même adresse" vm_create t47-a sn-t47a 10.47.1.10 "${KEY}"
check "t47-a joignable sur hv2" vm_wait vp-t47 10.47.1.10
check "hv2 : fip-t47 recréée sur hv2, même IP" api_post floating-ips '{"name":"fip-t47","vm":"t47-a","interface":"nic-0","ip":"240.0.0.47/24","gateway":"240.0.0.1"}'
check "hv2 : fip-t47 running" wait_state floating-ips/fip-t47 running
check "hv2 : même MAC 02:05:f0:00:00:2f qu'avant le déplacement" sh -c "ip -n vp-t47 -o link show fip-f000002f | grep -q 'link/ether 02:05:f0:00:00:2f '"
check "t47-a (hv2) sort par son IP flottante" vm_ssh vp-t47 10.47.1.10 'ping -n -c 3 -W 2 240.255.0.1'
NODE

on hv3 <<'NODE'
check "hv3 : 02:05:f0:00:00:2f a suivi la VM, apprise via hv2" wait_evpn_mac 1 02:05:f0:00:00:2f 192.168.14.12
START=$(date +%s)
for i in $(seq 1 30); do ip netns exec t47-pub ping -n -c 1 -W 1 240.0.0.47 >/dev/null 2>&1 && break; sleep 2; done
info "hv3 : 240.0.0.47 joignable $(( $(date +%s) - START )) s après l'apprentissage de la MAC via hv2"
check "hv3 : t47-pub joint t47-a sur hv2 par son IP flottante" ip netns exec t47-pub ping -n -c 3 -W 2 240.0.0.47
NODE
