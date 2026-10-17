# fonctionnalité: subnet en mode bridge rattaché au bridge br-lan de l'hôte — un hôte, puis le même subnet sur deux hôtes (une seule réponse ARP pour interface_ip, métadonnées servies par l'hôte de la VM)
# hôtes: hv1 hv2
# tickets: #70

on hv1 <<'NODE'
KEY=$(vm_key)
check "prérequis : br-lan présent sur hv1, avec l'adresse de l'hôte 10.33.0.11" sh -c "ip -4 -o addr show dev br-lan | grep -q ' 10.33.0.11/24 '"
check "prérequis : l'interface du segment lan est un port de br-lan, sans adresse" sh -c "ip -o link show lan | grep -q 'master br-lan' && [ -z \"\$(ip -4 -o addr show dev lan)\" ]"
check "VPC vp-t33" vpc_create vp-t33 10.33.0.0/16
check "subnet sn-t33a en mode bridge sur br-lan (iface_type lan, interface_ip 10.33.0.1)" subnet_create_bridge sn-t33a vp-t33 10.33.0.0/24 10.33.0.1
check "sn-t33a : mode bridge et interface_ip lus dans le GET" sh -c "curl -fsS ${API}/subnets/sn-t33a | jq -e '.mode == \"bridge\" and .interface_ip == \"10.33.0.1\"'"
check "sn-t33a : v-t33a-e rattaché à br-lan" sh -c "ip -o link show v-t33a-e | grep -q 'master br-lan'"
check "sn-t33a : aucun VXLAN pour ce subnet" sh -c "! ip -o link show | grep -q 'vxlan.*master br-t33a'"
check "sn-t33a : interface_ip 10.33.0.1 sur br-t33a dans le netns" sh -c "ip -n vp-t33 -4 -o addr show dev br-t33a | grep -q ' 10.33.0.1/'"
check "sn-t33a : pas de gateway anycast en mode bridge (pas de gw-t33a)" sh -c "! ip -n vp-t33 link show gw-t33a >/dev/null 2>&1"
check "VM t33-a1 10.33.0.21" vm_launch t33-a1 --nic sn-t33a=10.33.0.21 --sshkey "${KEY}"
check "VM t33-a2 10.33.0.22" vm_launch t33-a2 --nic sn-t33a=10.33.0.22 --sshkey "${KEY}"
for ip in 10.33.0.21 10.33.0.22; do
    check "${ip} joignable" vm_wait vp-t33 "${ip}"
    check "${ip} : adresse posée" has_addr vp-t33 "${ip}"
    check "${ip} : bail servi par interface_ip 10.33.0.1 (server-id)" test "$(vm_lease vp-t33 "${ip}" SERVER_ADDRESS)" = 10.33.0.1
    check "${ip} : route par défaut via 10.33.0.1" route_via vp-t33 "${ip}" default 10.33.0.1
    check "${ip} : route /32 vers les métadonnées via 10.33.0.1" route_via vp-t33 "${ip}" 169.254.169.254 10.33.0.1
    check "${ip} : pas de route vers la VPC 10.33.0.0/16 (option 121 sans route du VPC en bridge)" sh -c "[ -z \"\$1\" ]" _ "$(vm_ssh vp-t33 "${ip}" 'ip -4 route show 10.33.0.0/16')"
done
check "t33-a1 : métadonnées de t33-a1 (instance-id)" vm_grep vp-t33 10.33.0.21 'curl -fsS http://169.254.169.254/meta-data' '^instance-id: t33-a1$'
check "t33-a1 joint t33-a2, même hôte" vm_ssh vp-t33 10.33.0.21 'ping -n -c 3 -W 2 10.33.0.22'
check "hv1 joint t33-a1 depuis br-lan, hors netns" ping -c 2 -W 2 10.33.0.21
check "une seule réponse ARP pour 10.33.0.1 avec un seul hôte (témoin)" test "$(vm_frames vp-t33 10.33.0.21 arp-who ens3 10.33.0.21 10.33.0.1 2 | grep -c .)" -eq 1
NODE

on hv2 <<'NODE'
KEY=$(vm_key)
check "prérequis : br-lan présent sur hv2, avec l'adresse de l'hôte 10.33.0.12" sh -c "ip -4 -o addr show dev br-lan | grep -q ' 10.33.0.12/24 '"
check "hv2 : VPC vp-t33" vpc_create vp-t33 10.33.0.0/16
check "hv2 : même subnet sn-t33a en mode bridge" subnet_create_bridge sn-t33a vp-t33 10.33.0.0/24 10.33.0.1
check "hv2 : VM t33-b1 10.33.0.23" vm_launch t33-b1 --nic sn-t33a=10.33.0.23 --sshkey "${KEY}"
check "hv2 : 10.33.0.23 joignable" vm_wait vp-t33 10.33.0.23
NODE

on hv1 <<'NODE'
own_meta () {
    local n ok=0
    for n in 1 2 3 4 5; do
        vm_ssh vp-t33 "${1}" 'sudo ip neigh flush dev ens3' >/dev/null 2>&1
        vm_grep vp-t33 "${1}" 'curl -fsS -m 3 http://169.254.169.254/meta-data' "^instance-id: ${2}\$" && ok=$((ok + 1))
    done
    [ "${ok}" -eq 5 ] || { echo "${2} : ${ok}/5 réponses avec ses propres métadonnées"; return 1; }
}
RESPONDERS=$(vm_frames vp-t33 10.33.0.21 arp-who ens3 10.33.0.21 10.33.0.1 2)
info "MAC qui répondent pour 10.33.0.1 vues de t33-a1 : $(echo ${RESPONDERS})"
check "deux hôtes : t33-a1 (hv1) joint t33-b1 (hv2) sur le segment lan" vm_ssh vp-t33 10.33.0.21 'ping -n -c 3 -W 2 10.33.0.23'
check "deux hôtes : une seule MAC répond pour interface_ip 10.33.0.1" test "$(grep -c . <<<"${RESPONDERS}")" -eq 1
check "deux hôtes : t33-a1 reçoit toujours ses propres métadonnées (5 essais, cache ARP vidé)" own_meta 10.33.0.21 t33-a1
NODE

on hv2 <<'NODE'
own_meta () {
    local n ok=0
    for n in 1 2 3 4 5; do
        vm_ssh vp-t33 "${1}" 'sudo ip neigh flush dev ens3' >/dev/null 2>&1
        vm_grep vp-t33 "${1}" 'curl -fsS -m 3 http://169.254.169.254/meta-data' "^instance-id: ${2}\$" && ok=$((ok + 1))
    done
    [ "${ok}" -eq 5 ] || { echo "${2} : ${ok}/5 réponses avec ses propres métadonnées"; return 1; }
}
check "deux hôtes : t33-b1 (hv2) reçoit toujours ses propres métadonnées (5 essais, cache ARP vidé)" own_meta 10.33.0.23 t33-b1
check "deux hôtes : t33-b1 (hv2) joint t33-a2 (hv1)" vm_ssh vp-t33 10.33.0.23 'ping -n -c 3 -W 2 10.33.0.22'
NODE
