on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv1)"; exit 0; }
check "hv1 : VPC vp-s8, l3vni 2800" vpc_create vp-s8 10.242.0.0/16 2800
check "hv1 : subnet sn-s8s (VNI 2811)" subnet_create sn-s8s vp-s8 2811 10.242.1.1 10.242.1.0/24
check "hv1 : VM s8-m1 10.242.1.50" vm_create s8-m1 sn-s8s 10.242.1.50
check "hv1 : s8-m1 répond depuis son netns" netns_ping_wait vp-s8 10.242.1.50
check "hv1 : /24 et /32 exportés par l'agent" sh -c "ip -4 route show vrf vrf-s8 | grep -q '^10.242.1.0/24 via 169.254.0.2' && ip -4 route show vrf vrf-s8 | grep -q '^10.242.1.50 via 169.254.0.2'"
NODE
on hv2 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv2)"; exit 0; }
check "hv2 : VPC vp-s8, l3vni 2800" vpc_create vp-s8 10.242.0.0/16 2800
check "hv2 : aucun subnet de vp-s8 au départ" sh -c "! ip link show vxlan-2811 >/dev/null 2>&1"
NODE
on hv3 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (hv3)"; exit 0; }
KEY=$(vm_key)
check "hv3 : VPC vp-s8, l3vni 2800" vpc_create vp-s8 10.242.0.0/16 2800
check "hv3 : subnet sn-s8t (VNI 2812), seulement sur hv3" subnet_create sn-s8t vp-s8 2812 10.242.2.1 10.242.2.0/24
check "hv3 : VM observatrice s8-o 10.242.2.10" vm_create s8-o sn-s8t 10.242.2.10 "${KEY}"
check "s8-o joignable" vm_wait vp-s8 10.242.2.10
check "hv3 : aucun VXLAN de sn-s8s sur l'observateur" sh -c "! ip link show vxlan-2811 >/dev/null 2>&1"
check "hv3 : /32 de s8-m1 via hv1 seulement" wait_vrf_nexthops vp-s8 10.242.1.50 192.168.14.11
for i in $(seq 1 24); do vm_ssh vp-s8 10.242.2.10 'ping -c 1 -W 2 10.242.1.50' >/dev/null 2>&1 && break; sleep 5; done
check "s8-o (hv3) joint s8-m1 (hv1)" vm_ssh vp-s8 10.242.2.10 'ping -n -c 3 -W 2 10.242.1.50'
check "1472 octets en -M do" vm_ssh vp-s8 10.242.2.10 'ping -n -M do -s 1472 -c 2 -W 2 10.242.1.50'
NODE
on hv1 <<'NODE'
check "migration simple, 1 — suppression de s8-m1 sur hv1 : son /32 part en premier" vm_delete s8-m1
check "hv1 : plus de /32 pour 10.242.1.50 dans vrf-s8" test -z "$(ip -4 route show vrf vrf-s8 10.242.1.50/32 | grep 169.254.0.2)"
NODE
on hv3 <<'NODE'
check "hv3 : plus aucun /32 pour 10.242.1.50" wait_vrf_nexthops vp-s8 10.242.1.50 ""
check "hv3 : le /24 de sn-s8s reste annoncé par hv1 (filet)" wait_vrf_nexthops vp-s8 10.242.1.0/24 192.168.14.11
NODE
on hv2 <<'NODE'
check "migration simple, 2 — sn-s8s créé sur hv2 (même VNI 2811)" subnet_create sn-s8s vp-s8 2811 10.242.1.1 10.242.1.0/24
check "migration simple, 3 — VM s8-m2 10.242.1.50 sur hv2" vm_create s8-m2 sn-s8s 10.242.1.50
check "hv2 : s8-m2 répond depuis son netns" netns_ping_wait vp-s8 10.242.1.50
NODE
on hv3 <<'NODE'
start=${SECONDS}
check "hv3 : /32 de 10.242.1.50 via hv2 seulement" wait_vrf_nexthops vp-s8 10.242.1.50 192.168.14.12
info "hv3 : /32 de hv2 installé $(( SECONDS - start )) s après le passage en running de s8-m2"
check "hv3 : /24 de sn-s8s en ECMP vers hv1 et hv2" wait_vrf_nexthops vp-s8 10.242.1.0/24 "192.168.14.11 192.168.14.12"
for i in $(seq 1 24); do vm_ssh vp-s8 10.242.2.10 'ping -c 1 -W 2 10.242.1.50' >/dev/null 2>&1 && break; sleep 5; done
check "s8-o joint s8-m2 par son /32" vm_ssh vp-s8 10.242.2.10 'ping -n -c 3 -W 2 10.242.1.50'
info "après migration simple : $(ping_rate vp-s8 10.242.2.10 10.242.1.50 50)/50 réponses"
NODE
on hv1 <<'NODE'
info "orchestrée, 1 — sn-s8s présent sur hv1 (destination), /24 annoncé"
check "orchestrée, 2 — VM s8-m3 10.242.1.50 sur hv1 pendant que s8-m2 tourne sur hv2" vm_create s8-m3 sn-s8s 10.242.1.50
check "hv1 : s8-m3 répond depuis son netns" netns_ping_wait vp-s8 10.242.1.50
NODE
on rr1 <<'NODE'
check "rr1 : MAC de gateway de sn-s8s propre à chaque hôte, hv1 (02:03:c0:a8:0e:0b) et hv2 (02:03:c0:a8:0e:0c)" sh -c "r=\$(vtysh -c 'show bgp l2vpn evpn route type macip'); echo \"\$r\" | grep -qi '02:03:c0:a8:0e:0b' && echo \"\$r\" | grep -qi '02:03:c0:a8:0e:0c'"
NODE
on hv1 <<'NODE'
check "hv1 : aucune MAC en double sur la VNI 2811" sh -c "! vtysh -c 'show evpn mac vni 2811 duplicate' | grep -qiE '([0-9a-f]{2}:){5}[0-9a-f]{2}'"
NODE
on hv3 <<'NODE'
for i in $(seq 1 30); do [ "$(bgp_paths vp-s8 10.242.1.50/32)" = 2 ] && break; sleep 2; done
info "orchestrée : chemins BGP pour 10.242.1.50/32 : $(bgp_paths vp-s8 10.242.1.50/32) ; meilleur : $(bgp_best vp-s8 10.242.1.50/32) ; installés : $(vrf_nexthops vp-s8 10.242.1.50)"
check "orchestrée : deux /32 annoncés, hv1 et hv2" wait_vrf_nexthops vp-s8 10.242.1.50 "192.168.14.11 192.168.14.12"
check "deux /32, subnet présent des deux côtés : les 40 sources joignent 10.242.1.50, quel que soit le chemin ECMP" sh -c "[ \"\$1\" = 40/40 ]" _ "$(sources_reach vp-s8 br-s8t 10.242.2 10.242.1.50)"
NODE
on hv2 <<'NODE'
check "orchestrée, 3 — sn-s8s non supprimable sur hv2 tant que s8-m2 existe (409)" test "$(api_code DELETE subnets/sn-s8s)" = 409
check "orchestrée, 4 — suppression de s8-m2 sur hv2 (source) : son /32 part en premier" vm_delete s8-m2
check "orchestrée, 5 — suppression de sn-s8s sur hv2" api_delete subnets/sn-s8s
check "sn-s8s supprimé sur hv2" wait_state subnets/sn-s8s deleted
NODE
on hv3 <<'NODE'
check "hv3 : /32 de 10.242.1.50 via hv1 seulement" wait_vrf_nexthops vp-s8 10.242.1.50 192.168.14.11
check "hv3 : /24 de sn-s8s via hv1 seulement" wait_vrf_nexthops vp-s8 10.242.1.0/24 192.168.14.11
check "après la séquence orchestrée : s8-o joint s8-m3" vm_ssh vp-s8 10.242.2.10 'ping -n -c 3 -W 2 10.242.1.50'
check "après la séquence orchestrée : les 40 sources joignent s8-m3" sh -c "[ \"\$1\" = 40/40 ]" _ "$(sources_reach vp-s8 br-s8t 10.242.2 10.242.1.50)"
NODE
on hv1 <<'NODE'
check "ex-pire cas — suppression de sn-s8s sur hv1 refusée tant que s8-m3 existe (409)" test "$(api_code DELETE subnets/sn-s8s)" = 409
check "ex-pire cas — sn-s8s toujours running sur hv1" test "$(api_state subnets/sn-s8s)" = running
check "ex-pire cas — /32 et /24 toujours exportés par hv1" sh -c "ip -4 route show vrf vrf-s8 | grep -q '^10.242.1.50 via 169.254.0.2' && ip -4 route show vrf vrf-s8 | grep -q '^10.242.1.0/24 via 169.254.0.2'"
NODE
on hv1 <<'NODE'
check "watchdog : /32 orphelin posé à la main dans vrf-s8" ip route replace 10.242.1.99/32 via 169.254.0.2 dev l3-s8-o vrf vrf-s8
check "watchdog : /32 orphelin signalé comme trou noir" wait_watchdog "route 10.242.1.99/32 exported in vrf-s8 without a running owner"
check "watchdog : retrait du /32 orphelin" ip route del 10.242.1.99/32 vrf vrf-s8
check "watchdog : /32 de s8-m3 retiré à la main" ip route del 10.242.1.50/32 vrf vrf-s8
check "watchdog : /32 manquant signalé" wait_watchdog "route 10.242.1.50/32 not exported in vrf-s8"
check "watchdog : /32 de s8-m3 rétabli" ip route replace 10.242.1.50/32 via 169.254.0.2 dev l3-s8-o vrf vrf-s8
check "watchdog : aucune alerte BGP ni L3VNI sur hv1" sh -c "! journalctl -u agent --since '-60s' --no-pager -o cat | grep -F 'watchdog: inconsistency detected' | grep -qE '\"kind\":\"bgp\"|l3vni [0-9]|interface (vrf|brl3|vxl3|l3)-'"
NODE
on hv3 <<'NODE'
check "après les essais du watchdog : s8-o joint s8-m3" vm_ssh vp-s8 10.242.2.10 'ping -n -c 3 -W 2 10.242.1.50'
NODE
