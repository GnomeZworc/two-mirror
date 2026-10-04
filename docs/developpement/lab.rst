Lab de test multi-nœud
======================

Ce qui fait l'intérêt de two ne se voit qu'à partir de **deux hyperviseurs** : sur un nœud isolé,
le trafic reste sur le bridge local et l'absence de plan de contrôle passe inaperçue (voir
:doc:`/deploiement/architecture-cluster`). Le lab reproduit la topologie du cluster —
hyperviseurs, route reflector, switch L3 — sous forme de VM, sur un serveur physique loué à
l'heure.

Le pourquoi des choix (serveur physique plutôt que VM cloud, câbles QEMU, MTU 9000, versions) est
consigné sur le ticket `#50 <https://git.g3e.fr/syonad/two/issues/50>`_. Cette page décrit
comment s'en servir.

.. note::

   État actuel : étapes **E0** à **E3** livrées — le cycle de vie du serveur qui porte le lab
   (``scripts/lab-host.sh``), la description de la topologie et le calcul de son plan
   (``lab plan``), la génération des arguments QEMU et des fichiers cloud-init de chaque VM
   (``lab render``), puis leur lancement sur le serveur (``lab up`` / ``status`` / ``down`` /
   ``ssh``). Les rôles — FRR sur le switch et le route reflector, two sur les hyperviseurs —
   viendront avec l'étape suivante.

Le serveur de lab
-----------------

Un serveur **Scaleway Elastic Metal**, créé pour une campagne de tests puis supprimé.

.. list-table::
   :widths: 25 75

   * - Offre
     - ``EM-B212X-SSD``, zone ``fr-par-1``, **facturation horaire** : 0,321 € HT de l'heure, sans
       frais de mise en service
   * - Matériel
     - 2 × Xeon E5-2620 v4 *or equivalent*, 256 Go, 2 × 1 To SSD
   * - Système
     - Debian 12, installé par Scaleway à la création
   * - Pourquoi Intel
     - lab3 est en Intel : two lance ses VM en ``-cpu host``, et KVM a deux implémentations
       distinctes (``kvm_intel``, ``kvm_amd``)

*Or equivalent* n'est pas une clause de style : le premier serveur livré était un
**Xeon E5-2640 v3** (Haswell, la génération de lab3), pas le E5-2620 v4 annoncé. Relever
``lscpu`` au début de chaque campagne.

Prérequis côté Scaleway
-----------------------

#. **Un projet dédié au lab**, séparé de toute autre ressource. ``lab-host.sh down`` supprime
   tout serveur de lab du projet : il ne doit rien y avoir d'autre.
#. **Une clé d'API limitée à ce projet**, avec les droits Elastic Metal et la lecture des clés SSH
   du projet. Rien d'autre.
#. **Les clés SSH publiques enregistrées dans le projet**, injectées à l'installation : sans elle,
   le serveur serait facturé sans que personne puisse s'y connecter, et ``plan`` refuse de
   continuer.
#. **Le quota Elastic Metal.** L'``EM-B212X-SSD`` exige un compte dont le moyen de paiement *et*
   l'identité sont validés ; le quota est alors de 2. Vérifier dans la console : Organisation →
   Quotas → Elastic Metal. Voir `les quotas Scaleway
   <https://www.scaleway.com/en/docs/organizations-and-projects/organization/organization-quotas/>`_.

Fichiers locaux
---------------

Tout ce dont le script a besoin vit sous ``~/.config/two-lab/``, hors du dépôt.

``~/.config/two-lab/scaleway.env``
   Identifiants Scaleway, une ligne ``CLÉ=valeur`` chacun :

   .. code-block:: bash

      SCW_SECRET_KEY=<clé secrète>
      SCW_DEFAULT_PROJECT_ID=<identifiant du projet de lab>
      SCW_DEFAULT_ZONE=fr-par-1

   * le fichier doit être en ``0600`` : le script refuse de s'en servir s'il est lisible par
     d'autres que son propriétaire ;
   * il est **lu, jamais exécuté** — pas de ``source`` ; seules ces trois clés sont reconnues ;
   * une variable d'environnement du même nom l'emporte sur le fichier ;
   * la clé d'accès (``SCW…``) n'est pas nécessaire : l'API REST n'authentifie que par la clé
     secrète, dans l'en-tête ``X-Auth-Token``.

   Pour changer de clé, remplacer la ligne ``SCW_SECRET_KEY=`` ; rien d'autre à modifier.

``~/.config/two-lab/ssh/lab_ed25519``
   Clé SSH dédiée au lab, **sans phrase de passe**, pour que les sessions tournent sans
   intervention. Sa partie publique doit être enregistrée dans le projet. Quand elle existe, le
   script l'utilise **seule** (``IdentitiesOnly``, agent désactivé) ; sinon il retombe sur
   l'agent SSH.

   Elle ne doit ouvrir que les serveurs éphémères du projet de lab : **ne jamais l'installer sur
   lab3 ni sur une machine durable**. En cas de doute, la retirer du projet et en générer une
   autre :

   .. code-block:: bash

      mkdir -p ~/.config/two-lab/ssh && chmod 700 ~/.config/two-lab ~/.config/two-lab/ssh
      ssh-keygen -t ed25519 -N '' -C two-lab-automation -f ~/.config/two-lab/ssh/lab_ed25519

