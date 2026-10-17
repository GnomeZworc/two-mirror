# fonctionnalité: hypothèses d'environnement de la suite — firmware UEFI, unités modèles de l'agent, journalctl, format de ebtables -L, br-lan, priorités VRRP lues dans FRR
# hôtes: hv1 hv2 hv3
# tickets: #70

for sw in sw1 sw2; do
on "${sw}" "SW=${sw}" <<'NODE'
check "${SW} : ligne « vrrp <N> priority <P> » dans la running-config de FRR (lue par 72)" sh -c "vtysh -c 'show running-config' | awk '\$1 == \"vrrp\" && \$3 == \"priority\" && \$4 ~ /^[0-9]+\$/ {f = 1} END {exit !f}'"
NODE
done

for host in hv1 hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
for f in /usr/share/OVMF/OVMF_CODE.fd /usr/share/OVMF/OVMF_VARS.fd; do
    check "${HOST} : firmware UEFI ${f} présent (défaut de qemu.ovmf_code_path / ovmf_vars_template)" test -r "${f}"
done
check "${HOST} : journalctl accepte --since @<epoch>" journalctl --since "@$(date +%s)" -n 0 --no-pager -q
for unit in dhcp@.service dnsmasq@.service metadata@.service; do
    check "${HOST} : unité modèle ${unit} installée" systemctl cat "${unit}"
done
NS=t02-ebt
MAC=02:01:00:00:0c:27
ip netns add "${NS}"
check "${HOST} : ebtables -L, aucune règle avant (témoin)" test "$(ebt_confined "${NS}" v-t02-i "${MAC}")" -eq 0
ip netns exec "${NS}" ebtables -t nat -A PREROUTING --in-interface v-t02-i -s "${MAC}" -j DROP
ip netns exec "${NS}" ebtables -t filter -A FORWARD --out-interface v-t02-i -s "${MAC}" -j DROP
ip netns exec "${NS}" ebtables -t filter -A OUTPUT --out-interface v-t02-i -s "${MAC}" -j DROP
ip netns exec "${NS}" ebtables -A FORWARD --out-interface v-t02-i -p arp --arp-op Request --arp-ip-dst 10.2.1.254 -j DROP
ip netns exec "${NS}" ebtables -A FORWARD --out-interface v-t02-i -p IPv4 --ip-protocol udp --ip-source-port 67:68 --ip-destination-port 67:68 --ip-source 10.2.1.254 -j DROP
check "${HOST} : ebtables -L, confinement de MAC lu par ebt_confined (3 règles)" test "$(ebt_confined "${NS}" v-t02-i "${MAC}")" -eq 3
check "${HOST} : ebtables -L, blocage ARP vers la gateway lu par ebt_arp_drop" ebt_arp_drop "${NS}" v-t02-i 10.2.1.254
check "${HOST} : ebtables -L, blocage DHCP lu par ebt_dhcp_drop" ebt_dhcp_drop "${NS}" v-t02-i 10.2.1.254
other_ip_ignored () { ! ebt_arp_drop "${NS}" v-t02-i 10.2.1.25; }
check "${HOST} : ebt_arp_drop ne confond pas une autre adresse (témoin)" other_ip_ignored
ip netns del "${NS}"
case "${HOST}" in
    hv1) LAN=10.33.0.11 ;;
    hv2) LAN=10.33.0.12 ;;
    *) LAN="" ;;
esac
if [ -n "${LAN}" ]; then
    check "${HOST} : br-lan porte l'adresse de l'hôte ${LAN}/24" sh -c "ip -4 -o addr show dev br-lan | grep -q ' ${LAN}/24 '"
    check "${HOST} : lan rattachée à br-lan" sh -c "ip -o link show lan | grep -q 'master br-lan'"
    check "${HOST} : aucune adresse IPv4 restée sur lan (gestionnaire réseau de l'image)" sh -c "! ip -4 -o addr show dev lan | grep -q inet"
fi
NODE
done
