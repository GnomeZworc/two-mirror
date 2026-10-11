VPC, subnet et VM
=================

Trois types de ressources, une hiérarchie stricte.

.. mermaid::

   graph TD
     VPC["VPC<br/><i>network namespace</i><br/>cidr"] --> SN1["Subnet<br/><i>bridge + VXLAN</i><br/>interface_ip, cidr"]
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

À la création, l'agent pose sur l'hyperviseur, pour un VPC ``vp-<id>`` :

.. code-block:: text

   vrf-<id>         VRF, table de routage = l3vni
   ├── brl3-<id>    bridge sans adresse, MAC de routeur 02:02:<IPv4 du VTEP>
   │   └── vxl3-<id>    VXLAN du l3vni, VTEP = IPv4 primaire de default_interface
   └── l3-<id>-o    169.254.0.1/30 ── l3-<id>-i 169.254.0.2/30, dans le netns du VPC

Dans le netns, le ``cidr`` du VPC est routé vers ``169.254.0.1`` ; le transit ``/30`` est le même
sur tous les hyperviseurs et pour tous les VPC. Le forwarding IPv4 est activé dans le netns, et
sur l'hôte **seulement** sur ``brl3-<id>`` et ``l3-<id>-o`` : l'hyperviseur ne devient pas un
routeur entre ses autres interfaces. FRR reçoit la VRF, son ``vni``, une instance
``router bgp <asn> vrf vrf-<id>`` avec RD ``<VTEP>:<l3vni>`` et RT ``<asn>:<l3vni>``, et
l'export des routes du ``cidr`` présentes dans la VRF.

La suppression retire d'abord la configuration de routage de FRR, puis les interfaces, puis la
VRF de FRR. Cet ordre est imposé : supprimer la VRF du noyau sous une instance BGP encore
configurée fait planter ``bgpd`` (FRR 10.7.1).

Subnet
------

Un subnet appartient à un VPC et pose, dans son netns, un bridge qui porte ``interface_ip`` — la
gateway vue par les VM. Il fournit aussi le DHCP — dnsmasq ou le serveur intégré selon
``dhcp.backend`` — et les routes annoncées aux guests.

``iface_type`` est une clé **logique** (``vms``, ``internet``, ``admin``…), traduite en nom de
bridge physique par la configuration de l'agent. Une clé absente ou inconnue retombe sur
``default_interface``. Ce niveau d'indirection permet au même appel d'API de fonctionner sur des
hosts dont le nommage réseau diffère.

Le comportement réseau dépend du :doc:`mode </concepts/modes-reseau>`.

VM
--

Une VM est un processus QEMU/KVM raccordé à un ou plusieurs subnets par des taps.

**Interfaces.** L'ordre du tableau ``interfaces`` détermine le slot PCI (``0x03 + index``), donc
le nom de l'interface dans le guest. Exactement une interface doit être ``primary`` : elle porte
la route par défaut et le serveur de metadata. Tous les subnets d'une VM doivent appartenir au
**même VPC**.

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
