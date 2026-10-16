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

Un VPC ne peut être supprimé que si tous ses subnets le sont déjà — sinon 409.

Routage entre hyperviseurs : ``l3vni``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Un VPC créé avec ``l3vni`` route ses subnets entre hyperviseurs par un **VNI de transit** propre
au VPC (IRB symétrique, `#41 <https://git.g3e.fr/syonad/two/issues/41>`_) : un hyperviseur n'a
plus besoin de porter un subnet pour joindre ses VM. Sans ``l3vni``, le VPC se comporte comme
avant. Le champ exige la section ``bgp`` de l'agent (:doc:`/exploitation/configuration`) et ne
se change pas après création.

**Plan de VNI.** L3VNI et ``vxlan_id`` des subnets partagent l'espace des VNI VXLAN : la plage
**1–65535** (sauf 253–255, tables du noyau) est réservée aux L3VNI des VPC, **65536–16777215** aux
subnets. L'agent n'impose que la première borne (le RD ``<VTEP>:<l3vni>`` porte un numéro sur
16 bits) ; le respect du plan revient à l'appelant, qui alloue les deux.

À la création, l'agent pose sur l'hyperviseur, pour un VPC ``vp-<id>`` :

.. code-block:: text

   vrf-<id>         VRF, table de routage = l3vni
   ├── brl3-<id>    bridge sans adresse, MAC de routeur 02:02:<IPv4 du VTEP>
   │   └── vxl3-<id>    VXLAN du l3vni, VTEP = IPv4 primaire de default_interface
   └── l3-<id>-o    169.254.0.1/30 ── l3-<id>-i 169.254.0.2/30, dans le netns du VPC

Dans le netns, le ``cidr`` du VPC est routé vers ``169.254.0.1`` ; le transit ``/30`` est le même
sur tous les hyperviseurs et pour tous les VPC. L'agent active le forwarding IPv4 dans le netns,
et sur l'hôte sur ``brl3-<id>`` et ``l3-<id>-o``, les deux interfaces d'entrée de la VRF ; il ne
touche pas au réglage global, que ``bootstrap_kvm.sh`` met déjà à 1. FRR reçoit la VRF, son ``vni``, une instance
``router bgp <asn> vrf vrf-<id>`` avec RD ``<VTEP>:<l3vni>`` et RT ``<asn>:<l3vni>``, et
l'export des routes du ``cidr`` présentes dans la VRF.

La suppression retire d'abord la configuration de routage de FRR, puis les interfaces, puis la
VRF de FRR. Cet ordre est imposé : supprimer la VRF du noyau sous une instance BGP encore
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

**Routes.** Dans un VPC routé par ``l3vni``, chaque interface sur un subnet ``vxlan`` est annoncée
en ``/32`` dans la VRF du VPC une fois QEMU démarré, juste avant le passage en ``running``. À la
suppression, ces ``/32`` sont retirés **avant** l'arrêt de QEMU. Comme un subnet ne peut être
supprimé tant qu'une VM l'utilise, le ``/32`` d'une VM est toujours retiré avant le ``/24`` de son
subnet — l'ordre qui évite le trou noir de `#41 <https://git.g3e.fr/syonad/two/issues/41>`_.

**Stockage.** Un seul disque ``vdX`` (virtio-blk) par VM ; les disques supplémentaires passent
par le contrôleur SCSI (``sdX``). Cette contrainte vient de la carte PCI figée — voir
:doc:`/architecture/contraintes`.

Ce qui est stocké, et où
------------------------

Une ressource ne porte en base que ce qui lui est propre. Une VM stocke le **lien** vers son
subnet (``vm/<name>/subnet``), pas le VPC ni le bridge ni la gateway : ces valeurs sont lues
depuis le subnet, leur source canonique. Le schéma complet des clés est dans
:doc:`/architecture/stockage`.

Nommage
-------

L'API est machine-to-machine : elle **ne valide pas** les conventions de nommage, à l'exception
du motif documenté pour les VPC (``vp-…``). Les exemples de cette documentation suivent la
convention ``vp-`` / ``sn-`` / ``i-``, mais c'est à l'appelant de la faire respecter.
