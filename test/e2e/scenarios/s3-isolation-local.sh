on hv1 <<'NODE'
two_image || { echo "ÉCHOUÉ: image compatible two"; exit 0; }
KEY=$(vm_key)
check "VPC vp-s3a" vpc_create vp-s3a 10.230.0.0/16
check "VPC vp-s3b" vpc_create vp-s3b 10.231.0.0/16
check "subnet sn-s3a" subnet_create sn-s3a vp-s3a 2301 10.230.1.1 10.230.1.0/24
check "subnet sn-s3b" subnet_create sn-s3b vp-s3b 2302 10.231.1.1 10.231.1.0/24
check "VM s3-x dans vp-s3a" vm_create s3-x sn-s3a 10.230.1.10 "${KEY}"
check "VM s3-y dans vp-s3b" vm_create s3-y sn-s3b 10.231.1.10 "${KEY}"
check "s3-x joignable depuis sa VPC" vm_wait vp-s3a 10.230.1.10
check "s3-y joignable depuis sa VPC (témoin : la cible est vivante)" vm_wait vp-s3b 10.231.1.10
check "s3-x joint sa passerelle (témoin)" vm_ssh vp-s3a 10.230.1.10 'ping -c 2 -W 2 10.230.1.1'
check "s3-x ne joint pas s3-y en ICMP" vm_fails vp-s3a 10.230.1.10 'ping -c 2 -W 2 10.231.1.10'
check "s3-x n'ouvre pas de connexion TCP vers s3-y:22" vm_fails vp-s3a 10.230.1.10 'timeout 5 bash -c "</dev/tcp/10.231.1.10/22"'
check "s3-y ne joint pas s3-x en ICMP" vm_fails vp-s3b 10.231.1.10 'ping -c 2 -W 2 10.230.1.10'
NODE
