on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two"; exit 0; }
KEY=$(vm_key)
check "VPC vp-s2" vpc_create vp-s2 10.220.0.0/16
check "subnet sn-s2a, default_route absent" subnet_create sn-s2a vp-s2 2201 10.220.1.1 10.220.1.0/24
check "subnet sn-s2b, default_route et gateway 10.220.2.254" subnet_create sn-s2b vp-s2 2202 10.220.2.1 10.220.2.0/24 ',"default_route":true,"gateway":"10.220.2.254"'
check "VM s2-a" vm_create s2-a sn-s2a 10.220.1.10 "${KEY}"
check "VM s2-b" vm_create s2-b sn-s2b 10.220.2.10 "${KEY}"
check "s2-a joignable" vm_wait vp-s2 10.220.1.10
check "s2-b joignable" vm_wait vp-s2 10.220.2.10
check "sn-s2a : route par défaut via interface_ip 10.220.1.1" route_via vp-s2 10.220.1.10 default 10.220.1.1
check "sn-s2a : route vers la VPC via interface_ip" route_via vp-s2 10.220.1.10 10.220.0.0/16 10.220.1.1
check "sn-s2a : route /32 vers les métadonnées via interface_ip" route_via vp-s2 10.220.1.10 169.254.169.254 10.220.1.1
check "sn-s2b : route par défaut via la gateway 10.220.2.254" route_via vp-s2 10.220.2.10 default 10.220.2.254
check "sn-s2b : route vers la VPC via interface_ip, pas via la gateway" route_via vp-s2 10.220.2.10 10.220.0.0/16 10.220.2.1
check "sn-s2b : route /32 vers les métadonnées via interface_ip" route_via vp-s2 10.220.2.10 169.254.169.254 10.220.2.1
check "suppression de s2-a" vm_delete s2-a
check "suppression de sn-s2a" api_delete subnets/sn-s2a
check "sn-s2a supprimé" wait_state subnets/sn-s2a deleted
check "sn-s2a recréé" subnet_create sn-s2a vp-s2 2201 10.220.1.1 10.220.1.0/24
check "VM s2-a2 sur le subnet recréé" vm_create s2-a2 sn-s2a 10.220.1.11 "${KEY}"
check "s2-a2 joignable" vm_wait vp-s2 10.220.1.11
check "après recréation : route par défaut via interface_ip 10.220.1.1" route_via vp-s2 10.220.1.11 default 10.220.1.1
check "après recréation : route vers la VPC via interface_ip" route_via vp-s2 10.220.1.11 10.220.0.0/16 10.220.1.1
check "après recréation : route /32 vers les métadonnées" route_via vp-s2 10.220.1.11 169.254.169.254 10.220.1.1
NODE
