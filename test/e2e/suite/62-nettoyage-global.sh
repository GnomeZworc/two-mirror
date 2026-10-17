# fonctionnalité: nettoyage global en fin de suite — aucun hôte ne garde de ressource : API vide, ni netns, tap, VRF, unité, instance DHCP ou de métadonnées, processus QEMU, règle de port, volume ; FRR réduit à la base BGP
# hôtes:
# tickets: #70

for host in hv1 hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
none () { local out; out=$("$@" 2>/dev/null) || true; [ -z "${out}" ] && return 0; printf '%s\n' "${out}" | head -n 3; return 1; }
check "${HOST} : API joignable (témoin)" curl -fsS -o /dev/null "${API}/vpcs"
for kind in vms subnets vpcs; do
    check "${HOST} : aucune ressource ${kind} non supprimée dans l'API" none api_names "${kind}"
done
check "${HOST} : aucun netns de VPC" none sh -c "ip netns list | awk '{print \$1}' | grep '^vp-'"
check "${HOST} : aucun tap" none sh -c "ip -o link show | awk -F': ' '{print \$2}' | grep '^tap'"
check "${HOST} : aucune interface de subnet (v-*, vxlan-*)" none sh -c "ip -o link show | awk -F': ' '{print \$2}' | cut -d@ -f1 | grep -E '^(v-|vxlan-)'"
check "${HOST} : aucune VRF de VPC (vrf-*)" none sh -c "ip -o link show type vrf | awk -F': ' '{print \$2}' | grep '^vrf-'"
check "${HOST} : aucune unité two-vm-*" none systemctl list-units --all --plain --no-legend 'two-vm-*'
check "${HOST} : aucune instance DHCP (dhcp@, dnsmasq@) ni de métadonnées (metadata@) active" none sh -c "systemctl list-units --plain --no-legend 'dhcp@*' 'dnsmasq@*' 'metadata@*' | grep -w active"
known "#77" "${HOST} : aucun socket de contrôle DHCP" none sh -c "ls /run/two/dhcp/*.sock 2>/dev/null"
check "${HOST} : aucun processus QEMU" none pgrep -a qemu-system
check "${HOST} : aucune règle ebtables qui vise une interface de subnet" none sh -c "{ ebtables -t filter -L; ebtables -t nat -L; } | grep -E -- '-(i|o) v-'"
check "${HOST} : aucune règle iptables qui vise une interface de subnet ou de VPC" none sh -c "iptables-save | grep -E -- '(v-|vp-|tap)[a-z0-9]'"
check "${HOST} : aucun volume de VM de test" none sh -c "ls '${VOLUMES}'/t[0-9][0-9]-* 2>/dev/null"
check "${HOST} : FRR garde la base BGP (témoin)" sh -c "vtysh -c 'show running-config' | grep -q 'peer-group fabric'"
check "${HOST} : FRR sans aucune VRF de VPC (en mémoire)" none sh -c "vtysh -c 'show running-config' | grep -E '^(vrf |router bgp [0-9]+ vrf )'"
check "${HOST} : frr.conf sans aucune VRF de VPC" none grep -E '^(vrf |router bgp [0-9]+ vrf )' /etc/frr/frr.conf
NODE
done
