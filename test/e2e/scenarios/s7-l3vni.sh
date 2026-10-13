on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv1)"; exit 0; }
KEY=$(vm_key)
check "hv1 : base BGP posée par l'agent et enregistrée dans frr.conf" grep -qx ' neighbor 10.255.255.1 peer-group fabric' /etc/frr/frr.conf
check "hv1 : VPC vp-s7, l3vni 2700" vpc_create vp-s7 10.241.0.0/16 2700
check "hv1 : subnet sn-s7d (VNI 2711), seulement sur hv1" subnet_create sn-s7d vp-s7 2711 10.241.1.0/24
check "hv1 : VM s7-a" vm_create s7-a sn-s7d 10.241.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv2)"; exit 0; }
check "hv2 : VPC vp-s7, l3vni 2700" vpc_create vp-s7 10.241.0.0/16 2700
check "hv2 : subnet sn-s7e (VNI 2712), seulement sur hv2" subnet_create sn-s7e vp-s7 2712 10.241.2.0/24
check "hv2 : VM s7-b" vm_create s7-b sn-s7e 10.241.2.10
check "hv2 : VPC vp-s7b, même plage, l3vni 2701" vpc_create vp-s7b 10.241.0.0/16 2701
check "hv2 : subnet sn-s7x (VNI 2713), même plage que sn-s7e" subnet_create sn-s7x vp-s7b 2713 10.241.2.0/24
check "hv2 : VM s7-x dans vp-s7b" vm_create s7-x sn-s7x 10.241.2.20
check "hv2 : l3vni 2700 déjà prise par vp-s7, refusée (409)" test "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"name":"vp-s7c","cidr":"10.249.0.0/16","l3vni":2700}' "${API}/vpcs")" = 409
NODE
on hv1 <<'NODE'
check "s7-a joignable" vm_wait vp-s7 10.241.1.10
check "s7-a : route vers la VPC 10.241.0.0/16 via la gateway (DHCP)" route_via vp-s7 10.241.1.10 10.241.0.0/16 10.241.1.254
check "hv1 : vrf-s7 contient exactement brl3-s7 et l3-s7-o" sh -c "[ \"\$1\" = 'brl3-s7 l3-s7-o' ]" _ "$(vrf_members vp-s7)"
check "hv1 : brl3-s7 sans adresse IPv4" sh -c "! ip -4 -o addr show dev brl3-s7 | grep -q inet"
check "hv1 : RMAC de brl3-s7 dérivée du VTEP 192.168.14.11" sh -c "ip -o link show brl3-s7 | grep -q 'link/ether 02:02:c0:a8:0e:0b '"
check "hv1 : route du supernet dans vp-s7 vers le transit" sh -c "ip -n vp-s7 -4 route show 10.241.0.0/16 | grep -q 'via 169.254.0.1 dev l3-s7-i'"
check "hv1 : forwarding sur brl3-s7 et l3-s7-o" sh -c "[ \"\$(sysctl -n net.ipv4.conf.brl3-s7.forwarding net.ipv4.conf.l3-s7-o.forwarding | tr '\n' ' ')\" = '1 1 ' ]"
check "hv1 : ip_forward dans le netns vp-s7" sh -c "[ \"\$(ip netns exec vp-s7 sysctl -n net.ipv4.ip_forward)\" = 1 ]"
info "hv1 : ip_forward global de l'hôte : $(sysctl -n net.ipv4.ip_forward) (non modifié par l'agent)"
check "hv1 : vrf-s7 exporte le subnet local 10.241.1.0/24 (agent)" vrf_route_via vp-s7 10.241.1.0/24 169.254.0.2
check "hv1 : vrf-s7 exporte s7-a 10.241.1.10/32 (agent)" vrf_route_via vp-s7 10.241.1.10 169.254.0.2
check "hv1 : session vers le RR en iBGP (internal link)" sh -c "vtysh -c 'show bgp neighbors 10.255.255.1' | grep -q 'internal link'"
NODE
on hv2 <<'NODE'
check "hv2 : s7-b joignable depuis son netns" netns_ping_wait vp-s7 10.241.2.10
check "hv2 : s7-x joignable depuis son netns (témoin)" netns_ping_wait vp-s7b 10.241.2.20
check "hv2 : VNI 2700 de type L3, active, RMAC 02:02:c0:a8:0e:0c" wait_l3vni 2700 02:02:c0:a8:0e:0c
check "hv2 : VNI 2701 de type L3, active, RMAC 02:02:c0:a8:0e:0c" wait_l3vni 2701 02:02:c0:a8:0e:0c
check "hv2 : vrf-s7 apprend 10.241.1.10/32 via le VTEP de hv1" wait_vrf_route vp-s7 10.241.1.10 192.168.14.11
check "hv2 : vrf-s7 apprend 10.241.1.0/24 via le VTEP de hv1" wait_vrf_route vp-s7 10.241.1.0/24 192.168.14.11
check "hv2 : RMAC de hv1 dans la FDB de vxl3-s7" sh -c "bridge fdb show dev vxl3-s7 | grep -q '02:02:c0:a8:0e:0b dst 192.168.14.11'"
NODE
on rr1 <<'NODE'
check "rr1 : RT-5 10.241.1.10/32 de hv1 (RD 192.168.14.11:2700) reçue et réfléchie" sh -c "vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.11:2700 type prefix' | grep -q '\[10.241.1.10\]'"
check "rr1 : RT-5 10.241.2.20/32 de vp-s7b (RD 192.168.14.12:2701) présente (témoin du test négatif)" sh -c "vtysh -c 'show bgp l2vpn evpn route rd 192.168.14.12:2701 type prefix' | grep -q '\[10.241.2.20\]'"
NODE
on hv1 <<'NODE'
check "hv1 : VNI 2700 de type L3, active, RMAC 02:02:c0:a8:0e:0b" wait_l3vni 2700 02:02:c0:a8:0e:0b
check "hv1 : vrf-s7 apprend 10.241.2.10/32 via le VTEP de hv2" wait_vrf_route vp-s7 10.241.2.10 192.168.14.12
check "hv1 : aucun VXLAN de sn-s7e (VNI 2712) sur hv1" sh -c "! ip link show vxlan-2712 >/dev/null 2>&1"
for i in $(seq 1 24); do vm_ssh vp-s7 10.241.1.10 'ping -c 1 -W 2 10.241.2.10' >/dev/null 2>&1 && break; sleep 5; done
check "s7-a (sn-s7d, hv1) joint s7-b (sn-s7e, hv2) par le L3VNI, forwarding par interface" vm_ssh vp-s7 10.241.1.10 'ping -c 3 -W 2 10.241.2.10'
check "trame pleine taille : 1472 octets en -M do" vm_ssh vp-s7 10.241.1.10 'ping -M do -s 1472 -c 2 -W 2 10.241.2.10'
check "1473 octets refusés localement (MTU 1500)" vm_grep vp-s7 10.241.1.10 'ping -M do -s 1473 -c 1 -W 2 10.241.2.10' 'message too long'
check "TTL 1 : expire à la gateway du subnet, netns de hv1 (10.241.1.254)" vm_grep vp-s7 10.241.1.10 'ping -n -c 3 -i 1.2 -W 2 -t 1 10.241.2.10' 'From 10.241.1.254 .*exceeded'
check "TTL 2 : expire dans la VRF de hv1 (169.254.0.1)" vm_grep vp-s7 10.241.1.10 'ping -n -c 3 -i 1.2 -W 2 -t 2 10.241.2.10' 'From 169.254.0.1 .*exceeded'
check "TTL 5 : atteint s7-b — quatre routeurs : gateway hv1, VRF hv1, VRF hv2, netns hv2" vm_ssh vp-s7 10.241.1.10 'ping -n -c 1 -W 2 -t 5 10.241.2.10'
check "hv1 : vrf-s7 n'importe pas 10.241.2.20/32 de vp-s7b (RT 64600:2701)" sh -c "[ -z \"\$(ip -4 route show vrf vrf-s7 10.241.2.20)\" ]"
check "s7-a ne joint pas s7-x, autre VPC, même plage" vm_fails vp-s7 10.241.1.10 'ping -c 3 -W 2 10.241.2.20'
NODE
on hv2 <<'NODE'
BGPD=$(pidof bgpd)
check "hv2 : suppression de sn-s7x refusée tant que s7-x existe (409)" test "$(api_code DELETE subnets/sn-s7x)" = 409
check "hv2 : suppression de s7-x" vm_delete s7-x
check "hv2 : vrf-s7b ne porte plus le /32 de s7-x" test -z "$(ip -4 route show vrf vrf-s7b 10.241.2.20/32)"
check "hv2 : suppression de sn-s7x" api_delete subnets/sn-s7x
check "hv2 : sn-s7x supprimé" wait_state subnets/sn-s7x deleted
check "hv2 : suppression de vp-s7b" api_delete vpcs/vp-s7b
check "hv2 : vp-s7b supprimé" wait_state vpcs/vp-s7b deleted
check "hv2 : plus aucune interface de vp-s7b" sh -c "! ip -o link show | grep -qE '(vrf|brl3|vxl3)-s7b|l3-s7b-o'"
check "hv2 : plus rien de vrf-s7b dans FRR, en mémoire ni dans frr.conf" sh -c "! vtysh -c 'show running-config' | grep -q 'vrf-s7b' && ! grep -q 'vrf-s7b' /etc/frr/frr.conf"
check "hv2 : bgpd n'a pas redémarré pendant le démontage" sh -c "[ \"\$(pidof bgpd)\" = '${BGPD}' ]"
check "hv2 : vp-s7 toujours en service" wait_l3vni 2700 02:02:c0:a8:0e:0c
NODE
