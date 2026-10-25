Architecture du cluster
=======================

Vue d'ensemble
--------------

.. figure:: /schemas/architecture-ensemble.svg
   :alt: Vue d'ensemble : Internet, bordures, deux clusters, route reflectors, services partagés et hyperviseurs
   :align: center
   :width: 100%
   :class: only-light

   Architecture cible. Trait bleu : plan de données (VXLAN). Tirets violets : plan de contrôle
   (BGP). Tirets verts : orchestration. Trait orange : trafic Internet. Cadre en pointillés :
   élément à venir, avec son ticket.

.. figure:: /schemas/architecture-ensemble-dark.svg
   :alt: Vue d'ensemble : Internet, bordures, deux clusters, route reflectors, services partagés et hyperviseurs
   :align: center
   :width: 100%
   :class: only-dark

   Architecture cible. Trait bleu : plan de données (VXLAN). Tirets violets : plan de contrôle
   (BGP). Tirets verts : orchestration. Trait orange : trafic Internet. Cadre en pointillés :
   élément à venir, avec son ticket.

Plusieurs clusters, chacun avec son underlay et ses deux routeurs, sont reliés en eBGP entre eux
et aux **routeurs de bordure** ; les route reflectors sont décrits plus bas. Ce qui n'existe pas
encore est décrit sur son ticket :

