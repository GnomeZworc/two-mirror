# fonctionnalité: subnet public_ip — VM qui porte son IP publique, subnet raccordé au bridge public de l'hôte (iface_type ignoré), même plage et même interface_ip pour plusieurs VPC sur un même hôte et sur deux hôtes, interface_ip invisible sur le segment, gateway et DNS propres à chaque VPC, sortie et entrée sans NAT, usurpation refusée, IP flottante refusée
# hôtes: hv1 hv2 hv3
# tickets: #89

on hv1 <<'NODE'
KEY=$(vm_key)
pub () { api_post subnets "{\"name\":\"${1}\",\"vpc\":\"${2}\",\"mode\":\"public_ip\",\"iface_type\":\"vms\",\"cidr\":\"240.0.0.0/24\",\"interface_ip\":\"240.0.0.250\",\"default_route\":true,\"gateway\":\"240.0.0.1\"}" && wait_state "subnets/${1}" running; }
check "hv1 : VPC vp-t48a" vpc_create vp-t48a 10.48.0.0/16
check "hv1 : VPC vp-t48b" vpc_create vp-t48b 10.49.0.0/16
check "hv1 : subnet public sn-t48a de vp-t48a, 240.0.0.0/24, interface_ip 240.0.0.250" pub sn-t48a vp-t48a
check "hv1 : subnet public sn-t48b de vp-t48b, même plage, même interface_ip" pub sn-t48b vp-t48b
check "hv1 : sn-t48a raccordé au bridge public malgré iface_type vms" test "$(api_field subnets/sn-t48a local_iface)" = br-public
check "hv1 : v-t48a-e et v-t48b-e esclaves de br-public" sh -c "ip -o link show v-t48a-e | grep -q 'master br-public' && ip -o link show v-t48b-e | grep -q 'master br-public'"
for v in a b; do
    check "hv1 : 240.0.0.250 sur br-t48${v} dans vp-t48${v}" sh -c "ip -n vp-t48${v} -4 -o addr show dev br-t48${v} | grep -q ' 240.0.0.250/32 '"
done
check "hv1 : VM t48-a 240.0.0.80 dans vp-t48a" vm_create t48-a sn-t48a 240.0.0.80 "${KEY}"
check "hv1 : VM t48-b 240.0.0.81 dans vp-t48b" vm_create t48-b sn-t48b 240.0.0.81 "${KEY}"
check "t48-a joignable" vm_wait vp-t48a 240.0.0.80
check "t48-b joignable" vm_wait vp-t48b 240.0.0.81
check "t48-a : route par défaut vers la bordure 240.0.0.1" route_via vp-t48a 240.0.0.80 default 240.0.0.1
check "t48-a : route du VPC 10.48.0.0/16 via interface_ip" route_via vp-t48a 240.0.0.80 10.48.0.0/16 240.0.0.250
check "t48-a : DNS et NTP annoncés = 240.0.0.250" sh -c "[ \"\$1\" = 240.0.0.250 ] && [ \"\$2\" = 240.0.0.250 ]" _ "$(vm_lease vp-t48a 240.0.0.80 DNS)" "$(vm_lease vp-t48a 240.0.0.80 NTP)"
vm_ssh vp-t48a 240.0.0.80 'ping -n -c 1 -W 1 240.0.0.250' >/dev/null 2>&1
vm_ssh vp-t48b 240.0.0.81 'ping -n -c 1 -W 1 240.0.0.250' >/dev/null 2>&1
MA=$(ip -n vp-t48a -o link show br-t48a | grep -o 'link/ether [0-9a-f:]*' | cut -d' ' -f2)
MB=$(ip -n vp-t48b -o link show br-t48b | grep -o 'link/ether [0-9a-f:]*' | cut -d' ' -f2)
info "MAC de la gateway : vp-t48a ${MA}, vp-t48b ${MB}"
check "témoin : les deux gateways 240.0.0.250 ont des MAC distinctes" sh -c "[ -n \"\$1\" ] && [ \"\$1\" != \"\$2\" ]" _ "${MA}" "${MB}"
check "t48-a résout 240.0.0.250 vers la gateway de son VPC" test "$(vm_neigh_mac vp-t48a 240.0.0.80 240.0.0.250)" = "${MA}"
check "t48-b résout 240.0.0.250 vers la gateway de son VPC" test "$(vm_neigh_mac vp-t48b 240.0.0.81 240.0.0.250)" = "${MB}"
S=$(mktemp -d)
sniff_start "${S}/a" v-t48a-e 10 icmp-from 240.0.0.80
for i in $(seq 1 10); do vm_ssh vp-t48a 240.0.0.80 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t48-a joint 240.255.0.1 derrière les bordures" vm_ssh vp-t48a 240.0.0.80 'ping -n -c 3 -W 2 240.255.0.1'
wait
check "t48-a sort avec son IP publique 240.0.0.80, sans NAT" test "$(cat "${S}/a")" -gt 0
sniff_start "${S}/spoof" v-t48b-e 10 arp-from 240.0.0.80
sniff_start "${S}/own" v-t48b-e 10 arp-from 240.0.0.81
vm_frames vp-t48b 240.0.0.81 garp ens3 240.0.0.80 3 >/dev/null 2>&1
vm_frames vp-t48b 240.0.0.81 garp ens3 240.0.0.81 3 >/dev/null 2>&1
wait
info "ARP émis par t48-b vus sur v-t48b-e : au nom de 240.0.0.80 $(cat "${S}/spoof"), au sien $(cat "${S}/own")"
check "témoin : l'ARP de t48-b pour sa propre adresse atteint le segment" test "$(cat "${S}/own")" -gt 0
check "t48-b ne peut pas annoncer l'adresse de t48-a sur le segment" test "$(cat "${S}/spoof")" -eq 0
rm -r "${S}"
check "IP flottante refusée sur une interface public_ip (409)" expect_code 409 api_post_code floating-ips '{"name":"fip-t48","vm":"t48-a","interface":"nic-0","ip":"240.0.0.90/24","gateway":"240.0.0.1"}'
NODE

