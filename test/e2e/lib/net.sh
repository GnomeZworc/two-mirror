vpc_create () {
    local l3vni="${3:+,\"l3vni\":${3}}"
    api_state "vpcs/${1}" | grep -qx running && return 0
    api_post vpcs "{\"name\":\"${1}\",\"cidr\":\"${2}\"${l3vni}}" && wait_state "vpcs/${1}" running
}

subnet_create () {
    local name="${1}" vpc="${2}" vni="${3}" cidr="${4}" extra="${5:-}"
    api_state "subnets/${name}" | grep -qx running && return 0
    api_post subnets "{\"name\":\"${name}\",\"vpc\":\"${vpc}\",\"mode\":\"vxlan\",\"vxlan_id\":${vni},\"iface_type\":\"vms\",\"cidr\":\"${cidr}\"${extra}}" \
        && wait_state "subnets/${name}" running
}

anycast_mac () {
    printf '02:01:00:%02x:%02x:%02x\n' $(( ${1} >> 16 & 255 )) $(( ${1} >> 8 & 255 )) $(( ${1} & 255 ))
}

dhcp_hosts () {
    python3 - "/run/two/dhcp/${1}.sock" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sys.argv[1])
s.sendall(b'{"verb":"get-state"}\n')
buf = b""
while not buf.endswith(b"\n"):
    c = s.recv(65536)
    if not c:
        break
    buf += c
state = json.loads(buf).get("state") or {}
print(" ".join(sorted(h["mac"].lower() + "=" + h["ip"] for h in state.get("hosts") or [])))
PY
}

netns_ping_wait () {
    local i
    for i in $(seq 1 40); do
        ip netns exec "${1}" ping -c 1 -W 2 "${2}" >/dev/null 2>&1 && return 0
        sleep 5
    done
    echo "${2} injoignable depuis ${1}" >&2
    return 1
}

sources_reach () {
    local vpc="${1}" bridge="${2}" base="${3}" dst="${4}" n ok=0 total=0
    for n in $(seq 20 59); do
        ip -n "${vpc}" addr add "${base}.${n}/32" dev "${bridge}" 2>/dev/null
        ip netns exec "${vpc}" ping -n -c 2 -W 1 -I "${base}.${n}" "${dst}" >/dev/null 2>&1 && ok=$((ok + 1))
        total=$((total + 1))
        ip -n "${vpc}" addr del "${base}.${n}/32" dev "${bridge}"
    done
    echo "${ok}/${total}"
}

subnet_create_bridge () {
    local name="${1}" vpc="${2}" cidr="${3}" ip="${4}" type="${5:-lan}"
    api_state "subnets/${name}" | grep -qx running && return 0
    api_post subnets "{\"name\":\"${name}\",\"vpc\":\"${vpc}\",\"mode\":\"bridge\",\"iface_type\":\"${type}\",\"interface_ip\":\"${ip}\",\"cidr\":\"${cidr}\"}" \
        && wait_state "subnets/${name}" running
}

