# fonctionnalité: VM à plusieurs disques — virtio (vdX) et SCSI (sdX), tailles et bus vus par la VM, démarrage sur vda
# hôtes: hv1
# tickets: #32

on hv1 <<'NODE'
KEY=$(vm_key)
mkdir -p "${VOLUMES}"
check "volume t26-d-vdb 1 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-vdb.qcow2" 1G
check "volume t26-d-sda 2 Gio" qemu-img create -q -f qcow2 "${VOLUMES}/t26-d-sda.qcow2" 2G
check "VPC vp-t26" vpc_create vp-t26 10.26.0.0/16
check "subnet sn-t26" subnet_create sn-t26 vp-t26 2601 10.26.1.0/24
check "VM t26-d : vda (système), vdb, sda" vm_launch t26-d --nic sn-t26=10.26.1.10 --sshkey "${KEY}" \
    --disk "vdb=${VOLUMES}/t26-d-vdb.qcow2" --disk "sda=${VOLUMES}/t26-d-sda.qcow2"
check "API : trois disques" test "$(curl -fsS "${API}/vms/t26-d" | jq -r '[.storage[].dev] | sort | join(" ")')" = "sda vda vdb"
check "t26-d joignable (démarrée sur vda)" vm_wait vp-t26 10.26.1.10
check "t26-d : racine sur vda" vm_grep vp-t26 10.26.1.10 'findmnt -n -o SOURCE /' '^/dev/vda'
check "t26-d : vdb de 1 Gio" test "$(vm_ssh vp-t26 10.26.1.10 'lsblk -b -dn -o SIZE /dev/vdb')" = 1073741824
check "t26-d : sda de 2 Gio" test "$(vm_ssh vp-t26 10.26.1.10 'lsblk -b -dn -o SIZE /dev/sda')" = 2147483648
check "t26-d : vdb en virtio-blk" vm_grep vp-t26 10.26.1.10 'readlink -f /sys/block/vdb/device/driver' 'virtio_blk$'
check "t26-d : sda en SCSI (virtio-scsi)" vm_ssh vp-t26 10.26.1.10 'test -d /sys/block/sda/device/scsi_device && readlink -f /sys/block/sda | grep -q virtio'
check "t26-d : vda n'est pas en SCSI (témoin)" vm_fails vp-t26 10.26.1.10 'test -d /sys/block/vda/device/scsi_device'
check "t26-d : écriture sur vdb et sda" vm_ssh vp-t26 10.26.1.10 'sudo dd if=/dev/zero of=/dev/vdb bs=1M count=8 oflag=direct status=none && sudo dd if=/dev/zero of=/dev/sda bs=1M count=8 oflag=direct status=none'
NODE
