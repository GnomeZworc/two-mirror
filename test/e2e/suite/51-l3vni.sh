# fonctionnalité: routage inter-subnet entre hôtes par L3VNI (IRB symétrique) — VRF, RMAC, exports /24 et /32, RT-5 réfléchies, TTL, isolation par RT, démontage d'un VPC routé
# hôtes: hv1 hv2
# tickets: #41

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t51, l3vni 5100" vpc_create vp-t51 10.51.0.0/18 5100
check "hv1 : subnet sn-t51d (VNI 5111), seulement sur hv1" subnet_create sn-t51d vp-t51 5111 10.51.1.0/24
check "hv1 : VM t51-a" vm_create t51-a sn-t51d 10.51.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
check "hv2 : VPC vp-t51, l3vni 5100" vpc_create vp-t51 10.51.0.0/18 5100
check "hv2 : subnet sn-t51e (VNI 5112), seulement sur hv2" subnet_create sn-t51e vp-t51 5112 10.51.2.0/24
check "hv2 : VM t51-b" vm_create t51-b sn-t51e 10.51.2.10
check "hv2 : VPC vp-t51b, même plage, l3vni 5101" vpc_create vp-t51b 10.51.0.0/18 5101
check "hv2 : subnet sn-t51x (VNI 5113), même plage que sn-t51e" subnet_create sn-t51x vp-t51b 5113 10.51.2.0/24
check "hv2 : VM t51-x dans vp-t51b" vm_create t51-x sn-t51x 10.51.2.20
NODE
on hv1 <<'NODE'
check "t51-a joignable" vm_wait vp-t51 10.51.1.10
check "t51-a : route vers le VPC 10.51.0.0/18 via la gateway (DHCP)" route_via vp-t51 10.51.1.10 10.51.0.0/18 10.51.1.254
check "hv1 : vrf-t51 contient exactement brl3-t51 et l3-t51-o" sh -c "[ \"\$1\" = 'brl3-t51 l3-t51-o' ]" _ "$(vrf_members vp-t51)"
check "hv1 : brl3-t51 sans adresse IPv4" sh -c "! ip -4 -o addr show dev brl3-t51 | grep -q inet"
check "hv1 : RMAC de brl3-t51 dérivée du VTEP 192.168.14.11" sh -c "ip -o link show brl3-t51 | grep -q 'link/ether 02:02:c0:a8:0e:0b '"
check "hv1 : route du supernet dans vp-t51 vers le transit" sh -c "ip -n vp-t51 -4 route show 10.51.0.0/18 | grep -q 'via 169.254.0.1 dev l3-t51-i'"
check "hv1 : forwarding sur brl3-t51 et l3-t51-o" sh -c "[ \"\$(sysctl -n net.ipv4.conf.brl3-t51.forwarding net.ipv4.conf.l3-t51-o.forwarding | tr '\n' ' ')\" = '1 1 ' ]"
check "hv1 : ip_forward dans le netns vp-t51" sh -c "[ \"\$(ip netns exec vp-t51 sysctl -n net.ipv4.ip_forward)\" = 1 ]"
info "hv1 : ip_forward global de l'hôte : $(sysctl -n net.ipv4.ip_forward) (non modifié par l'agent)"
check "hv1 : vrf-t51 exporte le subnet local 10.51.1.0/24 (agent)" vrf_route_via vp-t51 10.51.1.0/24 169.254.0.2
check "hv1 : vrf-t51 exporte t51-a 10.51.1.10/32 (agent)" vrf_route_via vp-t51 10.51.1.10 169.254.0.2
check "hv1 : session vers le RR en iBGP (internal link)" sh -c "vtysh -c 'show bgp neighbors 10.255.255.1' | grep -q 'internal link'"
NODE
on hv2 <<'NODE'
check "hv2 : t51-b joignable depuis son netns" netns_ping_wait vp-t51 10.51.2.10
check "hv2 : t51-x joignable depuis son netns (témoin)" netns_ping_wait vp-t51b 10.51.2.20
check "hv2 : VNI 5100 de type L3, active, RMAC 02:02:c0:a8:0e:0c" wait_l3vni 5100 02:02:c0:a8:0e:0c
check "hv2 : VNI 5101 de type L3, active, RMAC 02:02:c0:a8:0e:0c" wait_l3vni 5101 02:02:c0:a8:0e:0c
check "hv2 : vrf-t51 apprend 10.51.1.10/32 via le VTEP de hv1" wait_vrf_route vp-t51 10.51.1.10 192.168.14.11
check "hv2 : vrf-t51 apprend 10.51.1.0/24 via le VTEP de hv1" wait_vrf_route vp-t51 10.51.1.0/24 192.168.14.11
check "hv2 : RMAC de hv1 dans la FDB de vxl3-t51" sh -c "bridge fdb show dev vxl3-t51 | grep -q '02:02:c0:a8:0e:0b dst 192.168.14.11'"
NODE
on rr1 <<'NODE'
check "rr1 : RT-5 10.51.1.10/32 de hv1 (RD 192.168.14.11:5100) reçue et réfléchie" sh -c "vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.11:5100 type prefix' | grep -q '\[10.51.1.10\]'"
check "rr1 : RT-5 10.51.2.20/32 de vp-t51b (RD 192.168.14.12:5101) présente (témoin du test négatif)" sh -c "vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.12:5101 type prefix' | grep -q '\[10.51.2.20\]'"
NODE
on hv1 <<'NODE'
check "hv1 : VNI 5100 de type L3, active, RMAC 02:02:c0:a8:0e:0b" wait_l3vni 5100 02:02:c0:a8:0e:0b
check "hv1 : vrf-t51 apprend 10.51.2.10/32 via le VTEP de hv2" wait_vrf_route vp-t51 10.51.2.10 192.168.14.12
check "hv1 : aucun VXLAN de sn-t51e (VNI 5112) sur hv1" sh -c "! ip link show vxlan-5112 >/dev/null 2>&1"
for i in $(seq 1 24); do vm_ssh vp-t51 10.51.1.10 'ping -c 1 -W 2 10.51.2.10' >/dev/null 2>&1 && break; sleep 5; done
check "t51-a (sn-t51d, hv1) joint t51-b (sn-t51e, hv2) par le L3VNI, forwarding par interface" vm_ssh vp-t51 10.51.1.10 'ping -c 3 -W 2 10.51.2.10'
check "trame pleine taille : 1472 octets en -M do" vm_ssh vp-t51 10.51.1.10 'ping -M do -s 1472 -c 2 -W 2 10.51.2.10'
check "1473 octets refusés localement (MTU 1500)" vm_grep vp-t51 10.51.1.10 'ping -M do -s 1473 -c 1 -W 2 10.51.2.10' 'message too long'
check "TTL 1 : expire à la gateway du subnet, netns de hv1 (10.51.1.254)" vm_grep vp-t51 10.51.1.10 'ping -n -c 3 -i 1.2 -W 2 -t 1 10.51.2.10' 'From 10.51.1.254 .*exceeded'
check "TTL 2 : expire dans la VRF de hv1 (169.254.0.1)" vm_grep vp-t51 10.51.1.10 'ping -n -c 3 -i 1.2 -W 2 -t 2 10.51.2.10' 'From 169.254.0.1 .*exceeded'
check "TTL 5 : atteint t51-b — quatre routeurs : gateway hv1, VRF hv1, VRF hv2, netns hv2" vm_ssh vp-t51 10.51.1.10 'ping -n -c 1 -W 2 -t 5 10.51.2.10'
check "hv1 : vrf-t51 n'importe pas 10.51.2.20/32 de vp-t51b (RT 64600:5101)" sh -c "[ -z \"\$(ip -4 route show vrf vrf-t51 10.51.2.20)\" ]"
check "t51-a ne joint pas t51-x, autre VPC, même plage" vm_fails vp-t51 10.51.1.10 'ping -c 3 -W 2 10.51.2.20'
NODE
on hv2 <<'NODE'
BGPD=$(pidof bgpd)
check "hv2 : suppression de sn-t51x refusée tant que t51-x existe (409)" test "$(api_code DELETE subnets/sn-t51x)" = 409
check "hv2 : suppression de t51-x" vm_delete t51-x
check "hv2 : vrf-t51b ne porte plus le /32 de t51-x" test -z "$(ip -4 route show vrf vrf-t51b 10.51.2.20/32)"
check "hv2 : suppression de sn-t51x" api_delete subnets/sn-t51x
check "hv2 : sn-t51x supprimé" wait_state subnets/sn-t51x deleted
check "hv2 : suppression de vp-t51b" api_delete vpcs/vp-t51b
check "hv2 : vp-t51b supprimé" wait_state vpcs/vp-t51b deleted
check "hv2 : plus aucune interface de vp-t51b" sh -c "! ip -o link show | grep -qE '(vrf|brl3|vxl3)-t51b|l3-t51b-o'"
check "hv2 : plus rien de vrf-t51b dans FRR, en mémoire ni dans frr.conf" sh -c "! vtysh -c 'show running-config' | grep -q 'vrf-t51b' && ! grep -q 'vrf-t51b' /etc/frr/frr.conf"
check "hv2 : bgpd n'a pas redémarré pendant le démontage" sh -c "[ \"\$(pidof bgpd)\" = '${BGPD}' ]"
check "hv2 : vp-t51 toujours en service" wait_l3vni 5100 02:02:c0:a8:0e:0c
NODE
