# syonad/two

Orchestrateur réseau et machines virtuelles mono-nœud, pensé pour être piloté par un logiciel
plutôt que par un humain.

Il expose une API HTTP qui crée des **VPC** — isolés par network namespace —, des **subnets** —
en VXLAN ou attachés à un bridge existant — et des **VM** QEMU/KVM raccordées à ces subnets, avec
DHCP, routage et metadata cloud-init fournis automatiquement.

## Installation

```bash
curl -O https://git.g3e.fr/syonad/two/raw/branch/main/scripts/deploy.sh
bash ./deploy.sh -t 0.1.0 -i
```

`deploy.sh` se met à jour lui-même depuis la branche, télécharge binaires, units systemd et
scripts depuis la release, et les vérifie contre le manifeste `SHA256SUMS`. Le drapeau `-i`
prépare l'host : paquets, module `br_netfilter`, `sysctl`, et bridges.

Options utiles :

| Option | Effet |
|---|---|
| `-t <tag>` | déployer une release donnée |
| `-b <branche>` | déployer depuis une branche au lieu d'une release |
| `-i` | préparer l'host (paquets, noyau, réseau) |
| `-u <iface>` | interface physique d'uplink, `eno1` par défaut |
| `-B <bridge>` | bridge principal auquel l'uplink est rattaché |
| `-d` | dry-run : affiche les commandes sans les exécuter |
| `-V` | désactiver la vérification des sommes de contrôle |

Un déploiement relève les instances `dnsmasq@` et `metadata@` actives **avant** l'arrêt des
services, et les redémarre ensuite — c'est la seule façon de savoir lesquelles relancer.

## Configuration

Un seul fichier, `/etc/two/agent.yml`, partagé par les trois binaires. Voir
[`conf/agent/config.exemple.yml`](conf/agent/config.exemple.yml) pour l'ensemble des options :
chemins de la base et des sockets QEMU, pool de workers, correspondance des types d'interface vers
les bridges physiques, watchdog, API d'administration, journalisation.

## Prise en main

```bash
# Un VPC, avec son CIDR interne
curl -X POST http://127.0.0.1:8080/vpcs \
  -d '{"name": "vp-admin", "cidr": "192.168.0.0/16"}'

# Un subnet en VXLAN dans ce VPC
curl -X POST http://127.0.0.1:8080/subnets \
  -d '{"name": "sn-000001", "vpc": "vp-admin", "mode": "vxlan", "vxlan_id": 1,
       "iface_type": "vms", "interface_ip": "10.1.1.1", "cidr": "10.1.0.0/23"}'

# Une VM, avec une clé SSH et un user-data cloud-init en base64
curl -X POST http://127.0.0.1:8080/vms \
  -d '{"name": "i-web", "memory": 2048, "cpus": 2,
       "metadata": {"sshkey": "ssh-ed25519 AAAA…",
                    "user_data": "'"$(base64 -w0 < user-data.yml)"'"},
       "interfaces": [{"subnet": "sn-000001", "ip": "10.1.1.2", "primary": true}],
       "storage": [{"path": "/data/disks/vms/i-web.qcow2", "dev": "vda"}]}'
```

Les créations sont **asynchrones** : l'API répond `202` et l'état de la ressource passe de
`creating` à `running` en base. `GET /vms/i-web` renvoie l'état courant.

Une VM peut porter plusieurs interfaces, dans un même VPC ; exactement une doit être marquée
`primary` — elle porte la route par défaut et le serveur de metadata.

La spécification complète est dans [`api/agent.yaml`](api/agent.yaml).

## Composants

| Binaire | Rôle |
|---|---|
| `agent` | processus principal : API, dispatcher, exécution, watchdog |
| `metadata` | serveur de metadata cloud-init, une instance par VM dans le netns du VPC |
| `db` | inspection de la base clé-valeur en ligne de commande |

L'agent prend `-config`, les deux autres `-conf`.