``~/.cache/two-lab/``
   État de la session en cours : adresse et utilisateur du serveur, ``known_hosts`` dédié. Vidé
   par ``down``.

Commandes
---------

.. code-block:: text

   usage: lab-host.sh <commande> [arguments]

     plan              résout l'offre horaire, l'OS et les clés SSH, affiche la requête de création
                       et le prix ; ne crée rien
     up                crée le serveur de lab, attend la fin de son installation et son SSH,
                       puis le prépare (voir prepare)
     status            liste les serveurs de lab du projet
     ssh [commande]    se connecte au serveur de lab ; avec une commande, un terminal n'est demandé
                       que si l'entrée standard en est un
     prepare           installe sur le serveur ce dont lab a besoin (qemu, genisoimage), vérifie
                       /dev/kvm et la virtualisation imbriquée ; lancé aussi par up
     push <topologie>  compile cmd/lab pour linux/amd64 et dépose sur le serveur ~/lab et le
                       répertoire de la topologie dans ~/topology/ (avec les fichiers qu'elle
                       référence) ; ensuite : ssh './lab up topology/<topologie>'
     down              supprime tous les serveurs de lab du projet et attend leur disparition
     session [cmd]     up, puis la commande distante (ou un shell), puis down quoi qu'il arrive

``plan`` et ``status`` sont **gratuits** ; ``up`` et ``session`` **créent un serveur facturé**.

Toujours commencer par ``plan``. Il valide la clé d'API, le quota d'offre, l'OS et les clés SSH,
et montre exactement ce qui serait commandé :

.. code-block:: text

   $ scripts/lab-host.sh plan
   == offre   : EM-B212X-SSD (ddaf8ba6-b2b2-4279-8af3-51930fb602f8), facturation hourly, stock available
   == prix    : 0.321 EUR HT par heure, frais de mise en service 0 EUR
   == os      : Debian 12 (Bookworm) (83640d93-a0b8-45ad-9c9f-30cae48380a4), utilisateur root
   == clés    : 2 clé(s) SSH du projet
   == requête : POST /baremetal/v1/zones/fr-par-1/servers

``session`` est la forme normale d'usage : le serveur est supprimé à la fin, que la commande
réussisse, échoue, ou que la session soit interrompue (Ctrl-C, ``TERM``, fermeture du terminal).

.. code-block:: text

   $ scripts/lab-host.sh session 'uname -a; lscpu | grep -E "Model name|^CPU\(s\)|Virtualization"; free -g | head -2; echo "nested=$(cat /sys/module/kvm_intel/parameters/nested)"; ls -l /dev/kvm'
   == création de two-lab (EM-B212X-SSD, 0.321 EUR/h HT)
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 créé, facturé jusqu'à 'lab-host.sh down'
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ordered, installation to_install
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ready, installation installing
   …
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ready, installation completed
   == SSH pas encore joignable, nouvel essai dans 20s
   …
   == prêt : root@<adresse>
   Linux two-lab 6.1.0-53-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.1.187-1 (2026-09-07) x86_64 GNU/Linux
   CPU(s):                                  32
   Model name:                              Intel(R) Xeon(R) CPU E5-2640 v3 @ 2.60GHz
   …
   Virtualization:                          VT-x
                  total        used        free      shared  buff/cache   available
   Mem:             251           1         250           0           0         249
   nested=Y
   crw-rw---- 1 root kvm 10, 232 Oct  3 17:23 /dev/kvm
   == session terminée (code 0), suppression du serveur
   == suppression de 2ecc1e6a-0de8-48c0-a198-93857eee5957
   == aucun serveur de lab ne reste dans le projet

Extrait de la première campagne (lignes répétées remplacées par ``…``). Compter **environ 15 minutes** entre la création et le SSH disponible : 13 min 30 à la première
campagne, suppression comprise. SSH ne répond pas tout de suite après la fin de l'installation —
environ 100 secondes la première fois — d'où l'attente intégrée à ``up``. Le code de sortie de
``session`` est celui de la commande distante.

``up``, ``ssh`` et ``down`` séparément servent au debug interactif — et laissent la suppression
à la charge de l'utilisateur.

Une campagne sur le lab enchaîne ces commandes depuis le Mac ; ``lab`` s'exécute sur le serveur
(voir `Lancement des VM`_) :

.. code-block:: text

   scripts/lab-host.sh up
   scripts/lab-host.sh push conf/lab/evpn-2hv.yml
   scripts/lab-host.sh ssh './lab up topology/evpn-2hv.yml'
   scripts/lab-host.sh ssh './lab ssh hv1'          # shell interactif sur hv1
   scripts/lab-host.sh ssh './lab ssh hv1 ip -br a' # commande, code de retour propagé
   scripts/lab-host.sh down

