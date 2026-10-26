# fonctionnalité: DNS et NTP des VM par two-hostsvc — options DHCP 6 et 42 = interface_ip, redirection vers 169.254.169.253 dans le VPC (vxlan et bridge), seuls 53 et 123 servis, alertes du watchdog, VPC en error si two-hostsvc ne répond pas
# hôtes: hv1
# tickets: #86

on hv1 <<'NODE'
KEY=$(vm_key)
dns_py () {
    cat <<'PY'
import socket, struct, sys
server, proto, name = sys.argv[1], sys.argv[2], sys.argv[3]
q = struct.pack('!HHHHHH', 0x2929, 0x0100, 1, 0, 0, 0)
q += b''.join(bytes([len(p)]) + p.encode() for p in name.split('.')) + b'\0' + struct.pack('!HH', 1, 1)
if proto == 'tcp':
    s = socket.create_connection((server, 53), timeout=5)
    s.sendall(struct.pack('!H', len(q)) + q)
    size = struct.unpack('!H', s.recv(2))[0]
    a = b''
    while len(a) < size:
        a += s.recv(size - len(a))
else:
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(5)
    s.sendto(q, (server, int(sys.argv[4]) if len(sys.argv) > 4 else 53))
    a = s.recv(65535)
print('id=%d rcode=%d answers=%d' % (struct.unpack('!H', a[:2])[0], a[3] & 15, struct.unpack('!H', a[6:8])[0]))
PY
}
ntp_py () {
    cat <<'PY'
import socket, struct, sys, time
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)
req = b'\x23' + b'\0' * 39 + struct.pack('!Q', 0x2929292929292929)
s.sendto(req, (sys.argv[1], 123))
r = s.recv(1024)
t = struct.unpack('!I', r[40:44])[0] - 2208988800
print('mode=%d origin=%s li=%d stratum=%d offset=%d' % (r[0] & 7, 'ok' if r[24:32] == req[40:48] else 'ko', r[0] >> 6, r[1], abs(t - time.time())))
PY
}
vm_py () {
    local vpc="${1}" ip="${2}" code="${3}"
    shift 3
    vm_ssh "${vpc}" "${ip}" "echo $(${code} | base64 | tr -d '\n') | base64 -d | timeout 10 python3 - $*"
}
dns_ok () { vm_py "${1}" "${2}" dns_py "${3}" "${4}" deb.debian.org | grep -qE '^id=10537 rcode=0 answers=[1-9]'; }
ntp_ok () {
    local out
    out=$(vm_py "${1}" "${2}" ntp_py "${3}") || { echo "pas de réponse NTP de ${3}"; return 1; }
    echo "${out}" | grep -qE '^mode=4 origin=ok li=[0-3] stratum=[0-9]+ offset=[0-4]$' || { echo "${out}"; return 1; }
}
no_answer () { ! vm_py "${1}" "${2}" dns_py "${3}" udp deb.debian.org "${4}" >/dev/null 2>&1; }
served_ports () { ip netns exec "${1}" ss -Hlntu | awk '$5 ~ /^169\.254\.169\.253:/ {print $1, $5}' | sort | tr '\n' ' '; }
rules () { ip netns exec "${1}" iptables -t nat -S TWO-HOSTSVC; }
state_has () { grep -q "\"${1}\"" /run/two/hostsvc/hostsvc.state; }
excluded_first () { [ "$(rules "${1}" | sed -n '2p;3p' | tr '\n' ' ')" = "-A TWO-HOSTSVC -i br-public -j RETURN -A TWO-HOSTSVC -i l3-+ -j RETURN " ]; }
no_dns () { ! dns_ok "$@" >/dev/null 2>&1; }
no_ntp () { ! ntp_ok "$@" >/dev/null 2>&1; }
vpc_in_error () {
    local out
    out=$(wait_state "vpcs/${1}" running 2>&1) && { echo "${1} est running"; return 1; }
    grep -q "vpcs/${1}: error" <<<"${out}" || { echo "${out}"; return 1; }
}
restored () { state_has vp-t29 && state_has vp-t29b && ! state_has vp-t29c; }
vpc_delete () { api_delete "vpcs/${1}" && wait_state "vpcs/${1}" deleted; }

check "two-hostsvc actif" systemctl is-active --quiet two-hostsvc.service
check "VPC vp-t29 (vxlan)" vpc_create vp-t29 10.29.0.0/16
check "subnet sn-t29 (vxlan)" subnet_create sn-t29 vp-t29 2901 10.29.1.0/24
check "VPC vp-t29b (bridge)" vpc_create vp-t29b 10.33.0.0/16
check "subnet sn-t29b en mode bridge sur br-lan, interface_ip 10.33.0.29" subnet_create_bridge sn-t29b vp-t29b 10.33.0.0/24 10.33.0.29
for vpc in vp-t29 vp-t29b; do
    check "${vpc} : 169.254.169.253 sur la dummy hostsvc" sh -c "ip -n ${vpc} -4 -o addr show dev hostsvc | grep -q ' 169.254.169.253/32 '"
    check "${vpc} : two-hostsvc n'écoute que 53/udp, 53/tcp et 123/udp sur 169.254.169.253" test "$(served_ports "${vpc}")" = "tcp 169.254.169.253:53 udp 169.254.169.253:123 udp 169.254.169.253:53 "
    check "${vpc} : chaîne TWO-HOSTSVC, br-public et l3-+ exclus avant toute redirection" excluded_first "${vpc}"
    check "${vpc} : redirection de 53/udp, 53/tcp et 123/udp vers 169.254.169.253" test "$(rules "${vpc}" | grep -c 'DNAT --to-destination 169.254.169.253')" -eq 3
    check "${vpc} : appel depuis PREROUTING pour les destinations locales seulement" sh -c "ip netns exec ${vpc} iptables -t nat -S PREROUTING | grep -qx -- '-A PREROUTING -m addrtype --dst-type LOCAL -j TWO-HOSTSVC'"
    check "${vpc} : inscrit dans l'état de two-hostsvc" state_has "${vpc}"
