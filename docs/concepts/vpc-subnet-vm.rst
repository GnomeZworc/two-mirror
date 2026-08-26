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

Subnet
------

Un subnet appartient à un VPC et pose, dans son netns, un bridge qui porte ``interface_ip`` — la
gateway vue par les VM. Il fournit aussi le DHCP (dnsmasq) et les routes annoncées aux guests.

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
