VPC, subnet et VM
=================

Trois types de ressources, une hiérarchie stricte.

.. mermaid::

   graph TD
     VPC["VPC<br/><i>network namespace</i><br/>cidr"] --> SN1["Subnet<br/><i>bridge + VXLAN</i><br/>cidr, gateway"]
     VPC --> SN2["Subnet"]
     SN1 --> VM1["VM<br/><i>QEMU/KVM</i>"]
     SN1 --> VM2["VM"]
     SN2 --> VM2

VPC
---

Un VPC est un **network namespace** portant un espace d'adressage (``cidr``). C'est l'unité
d'isolation : deux VPC ne se voient pas, et peuvent réutiliser les mêmes plages d'adresses.

À la création, l'agent relie le netns au **bridge public** de l'host par un veth ``vp-<id>-e``
(côté host) ↔ ``vp-<id>-i`` (côté netns, rattaché à un ``br-public`` propre au netns). Le bridge
public est celui de ``interfaces.internet``, ``br-public`` si la clé est absente, jamais
``default_interface`` (:doc:`/exploitation/configuration`). ``deploy.sh -i`` le crée et y
accroche ``vxlan-public``, le VXLAN du segment public partagé avec les bordures
(:doc:`/demarrage/installation`) ; sans ce bridge, toute création de VPC échoue.

Dans le netns, l'agent pose aussi la ``dummy`` ``hostsvc`` (``169.254.169.253/32``) et la chaîne
iptables ``TWO-HOSTSVC``, puis demande à ``two-hostsvc`` de servir le VPC : c'est le chemin du
DNS et du NTP des VM (voir :doc:`dns-ntp`). Si ``two-hostsvc`` ne répond pas, la création du VPC
échoue.

Un VPC ne peut être supprimé que si tous ses subnets le sont déjà — sinon 409.

Routage entre hyperviseurs : ``l3vni``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Un VPC créé avec ``l3vni`` route ses subnets entre hyperviseurs par un **VNI de transit** propre
au VPC (IRB symétrique, `#41 <https://git.g3e.fr/syonad/two/issues/41>`_) : un hyperviseur n'a
plus besoin de porter un subnet pour joindre ses VM. Sans ``l3vni``, le VPC se comporte comme
avant. Le champ exige la section ``bgp`` de l'agent (:doc:`/exploitation/configuration`) et ne
se change pas après création.

**Plan de VNI.** Segment public, L3VNI et ``vxlan_id`` des subnets partagent l'espace des VNI
VXLAN : le VNI **1** est celui du segment public (``deploy.sh --public_vni``, 1 par défaut), la
plage **2–65535** (sauf 253–255, tables du noyau) est réservée aux L3VNI des VPC,
**65536–16777215** aux subnets. L'agent n'impose que la borne haute des L3VNI (le RD
``<VTEP>:<l3vni>`` porte un numéro sur 16 bits) ; le respect du plan revient à l'appelant, qui
alloue les L3VNI et les ``vxlan_id``. Un VPC créé avec le VNI du segment public pour ``l3vni``
passe en ``error`` : ce VNI est déjà porté par ``vxlan-public``.

À la création, l'agent pose sur l'hyperviseur, pour un VPC ``vp-<id>`` :

.. code-block:: text

   vrf-<id>         VRF, table de routage = l3vni, unreachable default metric 4278198272
   ├── brl3-<id>    bridge sans adresse, MAC de routeur 02:02:<IPv4 du VTEP>
   │   └── vxl3-<id>    VXLAN du l3vni, VTEP = IPv4 primaire de default_interface
   └── l3-<id>-o    169.254.0.1/30 ── l3-<id>-i 169.254.0.2/30, dans le netns du VPC

Dans le netns, le ``cidr`` du VPC est routé vers ``169.254.0.1`` ; le transit ``/30`` est le même
sur tous les hyperviseurs et pour tous les VPC. L'agent active le forwarding IPv4 dans le netns,
et sur l'hôte sur ``brl3-<id>`` et ``l3-<id>-o``, les deux interfaces d'entrée de la VRF ; il ne
touche pas au réglage global, que ``bootstrap_kvm.sh`` met déjà à 1. FRR reçoit la VRF, son ``vni``, une instance
``router bgp <asn> vrf vrf-<id>`` avec RD ``<VTEP>:<l3vni>`` et RT ``<asn>:<l3vni>``, et
l'export des routes du ``cidr`` présentes dans la VRF.

La table de la VRF reçoit, juste après sa création, une route ``unreachable default`` de métrique
maximale (``4278198272``, valeur recommandée par la documentation VRF du noyau). Sans elle, une
recherche qui échoue dans la VRF continue dans les règles suivantes, donc dans la table ``main``
de l'hôte : un paquet d'un locataire vers une adresse inconnue de la VRF sortirait par l'underlay,
avec ``169.254.0.2`` pour source (mesuré, `#81 <https://git.g3e.fr/syonad/two/issues/81>`_). La
métrique maximale laisse passer avant elle toute route réelle de la VRF. Cette route n'est pas
annoncée en EVPN : le route-map de la VRF n'exporte que le ``cidr`` du VPC. Elle n'est liée à
aucune interface et ne disparaît pas avec la VRF : la suppression la retire explicitement. IPv4
seulement.

La suppression retire d'abord la configuration de routage de FRR, puis les interfaces, la route
``unreachable default``, puis la VRF de FRR. Cet ordre est imposé : supprimer la VRF du noyau sous une instance BGP encore
configurée fait planter ``bgpd`` (FRR 10.7.1).

Subnet
------

