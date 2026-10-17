# fonctionnalité: filtrage au port — en sortie vers le fabric (v-<id>-i) : ARP vers la gateway et l'adresse d'émission, DHCP émis depuis ces adresses, MAC anycast confinée dans les deux sens ; entre VM d'un même hôte : faux serveur DHCP et usurpation ARP de la gateway (comportement attendu, non filtré à ce jour)
# hôtes: hv1
# tickets: #57

on hv1 <<'NODE'
KEY=$(vm_key)
MAC=$(anycast_mac 3501)
FAB=v-t35a-e
S=/tmp/t35
check "VPC vp-t35" vpc_create vp-t35 10.35.0.0/16
check "subnet sn-t35a (VNI 3501)" subnet_create sn-t35a vp-t35 3501 10.35.1.0/24
check "VM t35-a" vm_create t35-a sn-t35a 10.35.1.10 "${KEY}"
check "VM t35-b" vm_create t35-b sn-t35a 10.35.1.11 "${KEY}"
check "t35-a joignable" vm_wait vp-t35 10.35.1.10
check "t35-b joignable" vm_wait vp-t35 10.35.1.11
check "python3 présent dans la VM (prérequis des trames brutes)" vm_ssh vp-t35 10.35.1.10 'command -v python3'

seen_on_fabric () {
    local out="${1}" kind="${2}"
    shift 2
    sniff_start "${out}" "${FAB}" 6 "${kind}" "${@:1:$(( $# - 1 ))}"
    vm_frames vp-t35 10.35.1.10 ${!#} >/dev/null 2>&1
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
    n=$(seen_on_fabric "${S}.dhcp" udp-from "${ip}" 67 "dhcp ens3 ${ip} 68 5")
    check "réponse DHCP émise par t35-a depuis ${ip} : aucune ne sort vers le fabric (${n} vue(s))" test "${n}" -eq 0
done
n=$(seen_on_fabric "${S}.dhcp" udp-from 10.35.1.10 67 "dhcp ens3 10.35.1.10 68 5")
check "réponse DHCP émise par t35-a depuis sa propre adresse : sort vers le fabric (témoin, ${n} vue(s))" test "${n}" -gt 0

n=$(seen_on_fabric "${S}.mac" marker t35-anycast "marker ens3 ${MAC} t35-anycast 5")
check "trame de t35-a avec la MAC anycast ${MAC} en source : aucune ne sort vers le fabric (${n} vue(s))" test "${n}" -eq 0
n=$(seen_on_fabric "${S}.mac" marker t35-autre "marker ens3 02:35:00:00:00:01 t35-autre 5")
check "trame de t35-a avec une autre MAC source : sort vers le fabric (témoin, ${n} vue(s))" test "${n}" -gt 0

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
UNF="attendu ; aucun filtrage entre VM d'un même hôte à ce jour"
vm_sniff_start "${S}.rogue" vp-t35 10.35.1.11 ens3 8 udp-from 10.35.1.10 67
vm_frames vp-t35 10.35.1.10 dhcp ens3 10.35.1.10 68 5 >/dev/null 2>&1
wait
check "faux serveur DHCP dans t35-a (67 → 68) : n'atteint pas t35-b, même subnet, même hôte (${UNF} ; $(cat "${S}.rogue") vue(s))" test "$(cat "${S}.rogue")" = 0
vm_sniff_start "${S}.rogue" vp-t35 10.35.1.11 ens3 8 udp-from 10.35.1.10 67
vm_frames vp-t35 10.35.1.10 dhcp ens3 10.35.1.10 6868 5 >/dev/null 2>&1
wait
check "même envoi de t35-a vers le port 6868 : reçu par t35-b (témoin, $(cat "${S}.rogue") vue(s))" test "$(cat "${S}.rogue")" -gt 0

AMAC=$(vm_ssh vp-t35 10.35.1.10 'cat /sys/class/net/ens3/address')
vm_ssh vp-t35 10.35.1.11 'ping -n -c 2 -W 1 10.35.1.254; ping -n -c 2 -W 1 10.35.1.253' >/dev/null 2>&1
RELAY_MAC=$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253)
check "prérequis : t35-b résout la gateway vers la MAC anycast ${MAC}" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254)" = "${MAC}"
check "prérequis : t35-b résout l'adresse d'émission 10.35.1.253 (${RELAY_MAC:-non résolue})" test -n "${RELAY_MAC}"
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.254 5 >/dev/null 2>&1
check "usurpation ARP de la gateway 10.35.1.254 par t35-a : t35-b garde la MAC anycast (${UNF} ; lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.254)" = "${MAC}"
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.253 5 >/dev/null 2>&1
check "usurpation ARP de l'adresse d'émission 10.35.1.253 par t35-a : t35-b garde ${RELAY_MAC:-?} (${UNF} ; lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.253)" = "${RELAY_MAC}"
vm_ssh vp-t35 10.35.1.11 'sudo ip neigh replace 10.35.1.200 lladdr 02:35:00:00:00:99 dev ens3 nud stale' >/dev/null 2>&1
vm_frames vp-t35 10.35.1.10 garp ens3 10.35.1.200 5 >/dev/null 2>&1
check "usurpation ARP de 10.35.1.200 (adresse non protégée) par t35-a : t35-b l'associe à ${AMAC} (témoin, lue : $(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.200))" test "$(vm_neigh_mac vp-t35 10.35.1.11 10.35.1.200)" = "${AMAC}"
rm -f "${S}".*
NODE
