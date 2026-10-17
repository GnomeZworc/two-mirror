# fonctionnalité: watchdog d'un VPC routé — /32 orphelin signalé comme trou noir, /32 manquant signalé, aucune fausse alerte BGP ni L3VNI, trafic intact après les essais
# hôtes: hv1 hv3
# tickets: #41

on hv1 <<'NODE'
check "hv1 : VPC vp-t53, l3vni 5300" vpc_create vp-t53 10.53.0.0/16 5300
check "hv1 : subnet sn-t53s (VNI 5311)" subnet_create sn-t53s vp-t53 5311 10.53.1.0/24
check "hv1 : VM t53-m 10.53.1.50" vm_create t53-m sn-t53s 10.53.1.50
check "hv1 : t53-m répond depuis son netns" netns_ping_wait vp-t53 10.53.1.50
check "hv1 : /32 de t53-m exporté par l'agent" vrf_route_via vp-t53 10.53.1.50 169.254.0.2
NODE
on hv3 <<'NODE'
KEY=$(vm_key)
check "hv3 : VPC vp-t53, l3vni 5300" vpc_create vp-t53 10.53.0.0/16 5300
check "hv3 : subnet sn-t53t (VNI 5312)" subnet_create sn-t53t vp-t53 5312 10.53.2.0/24
check "hv3 : VM observatrice t53-o 10.53.2.10" vm_create t53-o sn-t53t 10.53.2.10 "${KEY}"
check "t53-o joignable" vm_wait vp-t53 10.53.2.10
check "hv3 : /32 de t53-m via hv1" wait_vrf_nexthops vp-t53 10.53.1.50 192.168.14.11
for i in $(seq 1 24); do vm_ssh vp-t53 10.53.2.10 'ping -c 1 -W 2 10.53.1.50' >/dev/null 2>&1 && break; sleep 5; done
check "avant les essais du watchdog : t53-o joint t53-m (témoin)" vm_ssh vp-t53 10.53.2.10 'ping -n -c 3 -W 2 10.53.1.50'
NODE
on hv1 <<'NODE'
check "watchdog : aucune alerte BGP ni L3VNI avant les essais (témoin)" watchdog_l3_quiet 60
check "watchdog : /32 orphelin posé à la main dans vrf-t53" ip route replace 10.53.1.99/32 via 169.254.0.2 dev l3-t53-o vrf vrf-t53
check "watchdog : /32 orphelin signalé comme trou noir" wait_watchdog "route 10.53.1.99/32 exported in vrf-t53 without a running owner"
check "watchdog : retrait du /32 orphelin" ip route del 10.53.1.99/32 vrf vrf-t53
check "watchdog : /32 de t53-m retiré à la main" ip route del 10.53.1.50/32 vrf vrf-t53
check "hypothèse du test : FRR retire le /32 de t53-m de sa table BGP quand la route du noyau disparaît" wait_bgp_paths vp-t53 10.53.1.50/32 0
check "watchdog : /32 manquant signalé" wait_watchdog "route 10.53.1.50/32 not exported in vrf-t53"
check "watchdog : /32 de t53-m rétabli" ip route replace 10.53.1.50/32 via 169.254.0.2 dev l3-t53-o vrf vrf-t53
check "watchdog : aucune alerte BGP ni L3VNI sur hv1" watchdog_l3_quiet 60
NODE
on hv3 <<'NODE'
check "hv3 : /32 de t53-m de nouveau via hv1" wait_vrf_nexthops vp-t53 10.53.1.50 192.168.14.11
check "après les essais du watchdog : t53-o joint t53-m" vm_ssh vp-t53 10.53.2.10 'ping -n -c 3 -W 2 10.53.1.50'
NODE
