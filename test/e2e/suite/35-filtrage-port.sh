# fonctionnalité: filtrage au port — en sortie vers le fabric (v-<id>-i) : ARP vers la gateway et l'adresse d'émission, DHCP émis depuis ces adresses, MAC anycast confinée dans les deux sens ; à la sortie de chaque VM (pre-tap<id>) : MAC, IP et ARP usurpés, faux serveur DHCP, source 0.0.0.0 hors DHCP, type de trame non IP, NIC autorisée en serveur DHCP (dhcp_server), règles surveillées par le watchdog et retirées avec la VM
# hôtes: hv1
# tickets: #57, #76

on hv1 <<'NODE'
KEY=$(vm_key)
MAC=$(anycast_mac 3501)
FAB=v-t35a-e
INJ=t35-inj
S=/tmp/t35
check "VPC vp-t35" vpc_create vp-t35 10.35.0.0/16
check "subnet sn-t35a (VNI 3501)" subnet_create sn-t35a vp-t35 3501 10.35.1.0/24
check "VM t35-a" vm_create t35-a sn-t35a 10.35.1.10 "${KEY}"
check "VM t35-b" vm_create t35-b sn-t35a 10.35.1.11 "${KEY}"
check "t35-a joignable" vm_wait vp-t35 10.35.1.10
check "VM t35-c, autorisée en serveur DHCP" vm_launch t35-c --nic sn-t35a=10.35.1.12 --dhcp-server --sshkey "${KEY}"
check "t35-b joignable" vm_wait vp-t35 10.35.1.11
check "t35-c joignable" vm_wait vp-t35 10.35.1.12
check "python3 présent dans la VM (prérequis des trames brutes)" vm_ssh vp-t35 10.35.1.10 'command -v python3'
check "t35-c : dhcp_server vrai dans le GET" sh -c "curl -fsS ${API}/vms/t35-c | jq -e '.interfaces[0].dhcp_server == true'"
check "t35-a : dhcp_server faux dans le GET" sh -c "curl -fsS ${API}/vms/t35-a | jq -e '.interfaces[0].dhcp_server == false'"
AMAC=$(vm_ssh vp-t35 10.35.1.10 'cat /sys/class/net/ens3/address')
CMAC=$(vm_ssh vp-t35 10.35.1.12 'cat /sys/class/net/ens3/address')
ATAP=$(nft_tap_of vp-t35 "${AMAC}")
CTAP=$(nft_tap_of vp-t35 "${CMAC}")
check "t35-a : chaîne pre-<tap> trouvée par sa MAC ${AMAC} (${ATAP:-aucune})" test -n "${ATAP}"
check "t35-c : chaîne pre-<tap> trouvée par sa MAC ${CMAC} (${CTAP:-aucune})" test -n "${CTAP}"
check "t35-a : ${ATAP} est un port de br-t35a" sh -c "ip -n vp-t35 -o link show ${ATAP} | grep -q 'master br-t35a'"
check "t35-a : 8 règles sur ${ATAP}, dont le blocage du serveur DHCP" test "$(nft_rules vp-t35 "${ATAP}" | grep -c .)" -eq 8 -a "$(nft_count vp-t35 "${ATAP}" 'udp sport 67 drop')" -eq 1
check "t35-c : 7 règles sur ${CTAP}, sans le blocage du serveur DHCP" test "$(nft_rules vp-t35 "${CTAP}" | grep -c .)" -eq 7 -a "$(nft_count vp-t35 "${CTAP}" 'udp sport 67 drop')" -eq 0
ip -n vp-t35 link add "${INJ}" type veth peer name "${INJ}p"
ip -n vp-t35 link set "${INJ}" master br-t35a up
ip -n vp-t35 link set "${INJ}p" up
check "port d'injection ${INJ} sur br-t35a, sans filtre (trames d'une VM non filtrée)" sh -c "ip -n vp-t35 -o link show ${INJ} | grep -q 'master br-t35a'"

