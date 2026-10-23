# fonctionnalité: MTU des VM (network.mtu_vm, 1500 par défaut) — annoncé par le DHCP (option 26, serveur intégré sur hv1, dnsmasq sur hv2) et par QEMU (host_mtu), porté par le tap, le bridge, le veth et le VXLAN du subnet ; subnet vxlan refusé sur un underlay qui n'est pas à 9000
# hôtes: hv1 hv2
# tickets: #56

on hv1 <<'NODE'
KEY=$(vm_key)
link_mtu () { cat "/sys/class/net/${1}/mtu"; }
check "prérequis : underlay br-000000 à 9000" test "$(link_mtu br-000000)" -eq 9000
check "prérequis : br-lan à 1500" test "$(link_mtu br-lan)" -eq 1500
check "VPC vp-t28" vpc_create vp-t28 10.28.0.0/16
check "subnet vxlan sur br-lan (1500, pas 9000) refusé, 422" expect_code 422 api_post_code subnets '{"name":"sn-t28x","vpc":"vp-t28","mode":"vxlan","vxlan_id":2809,"iface_type":"lan","cidr":"10.28.9.0/24"}'
check "subnet refusé : aucune trace de sn-t28x" expect_code 404 api_code GET subnets/sn-t28x
check "subnet sn-t28 sur br-000000" subnet_create sn-t28 vp-t28 2801 10.28.1.0/24
for iface in v-t28-e br-t28 vxlan-2801; do
    check "hôte : ${iface} à 1500" test "$(link_mtu "${iface}")" -eq 1500
done
for iface in v-t28-i br-t28 gw-t28; do
    check "netns vp-t28 : ${iface} à 1500" test "$(ip netns exec vp-t28 cat "/sys/class/net/${iface}/mtu")" -eq 1500
done
check "VM t28-m" vm_launch t28-m --nic sn-t28=10.28.1.10 --sshkey "${KEY}"
check "t28-m joignable" vm_wait vp-t28 10.28.1.10
info "t28-m : taps du netns : $(ip -n vp-t28 -o link show | awk -F': ' '/: tap/ {print $2}' | tr '\n' ' ')"
check "netns vp-t28 : tap de t28-m à 1500" sh -c "ip -n vp-t28 -o link show | grep ': tap' | grep -q ' mtu 1500 '"
check "QEMU : host_mtu=1500 sur virtio-net-pci" sh -c "ps -eo args | grep '[q]emu.*t28-m' | grep -q 'virtio-net-pci,[^ ]*host_mtu=1500'"
check "t28-m : bail DHCP avec MTU=1500 (option 26)" test "$(vm_lease vp-t28 10.28.1.10 MTU)" = 1500
check "t28-m : ens3 à 1500" vm_grep vp-t28 10.28.1.10 'cat /sys/class/net/ens3/mtu' '^1500$'
check "t28-m : ping de 1472 octets sans fragmentation vers la gateway" vm_ssh vp-t28 10.28.1.10 'ping -n -c 2 -W 2 -M do -s 1472 10.28.1.254'
check "t28-m : 1473 octets sans fragmentation refusés par la VM elle-même (témoin)" vm_grep vp-t28 10.28.1.10 'ping -n -c 1 -W 2 -M do -s 1473 10.28.1.254' 'too long'
NODE

on hv2 <<'NODE'
KEY=$(vm_key)
check "hv2 : VPC vp-t28" vpc_create vp-t28 10.28.0.0/16
check "hv2 : subnet sn-t28" subnet_create sn-t28 vp-t28 2801 10.28.1.0/24
check "hv2 : dnsmasq annonce l'option 26" sh -c "grep -rqx 'dhcp-option=26,1500' /etc/dnsmasq.d/"
check "hv2 : VM t28-d" vm_launch t28-d --nic sn-t28=10.28.1.20 --sshkey "${KEY}"
check "t28-d joignable" vm_wait vp-t28 10.28.1.20
check "t28-d : bail DHCP (dnsmasq) avec MTU=1500 (option 26)" test "$(vm_lease vp-t28 10.28.1.20 MTU)" = 1500
check "t28-d : ens3 à 1500" vm_grep vp-t28 10.28.1.20 'cat /sys/class/net/ens3/mtu' '^1500$'
NODE
