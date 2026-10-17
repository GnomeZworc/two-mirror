# fonctionnalité: migration d'une adresse de VM entre hôtes d'un VPC routé — /32 retiré en premier, /24 en filet puis en ECMP, séquence orchestrée, 40 sources, suppression du subnet refusée tant qu'une VM l'utilise, MAC anycast hors EVPN
# hôtes: hv1 hv2 hv3
# tickets: #41, #57

on hv1 <<'NODE'
check "hv1 : VPC vp-t52, l3vni 5200" vpc_create vp-t52 10.52.0.0/16 5200
check "hv1 : subnet sn-t52s (VNI 5211)" subnet_create sn-t52s vp-t52 5211 10.52.1.0/24
check "hv1 : VM t52-m1 10.52.1.50" vm_create t52-m1 sn-t52s 10.52.1.50
check "hv1 : t52-m1 répond depuis son netns" netns_ping_wait vp-t52 10.52.1.50
check "hv1 : /24 et /32 exportés par l'agent" sh -c "ip -4 route show vrf vrf-t52 | grep -q '^10.52.1.0/24 via 169.254.0.2' && ip -4 route show vrf vrf-t52 | grep -q '^10.52.1.50 via 169.254.0.2'"
NODE
on hv2 <<'NODE'
check "hv2 : VPC vp-t52, l3vni 5200" vpc_create vp-t52 10.52.0.0/16 5200
check "hv2 : aucun subnet de vp-t52 au départ" sh -c "! ip link show vxlan-5211 >/dev/null 2>&1"
NODE
on hv3 <<'NODE'
KEY=$(vm_key)
check "hv3 : VPC vp-t52, l3vni 5200" vpc_create vp-t52 10.52.0.0/16 5200
check "hv3 : subnet sn-t52t (VNI 5212), seulement sur hv3" subnet_create sn-t52t vp-t52 5212 10.52.2.0/24
check "hv3 : VM observatrice t52-o 10.52.2.10" vm_create t52-o sn-t52t 10.52.2.10 "${KEY}"
check "t52-o joignable" vm_wait vp-t52 10.52.2.10
check "hv3 : aucun VXLAN de sn-t52s sur l'observateur" sh -c "! ip link show vxlan-5211 >/dev/null 2>&1"
check "hv3 : /32 de t52-m1 via hv1 seulement" wait_vrf_nexthops vp-t52 10.52.1.50 192.168.14.11
for i in $(seq 1 24); do vm_ssh vp-t52 10.52.2.10 'ping -c 1 -W 2 10.52.1.50' >/dev/null 2>&1 && break; sleep 5; done
check "t52-o (hv3) joint t52-m1 (hv1)" vm_ssh vp-t52 10.52.2.10 'ping -n -c 3 -W 2 10.52.1.50'
check "1472 octets en -M do" vm_ssh vp-t52 10.52.2.10 'ping -n -M do -s 1472 -c 2 -W 2 10.52.1.50'
NODE
on hv1 <<'NODE'
check "migration simple, 1 — suppression de t52-m1 sur hv1 : son /32 part en premier" vm_delete t52-m1
check "hv1 : plus de /32 pour 10.52.1.50 dans vrf-t52" test -z "$(ip -4 route show vrf vrf-t52 10.52.1.50/32 | grep 169.254.0.2)"
NODE
on hv3 <<'NODE'
check "hv3 : plus aucun /32 pour 10.52.1.50" wait_vrf_nexthops vp-t52 10.52.1.50 ""
check "hv3 : le /24 de sn-t52s reste annoncé par hv1 (filet)" wait_vrf_nexthops vp-t52 10.52.1.0/24 192.168.14.11
NODE
on hv2 <<'NODE'
check "migration simple, 2 — sn-t52s créé sur hv2 (même VNI 5211)" subnet_create sn-t52s vp-t52 5211 10.52.1.0/24
check "migration simple, 3 — VM t52-m2 10.52.1.50 sur hv2" vm_create t52-m2 sn-t52s 10.52.1.50
check "hv2 : t52-m2 répond depuis son netns" netns_ping_wait vp-t52 10.52.1.50
NODE
on hv3 <<'NODE'
start=${SECONDS}
check "hv3 : /32 de 10.52.1.50 via hv2 seulement" wait_vrf_nexthops vp-t52 10.52.1.50 192.168.14.12
info "hv3 : /32 de hv2 installé $(( SECONDS - start )) s après le passage en running de t52-m2"
check "hv3 : /24 de sn-t52s en ECMP vers hv1 et hv2" wait_vrf_nexthops vp-t52 10.52.1.0/24 "192.168.14.11 192.168.14.12"
for i in $(seq 1 24); do vm_ssh vp-t52 10.52.2.10 'ping -c 1 -W 2 10.52.1.50' >/dev/null 2>&1 && break; sleep 5; done
check "t52-o joint t52-m2 par son /32" vm_ssh vp-t52 10.52.2.10 'ping -n -c 3 -W 2 10.52.1.50'
info "après migration simple : $(ping_rate vp-t52 10.52.2.10 10.52.1.50 50)/50 réponses"
NODE
on hv1 <<'NODE'
info "orchestrée, 1 — sn-t52s présent sur hv1 (destination), /24 annoncé"
check "orchestrée, 2 — VM t52-m3 10.52.1.50 sur hv1 pendant que t52-m2 tourne sur hv2" vm_create t52-m3 sn-t52s 10.52.1.50
check "hv1 : t52-m3 répond depuis son netns" netns_ping_wait vp-t52 10.52.1.50
NODE
on rr1 <<'NODE'
MAC=$(anycast_mac 5211)
R=$(vtysh -c 'show bgp l2vpn evpn route type macip')
check "rr1 : routes EVPN de type 2 reçues (témoin)" sh -c 'echo "$1" | grep -q "\[2\]:"' _ "${R}"
check "rr1 : aucune route de type 2 pour la MAC anycast de sn-t52s (${MAC}), présente sur hv1 et hv2" sh -c '! echo "$1" | grep -qi "$2"' _ "${R}" "${MAC}"
NODE
on hv1 <<'NODE'
MAC=$(anycast_mac 5211)
check "hv1 : aucune MAC en double sur la VNI 5211" sh -c "! vtysh -c 'show evpn mac vni 5211 duplicate' | grep -qiE '([0-9a-f]{2}:){5}[0-9a-f]{2}'"
check "hv1 : table EVPN de la VNI 5211 non vide (témoin)" sh -c "vtysh -c 'show evpn mac vni 5211' | grep -qiE '([0-9a-f]{2}:){5}[0-9a-f]{2}'"
check "hv1 : MAC anycast ${MAC} absente de la table EVPN de la VNI 5211" sh -c '! vtysh -c "show evpn mac vni $1" | grep -qi "$2"' _ 5211 "${MAC}"
NODE
on hv3 <<'NODE'
for i in $(seq 1 30); do [ "$(bgp_paths vp-t52 10.52.1.50/32)" -ge 2 ] && break; sleep 2; done
info "orchestrée : chemins BGP pour 10.52.1.50/32 : $(bgp_paths vp-t52 10.52.1.50/32) ; meilleur : $(bgp_best vp-t52 10.52.1.50/32) ; installés : $(vrf_nexthops vp-t52 10.52.1.50)"
check "orchestrée : deux /32 annoncés, hv1 et hv2" wait_vrf_nexthops vp-t52 10.52.1.50 "192.168.14.11 192.168.14.12"
check "deux /32, subnet présent des deux côtés : les 40 sources joignent 10.52.1.50, quel que soit le chemin ECMP" sh -c "[ \"\$1\" = 40/40 ]" _ "$(sources_reach vp-t52 br-t52t 10.52.2 10.52.1.50)"
NODE
on hv2 <<'NODE'
check "orchestrée, 3 — sn-t52s non supprimable sur hv2 tant que t52-m2 existe (409)" test "$(api_code DELETE subnets/sn-t52s)" = 409
check "orchestrée, 4 — suppression de t52-m2 sur hv2 (source) : son /32 part en premier" vm_delete t52-m2
check "orchestrée, 5 — suppression de sn-t52s sur hv2" api_delete subnets/sn-t52s
check "sn-t52s supprimé sur hv2" wait_state subnets/sn-t52s deleted
NODE
on hv3 <<'NODE'
check "hv3 : /32 de 10.52.1.50 via hv1 seulement" wait_vrf_nexthops vp-t52 10.52.1.50 192.168.14.11
check "hv3 : /24 de sn-t52s via hv1 seulement" wait_vrf_nexthops vp-t52 10.52.1.0/24 192.168.14.11
check "après la séquence orchestrée : t52-o joint t52-m3" vm_ssh vp-t52 10.52.2.10 'ping -n -c 3 -W 2 10.52.1.50'
check "après la séquence orchestrée : les 40 sources joignent t52-m3" sh -c "[ \"\$1\" = 40/40 ]" _ "$(sources_reach vp-t52 br-t52t 10.52.2 10.52.1.50)"
NODE
on hv1 <<'NODE'
check "ex-pire cas — suppression de sn-t52s sur hv1 refusée tant que t52-m3 existe (409)" test "$(api_code DELETE subnets/sn-t52s)" = 409
check "ex-pire cas — sn-t52s toujours running sur hv1" test "$(api_state subnets/sn-t52s)" = running
check "ex-pire cas — /32 et /24 toujours exportés par hv1" sh -c "ip -4 route show vrf vrf-t52 | grep -q '^10.52.1.50 via 169.254.0.2' && ip -4 route show vrf vrf-t52 | grep -q '^10.52.1.0/24 via 169.254.0.2'"
NODE