frames_py () {
    cat <<'PY'
import os, socket, struct, sys, time

def macb(s): return bytes(int(x, 16) for x in s.split(':'))
def macs(b): return ':'.join('%02x' % x for x in b)
def ipb(s): return socket.inet_aton(s)
def ifmac(i): return os.environ.get('FRAMES_MAC') or open('/sys/class/net/%s/address' % i).read().strip()

def csum(b):
    if len(b) % 2:
        b += b'\0'
    s = sum(struct.unpack('!%dH' % (len(b) // 2), b))
    s = (s >> 16) + (s & 0xffff)
    s += s >> 16
    return ~s & 0xffff

def eth(dst, src, t, p): return macb(dst) + macb(src) + struct.pack('!H', t) + p

def arp_req(smac, sip, tip):
    return eth('ff:ff:ff:ff:ff:ff', smac, 0x0806,
               struct.pack('!HHBBH', 1, 0x0800, 6, 4, 1) + macb(smac) + ipb(sip) + bytes(6) + ipb(tip))

def udp_bcast(smac, sip, sport, dport, data):
    u = struct.pack('!HHHH', sport, dport, 8 + len(data), 0) + data
    h = struct.pack('!BBHHHBBH4s4s', 0x45, 0, 20 + len(u), 0, 0, 64, 17, 0, ipb(sip), ipb('255.255.255.255'))
    h = h[:10] + struct.pack('!H', csum(h)) + h[12:]
    return eth('ff:ff:ff:ff:ff:ff', smac, 0x0800, h + u)

def garp(smac, ip):
    return eth('ff:ff:ff:ff:ff:ff', smac, 0x0806,
               struct.pack('!HHBBH', 1, 0x0800, 6, 4, 2) + macb(smac) + ipb(ip) + macb('ff:ff:ff:ff:ff:ff') + ipb(ip))

def marker(smac, text): return eth('ff:ff:ff:ff:ff:ff', smac, 0x88b5, text.encode().ljust(46, b'\0'))

def sock(i):
    s = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(3))
    s.bind((i, 0))
    return s

def emit(i, frame, n):
    if os.environ.get('FRAMES_DRY'):
        print(frame.hex())
        return
    s = sock(i)
    for _ in range(n):
        s.send(frame)
        time.sleep(0.2)

def frames(s, sec):
    s.settimeout(0.2)
    end = time.time() + sec
    while time.time() < end:
        try:
            f, addr = s.recvfrom(4096)
        except socket.timeout:
            continue
        if addr[2] != socket.PACKET_OUTGOING:
            yield f

def match(kind, a, f):
    t = f[12:14]
    if kind == 'arp-req':
        return t == b'\x08\x06' and f[20:22] == b'\x00\x01' and f[38:42] == ipb(a[0])
    if kind == 'arp-from':
        return t == b'\x08\x06' and f[28:32] == ipb(a[0])
    if kind == 'udp-from':
        return t == b'\x08\x00' and f[23] == 17 and f[26:30] == ipb(a[0]) and struct.unpack('!H', f[34:36])[0] == int(a[1])
    if kind == 'marker':
        return t == b'\x88\xb5' and a[0].encode() in f[14:]
    if kind == 'src-mac':
        return macs(f[6:12]) == a[0].lower()
    raise SystemExit('kind inconnu : ' + kind)

cmd, a = sys.argv[1], sys.argv[2:]
if cmd == 'arp':
    emit(a[0], arp_req(ifmac(a[0]), a[1], a[2]), int(a[3]))
elif cmd == 'dhcp':
    emit(a[0], udp_bcast(ifmac(a[0]), a[1], 67, int(a[2]), b'two-e2e-dhcp'.ljust(240, b'\0')), int(a[3]))
elif cmd == 'garp':
    emit(a[0], garp(ifmac(a[0]), a[1]), int(a[2]))
elif cmd == 'marker':
    emit(a[0], marker(a[1], a[2]), int(a[3]))
elif cmd == 'count':
    print(sum(1 for f in frames(sock(a[0]), float(a[1])) if match(a[2], a[3:], f)))
elif cmd == 'arp-who':
    s = sock(a[0])
    s.send(arp_req(ifmac(a[0]), a[1], a[2]))
    seen = {macs(f[22:28]) for f in frames(s, float(a[3]))
            if f[12:14] == b'\x08\x06' and f[20:22] == b'\x00\x02' and f[28:32] == ipb(a[2])}
    print('\n'.join(sorted(seen)))
else:
    raise SystemExit('commande inconnue : ' + cmd)
PY
}

frames () {
    frames_py | python3 - "$@"
}

vm_frames () {
    local vpc="${1}" ip="${2}"
    shift 2
    vm_ssh "${vpc}" "${ip}" "echo $(frames_py | base64 | tr -d '\n') | base64 -d > /tmp/frames.py && sudo python3 /tmp/frames.py $*"
}

sniff_start () {
    local out="${1}"
    shift
    frames count "$@" > "${out}" 2>&1 &
    sleep 1
}

vm_sniff_start () {
    local out="${1}" vpc="${2}" ip="${3}"
    shift 3
    vm_frames "${vpc}" "${ip}" count "$@" > "${out}" 2>&1 &
    sleep 3
}

ebt_mac () {
    awk -F: 'BEGIN {OFS = ":"} {for (i = 1; i <= NF; i++) sub(/^0/, "", $i); $1 = $1; print tolower($0)}' <<<"${1}"
}

ebt_rules () {
    { ip netns exec "${1}" ebtables -t nat -L; ip netns exec "${1}" ebtables -t filter -L; } | grep -- "${2}"
}

ebt_confined () {
    ebt_rules "${1}" "${2}" | grep -ciE -- "-s ($(ebt_mac "${3}")|${3}) .*DROP"
}

ebt_arp_drop () {
    ebt_rules "${1}" "${2}" | grep -i arp | grep -i request | grep -q -- "${3} "
}

ebt_dhcp_drop () {
    ebt_rules "${1}" "${2}" | grep -i udp | grep -- "${3} " | grep -q "67:68"
}

vrrp_expected_master () {
    printf '%s\n' "$@" | awk -F: '{split($3, o, "."); k = sprintf("%05d%03d%03d%03d%03d", $2, o[1], o[2], o[3], o[4]); if (k > best) {best = k; name = $1}} END {print name}'
}

log_gap () {
    LC_ALL=C awk -F'[][]' -v from="${2:-0}" -v to="${3:-0}" '/bytes from/ {if (prev != "" && $2 >= from && (to == 0 || $2 <= to) && $2 - prev > max) max = $2 - prev; prev = $2} END {printf "%d", max * 10 + 0.5}' "${1}"
}
