# fonctionnalité: fenêtre ARP de la gateway anycast entre deux hôtes — relais L2 depuis l'adresse d'émission d'un hôte, gateway de la VM inchangée, métadonnées, renouvellement DHCP et TCP local sans perte pendant la fenêtre
# hôtes: hv1 hv2
# tickets: #57

for host in hv1 hv2; do
on "${host}" "HOST=${host}" <<'NODE'
KEY=$(vm_key)
case "${HOST}" in hv1) n=1 ;; hv2) n=2 ;; esac
check "${HOST} : VPC vp-t44, sans l3vni" vpc_create vp-t44 10.44.0.0/16
check "${HOST} : subnet sn-t44a (VNI 4411)" subnet_create sn-t44a vp-t44 4411 10.44.1.0/24
check "${HOST} : subnet sn-t44b (VNI 4412)" subnet_create sn-t44b vp-t44 4412 10.44.2.0/24
check "${HOST} : VM t44-a${n} 10.44.1.1${n}" vm_create "t44-a${n}" sn-t44a "10.44.1.1${n}" "${KEY}"
check "${HOST} : VM t44-b${n} 10.44.2.1${n}" vm_create "t44-b${n}" sn-t44b "10.44.2.1${n}" "${KEY}"
check "${HOST} : t44-a${n} joignable" vm_wait vp-t44 "10.44.1.1${n}"
check "${HOST} : t44-b${n} joignable" vm_wait vp-t44 "10.44.2.1${n}"
NODE
done

on hv2 <<'NODE'
check "prérequis : t44-a2 résout sa gateway vers la MAC anycast" vm_grep vp-t44 10.44.1.12 'ping -n -c 1 -W 2 10.44.1.254 >/dev/null; ip neigh show 10.44.1.254' "lladdr $(anycast_mac 4411) "
rm -f /tmp/t44.log
nohup ip netns exec vp-t44 bash -c 'for i in $(seq 1 80); do if timeout 1 bash -c "</dev/tcp/10.44.1.12/22" 2>/dev/null; then echo "$(date +%T.%N) ok"; else echo "$(date +%T.%N) KO"; fi; sleep 0.5; done' > /tmp/t44.log 2>&1 &
sleep 3
NODE

on hv1 <<'NODE'
check "hv1 : entrées ARP du netns retirées sur sn-t44a" ip -n vp-t44 neigh flush dev br-t44a
check "t44-b1 (hv1) joint t44-a2 (hv2), autre subnet : relais L2 depuis l'adresse d'émission de hv1" vm_ssh vp-t44 10.44.2.11 'ping -n -c 3 -W 2 10.44.1.12'
NODE

on hv2 <<'NODE'
MAC=$(anycast_mac 4411)
check "fenêtre ouverte : t44-a2 associe l'adresse d'émission 10.44.1.253 à la MAC de hv1" vm_grep vp-t44 10.44.1.12 'ip neigh show 10.44.1.253' 'lladdr 02:03:c0:a8:0e:0b '
check "pendant la fenêtre : la gateway de t44-a2 reste la MAC anycast ${MAC}" vm_grep vp-t44 10.44.1.12 'ip neigh show 10.44.1.254' "lladdr ${MAC} "
check "pendant la fenêtre : métadonnées servies à t44-a2" vm_grep vp-t44 10.44.1.12 "python3 -c \"import urllib.request; print(urllib.request.urlopen('http://169.254.169.254/meta-data', timeout=5).read().decode())\"" 'instance-id: t44-a2'
check "pendant la fenêtre : bail renouvelé (networkctl renew)" lease_renewed vp-t44 10.44.1.12
check "bail tenu par la gateway 10.44.1.254" test "$(vm_lease vp-t44 10.44.1.12 SERVER_ADDRESS)" = 10.44.1.254
check "t44-a2 garde son adresse après le renouvellement" has_addr vp-t44 10.44.1.12
for i in $(seq 1 60); do [ "$(wc -l < /tmp/t44.log)" -ge 80 ] && break; sleep 1; done
info "connexions TCP du netns de hv2 vers t44-a2:22 : $(grep -c ' ok$' /tmp/t44.log) réussies, $(grep -c ' KO$' /tmp/t44.log) perdues ; $(grep ' KO$' /tmp/t44.log | head -3 | tr '\n' ' ')"
check "aucune connexion TCP du netns de hv2 vers sa VM perdue pendant la fenêtre (80/80)" test "$(grep -c ' ok$' /tmp/t44.log)" -eq 80
NODE