done

check "VM t29-a 10.29.1.10 (vxlan)" vm_create t29-a sn-t29 10.29.1.10 "${KEY}"
check "VM t29-b 10.33.0.129 (bridge)" vm_launch t29-b --nic sn-t29b=10.33.0.129 --sshkey "${KEY}"
for vm in vp-t29:10.29.1.10 vp-t29b:10.33.0.129; do
    vpc=${vm%%:*}; ip=${vm#*:}
    check "${ip} joignable" vm_wait "${vpc}" "${ip}"
    GW=$(vm_lease "${vpc}" "${ip}" SERVER_ADDRESS)
    check "${ip} : gateway lue dans le bail (${GW:-absente})" test -n "${GW}"
    check "${ip} : option 6, DNS = la gateway ${GW}" test "$(vm_lease "${vpc}" "${ip}" DNS)" = "${GW}"
    check "${ip} : option 42, NTP = la gateway ${GW}" test "$(vm_lease "${vpc}" "${ip}" NTP)" = "${GW}"
    check "${ip} : DNS en UDP via ${GW}" dns_ok "${vpc}" "${ip}" "${GW}" udp
    check "${ip} : DNS en TCP via ${GW}" dns_ok "${vpc}" "${ip}" "${GW}" tcp
    check "${ip} : NTP via ${GW}, réponse serveur, origine vérifiée, écart < 5 s" ntp_ok "${vpc}" "${ip}" "${GW}"
    info "${ip} : NTP $(vm_py "${vpc}" "${ip}" ntp_py "${GW}" 2>&1)"
    info "${ip} : résolveur système : $(vm_ssh "${vpc}" "${ip}" 'getent hosts deb.debian.org' 2>&1 | head -1)"
    check "${ip} : port 5353 de la gateway non redirigé (aucune réponse)" no_answer "${vpc}" "${ip}" "${GW}" 5353
done

ip -n vp-t29 addr del 169.254.169.253/32 dev hostsvc
check "watchdog : adresse 169.254.169.253 retirée signalée" wait_watchdog "169.254.169.253 missing on hostsvc (in netns)"
ip -n vp-t29 addr add 169.254.169.253/32 dev hostsvc
check "10.29.1.10 : DNS de nouveau servi une fois l'adresse remise" dns_ok vp-t29 10.29.1.10 10.29.1.254 udp
ip netns exec vp-t29 iptables -t nat -D TWO-HOSTSVC -p udp -m udp --dport 123 -j DNAT --to-destination 169.254.169.253
check "watchdog : redirection incomplète signalée" wait_watchdog "dns and ntp redirection to 169.254.169.253 missing or incomplete (chain TWO-HOSTSVC)"
check "10.29.1.10 : sans la règle 123, plus de NTP (témoin)" no_ntp vp-t29 10.29.1.10 10.29.1.254
ip netns exec vp-t29 iptables -t nat -A TWO-HOSTSVC -p udp -m udp --dport 123 -j DNAT --to-destination 169.254.169.253
check "10.29.1.10 : NTP de nouveau servi une fois la règle remise" ntp_ok vp-t29 10.29.1.10 10.29.1.254

systemctl stop two-hostsvc.service
check "watchdog : two-hostsvc arrêté signalé" wait_watchdog "unreachable, no vpc of this host gets dns or ntp"
check "10.29.1.10 : plus de DNS pendant l'arrêt (témoin)" no_dns vp-t29 10.29.1.10 10.29.1.254 udp
api_post vpcs '{"name":"vp-t29c","cidr":"10.129.0.0/16"}'
check "VPC vp-t29c créé pendant l'arrêt : en error" vpc_in_error vp-t29c
systemctl start two-hostsvc.service
check "two-hostsvc redémarré" systemctl is-active --quiet two-hostsvc.service
check "redémarrage : vp-t29 et vp-t29b rouverts depuis le fichier d'état" restored
check "10.29.1.10 : DNS de nouveau servi après le redémarrage" dns_ok vp-t29 10.29.1.10 10.29.1.254 udp
check "10.33.0.129 : DNS de nouveau servi après le redémarrage" dns_ok vp-t29b 10.33.0.129 10.33.0.29 udp
check "VPC vp-t29c supprimé depuis error" vpc_delete vp-t29c
NODE
