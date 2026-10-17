# fonctionnalité: isolation entre deux VPC sur un même hôte — ni ICMP ni TCP d'une VPC à l'autre
# hôtes: hv1
# tickets: #50

on hv1 <<'NODE'
KEY=$(vm_key)
check "VPC vp-t34a" vpc_create vp-t34a 10.34.0.0/17
check "VPC vp-t34b" vpc_create vp-t34b 10.34.128.0/17
check "subnet sn-t34a" subnet_create sn-t34a vp-t34a 3401 10.34.1.0/24
check "subnet sn-t34b" subnet_create sn-t34b vp-t34b 3402 10.34.129.0/24
check "VM t34-x dans vp-t34a" vm_create t34-x sn-t34a 10.34.1.10 "${KEY}"
check "VM t34-y dans vp-t34b" vm_create t34-y sn-t34b 10.34.129.10 "${KEY}"
check "t34-x joignable depuis sa VPC" vm_wait vp-t34a 10.34.1.10
check "t34-y joignable depuis sa VPC (témoin : la cible est vivante)" vm_wait vp-t34b 10.34.129.10
check "t34-x joint sa passerelle (témoin)" vm_ssh vp-t34a 10.34.1.10 'ping -c 2 -W 2 10.34.1.254'
check "t34-x ne joint pas t34-y en ICMP" vm_fails vp-t34a 10.34.1.10 'ping -c 2 -W 2 10.34.129.10'
check "t34-x n'ouvre pas de connexion TCP vers t34-y:22" vm_fails vp-t34a 10.34.1.10 'timeout 5 bash -c "</dev/tcp/10.34.129.10/22"'
check "t34-y ne joint pas t34-x en ICMP" vm_fails vp-t34b 10.34.129.10 'ping -c 2 -W 2 10.34.1.10'
NODE
