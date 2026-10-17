# fonctionnalité: démarrage UEFI (OVMF) — firmware et variables par VM, VM démarrée en UEFI, témoin BIOS
# hôtes: hv1
# tickets: UEFI (sans ticket)

on hv1 <<'NODE'
KEY=$(vm_key)
check "prérequis : firmware OVMF /usr/share/OVMF/OVMF_CODE.fd présent" test -f /usr/share/OVMF/OVMF_CODE.fd
check "prérequis : modèle de variables /usr/share/OVMF/OVMF_VARS.fd présent" test -f /usr/share/OVMF/OVMF_VARS.fd
check "VPC vp-t24" vpc_create vp-t24 10.24.0.0/16
check "subnet sn-t24" subnet_create sn-t24 vp-t24 2401 10.24.1.0/24
check "VM t24-u en UEFI" vm_launch t24-u --nic sn-t24=10.24.1.10 --sshkey "${KEY}" --uefi
check "VM t24-b sans UEFI (témoin)" vm_launch t24-b --nic sn-t24=10.24.1.11 --sshkey "${KEY}"
check "t24-u : variables UEFI propres à la VM" test -f /run/two/vms/uefi/t24-u-uefi-vars.fd
check "t24-b : aucune variable UEFI" test ! -e /run/two/vms/uefi/t24-b-uefi-vars.fd
check "t24-u joignable" vm_wait vp-t24 10.24.1.10
check "t24-b joignable" vm_wait vp-t24 10.24.1.11
check "t24-u : démarrée en UEFI (/sys/firmware/efi)" vm_ssh vp-t24 10.24.1.10 'test -d /sys/firmware/efi'
check "t24-b : démarrée en BIOS, pas de /sys/firmware/efi" vm_fails vp-t24 10.24.1.11 'test -d /sys/firmware/efi'
check "suppression de t24-u" vm_delete t24-u
check "t24-u supprimée : variables UEFI retirées" test ! -e /run/two/vms/uefi/t24-u-uefi-vars.fd
NODE