on hv2 <<'NODE'
KEY=$(vm_key)
check "hv2 : VPC vp-t48c" vpc_create vp-t48c 10.50.0.0/16
pub () { api_post subnets "{\"name\":\"${1}\",\"vpc\":\"${2}\",\"mode\":\"public_ip\",\"cidr\":\"240.0.0.0/24\",\"interface_ip\":\"240.0.0.250\",\"default_route\":true,\"gateway\":\"240.0.0.1\"}" && wait_state "subnets/${1}" running; }
check "hv2 : subnet public sn-t48c, même plage, même interface_ip, sans iface_type" pub sn-t48c vp-t48c
check "hv2 : VM t48-c 240.0.0.82" vm_create t48-c sn-t48c 240.0.0.82 "${KEY}"
check "t48-c joignable" vm_wait vp-t48c 240.0.0.82
for i in $(seq 1 10); do vm_ssh vp-t48c 240.0.0.82 'ping -n -c 1 -W 2 240.255.0.1' >/dev/null 2>&1 && break; sleep 2; done
check "t48-c joint 240.255.0.1 derrière les bordures" vm_ssh vp-t48c 240.0.0.82 'ping -n -c 3 -W 2 240.255.0.1'
check "t48-c (hv2) joint t48-a (hv1) sur le segment public, sans NAT" vm_ssh vp-t48c 240.0.0.82 'ping -n -c 3 -W 2 240.0.0.80'
check "t48-c résout 240.0.0.250 vers la gateway de son VPC" test "$(vm_neigh_mac vp-t48c 240.0.0.82 240.0.0.250)" = "$(ip -n vp-t48c -o link show br-t48c | grep -o 'link/ether [0-9a-f:]*' | cut -d' ' -f2)"
NODE

on hv3 <<'NODE'
NS=t48-pub
ip netns add "${NS}"
ip link add t48-pub-h type veth peer name t48-pub-n netns "${NS}"
ip link set t48-pub-h master br-public up
ip -n "${NS}" link set t48-pub-n up
ip -n "${NS}" addr add 240.0.0.148/24 dev t48-pub-n
check "hv3 : témoin, ${NS} joint la gateway des bordures" ip netns exec "${NS}" ping -n -c 3 -W 1 240.0.0.1
check "hv3 : ${NS} joint t48-a (hv1) par son IP publique" ip netns exec "${NS}" ping -n -c 3 -W 2 240.0.0.80
check "hv3 : ${NS} joint t48-c (hv2) par son IP publique" ip netns exec "${NS}" ping -n -c 3 -W 2 240.0.0.82
R=$(ns_frames "${NS}" arp-who t48-pub-n 240.0.0.148 240.0.0.80 2)
check "hv3 : témoin, 240.0.0.80 répond à l'ARP depuis le segment (${R:-personne})" test -n "${R}"
R=$(ns_frames "${NS}" arp-who t48-pub-n 240.0.0.148 240.0.0.250 2)
check "hv3 : personne ne répond à l'ARP pour interface_ip 240.0.0.250 sur le segment (${R:-personne})" test -z "${R}"
NODE
