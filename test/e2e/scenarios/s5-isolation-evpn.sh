on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv1)"; exit 0; }
KEY=$(vm_key)
check "hv1 : VPC vp-s5a" vpc_create vp-s5a 10.245.0.0/16
check "hv1 : subnet sn-s5a (VNI 2501)" subnet_create sn-s5a vp-s5a 2501 10.245.1.1 10.245.1.0/24
check "hv1 : VM s5-r dans vp-s5a" vm_create s5-r sn-s5a 10.245.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv2)"; exit 0; }
check "hv2 : VPC vp-s5a" vpc_create vp-s5a 10.245.0.0/16
check "hv2 : subnet sn-s5a (VNI 2501)" subnet_create sn-s5a vp-s5a 2501 10.245.1.1 10.245.1.0/24
check "hv2 : VM s5-t dans vp-s5a (témoin)" vm_create s5-t sn-s5a 10.245.1.12
check "hv2 : VPC vp-s5b, même plage" vpc_create vp-s5b 10.245.0.0/16
check "hv2 : subnet sn-s5b (VNI 2502), même plage" subnet_create sn-s5b vp-s5b 2502 10.245.1.1 10.245.1.0/24
check "hv2 : VM s5-s dans vp-s5b" vm_create s5-s sn-s5b 10.245.1.11
NODE
on hv1 <<'NODE'
check "s5-r joignable" vm_wait vp-s5a 10.245.1.10
for i in $(seq 1 24); do vm_ssh vp-s5a 10.245.1.10 'ping -c 1 -W 2 10.245.1.12' >/dev/null 2>&1 && break; sleep 5; done
check "s5-r joint s5-t, même VPC sur hv2 (témoin)" vm_ssh vp-s5a 10.245.1.10 'ping -c 3 -W 2 10.245.1.12'
check "s5-r ne joint pas s5-s, autre VPC sur hv2, même plage" vm_fails vp-s5a 10.245.1.10 'ping -c 3 -W 2 10.245.1.11'
check "aucune entrée ARP résolue pour 10.245.1.11 dans s5-r" vm_fails vp-s5a 10.245.1.10 'ip neigh show 10.245.1.11 | grep -q lladdr'
NODE