seen_on_fabric () {
    local out="${1}" kind="${2}"
    shift 2
    sniff_start "${out}" "${FAB}" 6 "${kind}" "${@:1:$(( $# - 1 ))}"
    vm_frames vp-t35 10.35.1.10 ${!#} >/dev/null 2>&1
    wait
    cat "${out}"
}

injected_on_fabric () {
    local out="${1}" kind="${2}"
    shift 2
    sniff_start "${out}" "${FAB}" 6 "${kind}" "${@:1:$(( $# - 1 ))}"
    ns_frames vp-t35 ${!#} >/dev/null 2>&1
    wait
    cat "${out}"
}

received_by_b () {
    local out="${1}" sender="${2}" cmd="${3}"
    shift 3
    vm_sniff_start "${out}" vp-t35 10.35.1.11 ens3 8 "$@"
    vm_frames vp-t35 "${sender}" ${cmd} >/dev/null 2>&1
    wait
    cat "${out}"
}

for ip in 10.35.1.254 10.35.1.253; do
    n=$(seen_on_fabric "${S}.arp" arp-req "${ip}" "arp ens3 10.35.1.10 ${ip} 5")
    check "requête ARP de t35-a vers ${ip} : aucune ne sort vers le fabric (${n} vue(s))" test "${n}" -eq 0
done
n=$(seen_on_fabric "${S}.arp" arp-req 10.35.1.200 "arp ens3 10.35.1.10 10.35.1.200 5")
check "requête ARP de t35-a vers 10.35.1.200 : sort vers le fabric (témoin, ${n} vue(s))" test "${n}" -gt 0

for ip in 10.35.1.254 10.35.1.253; do
    n=$(injected_on_fabric "${S}.dhcp" udp-from "${ip}" 67 "dhcp ${INJ}p ${ip} 68 5")
    check "réponse DHCP injectée depuis ${ip} : aucune ne sort vers le fabric (${n} vue(s))" test "${n}" -eq 0
done
n=$(injected_on_fabric "${S}.dhcp" udp-from 10.35.1.10 67 "dhcp ${INJ}p 10.35.1.10 68 5")
check "réponse DHCP injectée depuis 10.35.1.10 : sort vers le fabric (témoin, ${n} vue(s))" test "${n}" -gt 0

n=$(injected_on_fabric "${S}.mac" marker t35-anycast "marker ${INJ}p ${MAC} t35-anycast 5")
check "trame injectée avec la MAC anycast ${MAC} en source : aucune ne sort vers le fabric (${n} vue(s))" test "${n}" -eq 0
n=$(injected_on_fabric "${S}.mac" marker t35-autre "marker ${INJ}p 02:35:00:00:00:01 t35-autre 5")
check "trame injectée avec une autre MAC source : sort vers le fabric (témoin, ${n} vue(s))" test "${n}" -gt 0

vm_sniff_start "${S}.in" vp-t35 10.35.1.11 ens3 8 marker t35-entrant
frames marker "${FAB}" "${MAC}" t35-entrant 5
wait
check "trame venue du fabric avec la MAC anycast en source : bloquée avant le bridge ($(cat "${S}.in") vue(s) par t35-b)" test "$(cat "${S}.in")" = 0
vm_sniff_start "${S}.in" vp-t35 10.35.1.11 ens3 8 marker t35-temoin
frames marker "${FAB}" 02:35:00:00:00:02 t35-temoin 5
wait
check "trame venue du fabric avec une autre MAC source : reçue par t35-b (témoin, $(cat "${S}.in") vue(s))" test "$(cat "${S}.in")" -gt 0

BRMAC=$(ip -n vp-t35 -o link show br-t35a | grep -o 'link/ether [0-9a-f:]*' | cut -d' ' -f2)
sniff_start "${S}.gw" "${FAB}" 10 src-mac "${MAC}"
sniff_start "${S}.br" "${FAB}" 10 src-mac "${BRMAC}"
vm_ssh vp-t35 10.35.1.10 'ping -n -c 5 -i 0.5 -W 1 10.35.1.254' >/dev/null 2>&1
ip netns exec vp-t35 ping -n -c 3 -W 1 10.35.1.11 >/dev/null 2>&1
ip netns exec vp-t35 ping -n -c 3 -W 1 10.35.1.200 >/dev/null 2>&1
wait
check "trafic de la gateway (ping des VM, réponses du netns) : aucune trame avec la MAC anycast sur le fabric ($(cat "${S}.gw") vue(s))" test "$(cat "${S}.gw")" -eq 0
check "trafic du netns vers une adresse inconnue : ARP du bridge ${BRMAC} sur le fabric (témoin, $(cat "${S}.br") vue(s))" test "$(cat "${S}.br")" -gt 0
n=$(received_by_b "${S}.r" 10.35.1.10 "dhcp ens3 10.35.1.10 68 5" udp-from 10.35.1.10 67)
check "faux serveur DHCP dans t35-a (67 → 68) : n'atteint pas t35-b, même subnet, même hôte (${n} vue(s))" test "${n}" -eq 0
n=$(received_by_b "${S}.r" 10.35.1.12 "dhcp ens3 10.35.1.12 68 5" udp-from 10.35.1.12 67)
check "réponse DHCP de t35-c, autorisée (dhcp_server) : atteint t35-b (témoin, ${n} vue(s))" test "${n}" -gt 0
n=$(received_by_b "${S}.r" 10.35.1.10 "udp ens3 10.35.1.10 4000 4000 5" udp-from 10.35.1.10 4000)
check "UDP de t35-a depuis sa propre adresse et sa MAC : atteint t35-b (témoin, ${n} vue(s))" test "${n}" -gt 0
for ip in 10.35.1.99 10.35.1.254 10.35.1.11; do
    n=$(received_by_b "${S}.r" 10.35.1.10 "udp ens3 ${ip} 4000 4000 5" udp-from "${ip}" 4000)
    check "UDP de t35-a avec l'adresse source usurpée ${ip} : n'atteint pas t35-b (${n} vue(s))" test "${n}" -eq 0
done
n=$(received_by_b "${S}.r" 10.35.1.10 "udp ens3 10.35.1.10 4002 4002 5 02:35:00:00:00:01" udp-from 10.35.1.10 4002)
check "UDP de t35-a avec une MAC source usurpée : n'atteint pas t35-b (${n} vue(s))" test "${n}" -eq 0
n=$(received_by_b "${S}.r" 10.35.1.10 "udp ens3 0.0.0.0 68 67 5" udp-from 0.0.0.0 68)
check "client DHCP de t35-a depuis 0.0.0.0 (68 → 67) : atteint t35-b (témoin, ${n} vue(s))" test "${n}" -gt 0
n=$(received_by_b "${S}.r" 10.35.1.10 "udp ens3 0.0.0.0 68 53 5" udp-from 0.0.0.0 68)
check "UDP de t35-a depuis 0.0.0.0 hors DHCP (68 → 53) : n'atteint pas t35-b (${n} vue(s))" test "${n}" -eq 0
n=$(received_by_b "${S}.r" 10.35.1.10 "arp ens3 0.0.0.0 10.35.1.11 5" arp-from 0.0.0.0)
check "sonde ARP de t35-a depuis 0.0.0.0 : atteint t35-b (témoin, ${n} vue(s))" test "${n}" -gt 0
n=$(received_by_b "${S}.r" 10.35.1.10 "arp ens3 10.35.1.99 10.35.1.11 5" arp-from 10.35.1.99)
check "requête ARP de t35-a avec l'adresse d'émetteur usurpée 10.35.1.99 : n'atteint pas t35-b (${n} vue(s))" test "${n}" -eq 0
n=$(received_by_b "${S}.r" 10.35.1.10 "marker ens3 ${AMAC} t35-type 5" marker t35-type)
check "trame de t35-a d'un type autre qu'IPv4, ARP ou IPv6 (0x88b5), avec sa MAC : n'atteint pas t35-b (${n} vue(s))" test "${n}" -eq 0
vm_sniff_start "${S}.r" vp-t35 10.35.1.11 ens3 8 marker t35-type-temoin
ns_frames vp-t35 marker "${INJ}p" 02:35:00:00:00:03 t35-type-temoin 5 >/dev/null 2>&1
wait
check "même type de trame par le port d'injection : reçu par t35-b (témoin, $(cat "${S}.r") vue(s))" test "$(cat "${S}.r")" -gt 0

vm_ssh vp-t35 10.35.1.11 'ping -n -c 2 -W 1 10.35.1.254; ping -n -c 2 -W 1 10.35.1.253' >/dev/null 2>&1
RELAY_MAC=$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253)
check "prérequis : t35-b résout la gateway vers la MAC anycast ${MAC}" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254)" = "${MAC}"
check "prérequis : t35-b résout l'adresse d'émission 10.35.1.253 (${RELAY_MAC:-non résolue})" test -n "${RELAY_MAC}"
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.254 5 >/dev/null 2>&1
check "usurpation ARP de la gateway 10.35.1.254 par t35-a : t35-b garde la MAC anycast (lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254)" = "${MAC}"
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.253 5 >/dev/null 2>&1
check "usurpation ARP de l'adresse d'émission 10.35.1.253 par t35-a : t35-b garde ${RELAY_MAC:-?} (lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253)" = "${RELAY_MAC}"
vm_ssh vp-t35 10.35.1.11 'sudo ip neigh replace 10.35.1.10 lladdr 02:35:00:00:00:99 dev ens3 nud stale' >/dev/null 2>&1
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.10 5 >/dev/null 2>&1
check "ARP gratuit de t35-a pour sa propre adresse : t35-b l'associe à ${AMAC} (témoin, lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.10))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.10)" = "${AMAC}"

