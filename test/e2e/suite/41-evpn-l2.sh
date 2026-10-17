# fonctionnalité: subnet vxlan étendu entre deux hôtes par EVPN — VTEP local, VTEP distant et inondation appris, MAC des VM en type 2, trafic ICMP et TCP, MTU 1500 de bout en bout
# hôtes: hv1 hv2
# tickets: #50, #51

on hv1 <<'NODE'
KEY=$(vm_key)
check "hv1 : VPC vp-t41" vpc_create vp-t41 10.41.0.0/16
check "hv1 : subnet sn-t41 (VNI 4101)" subnet_create sn-t41 vp-t41 4101 10.41.1.0/24
check "hv1 : vxlan-4101 porte l'adresse VTEP locale 192.168.14.11" sh -c "ip -d link show vxlan-4101 | grep -q 'local 192.168.14.11 '"
check "hv1 : VM t41-p" vm_create t41-p sn-t41 10.41.1.10 "${KEY}"
NODE
on hv2 <<'NODE'
KEY=$(vm_key)
check "hv2 : VPC vp-t41" vpc_create vp-t41 10.41.0.0/16
check "hv2 : subnet sn-t41 (VNI 4101)" subnet_create sn-t41 vp-t41 4101 10.41.1.0/24
check "hv2 : vxlan-4101 porte l'adresse VTEP locale 192.168.14.12" sh -c "ip -d link show vxlan-4101 | grep -q 'local 192.168.14.12 '"
check "hv2 : VM t41-q" vm_create t41-q sn-t41 10.41.1.11 "${KEY}"
NODE
on hv1 <<'NODE'
check "t41-p joignable" vm_wait vp-t41 10.41.1.10
for i in $(seq 1 30); do vtysh -c "show evpn vni 4101" | grep -q 192.168.14.12 && break; sleep 2; done
check "hv1 : VTEP distant 192.168.14.12 appris par EVPN pour la VNI 4101" sh -c "vtysh -c 'show evpn vni 4101' | grep -q '192.168.14.12'"
check "hv1 : entrée d'inondation vers 192.168.14.12" sh -c "bridge fdb show dev vxlan-4101 | grep -q '00:00:00:00:00:00 dst 192.168.14.12'"
for i in $(seq 1 24); do vm_ssh vp-t41 10.41.1.10 'ping -c 1 -W 2 10.41.1.11' >/dev/null 2>&1 && break; sleep 5; done
check "t41-p (hv1) joint t41-q (hv2)" vm_ssh vp-t41 10.41.1.10 'ping -c 3 -W 2 10.41.1.11'
for i in $(seq 1 24); do vm_ssh vp-t41 10.41.1.10 "timeout 5 bash -c '</dev/tcp/10.41.1.11/22'" >/dev/null 2>&1 && break; sleep 5; done
check "t41-p ouvre une connexion TCP vers t41-q:22" vm_ssh vp-t41 10.41.1.10 "timeout 5 bash -c '</dev/tcp/10.41.1.11/22'"
check "trame pleine taille : 1472 octets en -M do" vm_ssh vp-t41 10.41.1.10 'ping -M do -s 1472 -c 2 -W 2 10.41.1.11'
check "1473 octets refusés localement (MTU 1500)" vm_grep vp-t41 10.41.1.10 'ping -M do -s 1473 -c 1 -W 2 10.41.1.11' 'message too long'
MAC=$(vm_neigh_mac vp-t41 10.41.1.10 10.41.1.11)
check "hv1 : MAC de t41-q résolue par t41-p (${MAC:-aucune})" test -n "${MAC}"
check "hv1 : MAC de t41-q ${MAC} apprise en type 2 via le VTEP de hv2" wait_evpn_mac 4101 "${MAC}" 192.168.14.12
check "hv1 : forme JSON de FRR, chaque MAC distante de la VNI 4101 porte remoteVtep (hypothèse de evpn_mac_remote)" evpn_mac_json_shape 4101
NODE
on hv2 <<'NODE'
check "t41-q joignable" vm_wait vp-t41 10.41.1.11
check "t41-q (hv2) joint t41-p (hv1)" vm_ssh vp-t41 10.41.1.11 'ping -c 3 -W 2 10.41.1.10'
check "t41-q ouvre une connexion TCP vers t41-p:22" vm_ssh vp-t41 10.41.1.11 "timeout 5 bash -c '</dev/tcp/10.41.1.10/22'"
MAC=$(vm_neigh_mac vp-t41 10.41.1.11 10.41.1.10)
check "hv2 : MAC de t41-p résolue par t41-q (${MAC:-aucune})" test -n "${MAC}"
check "hv2 : MAC de t41-p ${MAC} apprise en type 2 via le VTEP de hv1" wait_evpn_mac 4101 "${MAC}" 192.168.14.11
NODE