Un subnet appartient à un VPC et pose, dans son netns, un bridge et la gateway vue par les VM,
``interface_ip`` — calculée par l'agent et portée par un macvlan à MAC anycast en mode ``vxlan``,
fournie par l'appelant et portée par le bridge dans les autres modes. Il fournit aussi le DHCP — dnsmasq ou le serveur intégré selon
``dhcp.backend`` — et les routes annoncées aux guests.

``iface_type`` est une clé **logique** (``vms``, ``internet``, ``admin``…), traduite en nom de
bridge physique par la configuration de l'agent. Une clé absente ou inconnue retombe sur
``default_interface``. Ce niveau d'indirection permet au même appel d'API de fonctionner sur des
hosts dont le nommage réseau diffère.

Le comportement réseau dépend du :doc:`mode </concepts/modes-reseau>`.

Un subnet ne peut être supprimé que si aucune VM non supprimée n'a d'interface dessus — sinon
409, avec la liste des VM. Dans un VPC routé par ``l3vni``, un subnet ``vxlan`` annonce son
préfixe dans la VRF du VPC à la création et le retire en premier à la suppression : c'est le
filet qui couvre une VM tant que son ``/32`` n'est pas annoncé.

VM
--

Une VM est un processus QEMU/KVM raccordé à un ou plusieurs subnets par des taps.

**Interfaces.** L'ordre du tableau ``interfaces`` détermine le slot PCI (``0x03 + index``), donc
le nom de l'interface dans le guest. Exactement une interface doit être ``primary`` : elle porte
la route par défaut et le serveur de metadata. Tous les subnets d'une VM doivent appartenir au
**même VPC**.

**MAC.** Chaque interface porte la MAC choisie par l'appelant, comme son IP. L'agent la donne
telle quelle à QEMU et à la réservation DHCP, sans en vérifier le format ni l'unicité : c'est à
l'appelant de garantir qu'elle est unique sur le segment L2.

**Filtrage.** Chaque interface ne peut émettre que depuis sa propre MAC et sa propre IP. L'agent
pose sur son tap, avant de le créer, des règles qui rejettent toute trame émise par la VM :

* dont la MAC source n'est pas celle de l'interface ;
* d'un autre type qu'IPv4, ARP ou IPv6 (une trame étiquetée VLAN par exemple) ;
* ARP, dont la MAC ou l'adresse d'émetteur n'est pas celle de l'interface, sauf l'adresse
  ``0.0.0.0`` d'une sonde de conflit d'adresse ;
* IPv4, dont la source n'est pas l'adresse de l'interface, sauf ``0.0.0.0`` vers le port 67 (un
  client DHCP qui n'a pas encore de bail) ;
* réponse de serveur DHCP (UDP depuis le port 67).

Une VM ne peut donc ni se faire passer pour la gateway ou une autre VM, ni distribuer de baux. Une
VM qui route pour d'autres machines ou porte des adresses supplémentaires ne peut pas émettre
depuis ces adresses. L'IPv6 n'est pas filtrée (#61). Les règles sont retirées après la suppression
du tap ; le watchdog signale une interface d'une VM ``running`` dont les règles manquent ou ont
été modifiées.

**Serveur DHCP.** ``dhcp_server: true`` sur une interface lève le seul blocage des réponses DHCP,
pour cette interface. Cas d'usage : une VM d'un subnet ``bridge`` qui sert le DHCP et le PXE de
machines diskless du segment physique, avec des réservations statiques. Les autres contrôles
restent en place. L'agent applique ce champ quel que soit le mode du subnet : le réserver au mode
``bridge`` revient à l'appelant. Le serveur de la VM reçoit aussi les requêtes des VM du subnet, à
côté du DHCP de l'agent : il ne doit répondre qu'aux machines qui lui sont destinées.

**Routes.** Dans un VPC routé par ``l3vni``, chaque interface sur un subnet ``vxlan`` est annoncée
en ``/32`` dans la VRF du VPC une fois QEMU démarré, juste avant le passage en ``running``. À la
suppression, ces ``/32`` sont retirés **avant** l'arrêt de QEMU. Comme un subnet ne peut être
supprimé tant qu'une VM l'utilise, le ``/32`` d'une VM est toujours retiré avant le ``/24`` de son
subnet — l'ordre qui évite le trou noir de `#41 <https://git.g3e.fr/syonad/two/issues/41>`_.

**Stockage.** Un seul disque virtio par VM, ``vda``, le disque racine ; les disques
supplémentaires passent par le contrôleur SCSI (``xvdX``, cible X). Cette contrainte vient de la
carte PCI figée — voir :doc:`/architecture/contraintes`. ``dev`` fixe l'emplacement du disque
(cible SCSI, slot PCI) et son numéro de série, pas le nom que lui donne le noyau du guest
(``/dev/sdY``, dans l'ordre de détection) : le disque se retrouve par ``/dev/xvdX`` dans une
image conforme, et toujours par ``/dev/disk/by-id/…<dev>``.

Ce qui est stocké, et où
------------------------

Une ressource ne porte en base que ce qui lui est propre. Une VM stocke le **lien** vers son
subnet (``vm/<name>/nic/<i>/subnet``), pas le VPC ni le bridge ni la gateway : ces valeurs sont lues
depuis le subnet, leur source canonique. Le schéma complet des clés est dans
:doc:`/architecture/stockage`.

Nommage
-------

L'API est machine-to-machine : elle **ne valide aucun nommage**, pas même le motif ``vp-…``
indiqué par la spécification pour les VPC. Une contrainte est pourtant réelle : le nom d'un VPC
ou d'un subnet doit **contenir un « - »**, car l'identifiant système (``vp-<id>-e``, ``br-<id>``,
``gw-<id>``…) est ce qui suit le premier tiret. Un nom sans tiret fait paniquer l'agent. Les
exemples de cette documentation suivent la convention ``vp-`` / ``sn-`` / ``i-``, mais c'est à
l'appelant de la faire respecter.
