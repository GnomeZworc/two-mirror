Modes réseau
============

Le champ ``mode`` d'un subnet détermine la façon dont il est raccordé à l'host, et les routes
annoncées aux VM.

.. list-table::
   :header-rows: 1
   :widths: 15 45 40

   * - Mode
     - Raccordement
     - État
   * - ``vxlan``
     - tunnel VXLAN (``vxlan_id``) + bridge dans le netns du VPC
     - défaut
   * - ``bridge``
     - rattachement direct à un bridge existant de l'host, résolu depuis ``iface_type``
     - disponible
   * - ``public_ip``
     - routé comme ``vxlan`` côté DHCP
     - **mise en place host non implémentée** — la création échoue à l'exécution
   * - ``vlan``
     - —
     - réservé, non implémenté

.. warning::

   ``public_ip`` est accepté par l'API et traité comme ``vxlan`` pour le DHCP, mais sa
   configuration réseau côté host n'existe pas encore : la création part en ``error`` dans
   ``Execute``. Ne pas s'appuyer dessus en production.

vxlan
-----

.. mermaid::

   graph LR
     VM --- TAP[tap] --- BR["br-&lt;subnet&gt;<br/>interface_ip"]
     BR --- VX["vxlan&lt;vni&gt;"] --- HBR["bridge host<br/>(iface_type)"] --- UP[uplink]

Le subnet vit dans le netns du VPC. La VM n'est donc **pas joignable depuis l'host** sans route
explicite — point à connaître avant de câbler un outil externe dessus.

bridge
------

Le subnet est rattaché directement à un bridge existant de l'host. Pas de tunnel, pas de route
VPC : le trafic sort par le bridge, et la VM est joignable depuis l'host.

Routes annoncées aux VM
-----------------------

Les routes sont poussées par DHCP, dans l'**option 121** (routes statiques sans classe,
RFC 3442). Trois entrées y figurent :

#. la route ``/32`` vers ``169.254.169.254``, le serveur de metadata ;
#. la route vers le CIDR du VPC ;
#. la route par défaut ``0.0.0.0/0``.

.. important::

   **Un client qui lit l'option 121 ignore l'option 3.** Toute route par défaut doit donc figurer
   dans l'option 121 ; l'option 3 ne sert que les clients qui n'implémentent pas la 121.

Route par défaut : ``default_route`` et ``gateway``
----------------------------------------------------------

Une route par défaut est **toujours** annoncée. Le champ ``default_route`` ne choisit que son
next-hop :

``default_route: false`` (défaut)
   next-hop = ``interface_ip`` du subnet.

``default_route: true``
   next-hop = le champ ``gateway`` s'il est fourni, sinon la gateway lue dans la table de routage
   de l'host.

``gateway`` n'est **pas validé** par l'agent : sa joignabilité et sa cohérence avec le CIDR du
subnet relèvent de l'appelant. Fourni avec ``default_route: false``, il est ignoré.

Dans tous les modes sauf ``bridge``, la route vers le CIDR du VPC garde ``interface_ip`` comme
next-hop : sur un subnet à IP publique, le trafic interne ne doit pas sortir par la gateway
publique.

Pourquoi la route ``/32`` vers le serveur de metadata est indispensable
----------------------------------------------------------------------------

Elle paraît redondante avec la route par défaut. Elle ne l'est pas.

La DNAT vers le serveur de metadata est posée dans le netns du VPC, en ``PREROUTING``. Le paquet
n'y est traité en L3 que si son next-hop est ``interface_ip``, portée par le bridge du netns.
Avec un autre next-hop, la trame est commutée en **L2** sans traverser ``PREROUTING`` : le
serveur de metadata devient injoignable et tout le provisionnement cloud-init échoue,
silencieusement.

.. danger::

   Ne jamais retirer cette route de l'option 121, quelle que soit l'apparence de redondance.
