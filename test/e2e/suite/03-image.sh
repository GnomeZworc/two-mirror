# fonctionnalité: hypothèses sur l'image de VM — outils (curl, python3), nom d'interface, effet réel de network-config, forme du compte verrouillé
# hôtes: hv1
# tickets: #70, #73

on hv1 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t03" vpc_create vp-t03 10.3.0.0/16
check "subnet sn-t03a (VNI 301)" subnet_create sn-t03a vp-t03 301 10.3.1.0/24
check "VM t03-a (sshkey seule)" vm_create t03-a sn-t03a 10.3.1.10 "${KEY}"
check "t03-a joignable" vm_wait vp-t03 10.3.1.10
for tool in curl python3; do
    check "t03-a : ${tool} présent dans l'image (utilisé par 23 et 35)" vm_ssh vp-t03 10.3.1.10 "command -v ${tool}"
done
check "t03-a : interface ens3 présente (nom supposé par la suite)" vm_ssh vp-t03 10.3.1.10 'test -e /sys/class/net/ens3'
check "t03-a : aucune interface eth0" vm_ssh vp-t03 10.3.1.10 '! test -e /sys/class/net/eth0'
check "network-config servi à t03-a : entrée eth0 visée par la MAC de sa NIC" vm_grep vp-t03 10.3.1.10 'curl -fsS http://169.254.169.254/network-config' "macaddress: \"$(mac_for_ip 10.3.1.10)\""
NETFILE=$(vm_ssh vp-t03 10.3.1.10 'networkctl status ens3' | sed -n 's/^ *Network File: *//p')
info "t03-a : ens3 configurée par ${NETFILE:-aucun fichier networkd}"
check "t03-a : ens3 configurée par systemd-networkd (fichier : ${NETFILE:-aucun})" test -n "${NETFILE}"
not_eth0_file () { case "${NETFILE}" in ""|*eth0*) return 1 ;; esac; }
check "hypothèse : ens3 tient sa configuration du repli de cloud-init, qui ne lit pas network-config sous Debian 12 (fichier : ${NETFILE:-aucun})" not_eth0_file
SHADOW=$(vm_ssh vp-t03 10.3.1.10 "sudo awk -F: '\$1 == \"syonad\" {print substr(\$2, 1, 1)}' /etc/shadow")
check "t03-a : compte syonad verrouillé (lock_passwd), champ de /etc/shadow commençant par « ! » (forme lue par 23 ; lu : « ${SHADOW} »)" test "${SHADOW}" = '!'
NODE
