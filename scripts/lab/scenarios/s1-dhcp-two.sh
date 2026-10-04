on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two"; exit 0; }
KEY=$(vm_key)
check "VPC vp-s1" vpc_create vp-s1 10.210.0.0/16
check "subnet sn-s1a" subnet_create sn-s1a vp-s1 2101 10.210.1.1 10.210.1.0/24
check "subnet sn-s1b" subnet_create sn-s1b vp-s1 2102 10.210.2.1 10.210.2.0/24
check "backend two : socket de contrôle de sn-s1a" test -S /run/two/dhcp/vp-s1_br-s1a.sock
check "backend two : socket de contrôle de sn-s1b" test -S /run/two/dhcp/vp-s1_br-s1b.sock
check "VM s1-a1, démarrée seule" vm_create s1-a1 sn-s1a 10.210.1.10 "${KEY}"
check "VM s1-a1 joignable" vm_wait vp-s1 10.210.1.10
check "VM s1-b1, démarrée seule" vm_create s1-b1 sn-s1b 10.210.2.10 "${KEY}"
check "VM s1-b1 joignable" vm_wait vp-s1 10.210.2.10
vm_create s1-a2 sn-s1a 10.210.1.11 "${KEY}" > /tmp/s1-a2.log 2>&1 & A=$!
vm_create s1-b2 sn-s1b 10.210.2.11 "${KEY}" > /tmp/s1-b2.log 2>&1 & B=$!
wait "${A}"; RA=$?
wait "${B}"; RB=$?
check "VM s1-a2, démarrée en même temps que s1-b2" test "${RA}" -eq 0
check "VM s1-b2, démarrée en même temps que s1-a2" test "${RB}" -eq 0
for vm in 10.210.1.10:10.210.1.1 10.210.1.11:10.210.1.1 10.210.2.10:10.210.2.1 10.210.2.11:10.210.2.1; do
    ip=${vm%%:*}; gw=${vm#*:}
    check "${ip} joignable" vm_wait vp-s1 "${ip}"
    check "${ip} : adresse de son propre subnet" has_addr vp-s1 "${ip}"
    check "${ip} : bail DHCP tenu par systemd-networkd" vm_ssh vp-s1 "${ip}" 'ls /run/systemd/netif/leases/ | grep -q .'
    check "${ip} : route par défaut via ${gw}" route_via vp-s1 "${ip}" default "${gw}"
    check "${ip} : route vers la VPC via ${gw}" route_via vp-s1 "${ip}" 10.210.0.0/16 "${gw}"
    check "${ip} : route /32 vers les métadonnées via ${gw}" route_via vp-s1 "${ip}" 169.254.169.254 "${gw}"
done
vm_host () { echo "$(vm_ssh vp-s1 "${1}" 'cat /sys/class/net/ens3/address')=${1}"; }
A_HOSTS=$(printf '%s\n' "$(vm_host 10.210.1.10)" "$(vm_host 10.210.1.11)" | sort | tr '\n' ' ' | sed 's/ $//')
B_HOSTS=$(printf '%s\n' "$(vm_host 10.210.2.10)" "$(vm_host 10.210.2.11)" | sort | tr '\n' ' ' | sed 's/ $//')
info "sn-s1a : ${A_HOSTS} ; sn-s1b : ${B_HOSTS}"
check "probe sn-s1a : uniquement les couples MAC/IP de sn-s1a" test "$(dhcp_hosts vp-s1_br-s1a)" = "${A_HOSTS}"
check "probe sn-s1b : uniquement les couples MAC/IP de sn-s1b" test "$(dhcp_hosts vp-s1_br-s1b)" = "${B_HOSTS}"
NODE