``push`` transfère par la connexion SSH du script — mêmes options, même clé, même
``known_hosts`` que ``ssh`` : le binaire par ``cat``, le répertoire de la topologie par ``tar``
(sans les métadonnées macOS), chacun renommé une fois complet. Tout le répertoire part, pour que
les fichiers que la topologie référence (``frr/*.conf``) arrivent avec elle.

Topologie
---------

Un lab est décrit par un fichier YAML : des **nœuds** (les VM) et des **segments** (des réseaux L2
portés par un switch). Exemple livré, ``conf/lab/evpn-2hv.yml`` :

.. literalinclude:: ../../conf/lab/evpn-2hv.yml
   :language: yaml

Chaque nœud non-switch est relié au switch de chacun de ses segments par un câble virtuel QEMU ;
le switch met ces câbles dans un bridge et porte la passerelle du segment.

Ce que le fichier déclare :

``images``
   ``url`` de l'image qcow2 et ``sums`` du fichier de sommes à vérifier, tous deux en ``https://``.

``segments``
   ``switch`` (un nœud de rôle ``switch``), ``cidr`` IPv4 entre ``/8`` et ``/30``, ``mtu``
   facultatif — 9000 par défaut, entre 1280 et 9000. Nom : 12 caractères au plus, minuscules et
   chiffres, parce qu'il devient le nom d'interface dans les VM et, préfixé de ``br-``, celui du
   bridge (15 caractères au plus sous Linux).

``nodes``
   ``role`` (``switch``, ``rr`` ou ``hypervisor``), ``image``, ``cpus``, ``memory`` en Mio
   (256 au moins), ``segments`` auxquels le nœud est relié, et ``addresses`` pour fixer
   l'adresse d'un nœud sur un segment (``addresses: {underlay: 192.168.14.50}``). Un switch ne
   déclare ni ``segments`` ni ``addresses`` : il porte ceux dont il est le ``switch``.

   Champs de rôle, facultatifs :

   * ``secondary`` — des adresses supplémentaires par segment, avec leur longueur de préfixe
     (``secondary: {underlay: [169.254.0.3/28]}``), posées sur la même interface que l'adresse
     principale : même L2, même MAC. Elles doivent être **hors** du CIDR du segment, pour ne
     jamais croiser l'attribution automatique. Sur un switch, elles vont sur le bridge du
     segment ;
   * ``loopback`` — une adresse sur une interface ``dummy`` nommée ``lo1``
     (``loopback: 10.255.255.1/32``) ;
   * ``frr`` — le chemin d'un ``frr.conf``, relatif au fichier de topologie : FRR est installé
     au démarrage et la configuration déposée **telle quelle** (voir `Rôles`_).
   * ``release`` — **obligatoire pour un hyperviseur**, refusé ailleurs : le tag de la release de
     two que ``deploy.sh`` installe (``release: 0.2.0rc002``). Sans lui, ``deploy.sh`` prendrait la
     dernière release, et le lab ne serait plus reproductible.

   ``mgmt0`` et ``lo1`` sont réservés : aucun segment ne peut porter ces noms.

Ce que l'outil en déduit, de façon déterministe — même fichier, même plan :

.. list-table::
   :widths: 30 70

   * - Passerelle d'un segment
     - la première adresse du CIDR, portée par le switch sur ``br-<segment>``
   * - Adresse d'un nœud
     - les suivantes, **dans l'ordre de déclaration des nœuds** ; une adresse fixée par
       ``addresses`` est réservée d'abord et sautée par l'attribution automatique
   * - Câbles
     - un par couple (segment, nœud), segments puis nœuds dans l'ordre de déclaration ; le
       câble *i* utilise les ports UDP ``20000 + 2i`` (côté nœud) et ``20001 + 2i`` (côté switch)
   * - MAC
     - ``02:4c:<nœud>:<nœud>:<segment>:<côté>`` — préfixe localement administré, rang du nœud
       sur deux octets, rang du segment, ``00`` côté nœud et ``01`` côté switch
   * - Interfaces
     - côté nœud, le nom du segment ; côté switch, ``p<i>``, du rang du câble
   * - SSH d'administration
     - ``127.0.0.1:<2200 + rang du nœud>`` sur l'hôte du lab

.. warning::

   Réordonner les nœuds ou les segments dans le fichier **change les adresses, les MAC et les
   ports**. C'est assumé pour un lab ; ``lab plan`` montre le résultat avant tout lancement.

Limites : 1000 nœuds, 256 segments, et autant de câbles que la plage UDP le permet (22 768).

``lab plan`` valide le fichier et affiche le plan, sans rien lancer :

.. code-block:: text

   $ go run ./cmd/lab plan conf/lab/evpn-2hv.yml
   lab evpn-2hv: nodes 4, segments 1, cables 3

   nodes
     name  role        image     cpus  memory     ssh
     sw1   switch      debian12  2     1024 MiB   127.0.0.1:2200
     rr1   rr          debian12  1     1024 MiB   127.0.0.1:2201
     hv1   hypervisor  debian12  4     16384 MiB  127.0.0.1:2202
     hv2   hypervisor  debian12  4     16384 MiB  127.0.0.1:2203

   roles
     name  loopback             secondary                frr       release
     sw1   -                    underlay 169.254.0.1/28  sw1.conf  -
     rr1   lo1 10.255.255.1/32  underlay 169.254.0.3/28  rr1.conf  -
     hv1   -                    -                        hv1.conf  0.2.0rc002
     hv2   -                    -                        hv2.conf  0.2.0rc002

   segment underlay: 192.168.14.0/24, mtu 9000, switch sw1, bridge br-underlay, gateway 192.168.14.1
     node  interface  address           mac                udp         switch port  mac                udp
     rr1   underlay   192.168.14.2/24   02:4c:00:01:00:00  20000  <->  sw1 p0       02:4c:00:01:00:01  20001
     hv1   underlay   192.168.14.11/24  02:4c:00:02:00:00  20002  <->  sw1 p1       02:4c:00:02:00:01  20003
     hv2   underlay   192.168.14.12/24  02:4c:00:03:00:00  20004  <->  sw1 p2       02:4c:00:03:00:01  20005

Un fichier invalide est refusé avec **toutes** ses erreurs à la fois, et un code de sortie 1. Les
champs inconnus et les clés en double sont refusés aussi :

.. code-block:: text

   $ lab plan cassee.yml
   lab: cassee.yml:
   segment underlay: rr1 is a rr, not a switch
   segment underlay: cidr 10.250.0.0/31 prefix length out of range [/8, /30]
   node sw1: switch carries no segment

Les ASN, la loopback du route reflector, le lien ``169.254.0.0/28`` et le subnet des hyperviseurs
de l'exemple sont **ceux de la production** (décision du 2026-10-04, #50) : les fichiers de
``conf/lab/`` restent ainsi au plus près de ce qui tourne réellement. Toutes les adresses y sont
**fixées** par ``addresses`` — le route reflector en ``.2``, les hyperviseurs à partir de ``.11`` —
pour que le modèle se lise sans le plan et ne dépende pas de l'ordre de déclaration : le
``frr.conf`` d'un hyperviseur, écrit à la main, porte son adresse en ``router-id``. Seul le switch
n'en déclare pas : il porte toujours la passerelle, la première adresse du segment.

Rôles
~~~~~

Les configurations FRR du lab vivent dans ``conf/lab/frr/``, une par nœud, **écrites à la main** :
ce sont les mêmes fichiers que la documentation de déploiement inclut, pour que le lab qualifie
exactement ce qu'elle prescrit. Celle du route reflector :

.. literalinclude:: ../../conf/lab/frr/rr1.conf
   :language: text

Au premier démarrage, cloud-init installe FRR (``frr-stable`` de ``deb.frrouting.org``, sans les
paquets recommandés), active ``bgpd`` — et ``bfdd`` sur le switch et le route reflector —, puis
dépose le ``frr.conf`` du nœud et redémarre FRR. La mise à jour des index de paquets est réessayée
pendant cinq minutes : un nœud peut démarrer avant que le switch, par lequel il sort, n'ait posé
son NAT.

La **clé du dépôt FRR** n'est pas téléchargée au démarrage : elle est enregistrée dans ``lab``
(``internal/lab/render/frrouting.gpg``) et déposée par cloud-init. Elle a été récupérée le
2026-10-04 sur ``deb.frrouting.org`` ; les empreintes de ses clés primaires sont publiées sous la
même valeur sur ``keys.openpgp.org`` et ``keyserver.ubuntu.com`` :

.. code-block:: text

   3D99 68AC 9AE7 BE11 6928  8DDB 1FD5 8398 95F5 7FDA   David Lamparter
   4A56 C773 8BB3 F815 95A8  05D2 A832 7699 08F1 3ED1   FRRouting Debian Repository
   A90F C36D 9429 4097 98E9  C2D8 74DE ED43 AB19 4DBF   Jafar Al-Gharaibeh

Une clé renouvelée par FRR fera échouer l'installation (signature inconnue) : remplacer le fichier
après avoir vérifié les nouvelles empreintes.

**Hyperviseurs.** Ils se déploient comme en production, par ``deploy.sh`` — celui **du dépôt**,
embarqué dans ``lab`` avec ``bootstrap_kvm.sh`` (paquet ``scripts``) et déposé dans
``/opt/two/scripts/``, où ``deploy.sh`` cherche d'abord ``bootstrap_kvm.sh`` :

.. code-block:: text

   deploy.sh --noup_script -i -u <segment> -t <release>

``--noup_script`` empêche l'auto-mise à jour de remplacer le script par celui de ``main`` : le lab
teste les scripts de sa branche. L'uplink ``-u`` est l'interface qui porte la route par défaut —
celle du premier segment de l'hyperviseur dans l'ordre de déclaration des segments — parce que
``deploy.sh`` y lit l'adresse et la passerelle qu'il déplace sur ``br-000000``. ``deploy.sh``
télécharge la release sur ``git.g3e.fr`` sans réessayer : le lancement attend d'abord que le serveur
réponde, à travers le switch. FRR est installé **après** : il démarre sur le réseau final.

**Un seul script de provisionnement par nœud.** cloud-init exécute ``runcmd`` comme un script
``sh`` sans ``set -e`` : seule la dernière commande compte, et un ``deploy.sh`` en échec suivi d'un
FRR installé avec succès passerait pour un démarrage réussi. Chaque nœud reçoit donc
``/usr/local/sbin/lab-provision``, en ``set -eu``, qui enchaîne ses étapes ; ``runcmd`` n'appelle
que lui, et la première étape en échec met cloud-init en erreur.

**Ce que vérifie** ``lab up``, une fois cloud-init terminé sans erreur : ``agent.service`` actif
sur chaque hyperviseur, ``frr`` actif sur chaque nœud qui en a un. Ce contrôle couvre ce que
cloud-init ne voit pas — si la migration réseau échoue, ``deploy.sh`` arme un redémarrage de
secours, et la VM redémarrée ne rejoue pas ``runcmd``.

.. warning::

   **Un hyperviseur du lab ne survit pas à un redémarrage.** En production, la racine est en
   tmpfs et ``deploy.sh --bootstrap`` est rejoué à chaque démarrage ; dans le lab, ``-i`` n'est
   exécuté qu'au premier, et la migration réseau, qui n'est pas persistée, est perdue. Recréer le
   lab : ``lab down`` puis ``lab up``.

Vérifié le 2026-10-04 sur le serveur de lab, topologie ``evpn-2hv``, release ``0.2.0rc002`` :

.. code-block:: text

   $ scripts/lab-host.sh ssh './lab up -timeout 25m topology/evpn-2hv.yml'
   sw1: started
   rr1: started
   hv1: started
   hv2: started
   sw1: ready
   rr1: ready
   hv1: ready
   hv2: ready

.. list-table::
   :header-rows: 1
   :widths: 55 45

   * - Vérification
     - Résultat
   * - ``lab up`` complet : FRR, ``deploy.sh`` et vérification des services de chaque rôle
     - 6 min 15
   * - session switch ↔ route reflector, IPv4 unicast, BFD
     - Established, BFD up ; le switch reçoit la seule loopback du route reflector
   * - sessions EVPN des deux hyperviseurs vers la loopback du route reflector
     - Established, voisins dynamiques, stables
   * - réseau d'un hyperviseur après ``deploy.sh``
     - adresse sur ``br-000000``, MTU 9000, API de l'agent qui répond
   * - VPC, subnet ``vxlan`` et VM Debian ``genericcloud`` créés par l'API de hv1
     - ``login:`` en 20 s, en KVM imbriqué
   * - DHCP et routes (option 121) servis par two à la VM
     - conformes, route ``/32`` vers ``169.254.169.254`` comprise
   * - métadonnées, image configurée selon :doc:`/deploiement/image-qcow2`
     - ``DataSourceNoCloudNet``, nom d'hôte appliqué — avec la barre oblique finale de
       ``seedfrom`` (voir cette page)
   * - VM ↔ VM entre les deux hyperviseurs, même subnet ``vxlan``
     - **échec** : les VXLAN de two n'ont pas d'adresse VTEP locale, rien n'est annoncé en EVPN —
       `#51 <https://git.g3e.fr/syonad/two/issues/51>`_ ; avec l'adresse posée, ping et MTU 1500
       passent

.. note::

   La configuration du switch (``conf/lab/frr/sw1.conf``) **n'est pas celle des routeurs** : écrite
   pour l'essai du 2026-10-04, elle se contente d'établir la session avec le route reflector et de
   n'accepter que sa loopback. Elle sera remplacée par la configuration réelle des routeurs.

Rendu des VM
------------

``lab render`` produit, pour chaque nœud, ce qu'il faut pour démarrer sa VM — sans rien lancer :

.. code-block:: text

   $ go run ./cmd/lab render -key ~/.config/two-lab/ssh/lab_ed25519.pub conf/lab/evpn-2hv.yml <répertoire>

``<répertoire>/<nœud>/`` reçoit :

``qemu.args``
   Les arguments de ``qemu-system-x86_64``, **un par ligne** : rien à échapper, rien à
   interpréter par un shell.

``meta-data``, ``user-data``, ``network-config``
   Les trois fichiers NoCloud de cloud-init, à mettre dans une image de volume ``cidata``.

Les chemins de la VM (``disk.qcow2``, ``seed.iso``, ``console.log``, ``qmp.sock``, ``qemu.pid``)
sont ceux du répertoire du nœud ; ``-key`` peut être répété, et accepte un fichier
``authorized_keys`` (lignes vides et commentaires ignorés). Les fichiers sont créés en ``0600``.

Ce que contiennent les arguments QEMU d'un hyperviseur — extrait réel, côté réseau :

.. code-block:: text

   -netdev
   user,id=mgmt0,restrict=on,ipv6=off,hostfwd=tcp:127.0.0.1:2202-:22
   -device
   virtio-net-pci,netdev=mgmt0,mac=02:4d:00:02:00:00,romfile=
   -netdev
   dgram,id=underlay,local.type=inet,local.host=127.0.0.1,local.port=20002,remote.type=inet,remote.host=127.0.0.1,remote.port=20003
   -device
   virtio-net-pci,netdev=underlay,mac=02:4c:00:02:00:00,host_mtu=9000,romfile=

Les choix qui s'y lisent :

* **machine** ``q35``, ``-accel kvm -cpu host`` — le KVM imbriqué des hyperviseurs du lab en
  dépend ; ``-nodefaults`` pour qu'aucun périphérique implicite ne s'ajoute ;
* **administration** (``mgmt0``) : le NAT de QEMU, MAC ``02:4d:<nœud>:<nœud>:00:00``, SSH redirigé
  sur la boucle locale de l'hôte. ``restrict=on`` pour tous les nœuds **sauf le switch** : un
  nœud isolé ne joint ni l'hôte ni l'extérieur par là, seule la redirection SSH passe.
  ``ipv6=off`` partout (voir plus bas) ;
* **câbles** : ``dgram`` sur ``127.0.0.1``, les deux extrémités d'un câble se répondent
  (port local de l'une = port distant de l'autre), ``host_mtu`` annonce le MTU du segment au
  guest ;
* ``romfile=`` vide sur toutes les cartes : pas de ROM de démarrage réseau, donc pas de repli
  sur un démarrage PXE si le firmware ne trouve pas le disque. Pendant les essais de #50, une VM
  restée bloquée sans rien écrire sur sa console, CPU au repos, avait toutes les apparences de
  ce repli ; la cause n'a pas été isolée, l'option est une précaution.

Ce que fait cloud-init :

* **toutes les VM** : interfaces nommées d'après leur MAC (``mgmt0``, nom du segment, ``p<i>``),
  ``dhcp4: false`` partout, ``mgmt0`` en ``10.0.2.15/24`` **sans passerelle** ; connexion SSH par
  clé seulement, utilisateur ``debian``, ``root`` désactivé, mot de passe refusé ;
* **un nœud** : adresse sur chaque segment, MTU du segment, route par défaut et DNS
  (``1.1.1.1``, ``8.8.8.8``) sur son **premier** segment — la sortie Internet passe par le
  switch ;
* **le switch** : route par défaut par ``mgmt0`` ; un service ``lab-switch`` crée un bridge
  ``br-<segment>`` par segment (STP désactivé, MTU du segment), y branche ses ports, porte la
  passerelle, active le routage et masque (NAT nftables) les segments vers ``mgmt0``. Le
  service est rejoué à chaque démarrage.

Vérifié sur de vraies VM
~~~~~~~~~~~~~~~~~~~~~~~~

Le switch et le route reflector n'ont pas besoin de KVM imbriqué : ``sw1`` et ``rr1`` de
l'exemple ont été démarrés **sur un Mac**, en émulation (TCG), avec Debian 12 ``generic`` et
les fichiers produits par ``lab render``.

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Vérification
     - Résultat
   * - interfaces nommées et adressées, bridge ``br-underlay`` en ``10.250.0.1/24``
     - conforme
   * - service ``lab-switch`` actif, y compris après redémarrage
     - conforme
   * - ``ping -M do -s 8972`` de ``rr1`` vers le switch (MTU 9000, sans fragmentation)
     - passe
   * - ``ping -M do -s 8973`` (MTU 9001)
     - refusé : ``message too long, mtu=9000``
   * - Internet depuis ``rr1`` en IPv4
     - passe, par ``10.250.0.1``
   * - ``rr1`` vers un service TCP de l'hôte par ``mgmt0`` — le switch, témoin, y parvient
     - bloqué
   * - ``rr1`` vers Internet par ``mgmt0``
     - bloqué

Un défaut trouvé par cet essai, et corrigé : sans ``ipv6=off``, le NAT de QEMU annonce un
préfixe IPv6 et ``mgmt0`` reçoit une **route IPv6 par défaut** — vers une impasse, puisque
``restrict=on`` bloque tout. Pas de fuite, mais chaque programme qui tente l'IPv6 d'abord (le DNS
renvoie d'abord des adresses IPv6) attend un délai avant de se rabattre sur l'IPv4.

.. note::

   Un ``ping`` vers ``10.0.2.2`` n'est pas un test d'isolation : c'est la passerelle virtuelle de
   QEMU qui répond elle-même, ``restrict=on`` ou non. Seule une connexion vers un vrai service de
   l'hôte, avec un témoin qui y parvient, le prouve.

Reste à vérifier sur le serveur de lab : les hyperviseurs, qui exigent KVM imbriqué.

Lancement des VM
----------------

``lab`` s'exécute **sur le serveur de lab**. Il garde l'état du lab dans un répertoire
(``-run``, par défaut ``~/lab-run``) : ``status``, ``down`` et ``ssh`` n'ont donc pas besoin de la
topologie.

.. code-block:: text

   lab up [-run dir] [-cache dir] [-timeout 20m] <topologie.yml>
   lab status [-run dir]
   lab down [-run dir]
   lab ssh [-run dir] <nœud> [commande…]

``lab up``
   1. refuse de continuer si un lab tourne déjà dans le répertoire ;
   2. télécharge chaque image dans le cache (``-cache``, par défaut ``~/.cache/two-lab``) et la
      vérifie contre ``SHA512SUMS`` ; une image déjà présente et toujours conforme n'est pas
      retéléchargée, la liste des sommes est relue à chaque fois ;
   3. génère une paire de clés SSH dans le répertoire du lab, si elle n'existe pas encore ;
   4. pour chaque nœud : fichiers de ``lab render``, disque **neuf** en overlay qcow2 sur
      l'image (``qemu-img create -b``, 20 Gio annoncés), image ``cidata`` (``genisoimage``) ;
   5. démarre les QEMU, **switchs d'abord**, détachés (``-daemonize``) : ils survivent à la
      session SSH qui les a lancés ;
   6. attend sur chaque nœud la fin de cloud-init (``cloud-init status --wait`` par SSH),
      jusqu'au délai ``-timeout``.

   La topologie est copiée dans ``<run>/topology.yml``. Un échec laisse les nœuds démarrés en
   place : ``lab status``, puis ``lab down``.

``lab status``
   Pour chaque nœud : rôle, état du processus QEMU, PID, port SSH sur la boucle locale.

``lab down``
   Arrête chaque QEMU par ``SIGTERM``, puis ``SIGKILL`` au bout de 30 s. Les disques sont
   conservés jusqu'au prochain ``up``, qui les recrée.

``lab ssh``
   Ouvre un shell sur un nœud, ou y exécute une commande, avec la clé générée par ``up``. ``lab``
   cède la place à ``ssh``, dont le code de retour est donc celui de la commande. Un terminal
   n'est demandé (``-t``) que si l'entrée de ``lab`` en est un : depuis un script, ni
   pseudo-terminal ni ``\r\n`` dans la sortie.

   Comme ``ssh``, ``lab ssh`` recolle ses arguments par des espaces et les confie à un shell
   distant — et depuis le Mac, il y en a **deux** : celui du serveur, puis celui de la VM.
   Une commande qui contient elle-même des guillemets se passe en une seule chaîne :

   .. code-block:: text

      $ echo | scripts/lab-host.sh ssh './lab ssh hv1 sh -c "exit 42"'; echo "rc 42=$?"
      rc 42=0
      $ echo | scripts/lab-host.sh ssh "./lab ssh hv1 'sh -c \"exit 42\"'"; echo "rc 42=$?"
      rc 42=42

   Dans le premier cas, la VM reçoit ``sh -c exit 42`` : ``exit`` sans argument, ``42`` en
   ``$0``. Pour plus d'une commande, passer un script sur l'entrée standard :
   ``scripts/lab-host.sh ssh "./lab ssh hv1 'sudo bash -s'" < script.sh``.

Un processus n'est tenu pour celui d'un nœud que si son PID, lu dans ``qemu.pid``, désigne un
processus vivant dont la ligne de commande (``/proc/<pid>/cmdline``) contient ``-name <nœud>``.
Un PID réutilisé par un autre programme n'est donc jamais signalé.

.. warning::

   Le cache range une image sous son nom de fichier, et l'URL de Debian est ``latest`` : une
   nouvelle publication remplace le fichier, et les overlays existants pointeraient sur une
   base différente. ``up`` recrée toujours les disques, ce qui suffit avec un lab par serveur ;
   **ne pas relancer un QEMU à la main** à partir d'un ``qemu.args`` après un ``up`` ultérieur.

Une campagne réelle, de la création du serveur à la première commande sur une VM — sorties du
2026-10-04 :

.. code-block:: text

   $ scripts/lab-host.sh up
   == création de two-lab (EM-B212X-SSD, 0.321 EUR/h HT)
   …
   == préparation du serveur : qemu, genisoimage, KVM imbriqué
   …
   qemu QEMU emulator version 7.2.22 (Debian 1:7.2+dfsg-7+deb12u18+b3), nested=Y
   == prêt : root@<adresse>
   $ scripts/lab-host.sh push conf/lab/evpn-2hv.yml
   == compilation de lab (linux/amd64)
   == déposés sur le serveur : ~/lab, ~/evpn-2hv.yml — ensuite : lab-host.sh ssh './lab up evpn-2hv.yml'
   $ scripts/lab-host.sh ssh './lab up evpn-2hv.yml'
   sw1: started
   rr1: started
   hv1: started
   hv2: started
   sw1: ready
   rr1: ready
   hv1: ready
   hv2: ready
   $ scripts/lab-host.sh ssh './lab status'
   node  role        state    pid   ssh
   sw1   switch      running  5158  127.0.0.1:2200
   rr1   rr          running  5171  127.0.0.1:2201
   hv1   hypervisor  running  5182  127.0.0.1:2202
   hv2   hypervisor  running  5196  127.0.0.1:2203

``lab up`` a pris **49 secondes**, téléchargement et vérification de l'image (427 Mio) compris ;
l'essentiel du temps d'une campagne est la livraison du serveur (environ 25 minutes avec
``prepare``).

Vérifié sur le serveur de lab
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Le 2026-10-04, sur un Xeon E5-2640 v3, Debian 12 et QEMU 7.2 sur le serveur, topologie
``evpn-2hv`` :

.. list-table::
   :header-rows: 1
   :widths: 60 40

   * - Vérification
     - Résultat
   * - ``ping -M do -s 8972`` de hv1 à hv2 à travers le switch
     - passe
   * - ``ping -M do -s 8973``
     - refusé : ``message too long, mtu=9000``
   * - sortie Internet de hv1
     - par ``10.250.0.1`` (le switch), HTTPS 200 ; aucune route IPv6 globale
   * - hv1 vers un service TCP du serveur par ``mgmt0`` — sw1, témoin, y parvient (200)
     - refusé
   * - hv1 vers Internet par ``mgmt0``, route forcée via ``10.0.2.2`` — sw1 y parvient
     - refusé
   * - ``/dev/kvm`` et ``nested`` dans hv1
     - présent, ``Y``
   * - racine de hv1 (overlay de 20 Gio)
     - 20 Go : ``growpart`` agrandit la partition au premier démarrage
   * - code de retour à travers ``lab-host.sh ssh`` et ``lab ssh``, sans terminal
     - propagé jusqu'au Mac
   * - ``scripts/lab-host.sh ssh './lab ssh hv1'`` depuis un terminal
     - shell interactif
   * - ``lab down`` puis ``lab up`` d'une autre topologie
     - conforme

Facturation
-----------

.. warning::

   Un serveur Elastic Metal est facturé **de sa création à sa suppression, éteint compris**.
   Éteindre ne suffit pas : il faut supprimer. La granularité n'est pas documentée par Scaleway —
   compter chaque heure entamée comme une heure pleine.

Ce que le script garantit :

* il ne commande **jamais** d'offre mensuelle : il exige une seule offre au nom demandé, en
  facturation horaire, en stock et sans frais de mise en service, sinon il refuse avant toute
  création. La CLI ``scw`` n'est pas utilisée pour cette raison : son ``server create type=…``
  choisit l'offre par son seul nom et peut tomber sur la mensuelle, qui engage un mois ;
* il refuse de créer un second serveur si un serveur de lab existe déjà ;
* ``down`` agit sur **tous** les serveurs portant le tag ``two-lab`` dans le projet, et
  ``session`` y ajoute l'identifiant reçu à la création : un serveur créé juste avant une
  interruption est rattrapé ;
* une suppression refusée pendant la livraison ou l'installation est réessayée tant qu'elle dure,
  dans la limite du délai d'installation augmenté du délai de suppression ;
* le serveur n'est déclaré supprimé qu'au 404 de l'API, jamais sur une erreur passagère ;
* un échec de suppression se termine par ``SERVEUR(S) DE LAB TOUJOURS FACTURÉ(S)`` et un code
  d'erreur.

Ce qu'il ne peut pas garantir : un ``SIGKILL``, une coupure de courant ou une mise en veille du
poste qui lance la session. En cas de doute, toujours :

.. code-block:: bash

   scripts/lab-host.sh status
   scripts/lab-host.sh down

Diagnostic
----------

``aucune clé SSH active dans le projet``
   Aucune clé SSH n'est enregistrée dans le projet de lab. En ajouter une dans la console (projet
   → Clés SSH).

``offre horaire EM-B212X-SSD : 0 correspondance(s)``
   L'offre n'existe pas dans la zone en facturation horaire. Vérifier ``SCW_DEFAULT_ZONE``.

``création incertaine``
   La création a échoué ou n'a pas rendu d'identifiant. Le serveur a pu être créé malgré tout —
   cas typique : le quota (identité non validée), ou une réponse perdue. Le script indique s'il
   voit un serveur de lab ; dans tous les cas, lancer ``status`` puis ``down``.

``SSH injoignable``
   L'installation est terminée mais SSH ne répond pas après 10 minutes. Avec une clé matérielle
   (Yubikey), chaque connexion demande un PIN ou un toucher : utiliser la clé dédiée du lab. Le
   serveur est toujours facturé : ``down``.

``SERVEUR(S) DE LAB TOUJOURS FACTURÉ(S)``
   La suppression n'a pas abouti dans les délais. Relancer ``down`` ; si l'erreur persiste,
   supprimer depuis la console Scaleway.

``HTTP 403 insufficient permissions``
   La clé d'API est authentifiée mais n'a pas le droit demandé — en général la lecture des clés
   SSH du projet. Compléter la politique de la clé, limitée au projet.

Sécurité
--------

* La clé secrète n'apparaît ni dans les arguments des processus (elle est passée à ``curl`` par
  un descripteur de fichier), ni dans les journaux, ni dans l'environnement de ``ssh``.
* **Ne jamais lancer le script sous** ``bash -x`` : la trace afficherait la clé.
* Le serveur n'expose que SSH, par clé. Le lab n'a aucune donnée personnelle ni secret de
  production.
* Une clé secrète qui a circulé ailleurs que dans ``scaleway.env`` (conversation, terminal
  partagé, capture d'écran) se régénère.
* **Clé SSH des VM** : générée par ``lab up`` sur le serveur, c'est la seule clé autorisée dans
  les VM. Elle ne quitte jamais le serveur, n'ouvre que les VM du lab — qui n'écoutent qu'en
  boucle locale — et disparaît avec lui. La clé publique du Mac n'est jamais envoyée aux VM.
* **Clés d'hôte des VM non vérifiées** par ``lab ssh`` (``known_hosts`` jetable) : elles changent
  à chaque ``up``. Acceptable uniquement parce que la connexion reste sur la boucle locale d'un
  serveur auquel on s'est authentifié.
* **Code exécuté sans épinglage par** ``deploy.sh`` : la bibliothèque ``shflags`` est récupérée
  par ``curl`` sur la branche ``main`` d'un autre dépôt (``H6N/tools``) et exécutée par ``eval``,
  sans vérification d'intégrité — dans le lab comme en production. Les artefacts de la release
  sont, eux, vérifiés contre ``SHA256SUMS``.
* **Image** : ``SHA512SUMS`` vient de la même origine que l'image, en HTTPS. La vérification
  protège contre la corruption, pas contre une origine compromise ; la signature GPG de Debian
  (``SHA512SUMS.sign``) n'est pas encore vérifiée.

Tests
-----

.. code-block:: bash

   bash scripts/lab-host_test.sh
   go test ./internal/lab/... ./cmd/lab/

Environ une minute et demie, sans réseau : la suite remplace ``curl`` par une fausse API Scaleway
qui se place dans le pire cas (offre mensuelle listée avant l'horaire, serveurs d'autres projets,
suppressions refusées, erreurs 503, serveur qui tarde à disparaître) et ``ssh`` par un faux client.
Elle tourne sous bash 5 comme sous le bash 3.2 de macOS.
