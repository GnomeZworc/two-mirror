on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv1)"; exit 0; }
KEY=$(vm_key)
check "hv1 : VPC vp-s4" vpc_create vp-s4 10.240.0.0/16
check "hv1 : subnet sn-s4" subnet_create sn-s4 vp-s4 2401 10.240.1.1 10.240.1.0/24
check "hv1 : vxlan-2401 porte l'adresse VTEP locale 192.168.14.11" sh -c "ip -d link show vxlan-2401 | grep -q 'local 192.168.14.11 '"
check "hv1 : VM s4-p" vm_create s4-p sn-s4 10.240.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv2)"; exit 0; }
check "hv2 : VPC vp-s4" vpc_create vp-s4 10.240.0.0/16
check "hv2 : subnet sn-s4" subnet_create sn-s4 vp-s4 2401 10.240.1.1 10.240.1.0/24
check "hv2 : vxlan-2401 porte l'adresse VTEP locale 192.168.14.12" sh -c "ip -d link show vxlan-2401 | grep -q 'local 192.168.14.12 '"
check "hv2 : VM s4-q" vm_create s4-q sn-s4 10.240.1.11
NODE
on hv1 <<'NODE'
check "s4-p joignable" vm_wait vp-s4 10.240.1.10
for i in $(seq 1 30); do vtysh -c "show evpn vni 2401" | grep -q 192.168.14.12 && break; sleep 2; done
check "hv1 : VTEP distant 192.168.14.12 appris par EVPN pour la VNI 2401" sh -c "vtysh -c 'show evpn vni 2401' | grep -q '192.168.14.12'"
check "hv1 : entrée d'inondation vers 192.168.14.12" sh -c "bridge fdb show dev vxlan-2401 | grep -q '00:00:00:00:00:00 dst 192.168.14.12'"
for i in $(seq 1 24); do vm_ssh vp-s4 10.240.1.10 'ping -c 1 -W 2 10.240.1.11' >/dev/null 2>&1 && break; sleep 5; done
check "s4-p (hv1) joint s4-q (hv2)" vm_ssh vp-s4 10.240.1.10 'ping -c 3 -W 2 10.240.1.11'
check "trame pleine taille : 1472 octets en -M do" vm_ssh vp-s4 10.240.1.10 'ping -M do -s 1472 -c 2 -W 2 10.240.1.11'
check "1473 octets refusés localement (MTU 1500)" vm_grep vp-s4 10.240.1.10 'ping -M do -s 1473 -c 1 -W 2 10.240.1.11' 'message too long'
NODE
