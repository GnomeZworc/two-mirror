# fonctionnalité: VM à plusieurs interfaces — une NIC par subnet, adresses posées, route par défaut sur la primaire seule
# hôtes: hv1
# tickets: #33

on hv1 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t25" vpc_create vp-t25 10.25.0.0/16
check "subnet sn-t25a" subnet_create sn-t25a vp-t25 2501 10.25.1.0/24
check "subnet sn-t25b" subnet_create sn-t25b vp-t25 2502 10.25.2.0/24
check "VM t25-m : primaire sur sn-t25a, seconde NIC sur sn-t25b" vm_launch t25-m --nic sn-t25a=10.25.1.10 --nic sn-t25b=10.25.2.10 --sshkey "${KEY}"
check "API : deux interfaces, une seule primaire" test "$(curl -fsS "${API}/vms/t25-m" | jq -c '[.interfaces[] | .primary]')" = '[true,false]'
check "t25-m joignable sur sa primaire 10.25.1.10" vm_wait vp-t25 10.25.1.10
check "t25-m : deux interfaces réseau (ens3, ens4)" vm_ssh vp-t25 10.25.1.10 'test -e /sys/class/net/ens3 && test -e /sys/class/net/ens4'
check "t25-m : 10.25.1.10 sur ens3" vm_grep vp-t25 10.25.1.10 'ip -4 -o addr show dev ens3' ' 10.25.1.10/'
check "t25-m : ens4 obtient un bail DHCP de systemd-networkd (hypothèse : network-config, qui vise eth0, est sans effet)" test "$(vm_lease vp-t25 10.25.1.10 ADDRESS ens4)" = 10.25.2.10
check "t25-m : 10.25.2.10 sur ens4" vm_grep vp-t25 10.25.1.10 'ip -4 -o addr show dev ens4' ' 10.25.2.10/'
check "t25-m joignable sur sa seconde adresse 10.25.2.10" vm_wait vp-t25 10.25.2.10
check "t25-m : une seule route par défaut" test "$(vm_ssh vp-t25 10.25.1.10 'ip -4 route show default' | wc -l)" -eq 1
check "t25-m : route par défaut via la gateway de la primaire 10.25.1.254" route_via vp-t25 10.25.1.10 default 10.25.1.254
check "t25-m : bail de ens4 servi par la gateway de sn-t25b" test "$(vm_lease vp-t25 10.25.1.10 SERVER_ADDRESS ens4)" = 10.25.2.254
check "t25-m : route /32 des métadonnées présente" vm_grep vp-t25 10.25.1.10 'ip -4 route show 169.254.169.254' 'via 10.25.'
NODE
