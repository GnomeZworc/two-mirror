# fonctionnalité: hypothèses d'environnement de la suite — firmware UEFI, unités modèles de l'agent, journalctl, format de nft list table, br-lan, priorités VRRP lues dans FRR
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
NS=t02-nft
MAC=02:01:00:00:0c:27
ip netns add "${NS}"
check "${HOST} : nft installé" command -v nft
check "${HOST} : nft list table, aucune règle avant (témoin)" test "$(nft_confined "${NS}" v-t02-i "${MAC}")" -eq 0
ip netns exec "${NS}" nft -f - <<NFT
add table bridge two
add chain bridge two pre-v-t02-i
add rule bridge two pre-v-t02-i ether saddr ${MAC} drop
add chain bridge two fwd-v-t02-i
add rule bridge two fwd-v-t02-i ether saddr ${MAC} drop
add rule bridge two fwd-v-t02-i arp operation request arp daddr ip 10.2.1.254 drop
add rule bridge two fwd-v-t02-i ip saddr 10.2.1.254 udp sport 67-68 udp dport 67-68 drop
add chain bridge two out-v-t02-i
add rule bridge two out-v-t02-i ether saddr ${MAC} drop
add chain bridge two fwd-v-t02x-i
add rule bridge two fwd-v-t02x-i arp operation request arp daddr ip 10.2.9.254 drop
NFT
check "${HOST} : nft list table, confinement de MAC lu par nft_confined (3 règles)" test "$(nft_confined "${NS}" v-t02-i "${MAC}")" -eq 3
check "${HOST} : nft list table, blocage ARP vers la gateway lu par nft_arp_drop" nft_arp_drop "${NS}" v-t02-i 10.2.1.254
check "${HOST} : nft list table, blocage DHCP lu par nft_dhcp_drop" nft_dhcp_drop "${NS}" v-t02-i 10.2.1.254
other_ip_ignored () { ! nft_arp_drop "${NS}" v-t02-i 10.2.1.25; }
check "${HOST} : nft_arp_drop ne confond pas une autre adresse (témoin)" other_ip_ignored
other_port_ignored () { ! nft_arp_drop "${NS}" v-t02-i 10.2.9.254; }
check "${HOST} : nft_rules ne lit pas la chaîne d'un autre port (témoin)" other_port_ignored
check "${HOST} : nft_handle trouve le numéro d'une règle" test -n "$(nft_handle "${NS}" out-v-t02-i "ether saddr ${MAC} drop")"
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
