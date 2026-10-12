for host in hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two (${HOST})"; exit 0; }
KEY=$(vm_key)
case "${HOST}" in hv2) n=1 ;; hv3) n=2 ;; esac
check "${HOST} : VPC vp-s9r, l3vni 2900" vpc_create vp-s9r 10.250.0.0/16 2900
check "${HOST} : subnet sn-s9r1 (VNI 2911)" subnet_create sn-s9r1 vp-s9r 2911 10.250.1.1 10.250.1.0/24
check "${HOST} : subnet sn-s9r2 (VNI 2912)" subnet_create sn-s9r2 vp-s9r 2912 10.250.2.1 10.250.2.0/24
check "${HOST} : VM s9-r1-${n} 10.250.1.1${n}" vm_create "s9-r1-${n}" sn-s9r1 "10.250.1.1${n}" "${KEY}"
check "${HOST} : VM s9-r2-${n} 10.250.2.1${n}" vm_create "s9-r2-${n}" sn-s9r2 "10.250.2.1${n}" "${KEY}"
check "${HOST} : VPC vp-s9n, sans l3vni" vpc_create vp-s9n 10.251.0.0/16
check "${HOST} : subnet sn-s9n1 (VNI 2921)" subnet_create sn-s9n1 vp-s9n 2921 10.251.1.1 10.251.1.0/24
check "${HOST} : subnet sn-s9n2 (VNI 2922)" subnet_create sn-s9n2 vp-s9n 2922 10.251.2.1 10.251.2.0/24
check "${HOST} : VM s9-n1-${n} 10.251.1.1${n}" vm_create "s9-n1-${n}" sn-s9n1 "10.251.1.1${n}" "${KEY}"
check "${HOST} : VM s9-n2-${n} 10.251.2.1${n}" vm_create "s9-n2-${n}" sn-s9n2 "10.251.2.1${n}" "${KEY}"
NODE
done

for host in hv2 hv3; do
on "${host}" "HOST=${host}" <<'NODE'
case "${HOST}" in hv2) n=1; mac=02:03:c0:a8:0e:0c ;; hv3) n=2; mac=02:03:c0:a8:0e:0d ;; esac
for v in r n; do
    vpc="vp-s9${v}"
    net="10.25$([ "${v}" = r ] && echo 0 || echo 1)"
    for s in 1 2; do
        src="${net}.${s}.1${n}"
        check "${HOST} : ${src} joignable" vm_wait "${vpc}" "${src}"
        check "${HOST} : ${src} joint sa gateway ${net}.${s}.1" vm_ssh "${vpc}" "${src}" "ping -n -c 2 -W 2 ${net}.${s}.1"
        check "${HOST} : ${src} voit la MAC de gateway de son hôte (${mac})" vm_grep "${vpc}" "${src}" "ip neigh show ${net}.${s}.1" "lladdr ${mac} "
        for d in 1 2; do
            for m in 1 2; do
                dst="${net}.${d}.1${m}"
                [ "${dst}" = "${src}" ] && continue
                for i in $(seq 1 12); do vm_ssh "${vpc}" "${src}" "ping -n -c 1 -W 2 ${dst}" >/dev/null 2>&1 && break; sleep 5; done
                check "${vpc} : ${src} (${HOST}) joint ${dst} ($([ "${d}" = "${s}" ] && echo 'même subnet' || echo 'autre subnet'), $([ "${m}" = "${n}" ] && echo 'même hôte' || echo 'autre hôte'))" \
                    vm_ssh "${vpc}" "${src}" "ping -n -c 3 -W 2 ${dst}"
            done
        done
    done
done
for vni in 2911 2912 2921 2922; do
    check "${HOST} : aucune MAC en double sur la VNI ${vni}" sh -c "! vtysh -c 'show evpn mac vni ${vni} duplicate' | grep -qiE '([0-9a-f]{2}:){5}[0-9a-f]{2}'"
done
NODE
done
