# fonctionnalité: socle du lab — underlay, passerelle VRRP, sessions EVPN, base BGP posée par l'agent, backend DHCP, watchdog silencieux
# hôtes: hv1 hv2 hv3
# tickets: #50, #51, #54, #41

vrrp_state () {
    on "${1}" <<'NODE' | sed -n 's/^VRRP: //p'
echo "VRRP: $(vtysh -c 'show vrrp' | awk '/Status \(v4\)/ {print $3}')"
NODE
}

SW1=$(vrrp_state sw1)
SW2=$(vrrp_state sw2)

for host in hv1 hv2 hv3; do
case "${host}" in
    hv1) addr=192.168.14.11 backend=two ;;
    hv2) addr=192.168.14.12 backend=dnsmasq ;;
    hv3) addr=192.168.14.13 backend=dnsmasq ;;
esac
on "${host}" "HOST=${host}" "ADDR=${addr}" "BACKEND=${backend}" "SW1=${SW1}" "SW2=${SW2}" <<'NODE'
check "${HOST} : agent actif" systemctl is-active --quiet agent
check "${HOST} : API de l'agent joignable" curl -fsS -o /dev/null "${API}/vpcs"
check "${HOST} : backend DHCP ${BACKEND} configuré" grep -A1 -x 'dhcp:' /etc/two/agent.yml | grep -qx "  backend: ${BACKEND}"
check "${HOST} : VTEP = adresse underlay ${ADDR}" test "$(vtep_host)" = "${ADDR}"
for peer in 192.168.14.11 192.168.14.12 192.168.14.13; do
    [ "${peer}" = "${ADDR}" ] && continue
    check "${HOST} : underlay, ${peer} joignable" ping -c 2 -W 2 "${peer}"
done
check "${HOST} : un seul maître VRRP sur la passerelle (sw1 ${SW1}, sw2 ${SW2})" sh -c "[ \"\$1 \$2\" = 'Master Backup' ] || [ \"\$1 \$2\" = 'Backup Master' ]" _ "${SW1}" "${SW2}"
check "${HOST} : passerelle 192.168.14.1 joignable" ping -c 2 -W 2 192.168.14.1
for rr in 10.255.255.1 10.255.255.2; do
    check "${HOST} : base BGP de l'agent, ${rr} dans le peer-group fabric de frr.conf" grep -qx " neighbor ${rr} peer-group fabric" /etc/frr/frr.conf
    check "${HOST} : session EVPN vers ${rr} établie" sh -c "vtysh -c 'show bgp neighbors ${rr}' | grep -q 'BGP state = Established'"
    check "${HOST} : loopback de ${rr} joignable" ping -c 2 -W 2 "${rr}"
done
check "${HOST} : aucune incohérence signalée par le watchdog depuis 5 min (hors réservation DHCP périmée, #78)" watchdog_quiet 300 'stale dhcp reservation'
known_flaky "#78" "${HOST} : aucune réservation DHCP signalée périmée par le watchdog depuis 5 min" watchdog_quiet_about 300 'stale dhcp reservation'
check "${HOST} : image de VM compatible two prête" two_image
check "${HOST} : clé SSH des VM prête" vm_key
NODE
done
