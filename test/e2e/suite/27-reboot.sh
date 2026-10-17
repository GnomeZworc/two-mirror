# fonctionnalité: redémarrage d'une VM depuis l'invité — même processus QEMU, même adresse, nouveau bail, user_data non rejoué
# hôtes: hv1
# tickets: #46

on hv1 <<'NODE'
KEY=$(vm_key)
UD64=$(printf '%s\n' '#cloud-config' 'runcmd:' "  - [sh, -c, 'date +%s%N >> /var/tmp/t27-runcmd']" | base64 -w0)
check "VPC vp-t27" vpc_create vp-t27 10.27.0.0/16
check "subnet sn-t27" subnet_create sn-t27 vp-t27 2701 10.27.1.0/24
check "VM t27-r, user_data qui écrit une ligne par exécution" vm_launch t27-r --nic sn-t27=10.27.1.10 --sshkey "${KEY}" --user-data "${UD64}"
check "t27-r joignable" vm_wait vp-t27 10.27.1.10
check "t27-r : runcmd exécuté une fois au premier démarrage" test "$(vm_ssh vp-t27 10.27.1.10 'wc -l < /var/tmp/t27-runcmd')" -eq 1
BOOT=$(vm_boot_id vp-t27 10.27.1.10)
QEMU=$(pgrep -f "${VOLUMES}/t27-r.qcow2" | sort | tr '\n' ' ')
info "boot_id ${BOOT:-illisible} ; processus QEMU ${QEMU:-introuvable}"
check "prérequis : boot_id et processus QEMU relevés" test -n "${BOOT}" -a -n "${QEMU}"
check "t27-r : redémarrage demandé depuis l'invité" vm_ssh vp-t27 10.27.1.10 'sudo systemd-run --on-active=2 systemctl reboot'
check "t27-r : redémarrée (boot_id changé) et de nouveau joignable" wait_reboot vp-t27 10.27.1.10 "${BOOT}"
check "t27-r : toujours running pour l'API" test "$(api_state vms/t27-r)" = running
check "t27-r : même processus QEMU (redémarrage invité, pas de recréation)" test "$(pgrep -f "${VOLUMES}/t27-r.qcow2" | sort | tr '\n' ' ')" = "${QEMU}"
check "t27-r : même adresse 10.27.1.10" has_addr vp-t27 10.27.1.10
check "t27-r : nouveau bail obtenu après le démarrage" vm_ssh vp-t27 10.27.1.10 'f=/run/systemd/netif/leases/$(cat /sys/class/net/ens3/ifindex); sudo test -f "$f" && [ "$(sudo stat -c %Y "$f")" -ge "$(( $(date +%s) - $(cut -d. -f1 /proc/uptime) ))" ]'
check "t27-r : bail servi par la gateway" test "$(vm_lease vp-t27 10.27.1.10 SERVER_ADDRESS)" = 10.27.1.254
check "t27-r : user_data non rejoué (même instance-id, une seule ligne)" test "$(vm_ssh vp-t27 10.27.1.10 'wc -l < /var/tmp/t27-runcmd')" -eq 1
NODE