HANDLE=$(nft_handle vp-t35 "pre-${ATAP}" 'udp sport 67 drop')
check "blocage du serveur DHCP retiré à la main de pre-${ATAP} (handle ${HANDLE:-absent})" ip netns exec vp-t35 nft delete rule bridge two "pre-${ATAP}" handle "${HANDLE}"
check "watchdog : règle manquante signalée pour t35-a" wait_watchdog "interface 0: nftables rule missing in netns vp-t35: pre-${ATAP}: udp sport 67 drop"
n=$(received_by_b "${S}.r" 10.35.1.10 "dhcp ens3 10.35.1.10 68 5" udp-from 10.35.1.10 67)
check "sans la règle, le faux serveur DHCP de t35-a atteint t35-b (témoin, ${n} vue(s))" test "${n}" -gt 0
check "blocage du serveur DHCP reposé" ip netns exec vp-t35 nft add rule bridge two "pre-${ATAP}" udp sport 67 drop
n=$(received_by_b "${S}.r" 10.35.1.10 "dhcp ens3 10.35.1.10 68 5" udp-from 10.35.1.10 67)
check "règle reposée : le faux serveur DHCP de t35-a n'atteint plus t35-b (${n} vue(s))" test "${n}" -eq 0

check "VM t35-c supprimée" api_delete vms/t35-c
check "t35-c en deleted" wait_state vms/t35-c deleted
check "t35-c supprimée : plus aucune règle pour ${CTAP}" test -z "$(nft_rules vp-t35 "${CTAP}")"
dispatched () {
    local map
    map=$(ip netns exec vp-t35 nft list map bridge two pre_ports) || return 2
    grep -qF "\"${1}\" : jump pre-${1}" <<<"${map}"
}
not_dispatched () { dispatched "${1}"; [ $? -eq 1 ]; }
check "t35-c supprimée : ${CTAP} retiré de pre_ports" not_dispatched "${CTAP}"
check "t35-a toujours aiguillé par pre_ports (témoin)" dispatched "${ATAP}"
check "t35-a garde ses 8 règles (témoin)" test "$(nft_rules vp-t35 "${ATAP}" | grep -c .)" -eq 8
check "port d'injection ${INJ} retiré" ip -n vp-t35 link del "${INJ}"
rm -f "${S}".*
NODE
