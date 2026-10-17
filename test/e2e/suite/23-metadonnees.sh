# fonctionnalité: serveur de métadonnées et cloud-init — sshkey, password (hash), user_data servi tel quel, documents absents (templates), VM sans compte
# hôtes: hv1
# tickets: #43, #46

on hv1 <<'NODE'
KEY=$(vm_key)
HASH=$(openssl passwd -6 -salt t23lab two-lab)
UD=$(printf '%s\n' '#cloud-config' 'runcmd:' "  - [sh, -c, 'echo t23 > /var/tmp/t23-runcmd']")
UD64=$(printf '%s\n' "${UD}" | base64 -w0)
served () { ip netns exec vp-t23 curl -fsS "http://$(cat "/run/two/metadata/${1}/bind_ip"):$(cat "/run/two/metadata/${1}/bind_port")/${2}"; }
check "VPC vp-t23" vpc_create vp-t23 10.23.0.0/16
check "subnet sn-t23" subnet_create sn-t23 vp-t23 2301 10.23.1.0/24
check "VM t23-k : sshkey, password et user_data" vm_launch t23-k --nic sn-t23=10.23.1.10 --sshkey "${KEY}" --password "${HASH}" --user-data "${UD64}"
check "VM t23-s : sshkey seule (témoin)" vm_launch t23-s --nic sn-t23=10.23.1.11 --sshkey "${KEY}"
check "VM t23-n : ni sshkey ni password" vm_launch t23-n --nic sn-t23=10.23.1.12

check "t23-k joignable en SSH (sshkey servie)" vm_wait vp-t23 10.23.1.10
check "t23-k : meta-data, instance-id = nom de la VM" vm_grep vp-t23 10.23.1.10 'curl -fsS http://169.254.169.254/meta-data' '^instance-id: t23-k$'
check "t23-k : user-data servi tel quel" test "$(vm_meta vp-t23 10.23.1.10 user-data | sha256sum)" = "$(printf '%s\n' "${UD}" | sha256sum)"
check "t23-k : runcmd du user_data exécuté" vm_grep vp-t23 10.23.1.10 'cat /var/tmp/t23-runcmd' '^t23$'
check "t23-k : vendor-data porte la clé SSH" vm_grep vp-t23 10.23.1.10 'curl -fsS http://169.254.169.254/vendor-data' "$(cut -d" " -f2 <<< "${KEY}")"
check "t23-k : hash du password posé sur le compte syonad" vm_ssh vp-t23 10.23.1.10 "sudo grep -qF 'syonad:${HASH}:' /etc/shadow"

check "t23-s joignable en SSH" vm_wait vp-t23 10.23.1.11
check "t23-s : user_data absent, user-data servi vide (template)" test -z "$(vm_meta vp-t23 10.23.1.11 user-data)"
check "t23-s : aucun runcmd exécuté" vm_fails vp-t23 10.23.1.11 'test -e /var/tmp/t23-runcmd'
check "t23-s : password absent, compte syonad verrouillé" vm_ssh vp-t23 10.23.1.11 "sudo grep -q '^syonad:!' /etc/shadow"
check "t23-s : vendor-data sans passwd" vm_fails vp-t23 10.23.1.11 'curl -fsS http://169.254.169.254/vendor-data | grep -q passwd'

served_line () { served "${1}" "${2}" | grep -qx -- "${3}"; }
check "t23-n : meta-data servi (témoin : serveur de métadonnées actif)" served_line t23-n meta-data 'instance-id: t23-n'
check "t23-n : vendor-data vide, aucun compte à créer" test -z "$(served t23-n vendor-data)"
check "t23-n : user-data vide" test -z "$(served t23-n user-data)"
check "t23-n : démarrée, SSH refusé faute de compte (Permission denied)" ssh_denied vp-t23 10.23.1.12
NODE
