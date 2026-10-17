# fonctionnalité: DHCP par le serveur intégré (backend two) — bail, server-id, renouvellement, option 121, un serveur par subnet, démarrages simultanés
# hôtes: hv1
# tickets: #46, #57

on hv1 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t21" vpc_create vp-t21 10.21.0.0/16
check "subnet sn-t21a" subnet_create sn-t21a vp-t21 2101 10.21.1.0/24
check "subnet sn-t21b" subnet_create sn-t21b vp-t21 2102 10.21.2.0/24
check "backend two : socket de contrôle de sn-t21a" test -S /run/two/dhcp/vp-t21_br-t21a.sock
check "backend two : socket de contrôle de sn-t21b" test -S /run/two/dhcp/vp-t21_br-t21b.sock
check "backend two : aucune instance dnsmasq pour sn-t21a" sh -c '! systemctl is-active --quiet dnsmasq@vp-t21_br-t21a.service && [ ! -e /etc/dnsmasq.d/vp-t21_br-t21a.conf ]'
check "VM t21-a1, démarrée seule" vm_create t21-a1 sn-t21a 10.21.1.10 "${KEY}"
check "VM t21-a1 joignable" vm_wait vp-t21 10.21.1.10
check "VM t21-b1, démarrée seule" vm_create t21-b1 sn-t21b 10.21.2.10 "${KEY}"
check "VM t21-b1 joignable" vm_wait vp-t21 10.21.2.10
vm_create t21-a2 sn-t21a 10.21.1.11 "${KEY}" > /tmp/t21-a2.log 2>&1 & A=$!
vm_create t21-b2 sn-t21b 10.21.2.11 "${KEY}" > /tmp/t21-b2.log 2>&1 & B=$!
wait "${A}"; RA=$?
wait "${B}"; RB=$?
check "VM t21-a2, démarrée en même temps que t21-b2" test "${RA}" -eq 0
check "VM t21-b2, démarrée en même temps que t21-a2" test "${RB}" -eq 0
for vm in 10.21.1.10:10.21.1.254 10.21.1.11:10.21.1.254 10.21.2.10:10.21.2.254 10.21.2.11:10.21.2.254; do
    ip=${vm%%:*}; gw=${vm#*:}
    check "${ip} joignable" vm_wait vp-t21 "${ip}"
    check "${ip} : adresse de son propre subnet" has_addr vp-t21 "${ip}"
    check "${ip} : bail DHCP tenu par systemd-networkd" vm_ssh vp-t21 "${ip}" 'ls /run/systemd/netif/leases/ | grep -q .'
    check "${ip} : bail servi par la gateway ${gw} (server-id)" test "$(vm_lease vp-t21 "${ip}" SERVER_ADDRESS)" = "${gw}"
    check "${ip} : route par défaut via ${gw}" route_via vp-t21 "${ip}" default "${gw}"
    check "${ip} : route vers la VPC via ${gw}" route_via vp-t21 "${ip}" 10.21.0.0/16 "${gw}"
    check "${ip} : route /32 vers les métadonnées via ${gw}" route_via vp-t21 "${ip}" 169.254.169.254 "${gw}"
done
check "10.21.1.10 : bail renouvelé (networkctl renew)" lease_renewed vp-t21 10.21.1.10
check "10.21.1.10 : même adresse après le renouvellement" has_addr vp-t21 10.21.1.10
check "10.21.1.10 : renouvellement servi par la gateway" test "$(vm_lease vp-t21 10.21.1.10 SERVER_ADDRESS)" = 10.21.1.254
vm_host () { echo "$(vm_ssh vp-t21 "${1}" 'cat /sys/class/net/ens3/address')=${1}"; }
A_HOSTS=$(printf '%s\n' "$(vm_host 10.21.1.10)" "$(vm_host 10.21.1.11)" | sort | tr '\n' ' ' | sed 's/ $//')
B_HOSTS=$(printf '%s\n' "$(vm_host 10.21.2.10)" "$(vm_host 10.21.2.11)" | sort | tr '\n' ' ' | sed 's/ $//')
info "sn-t21a : ${A_HOSTS} ; sn-t21b : ${B_HOSTS}"
check "probe sn-t21a : uniquement les couples MAC/IP de sn-t21a" test "$(dhcp_hosts vp-t21_br-t21a)" = "${A_HOSTS}"
check "probe sn-t21b : uniquement les couples MAC/IP de sn-t21b" test "$(dhcp_hosts vp-t21_br-t21b)" = "${B_HOSTS}"
NODE
