# syonad

A simple but powerful orchestrator, designed to be easy to use and API-first.

## deployer

```
curl 'https://git.g3e.fr/syonad/two/raw/branch/main/scripts/deploy.sh' -O
bash deploy.sh
mv deploy.sh /opt/two/bin/

curl 'https://git.g3e.fr/syonad/two/raw/branch/main/systemd/agent.service' -o '/etc/systemd/system/agent.service'
curl 'https://git.g3e.fr/syonad/two/raw/branch/main/systemd/dnsmasq@.service' -o '/etc/systemd/system/dnsmasq@.service'
curl 'https://git.g3e.fr/syonad/two/raw/branch/main/systemd/metadata@.service' -o '/etc/systemd/system/metadata@.service'
systemctl daemon-reload

modprobe br_netfilter
echo br_netfilter > /etc/modules-load.d/br_netfilter.conf
```
