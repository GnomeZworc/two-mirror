# fonctionnalité: VM à plusieurs disques — racine virtio (vda) et SCSI (xvdX), tailles et bus vus par la VM, liens /dev/xvdX et by-id par numéro de série, démarrage sur vda
# hôtes: hv1
# tickets: #32, #74

on hv1 <<'NODE'
KEY=$(vm_key)
mkdir -p "${VOLUMES}"
check "volume t26-d-xvdb 1 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-xvdb.qcow2" 1G
check "volume t26-d-xvda 2 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-xvda.qcow2" 2G
check "VPC vp-t26" vpc_create vp-t26 10.26.0.0/16
check "subnet sn-t26" subnet_create sn-t26 vp-t26 2601 10.26.1.0/24
check "VM t26-d : vda (racine), xvdb, xvda" vm_launch t26-d --nic sn-t26=10.26.1.10 --sshkey "${KEY}" \
    --disk "xvdb=${VOLUMES}/t26-d-xvdb.qcow2" --disk "xvda=${VOLUMES}/t26-d-xvda.qcow2"
check "API : trois disques" test "$(curl -fsS "${API}/vms/t26-d" | jq -r '[.storage[].dev] | sort | join(" ")')" = "vda xvda xvdb"
check "t26-d joignable (démarrée sur vda)" vm_wait vp-t26 10.26.1.10
check "t26-d : racine sur vda" vm_grep vp-t26 10.26.1.10 'findmnt -n -o SOURCE /' '^/dev/vda'
target_size () { local v; v=$(vm_ssh vp-t26 10.26.1.10 "lsblk -b -dn -o HCTL,SIZE" 2>&1 | awk -v t="${1}" '{split($1, h, ":")} h[3] == t {print $2}'); [ "${v}" = "${2}" ] || { echo "lu : ${v:-aucun disque sur la cible ${1}}"; return 1; }; }
check "t26-d : xvda déclaré → cible SCSI 0, 2 Gio" target_size 0 2147483648
check "t26-d : xvdb déclaré → cible SCSI 1, 1 Gio" target_size 1 1073741824
info "t26-d : /dev/disk/by-id : $(vm_ssh vp-t26 10.26.1.10 'ls /dev/disk/by-id' 2>&1 | tr '\n' ' ')"
info "t26-d : noms du noyau : $(vm_ssh vp-t26 10.26.1.10 'lsblk -dn -o NAME,HCTL,SIZE' 2>&1 | tr '\n' ' ')"
link_size () { local v; v=$(vm_ssh vp-t26 10.26.1.10 "lsblk -b -dn -o SIZE \$(readlink -e ${1})" 2>&1 | tr -d ' '); [ "${v}" = "${2}" ] || { echo "lu : ${v}"; return 1; }; }
check "t26-d : /dev/xvda → 2 Gio (règle udev de l'image)" link_size /dev/xvda 2147483648
check "t26-d : /dev/xvdb → 1 Gio (règle udev de l'image)" link_size /dev/xvdb 1073741824
check "t26-d : by-id scsi-0QEMU_QEMU_HARDDISK_xvda → 2 Gio" link_size /dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_xvda 2147483648
check "t26-d : by-id scsi-0QEMU_QEMU_HARDDISK_xvdb → 1 Gio" link_size /dev/disk/by-id/scsi-0QEMU_QEMU_HARDDISK_xvdb 1073741824
check "t26-d : by-id virtio-vda → /dev/vda" test "$(vm_ssh vp-t26 10.26.1.10 'readlink -e /dev/disk/by-id/virtio-vda')" = /dev/vda
check "t26-d : xvda et xvdb en SCSI (virtio-scsi)" vm_ssh vp-t26 10.26.1.10 'for d in xvda xvdb; do k=$(basename $(readlink -e /dev/$d)); test -d /sys/block/$k/device/scsi_device && readlink -f /sys/block/$k | grep -q virtio || exit 1; done'
check "t26-d : vda n'est pas en SCSI (témoin)" vm_fails vp-t26 10.26.1.10 'test -d /sys/block/vda/device/scsi_device'
check "t26-d : partition sur xvdb → lien /dev/xvdb1" vm_ssh vp-t26 10.26.1.10 'echo ",,L" | sudo sfdisk -q /dev/xvdb >/dev/null && sudo udevadm settle && test -b "$(readlink -e /dev/xvdb1)"'
check "t26-d : écriture sur xvda et xvdb1" vm_ssh vp-t26 10.26.1.10 'sudo dd if=/dev/zero of=/dev/xvda bs=1M count=8 oflag=direct status=none && sudo dd if=/dev/zero of=/dev/xvdb1 bs=1M count=8 oflag=direct status=none'
NODE
