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
     VM --- TAP[tap] --- BR["br-&lt;subnet&gt;<br/>adresse d'émission"]
     GW["gw-&lt;subnet&gt;<br/>gateway, MAC anycast"] -. macvlan .- BR
     BR --- VETH["v-&lt;subnet&gt;-i / -e"] --- VX["vxlan-&lt;vni&gt;"] --- UP[uplink]

Le subnet vit dans le netns du VPC. La VM n'est donc **pas joignable depuis l'host** sans route
explicite — point à connaître avant de câbler un outil externe dessus.

Adressage
~~~~~~~~~

L'agent calcule deux adresses à partir du ``cidr`` ; ``interface_ip`` n'est pas fourni par
l'appelant (refusé en 400) :

* la **gateway**, avant-dernière adresse du préfixe (``.254`` en ``/24``), exposée comme
  ``interface_ip`` ;
* l'**adresse d'émission**, juste avant (``.253``), exposée comme ``relay_ip``.

Le ``cidr`` doit être sans bits d'hôte et faire au plus ``/29``. Une VM ne peut prendre ni l'une
ni l'autre de ces adresses, ni l'adresse de réseau, de broadcast ou la ``gateway`` explicite du
subnet (422).

Gateway anycast
~~~~~~~~~~~~~~~

La gateway est portée par un macvlan ``gw-<subnet>`` posé sur le bridge du netns. Sa MAC est
**identique sur tous les hôtes qui portent le subnet** : ``02:01:00`` suivi de la VNI sur 24 bits
(VNI ``2911`` → ``02:01:00:00:0b:5f``). Où qu'elle tourne, une VM résout sa gateway vers l'hôte
qui l'héberge, et une entrée ARP restée en cache reste valable si la VM change d'hôte.

Le bridge ``br-<subnet>`` garde une **MAC propre à l'hôte**, ``02:03`` suivi des quatre octets du
VTEP — l'IPv4 primaire du ``local_iface`` (``192.168.14.12`` → ``02:03:c0:a8:0e:0c``) — et porte
l'adresse d'émission :

* le netns émet ses requêtes ARP depuis l'adresse d'émission (``arp_announce=2``) et le bridge ne
  répond pas pour la gateway (``arp_ignore=1``). Quand un hôte relaie en L2 vers une VM d'un autre
  hôte — trafic routé vers un subnet présent sur plusieurs hôtes, filet du ``/24`` d'un VPC
  routé —, la VM répond à la MAC de cet hôte, sans que sa gateway change ;
* la route du subnet a la gateway pour source : le netns parle à **ses** VM depuis la gateway, et
  leurs réponses reviennent à la MAC anycast, donc à l'hôte local ;
* le DHCP écoute sur ``gw-<subnet>`` et s'annonce avec la gateway comme identifiant de serveur :
  un renouvellement unicast reste servi par l'hôte de la VM.

L'adresse d'émission est la même sur tous les hôtes : une VM qui reçoit la requête ARP d'un autre
hôte associe ``.253`` à la MAC de celui-ci. C'est sans effet — la VM ne parle jamais d'elle-même à
``.253`` et le netns lui parle depuis la gateway. Mesuré en lab (``s11-fenetre-arp``) : pendant
cette fenêtre, métadonnées, renouvellement DHCP et connexions depuis le netns passent sans perte.

.. note::

   Depuis le netns d'un hôte, une VM d'un **autre** hôte ne répond qu'à l'adresse d'émission
   (``ping -I <relay_ip>``) : à la gateway, sa réponse repartirait vers son propre hôte. Le
   trafic routé entre VM n'est pas concerné, il garde sa source.

Confinement de la MAC anycast
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

La MAC anycast ne doit **jamais** sortir du netns : vue depuis le fabric, elle serait annoncée en
EVPN par chaque hôte, détectée en double, et déplacerait l'apprentissage des bridges. Trois
mécanismes la confinent :

* ebtables, sur le veth du subnet : toute trame de **source** anycast est rejetée à l'entrée
  (``nat PREROUTING``) et à la sortie (``FORWARD`` et ``OUTPUT``) ;
* une **entrée FDB locale** pour la MAC anycast dans le bridge du netns. Le macvlan ne la crée pas
  lui-même : sans elle, toute trame d'une VM vers sa gateway est inondée vers le fabric et les
  autres VM, et traitée par la gateway de chaque hôte ;
* les requêtes ARP vers la gateway ou l'adresse d'émission, et les réponses DHCP qui en viennent,
  sont bloquées vers le veth, comme avant.

IPv6 n'est pas désactivé sur le macvlan : ses trames (DAD, ND) sont arrêtées par le confinement.
Le watchdog contrôle le macvlan, sa MAC, les deux adresses et l'entrée FDB ; il ne relit pas les
règles ebtables.

Subnets créés avant la gateway anycast
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Un subnet ``vxlan`` sans ``relay_ip`` garde le modèle précédent jusqu'à sa recréation :
``interface_ip`` choisie à la création, portée par le bridge, avec la MAC propre à l'hôte. Une VM
qui reçoit alors la requête ARP d'un autre hôte associe **sa gateway** à cet hôte pendant
quelques secondes (#57).

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
   next-hop = ``interface_ip`` du subnet, la gateway calculée en ``vxlan``.

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
n'y est traité en L3 que si son next-hop est ``interface_ip``, locale au netns — sur le macvlan de
la gateway en ``vxlan``, sur le bridge dans les autres modes.
Avec un autre next-hop, la trame est commutée en **L2** sans traverser ``PREROUTING`` : le
serveur de metadata devient injoignable et tout le provisionnement cloud-init échoue,
silencieusement.

.. danger::

   Ne jamais retirer cette route de l'option 121, quelle que soit l'apparence de redondance.
