# fonctionnalité: validation de l'API — refus synchrones (400, 409, 422) des VPC, subnets et VM, sans trace en base
# hôtes: hv1
# tickets: #41, #57, #33, #62, #74, #81

on hv1 <<'NODE'
check "VPC vp-t11a, l3vni 1100 (témoin)" vpc_create vp-t11a 10.11.0.0/16 1100
check "VPC : nom déjà pris, 409" expect_code 409 api_post_code vpcs '{"name":"vp-t11a","cidr":"10.11.0.0/16"}'
check "VPC : cidr invalide, 400" expect_code 400 api_post_code vpcs '{"name":"vp-t11x","cidr":"10.11.0.0/33"}'
check "VPC : l3vni 1100 déjà prise par vp-t11a, 409" expect_code 409 api_post_code vpcs '{"name":"vp-t11x","cidr":"10.12.0.0/16","l3vni":1100}'
for vni in 0 253 254 255 65536; do
    check "VPC : l3vni ${vni} refusée, 400" expect_code 400 api_post_code vpcs "{\"name\":\"vp-t11x\",\"cidr\":\"10.12.0.0/16\",\"l3vni\":${vni}}"
done
check "VPC refusés : aucune trace de vp-t11x" expect_code 404 api_code GET vpcs/vp-t11x

check "subnet sn-t11a 10.11.1.0/24, VNI 1101 (témoin)" subnet_create sn-t11a vp-t11a 1101 10.11.1.0/24
check "subnet sn-t11g 10.11.2.0/24, VNI 1102, gateway explicite 10.11.2.1 (témoin)" subnet_create sn-t11g vp-t11a 1102 10.11.2.0/24 ',"default_route":true,"gateway":"10.11.2.1"'
check "sn-t11a : gateway calculée .254" test "$(api_field subnets/sn-t11a interface_ip)" = 10.11.1.254
check "sn-t11a : adresse d'émission calculée .253" test "$(api_field subnets/sn-t11a relay_ip)" = 10.11.1.253
check "subnet : nom déjà pris, 409" expect_code 409 api_post_code subnets '{"name":"sn-t11a","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1101,"cidr":"10.11.1.0/24"}'
check "subnet : mode inconnu, 422" expect_code 422 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vlan","vxlan_id":1109,"cidr":"10.11.9.0/24"}'
check "subnet : interface_ip fournie en vxlan, 400" expect_code 400 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1109,"cidr":"10.11.9.0/24","interface_ip":"10.11.9.1"}'
check "subnet : default_route sans gateway en vxlan, 400" expect_code 400 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1109,"cidr":"10.11.9.0/24","default_route":true}'
check "subnet : default_route sans gateway, mode omis, 400" expect_code 400 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","vxlan_id":1109,"cidr":"10.11.9.0/24","default_route":true}'
check "subnet : interface_ip absente en bridge, 400" expect_code 400 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"bridge","cidr":"10.11.9.0/24"}'
check "subnet : préfixe /30 en vxlan, 422" expect_code 422 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1109,"cidr":"10.11.9.0/30"}'
check "subnet : préfixe /29 en vxlan accepté (témoin)" expect_code 202 api_post_code subnets '{"name":"sn-t11p","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1103,"cidr":"10.11.3.0/29"}'
check "subnet : cidr avec bits d'hôte, 422" expect_code 422 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1109,"cidr":"10.11.9.1/24"}'
check "subnet : VPC inconnu, 422" expect_code 422 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11z","mode":"vxlan","vxlan_id":1109,"cidr":"10.11.9.0/24"}'
check "subnet : champ cidr manquant, 400" expect_code 400 api_post_code subnets '{"name":"sn-t11x","vpc":"vp-t11a","mode":"vxlan","vxlan_id":1109}'
check "subnets refusés : aucune trace de sn-t11x" expect_code 404 api_code GET subnets/sn-t11x
check "sn-t11p running" wait_state subnets/sn-t11p running

mkdir -p "${VOLUMES}"
check "VM t11-ok sur 10.11.1.10 (témoin)" vm_create t11-ok sn-t11a 10.11.1.10
check "VM : nom déjà pris, 409" expect_code 409 api_post_code vms "$(vm_body t11-ok sn-t11a 10.11.1.11)"
NONE="${VOLUMES}/t11-absent.qcow2"
for case in "gateway calculée:sn-t11a:10.11.1.254" "adresse d'émission:sn-t11a:10.11.1.253" "adresse de réseau:sn-t11a:10.11.1.0" \
            "broadcast:sn-t11a:10.11.1.255" "hors du CIDR:sn-t11a:10.11.9.10" "gateway explicite:sn-t11g:10.11.2.1"; do
    label=${case%%:*} rest=${case#*:}
    check "VM refusée sur ${label} (${rest#*:}), 422" expect_code 422 api_post_code vms "$(vm_body t11-x "${rest%%:*}" "${rest#*:}" "${NONE}")"
done
check "VM : subnet inconnu, 422" expect_code 422 api_post_code vms "$(vm_body t11-x sn-t11z 10.11.1.12 "${NONE}")"
check "VM : aucune interface primaire, 400" expect_code 400 api_post_code vms "$(vm_body t11-x sn-t11a 10.11.1.12 "${NONE}" | sed 's/"primary":true/"primary":false/')"
check "VM : interface sans mac, 400" expect_code 400 api_post_code vms "$(vm_body t11-x sn-t11a 10.11.1.12 "${NONE}" | jq -c 'del(.interfaces[0].mac)')"
for dev in sdb vdb xvdaa; do
    check "VM : disque additionnel ${dev}, 400" expect_code 400 api_post_code vms "$(vm_body t11-x sn-t11a 10.11.1.12 "${NONE}" | jq -c --arg d "${dev}" '.storage += [{"path":"/x.qcow2","dev":$d}]')"
done
check "VM : user_data en base64 invalide, 400" expect_code 400 api_post_code vms "$(vm_body t11-x sn-t11a 10.11.1.12 "${NONE}" '{"user_data":"pas du base64 !"}')"
check "VMs refusées : aucune trace de t11-x" expect_code 404 api_code GET vms/t11-x
check "volume de t11-ud" vm_volume t11-ud
check "VM t11-ud avec user_data en base64 valide, 202 (témoin)" expect_code 202 api_post_code vms "$(vm_body t11-ud sn-t11a 10.11.1.12 "" "{\"user_data\":\"$(printf '#cloud-config\n' | base64 | tr -d '\n')\"}")"
check "t11-ud running" wait_state vms/t11-ud running
NODE
