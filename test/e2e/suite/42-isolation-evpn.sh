# fonctionnalité: isolation entre VPC à travers EVPN — deux VPC sur la même plage, la VM d'un VPC ne joint pas celle de l'autre sur un hôte distant
# hôtes: hv1 hv2
# tickets: #50, #51

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t42a" vpc_create vp-t42a 10.42.0.0/16
check "hv1 : subnet sn-t42a (VNI 4201)" subnet_create sn-t42a vp-t42a 4201 10.42.1.0/24
check "hv1 : VM t42-r dans vp-t42a" vm_create t42-r sn-t42a 10.42.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
check "hv2 : VPC vp-t42a" vpc_create vp-t42a 10.42.0.0/16
check "hv2 : subnet sn-t42a (VNI 4201)" subnet_create sn-t42a vp-t42a 4201 10.42.1.0/24
check "hv2 : VM t42-t dans vp-t42a (témoin)" vm_create t42-t sn-t42a 10.42.1.12
check "hv2 : VPC vp-t42b, même plage" vpc_create vp-t42b 10.42.0.0/16
check "hv2 : subnet sn-t42b (VNI 4202), même plage" subnet_create sn-t42b vp-t42b 4202 10.42.1.0/24
check "hv2 : VM t42-s dans vp-t42b" vm_create t42-s sn-t42b 10.42.1.11
NODE
on hv1 <<'NODE'
check "t42-r joignable" vm_wait vp-t42a 10.42.1.10
for i in $(seq 1 24); do vm_ssh vp-t42a 10.42.1.10 'ping -c 1 -W 2 10.42.1.12' >/dev/null 2>&1 && break; sleep 5; done
check "t42-r joint t42-t, même VPC sur hv2 (témoin)" vm_ssh vp-t42a 10.42.1.10 'ping -c 3 -W 2 10.42.1.12'
check "t42-r ne joint pas t42-s, autre VPC sur hv2, même plage" vm_fails vp-t42a 10.42.1.10 'ping -c 3 -W 2 10.42.1.11'
check "aucune entrée ARP résolue pour 10.42.1.11 dans t42-r" vm_fails vp-t42a 10.42.1.10 'ip neigh show 10.42.1.11 | grep -q lladdr'
NODE
