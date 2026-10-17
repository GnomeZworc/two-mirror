# fonctionnalité: gateway d'un subnet vxlan — gateway anycast et adresse d'émission calculées, default_route et gateway explicite, routes DHCP, macvlan gw-, FDB locale, sysctls ARP, règles de confinement, suppression et recréation
# hôtes: hv1
# tickets: #57, #43

on hv1 <<'NODE'
KEY=$(vm_key)
MAC=$(anycast_mac 3101)
check "VPC vp-t31" vpc_create vp-t31 10.31.0.0/16
check "subnet sn-t31a, default_route absent" subnet_create sn-t31a vp-t31 3101 10.31.1.0/24
check "subnet sn-t31b, default_route et gateway 10.31.2.1" subnet_create sn-t31b vp-t31 3102 10.31.2.0/24 ',"default_route":true,"gateway":"10.31.2.1"'
check "sn-t31a : gateway calculée 10.31.1.254 et adresse d'émission 10.31.1.253 (GET)" sh -c "curl -fsS ${API}/subnets/sn-t31a | jq -e '.interface_ip == \"10.31.1.254\" and .relay_ip == \"10.31.1.253\"'"
check "sn-t31a : macvlan gw-t31a, MAC anycast ${MAC}" sh -c "ip -n vp-t31 -o link show gw-t31a | grep -qi 'link/ether ${MAC} '"
check "sn-t31a : gateway 10.31.1.254/32 sur gw-t31a" sh -c "ip -n vp-t31 -4 -o addr show dev gw-t31a | grep -q ' 10.31.1.254/32 '"
check "sn-t31a : adresse d'émission 10.31.1.253/32 sur br-t31a" sh -c "ip -n vp-t31 -4 -o addr show dev br-t31a | grep -q ' 10.31.1.253/32 '"
check "sn-t31a : la gateway n'est pas sur br-t31a" sh -c "! ip -n vp-t31 -4 -o addr show dev br-t31a | grep -q ' 10.31.1.254/'"
check "sn-t31a : MAC anycast en entrée FDB locale permanente de br-t31a" sh -c 'ip netns exec vp-t31 bridge fdb show br br-t31a | grep -qi "^$1 dev br-t31a master br-t31a permanent"' _ "${MAC}"
check "sn-t31a : route du subnet émise depuis la gateway" sh -c "ip -n vp-t31 -4 route show 10.31.1.0/24 | grep -q 'src 10.31.1.254'"
for dev in br-t31a gw-t31a; do
    check "sn-t31a : ${dev}, arp_ignore=1 et arp_announce=2" sh -c "[ \"\$(ip netns exec vp-t31 sysctl -n net.ipv4.conf.${dev}.arp_ignore) \$(ip netns exec vp-t31 sysctl -n net.ipv4.conf.${dev}.arp_announce)\" = '1 2' ]"
done
check "sn-t31a : confinement de la MAC anycast sur v-t31a-i (nat PREROUTING, FORWARD, OUTPUT)" test "$(ebt_confined vp-t31 v-t31a-i "${MAC}")" -eq 3
for ip in 10.31.1.254 10.31.1.253; do
    check "sn-t31a : requêtes ARP vers ${ip} bloquées en sortie de v-t31a-i" ebt_arp_drop vp-t31 v-t31a-i "${ip}"
    check "sn-t31a : DHCP émis depuis ${ip} bloqué en sortie de v-t31a-i" ebt_dhcp_drop vp-t31 v-t31a-i "${ip}"
done
check "VM t31-a" vm_create t31-a sn-t31a 10.31.1.10 "${KEY}"
check "VM t31-b" vm_create t31-b sn-t31b 10.31.2.10 "${KEY}"
check "t31-a joignable" vm_wait vp-t31 10.31.1.10
check "t31-b joignable" vm_wait vp-t31 10.31.2.10
check "sn-t31a : route par défaut via la gateway 10.31.1.254" route_via vp-t31 10.31.1.10 default 10.31.1.254
check "sn-t31a : route vers le VPC via la gateway" route_via vp-t31 10.31.1.10 10.31.0.0/16 10.31.1.254
check "sn-t31a : route /32 vers les métadonnées via la gateway" route_via vp-t31 10.31.1.10 169.254.169.254 10.31.1.254
check "sn-t31b : route par défaut via la gateway explicite 10.31.2.1" route_via vp-t31 10.31.2.10 default 10.31.2.1
check "sn-t31b : route vers le VPC via la gateway du subnet, pas via la gateway explicite" route_via vp-t31 10.31.2.10 10.31.0.0/16 10.31.2.254
check "sn-t31b : route /32 vers les métadonnées via la gateway du subnet" route_via vp-t31 10.31.2.10 169.254.169.254 10.31.2.254
check "t31-a résout sa gateway vers la MAC anycast ${MAC}" vm_grep vp-t31 10.31.1.10 'ping -n -c 1 -W 2 10.31.1.254 >/dev/null; ip neigh show 10.31.1.254' "lladdr ${MAC} "
check "le netns émet vers t31-a depuis la gateway 10.31.1.254" sh -c "ip -n vp-t31 -4 route get 10.31.1.10 | grep -q 'src 10.31.1.254 '"
check "suppression de t31-a" vm_delete t31-a
check "suppression de sn-t31a" api_delete subnets/sn-t31a
check "sn-t31a supprimé" wait_state subnets/sn-t31a deleted
check "sn-t31a : plus de macvlan gw-t31a dans le netns" sh -c "! ip -n vp-t31 link show gw-t31a >/dev/null 2>&1"
check "sn-t31a : plus aucune règle ebtables pour v-t31a-i" test -z "$(ebt_rules vp-t31 v-t31a-i)"
check "sn-t31b : règles de son propre subnet toujours présentes (témoin)" test -n "$(ebt_rules vp-t31 v-t31b-i)"
check "sn-t31a recréé" subnet_create sn-t31a vp-t31 3101 10.31.1.0/24
check "VM t31-a2 sur le subnet recréé" vm_create t31-a2 sn-t31a 10.31.1.11 "${KEY}"
check "t31-a2 joignable" vm_wait vp-t31 10.31.1.11
check "après recréation : route par défaut via la gateway 10.31.1.254" route_via vp-t31 10.31.1.11 default 10.31.1.254
check "après recréation : route vers le VPC via la gateway" route_via vp-t31 10.31.1.11 10.31.0.0/16 10.31.1.254
check "après recréation : route /32 vers les métadonnées" route_via vp-t31 10.31.1.11 169.254.169.254 10.31.1.254
check "après recréation : MAC anycast en FDB locale de br-t31a" sh -c 'ip netns exec vp-t31 bridge fdb show br br-t31a | grep -qi "^$1 dev br-t31a master br-t31a permanent"' _ "${MAC}"
NODE
