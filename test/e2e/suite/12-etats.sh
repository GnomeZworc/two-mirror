# fonctionnalité: machine d'états — 409 pendant creating, suppression depuis error, sockets QEMU d'une VM en error retirés, dépendances VM → subnet → VPC, recréation sous le même nom
# hôtes: hv1
# tickets: #41, #84

on hv1 <<'NODE'
check "VPC vp-t12a" vpc_create vp-t12a 10.12.0.0/16
check "volume de t12-a" vm_volume t12-a
post_both () { api_post subnets "${1}" && api_post vms "${2}"; }
check "subnet sn-t12a et VM t12-a envoyés sans attendre" post_both \
    '{"name":"sn-t12a","vpc":"vp-t12a","mode":"vxlan","vxlan_id":1201,"iface_type":"vms","cidr":"10.12.1.0/24"}' \
    "$(vm_body t12-a sn-t12a 10.12.1.10)"
STATE=$(api_state vms/t12-a)
CODE=$(api_code DELETE vms/t12-a)
check "t12-a en creating au moment de la suppression (témoin de la fenêtre)" test "${STATE}" = creating
check "suppression de t12-a pendant creating refusée, 409" test "${CODE}" = 409
check "t12-a atteint running malgré la suppression refusée" wait_state vms/t12-a running

check "suppression de sn-t12a refusée tant que t12-a l'utilise, 409" expect_code 409 api_code DELETE subnets/sn-t12a
check "suppression de vp-t12a refusée tant que sn-t12a existe, 409" expect_code 409 api_code DELETE vpcs/vp-t12a
check "suppression d'une VM inconnue, 404" expect_code 404 api_code DELETE vms/t12-inconnue
check "suppression d'un subnet inconnu, 404" expect_code 404 api_code DELETE subnets/sn-t12z
check "suppression d'un VPC inconnu, 404" expect_code 404 api_code DELETE vpcs/vp-t12z

check "VM t12-err sur un disque absent : acceptée, 202 (pas de refus synchrone)" expect_code 202 api_post_code vms "$(vm_body t12-err sn-t12a 10.12.1.11 "${VOLUMES}/t12-absent.qcow2")"
check "t12-err passe en error" wait_state vms/t12-err error
info "t12-err : sockets laissés par QEMU : $(ls /run/two/vms/serial/t12-err.sock /run/two/vms/monitor/t12-err.sock /run/two/vms/qmp/t12-err.sock 2>&1 | tr '\n' ' ')"
check "t12-err : les trois sockets QEMU posés à la main s'ils manquent" sh -c 'for d in serial monitor qmp; do [ -e "/run/two/vms/${d}/t12-err.sock" ] || : > "/run/two/vms/${d}/t12-err.sock" || exit 1; done'
check "suppression de t12-err depuis error, 202" expect_code 202 api_code DELETE vms/t12-err
check "t12-err supprimée" wait_state vms/t12-err deleted
check "t12-err : aucun socket QEMU laissé dans /run/two/vms" sh -c 'for d in serial monitor qmp; do [ ! -e "/run/two/vms/${d}/t12-err.sock" ] || { echo "/run/two/vms/${d}/t12-err.sock laissé"; exit 1; }; done'
check "t12-err : aucune unité two-vm-t12-err" sh -c '! systemctl list-units --all --plain --no-legend "two-vm-t12-err*" | grep -q .'

check "suppression de t12-a" vm_delete t12-a
check "t12-a recréée sous le même nom" vm_create t12-a sn-t12a 10.12.1.10
check "suppression de t12-a recréée" vm_delete t12-a
check "suppression de sn-t12a, plus utilisé, 202 (témoin)" expect_code 202 api_code DELETE subnets/sn-t12a
check "sn-t12a supprimé" wait_state subnets/sn-t12a deleted
check "sn-t12a recréé sous le même nom" subnet_create sn-t12a vp-t12a 1201 10.12.1.0/24
check "suppression de sn-t12a recréé" expect_code 202 api_code DELETE subnets/sn-t12a
check "sn-t12a recréé puis supprimé" wait_state subnets/sn-t12a deleted
check "suppression de vp-t12a, sans subnet, 202 (témoin)" expect_code 202 api_code DELETE vpcs/vp-t12a
check "vp-t12a supprimé" wait_state vpcs/vp-t12a deleted
check "vp-t12a recréé sous le même nom" vpc_create vp-t12a 10.12.0.0/16
NODE
