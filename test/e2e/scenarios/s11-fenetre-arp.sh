for host in hv1 hv2; do
on "${host}" "HOST=${host}" <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (${HOST})"; exit 0; }
KEY=$(vm_key)
case "${HOST}" in hv1) n=1 ;; hv2) n=2 ;; esac
check "${HOST} : VPC vp-s11, sans l3vni" vpc_create vp-s11 10.252.0.0/16
check "${HOST} : subnet sn-s11a (VNI 3111)" subnet_create sn-s11a vp-s11 3111 10.252.1.0/24
check "${HOST} : subnet sn-s11b (VNI 3112)" subnet_create sn-s11b vp-s11 3112 10.252.2.0/24
check "${HOST} : VM s11-a${n} 10.252.1.1${n}" vm_create "s11-a${n}" sn-s11a "10.252.1.1${n}" "${KEY}"
check "${HOST} : VM s11-b${n} 10.252.2.1${n}" vm_create "s11-b${n}" sn-s11b "10.252.2.1${n}" "${KEY}"
check "${HOST} : s11-a${n} joignable" vm_wait vp-s11 "10.252.1.1${n}"
check "${HOST} : s11-b${n} joignable" vm_wait vp-s11 "10.252.2.1${n}"
check "${HOST} : MAC anycast inscrite dans la FDB du bridge de sn-s11a" sh -c 'ip netns exec vp-s11 bridge fdb show br br-s11a | grep -qi "^$1 dev br-s11a master br-s11a permanent"' _ "$(anycast_mac 3111)"
check "${HOST} : le netns émet vers sn-s11a depuis la gateway 10.252.1.254" sh -c "ip -n vp-s11 -4 route get 10.252.1.1${n} | grep -q 'src 10.252.1.254 '"
NODE
done

on hv2 <<'NODE'
check "prérequis : s11-a2 résout sa gateway vers la MAC anycast" vm_grep vp-s11 10.252.1.12 'ping -n -c 1 -W 2 10.252.1.254 >/dev/null; ip neigh show 10.252.1.254' "lladdr $(anycast_mac 3111) "
rm -f /tmp/s11.log
nohup ip netns exec vp-s11 bash -c 'for i in $(seq 1 80); do if timeout 1 bash -c "</dev/tcp/10.252.1.12/22" 2>/dev/null; then echo "$(date +%T.%N) ok"; else echo "$(date +%T.%N) KO"; fi; sleep 0.5; done' > /tmp/s11.log 2>&1 &
sleep 3
NODE

on hv1 <<'NODE'
check "hv1 : entrées ARP du netns retirées sur sn-s11a" ip -n vp-s11 neigh flush dev br-s11a
check "s11-b1 (hv1) joint s11-a2 (hv2), autre subnet : relais L2 depuis l'adresse d'émission de hv1" vm_ssh vp-s11 10.252.2.11 'ping -n -c 3 -W 2 10.252.1.12'
NODE

on hv2 <<'NODE'
MAC=$(anycast_mac 3111)
check "fenêtre ouverte : s11-a2 associe l'adresse d'émission 10.252.1.253 à la MAC de hv1" vm_grep vp-s11 10.252.1.12 'ip neigh show 10.252.1.253' 'lladdr 02:03:c0:a8:0e:0b '
check "pendant la fenêtre : la gateway de s11-a2 reste la MAC anycast ${MAC}" vm_grep vp-s11 10.252.1.12 'ip neigh show 10.252.1.254' "lladdr ${MAC} "
check "pendant la fenêtre : métadonnées servies à s11-a2" vm_grep vp-s11 10.252.1.12 "python3 -c \"import urllib.request; print(urllib.request.urlopen('http://169.254.169.254/meta-data', timeout=5).read().decode())\"" 'instance-id: s11-a2'
BEFORE=$(vm_ssh vp-s11 10.252.1.12 'stat -c %Y /run/systemd/netif/leases/*')
sleep 2
check "pendant la fenêtre : renouvellement DHCP demandé" vm_ssh vp-s11 10.252.1.12 'sudo networkctl renew ens3'
renewed () {
    local i
    for i in $(seq 1 15); do
        [ "$(vm_ssh vp-s11 10.252.1.12 'stat -c %Y /run/systemd/netif/leases/*')" != "${BEFORE}" ] && return 0
        sleep 1
    done
    echo "bail non réécrit" >&2
    return 1
}
check "pendant la fenêtre : bail renouvelé" renewed
check "bail tenu par la gateway 10.252.1.254" vm_grep vp-s11 10.252.1.12 'cat /run/systemd/netif/leases/*' 'SERVER_ADDRESS=10.252.1.254'
check "s11-a2 garde son adresse après le renouvellement" has_addr vp-s11 10.252.1.12
for i in $(seq 1 60); do [ "$(wc -l < /tmp/s11.log)" -ge 80 ] && break; sleep 1; done
info "connexions TCP du netns de hv2 vers s11-a2:22 : $(grep -c ' ok$' /tmp/s11.log) réussies, $(grep -c ' KO$' /tmp/s11.log) perdues ; $(grep ' KO$' /tmp/s11.log | head -3 | tr '\n' ' ')"
check "aucune connexion TCP du netns de hv2 vers sa VM perdue pendant la fenêtre (80/80)" test "$(grep -c ' ok$' /tmp/s11.log)" -eq 80
NODE
