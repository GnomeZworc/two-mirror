# fonctionnalité: VM à plusieurs disques — virtio (vdX) et SCSI (sdX), tailles et bus vus par la VM, liens by-id par numéro de série, démarrage sur vda
# hôtes: hv1
# tickets: #32, #74

on hv1 <<'NODE'
KEY=$(vm_key)
mkdir -p "${VOLUMES}"
check "volume t26-d-sdb 1 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-sdb.qcow2" 1G
check "volume t26-d-sda 2 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-sda.qcow2" 2G
check "VPC vp-t26" vpc_create vp-t26 10.26.0.0/16
check "subnet sn-t26" subnet_create sn-t26 vp-t26 2601 10.26.1.0/24
check "VM t26-d : vda (système), sdb, sda" vm_launch t26-d --nic sn-t26=10.26.1.10 --sshkey "${KEY}" \
    --disk "sdb=${VOLUMES}/t26-d-sdb.qcow2" --disk "sda=${VOLUMES}/t26-d-sda.qcow2"
check "API : trois disques" test "$(curl -fsS "${API}/vms/t26-d" | jq -r '[.storage[].dev] | sort | join(" ")')" = "sda sdb vda"
check "t26-d joignable (démarrée sur vda)" vm_wait vp-t26 10.26.1.10
check "t26-d : racine sur vda" vm_grep vp-t26 10.26.1.10 'findmnt -n -o SOURCE /' '^/dev/vda'
target_size () { local v; v=$(vm_ssh vp-t26 10.26.1.10 "lsblk -b -dn -o HCTL,SIZE" 2>&1 | awk -v t="${1}" '{split($1, h, ":")} h[3] == t {print $2}'); [ "${v}" = "${2}" ] || { echo "lu : ${v:-aucun disque sur la cible ${1}}"; return 1; }; }
check "t26-d : sda déclaré → cible SCSI 0, 2 Gio" target_size 0 2147483648
check "t26-d : sdb déclaré → cible SCSI 1, 1 Gio" target_size 1 1073741824
info "t26-d : /dev/disk/by-id : $(vm_ssh vp-t26 10.26.1.10 'ls /dev/disk/by-id' 2>&1 | tr '\n' ' ')"
by_id_size () { local v; v=$(vm_ssh vp-t26 10.26.1.10 "lsblk -b -dn -o SIZE \$(readlink -e /dev/disk/by-id/${1})" 2>&1 | tr -d ' '); [ "${v}" = "${2}" ] || { echo "lu : ${v}"; return 1; }; }
check "t26-d : sdb déclaré → by-id scsi-0QEMU_QEMU_HARDDISK_sdb, 1 Gio" by_id_size scsi-0QEMU_QEMU_HARDDISK_sdb 1073741824
check "t26-d : sda déclaré → by-id scsi-0QEMU_QEMU_HARDDISK_sda, 2 Gio" by_id_size scsi-0QEMU_QEMU_HARDDISK_sda 2147483648
check "t26-d : vda déclaré → by-id virtio-vda, disque système" test "$(vm_ssh vp-t26 10.26.1.10 'readlink -e /dev/disk/by-id/virtio-vda')" = /dev/vda
check "t26-d : sdb en SCSI" vm_grep vp-t26 10.26.1.10 'readlink -f /sys/block/sdb/device/driver' 'sd$'
check "t26-d : sda en SCSI (virtio-scsi)" vm_ssh vp-t26 10.26.1.10 'test -d /sys/block/sda/device/scsi_device && readlink -f /sys/block/sda | grep -q virtio'
check "t26-d : vda n'est pas en SCSI (témoin)" vm_fails vp-t26 10.26.1.10 'test -d /sys/block/vda/device/scsi_device'
check "t26-d : écriture sur sdb et sda" vm_ssh vp-t26 10.26.1.10 'sudo dd if=/dev/zero of=/dev/sdb bs=1M count=8 oflag=direct status=none && sudo dd if=/dev/zero of=/dev/sda bs=1M count=8 oflag=direct status=none'
NODE