* **Internet** (`#31 <https://git.g3e.fr/syonad/two/issues/31>`_,
  `#42 <https://git.g3e.fr/syonad/two/issues/42>`_). Les bordures partagent avec les hyperviseurs
  un VXLAN public (VNI 1, ``vxlan-public`` sur ``br-public``, #87) et portent toutes la même IP et
  la même MAC de gateway. Elles restent sans état : tout NAT vit sur un hyperviseur. Trois façons
  de sortir :

  * une VM qui porte une IP publique a une NIC sur un subnet public de son VPC, relié au subnet
    public de l'hôte ;
  * une **IP flottante** est traduite 1:1, sans état, par l'hyperviseur de la VM ;
  * les autres VM sortent par la **passerelle NAT de leur VPC**, que la VRF atteint par une route
    par défaut de type 5, si le VPC autorise Internet.

* **Orchestration** (`#72 <https://git.g3e.fr/syonad/two/issues/72>`_). Le broker et
  l'orchestrateur sont des VM du VNI 66000, partagé entre clusters. Les agents s'y connectent
  **en sortie**, par leur passerelle ; les routeurs de chaque cluster les traduisent (NAT) vers le
  VNI 66000.
  L'orchestrateur a aussi une patte dans le segment public (VNI 1) : son API est joignable
  depuis Internet.

* **Route reflectors.** Au moins un ; deux sont prévus dans l'architecture et dans les exemples
  (plages d'adresses de :doc:`route-reflector`), placés ici dans deux clusters différents pour
  qu'un incident sur un cluster ne coupe pas les deux sessions EVPN. Chaque lien entre routeurs
  et route reflector est en ``169.254.0.0/28``, propre à son cluster. Le ``bgp listen range`` de
  chaque route reflector doit alors couvrir l'underlay de **tous** les clusters qu'il sert —
  l'exemple ``rr1.conf`` n'accepte que ``192.168.14.0/24``.

* **ASN.** Les routeurs de deux clusters étant voisins eBGP, chaque cluster a son propre ASN de
  routeurs : l'AS 65100 des exemples est la valeur d'**un** cluster.

Le lab (:doc:`/developpement/lab`) simplifie : un seul cluster, qui porte les deux route
reflectors.

Topologie
---------

.. figure:: /schemas/architecture-cluster.svg
   :alt: Détail d'un cluster : routeurs, route reflectors, hyperviseurs, netns et VRF des VPC, plans de données et de contrôle
   :align: center
   :width: 100%
   :class: only-light

   Détail d'un cluster et de deux hyperviseurs. Trait bleu : plan de données (VXLAN). Tirets
   violets : plan de contrôle (sessions BGP). Pointillé gris : créé par l'agent.

.. figure:: /schemas/architecture-cluster-dark.svg
   :alt: Détail d'un cluster : routeurs, route reflectors, hyperviseurs, netns et VRF des VPC, plans de données et de contrôle
   :align: center
   :width: 100%
   :class: only-dark

   Détail d'un cluster et de deux hyperviseurs. Trait bleu : plan de données (VXLAN). Tirets
   violets : plan de contrôle (sessions BGP). Pointillé gris : créé par l'agent.

Deux plans distincts, à ne pas confondre au moment du diagnostic :

Plan de données
   les tunnels VXLAN entre hyperviseurs, encapsulés sur le réseau qui les relie.

Plan de contrôle
   ce qui dit à chaque hyperviseur où se trouvent les adresses MAC des autres. C'est le rôle de
   FRR et des route reflectors.

Ce que l'agent suppose déjà en place
------------------------------------

L'agent ne configure **que** son propre hyperviseur, et seulement à partir du bridge d'uplink.
Tout ce qui est en amont — adressage des hyperviseurs, routage entre eux, plan de contrôle — lui
préexiste et n'est jamais créé ni vérifié par lui.

Concrètement, il attend :

* le bridge d'uplink de la configuration (``br-000000`` par défaut), avec l'interface physique
  esclave et l'adresse de l'hyperviseur portée par le bridge — c'est ce que fait
  ``deploy.sh --bootstrap``, voir :doc:`/demarrage/installation` ;
* une connectivité IP entre hyperviseurs sur cette adresse, port UDP **4789** ouvert dans les
  deux sens ;
* un plan de contrôle qui peuple la table de transfert VXLAN — voir ci-dessous.

Pourquoi un plan de contrôle est nécessaire
-------------------------------------------

L'agent crée les interfaces VXLAN sur le port 4789 **sans groupe multicast et avec
l'apprentissage désactivé** (``Learning: false``). Il n'y a donc ni inondation multicast, ni
apprentissage des adresses MAC depuis le trafic, ni voisin statique configuré.

.. important::

   Conséquence directe : sur un même VNI, **rien ne traverse d'un hyperviseur à l'autre** tant
   qu'un composant externe n'a pas peuplé la table de transfert (FDB) du VXLAN. Sur un nœud
   isolé le trafic reste sur le bridge local et cette absence ne se voit pas ; elle apparaît dès
   le deuxième nœud.

C'est exactement le rôle que remplissent FRR sur chaque hyperviseur et les route reflectors qui
les font converger.

MTU
---

Le MTU est une valeur **unique pour tout le cluster** : toutes les VM, et tous les bridges, veth,
taps et interfaces VXLAN que l'agent crée, sont au MTU des VM, ``network.mtu_vm`` (1500 par
défaut, cf. :doc:`/exploitation/configuration`). Il doit être identique sur tous les
hyperviseurs : un subnet est un seul domaine L2 qui s'étend sur plusieurs hôtes, et un élément à
un MTU plus petit que les autres jette les trames en silence — un bridge ne fragmente pas et
n'émet aucun ICMP. Le symptôme est trompeur : le ping passe, les petites requêtes passent, les
transferts volumineux et les poignées de main TLS échouent.

L'underlay, le réseau qui relie les hyperviseurs et porte le VXLAN, est attendu à **exactement
9000**. VXLAN ajoute 50 octets d'encapsulation ; 9000 laisse donc toute la marge à des VM à 1500.
L'agent refuse de créer un subnet ``vxlan`` (ou un VPC avec ``l3vni``) dont l'interface n'est
pas à 9000, sauf si ``network.allow_mtu_underlay_below_9000`` l'y autorise ; même alors,
l'underlay doit faire au moins ``network.mtu_vm`` + 50. Un underlay à 1500 impose donc des VM à
1450 au plus, et l'option ``network.allow_mtu_vm_below_1500``.

Le MTU est annoncé aux VM par deux voies : l'option DHCP 26 et ``host_mtu`` sur leur interface
virtio. Une image dont le client DHCP ignore l'option 26 s'aligne quand même.

.. note::

   **VM en mode bridge sur un bridge à 9000.** Un subnet ``bridge`` rattaché à ``br-000000``
   met ses VM, à 1500, sur un segment L2 à 9000 avec l'hôte et tout ce qui s'y trouve. Le
   routage vers les autres subnets reste correct — un routeur renvoie un « fragmentation
   needed » plutôt que de perdre en silence —, mais l'interface du routeur qui sert de gateway
   à ce segment doit être à 1500 : sinon elle peut envoyer à la VM des trames plus grandes
   qu'elle n'accepte. En TCP, le MSS annoncé par la VM protège l'échange ; en UDP ou en ICMP,
   les gros paquets venant d'un équipement à 9000 du même segment sont perdus.

Hyperviseurs sans état
----------------------

L'hyperviseur est **stateless** — sa racine est en tmpfs, rien de ce que pose
``deploy.sh --bootstrap`` ne survit à un redémarrage, et le script est rejoué à chaque démarrage.

Toute configuration ajoutée à un hyperviseur — FRR compris — doit donc être posée par un
mécanisme rejouable au démarrage, jamais par une modification manuelle d'un fichier sous
``/etc``.

Adressage
---------

.. note::

   **À rédiger** — cette page ne décrit pas encore le plan d'adressage du cluster. À documenter :

   * la plage utilisée pour les adresses d'hyperviseurs, et son rapport avec ``br-000000`` ;
   * l'allocation des VNI VXLAN : qui la tient, et comment on évite les collisions, puisque
     l'agent ne valide pas ``vxlan_id`` ;
   * le plan d'adressage des VPC, et ce qui garantit qu'ils ne se recouvrent pas entre clients.
