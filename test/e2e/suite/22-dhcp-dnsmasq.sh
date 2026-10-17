# fonctionnalité: DHCP par dnsmasq (backend dnsmasq) — bail, server-id, renouvellement, option 121, une instance par subnet écoutant sur la gateway anycast
# hôtes: hv2
# tickets: #46, #57

on hv2 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t22" vpc_create vp-t22 10.22.0.0/16
check "subnet sn-t22a" subnet_create sn-t22a vp-t22 2201 10.22.1.0/24
check "subnet sn-t22b" subnet_create sn-t22b vp-t22 2202 10.22.2.0/24
for s in a b; do
    inst="vp-t22_br-t22${s}"
    check "dnsmasq : unité dnsmasq@${inst} active" systemctl is-active --quiet "dnsmasq@${inst}.service"
    check "dnsmasq : configuration ${inst}.conf présente" test -f "/etc/dnsmasq.d/${inst}.conf"
    check "dnsmasq : sn-t22${s} écoute sur la gateway anycast gw-t22${s}" grep -qx "interface=gw-t22${s}" "/etc/dnsmasq.d/${inst}.conf"
    check "dnsmasq : sn-t22${s} exclut le bridge br-t22${s}" grep -qx "except-interface=br-t22${s}" "/etc/dnsmasq.d/${inst}.conf"
    check "dnsmasq : sn-t22${s}, aucune socket du serveur intégré" test ! -e "/run/two/dhcp/${inst}.sock"
done
check "VM t22-a1, démarrée seule" vm_create t22-a1 sn-t22a 10.22.1.10 "${KEY}"
check "VM t22-a1 joignable" vm_wait vp-t22 10.22.1.10
check "VM t22-b1, démarrée seule" vm_create t22-b1 sn-t22b 10.22.2.10 "${KEY}"
check "VM t22-b1 joignable" vm_wait vp-t22 10.22.2.10
vm_create t22-a2 sn-t22a 10.22.1.11 "${KEY}" > /tmp/t22-a2.log 2>&1 & A=$!
vm_create t22-b2 sn-t22b 10.22.2.11 "${KEY}" > /tmp/t22-b2.log 2>&1 & B=$!
wait "${A}"; RA=$?
wait "${B}"; RB=$?
check "VM t22-a2, démarrée en même temps que t22-b2" test "${RA}" -eq 0
check "VM t22-b2, démarrée en même temps que t22-a2" test "${RB}" -eq 0
for vm in 10.22.1.10:10.22.1.254 10.22.1.11:10.22.1.254 10.22.2.10:10.22.2.254 10.22.2.11:10.22.2.254; do
    ip=${vm%%:*}; gw=${vm#*:}
    check "${ip} joignable" vm_wait vp-t22 "${ip}"
    check "${ip} : adresse de son propre subnet" has_addr vp-t22 "${ip}"
    check "${ip} : bail DHCP tenu par systemd-networkd" vm_ssh vp-t22 "${ip}" 'ls /run/systemd/netif/leases/ | grep -q .'
    check "${ip} : bail servi par la gateway ${gw} (server-id)" test "$(vm_lease vp-t22 "${ip}" SERVER_ADDRESS)" = "${gw}"
    check "${ip} : route par défaut via ${gw}" route_via vp-t22 "${ip}" default "${gw}"
    check "${ip} : route vers le VPC via ${gw}" route_via vp-t22 "${ip}" 10.22.0.0/16 "${gw}"
    check "${ip} : route /32 vers les métadonnées via ${gw}" route_via vp-t22 "${ip}" 169.254.169.254 "${gw}"
done
check "10.22.1.10 : bail renouvelé (networkctl renew)" lease_renewed vp-t22 10.22.1.10
check "10.22.1.10 : même adresse après le renouvellement" has_addr vp-t22 10.22.1.10
check "10.22.1.10 : renouvellement servi par la gateway" test "$(vm_lease vp-t22 10.22.1.10 SERVER_ADDRESS)" = 10.22.1.254
vm_host () { echo "$(vm_ssh vp-t22 "${1}" 'cat /sys/class/net/ens3/address'),${1}"; }
reserved () { cat /etc/dnsmasq.d/"${1}".hosts.d/* | grep -hoiE '([0-9a-f]{2}:){5}[0-9a-f]{2},[0-9.]+' | tr 'A-F' 'a-f' | sort | tr '\n' ' ' | sed 's/ $//'; }
A_HOSTS=$(printf '%s\n' "$(vm_host 10.22.1.10)" "$(vm_host 10.22.1.11)" | sort | tr '\n' ' ' | sed 's/ $//')
B_HOSTS=$(printf '%s\n' "$(vm_host 10.22.2.10)" "$(vm_host 10.22.2.11)" | sort | tr '\n' ' ' | sed 's/ $//')
info "sn-t22a : ${A_HOSTS} ; sn-t22b : ${B_HOSTS}"
check "réservations dnsmasq de sn-t22a : uniquement les couples MAC/IP de sn-t22a" test "$(reserved vp-t22_br-t22a)" = "${A_HOSTS}"
check "réservations dnsmasq de sn-t22b : uniquement les couples MAC/IP de sn-t22b" test "$(reserved vp-t22_br-t22b)" = "${B_HOSTS}"
NODE
