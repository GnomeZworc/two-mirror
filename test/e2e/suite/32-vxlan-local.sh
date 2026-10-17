# fonctionnalité: routage entre deux subnets vxlan d'une même VPC sur un même hôte, sans et avec l3vni — ICMP, TCP, saut unique par la gateway
# hôtes: hv1
# tickets: #57, #41

on hv1 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t32n, sans l3vni" vpc_create vp-t32n 10.32.0.0/18
check "subnet sn-t32a (VNI 3201)" subnet_create sn-t32a vp-t32n 3201 10.32.1.0/24
check "subnet sn-t32b (VNI 3202)" subnet_create sn-t32b vp-t32n 3202 10.32.2.0/24
check "VPC vp-t32r, l3vni 3200" vpc_create vp-t32r 10.32.64.0/18 3200
check "subnet sn-t32c (VNI 3211)" subnet_create sn-t32c vp-t32r 3211 10.32.65.0/24
check "subnet sn-t32d (VNI 3212)" subnet_create sn-t32d vp-t32r 3212 10.32.66.0/24
check "VM t32-a" vm_create t32-a sn-t32a 10.32.1.10 "${KEY}"
check "VM t32-b" vm_create t32-b sn-t32b 10.32.2.10 "${KEY}"
check "VM t32-c" vm_create t32-c sn-t32c 10.32.65.10 "${KEY}"
check "VM t32-d" vm_create t32-d sn-t32d 10.32.66.10 "${KEY}"
for pair in vp-t32n:10.32.1.10:10.32.2.10:10.32.1.254:"sans l3vni" vp-t32r:10.32.65.10:10.32.66.10:10.32.65.254:"avec l3vni"; do
    IFS=: read -r vpc src dst gw label <<<"${pair}"
    check "${label} : ${src} joignable" vm_wait "${vpc}" "${src}"
    check "${label} : ${dst} joignable (témoin : la cible est vivante)" vm_wait "${vpc}" "${dst}"
    check "${label} : ${src} joint ${dst}, autre subnet, même hôte (ICMP)" vm_ssh "${vpc}" "${src}" "ping -n -c 3 -W 2 ${dst}"
    check "${label} : ${src} ouvre une connexion TCP vers ${dst}:22" vm_ssh "${vpc}" "${src}" "timeout 5 bash -c '</dev/tcp/${dst}/22'"
    check "${label} : ${dst} joint ${src} (retour)" vm_ssh "${vpc}" "${dst}" "ping -n -c 3 -W 2 ${src}"
    check "${label} : TTL 1 expire à la gateway ${gw}" vm_grep "${vpc}" "${src}" "ping -n -c 3 -i 1.2 -W 2 -t 1 ${dst}" "From ${gw} .*exceeded"
    check "${label} : TTL 2 atteint ${dst}, un seul saut" vm_ssh "${vpc}" "${src}" "ping -n -c 1 -W 2 -t 2 ${dst}"
done
check "avec l3vni : le /24 local de sn-t32d ne passe pas par la VRF (route connectée dans le netns)" sh -c "ip -n vp-t32r -4 route show 10.32.66.0/24 | grep -q 'dev br-t32d'"
NODE
