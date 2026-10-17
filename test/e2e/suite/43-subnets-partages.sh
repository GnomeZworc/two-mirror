# fonctionnalité: subnets vxlan partagés par deux hôtes, deux VM par hôte, dans une VPC avec et une VPC sans l3vni — toutes les paires, gateway anycast vue par chaque VM et absente d'EVPN, aucune MAC en double
# hôtes: hv2 hv3
# tickets: #57, #41

for host in hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
KEY=$(vm_key)
case "${HOST}" in hv2) n=1 ;; hv3) n=2 ;; esac
check "${HOST} : VPC vp-t43r, l3vni 4300" vpc_create vp-t43r 10.43.0.0/18 4300
check "${HOST} : subnet sn-t43r1 (VNI 4311)" subnet_create sn-t43r1 vp-t43r 4311 10.43.1.0/24
check "${HOST} : subnet sn-t43r2 (VNI 4312)" subnet_create sn-t43r2 vp-t43r 4312 10.43.2.0/24
check "${HOST} : VM t43-r1-${n} 10.43.1.1${n}" vm_create "t43-r1-${n}" sn-t43r1 "10.43.1.1${n}" "${KEY}"
check "${HOST} : VM t43-r2-${n} 10.43.2.1${n}" vm_create "t43-r2-${n}" sn-t43r2 "10.43.2.1${n}" "${KEY}"
check "${HOST} : VPC vp-t43n, sans l3vni" vpc_create vp-t43n 10.43.64.0/18
check "${HOST} : subnet sn-t43n1 (VNI 4321)" subnet_create sn-t43n1 vp-t43n 4321 10.43.65.0/24
check "${HOST} : subnet sn-t43n2 (VNI 4322)" subnet_create sn-t43n2 vp-t43n 4322 10.43.66.0/24
check "${HOST} : VM t43-n1-${n} 10.43.65.1${n}" vm_create "t43-n1-${n}" sn-t43n1 "10.43.65.1${n}" "${KEY}"
check "${HOST} : VM t43-n2-${n} 10.43.66.1${n}" vm_create "t43-n2-${n}" sn-t43n2 "10.43.66.1${n}" "${KEY}"
NODE
done

for host in hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
case "${HOST}" in hv2) n=1 ;; hv3) n=2 ;; esac
for v in r n; do
    vpc="vp-t43${v}"
    case "${v}" in r) base=0 vni=431 ;; n) base=64 vni=432 ;; esac
    for s in 1 2; do
        src="10.43.$(( base + s )).1${n}"
        gw="10.43.$(( base + s )).254"
        mac=$(anycast_mac "${vni}${s}")
        check "${HOST} : ${src} joignable" vm_wait "${vpc}" "${src}"
        check "${HOST} : ${src} joint sa gateway ${gw}" vm_ssh "${vpc}" "${src}" "ping -n -c 2 -W 2 ${gw}"
        check "${HOST} : ${src} voit la MAC anycast de la gateway (${mac})" vm_grep "${vpc}" "${src}" "ip neigh show ${gw}" "lladdr ${mac} "
        for d in 1 2; do
            for m in 1 2; do
                dst="10.43.$(( base + d )).1${m}"
                [ "${dst}" = "${src}" ] && continue
                for i in $(seq 1 12); do vm_ssh "${vpc}" "${src}" "ping -n -c 1 -W 2 ${dst}" >/dev/null 2>&1 && break; sleep 5; done
                check "${vpc} : ${src} (${HOST}) joint ${dst} ($([ "${d}" = "${s}" ] && echo 'même subnet' || echo 'autre subnet'), $([ "${m}" = "${n}" ] && echo 'même hôte' || echo 'autre hôte'))" \
                    vm_ssh "${vpc}" "${src}" "ping -n -c 3 -W 2 ${dst}"
            done
        done
    done
done
for vni in 4311 4312 4321 4322; do
    check "${HOST} : aucune MAC en double sur la VNI ${vni}" sh -c "! vtysh -c 'show evpn mac vni ${vni} duplicate' | grep -qiE '([0-9a-f]{2}:){5}[0-9a-f]{2}'"
    check "${HOST} : MAC anycast $(anycast_mac "${vni}") absente de la table EVPN de la VNI ${vni}" sh -c '! vtysh -c "show evpn mac vni $1" | grep -qi "$2"' _ "${vni}" "$(anycast_mac "${vni}")"
done
NODE
done
