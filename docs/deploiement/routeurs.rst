Routeurs
========

Trois niveaux de routage entourent le cluster, du plus proche des hyperviseurs au plus proche de
l'extérieur. Tous préexistent à l'agent : celui-ci ne les configure pas et n'en a aucune
connaissance.

Routeurs de cluster
   raccordent les hyperviseurs entre eux. C'est le niveau dont dépend directement le plan de
   données VXLAN.

Routeurs de datacentre
   agrègent les clusters d'un même site.

Routeurs de bordure
   terminent le routage vers l'extérieur.

Routeur de cluster
------------------

Le routeur de cluster est la passerelle des hyperviseurs et le voisin eBGP des route reflectors
— au moins un ; deux sont prévus dans l'architecture et dans les exemples —, dont il apprend les
loopbacks : c'est par lui que chaque hyperviseur joint ``10.255.255.1`` et ``10.255.255.2`` pour
ouvrir ses sessions EVPN (:doc:`route-reflector`).

Adressage
~~~~~~~~~

.. list-table::
   :header-rows: 1
   :widths: 30 25 45

   * - Adresse
     - Interface
     - Rôle
   * - ``192.168.14.1/24``
     - interface du segment des hyperviseurs, **adresse virtuelle VRRP**
     - passerelle par défaut des hyperviseurs et des route reflectors, portée par le routeur
       maître ; chaque routeur a en plus sa propre adresse sur le segment
   * - ``169.254.0.1/28``
     - même interface, **adresse secondaire**
     - lien avec les route reflectors (``169.254.0.3`` et ``.4``) ; ``router-id`` ; le second
       routeur prend ``169.254.0.2``

Le MTU minimal du segment, imposé par VXLAN, est donné plus bas.

Configuration de FRR
~~~~~~~~~~~~~~~~~~~~

``bgpd`` et ``bfdd`` activés dans ``/etc/frr/daemons``.

.. literalinclude:: ../../test/e2e/topologies/frr/sw1.conf
   :language: text

Ce qu'elle établit :

* **Route reflectors** : une session eBGP vers chacun, de l'AS 65100 vers l'AS 65000, avec BFD,
  en IPv4 unicast.
* **En entrée**, seules les loopbacks des route reflectors sont acceptées (``RR-IN``) ; **en sortie**,
  rien n'est annoncé (``NO-OUT``). Symétrique de ``RR-LOOPBACK-OUT`` et ``NO-IN`` côté route
  reflector : chaque côté filtre, aucun ne dépend du filtre de l'autre.
* **Aucune session avec les hyperviseurs** : le routeur leur sert de passerelle, pas de voisin BGP.

Celle du second routeur (``test/e2e/topologies/frr/sw2.conf``) ne diffère que par ``hostname`` et
``router-id`` (``169.254.0.2``), que les route reflectors déclarent aussi. Les deux routeurs se
partagent la passerelle ``192.168.14.1`` en **VRRP v3** : le maître la porte, l'autre attend, sans
préemption (`#54 <https://git.g3e.fr/syonad/two/issues/54>`_, fermé).

.. note::

   Ce qui précède est le contrat du routeur de cluster : adresses, ASN, session, filtres. Sa
   traduction dans la configuration d'un constructeur n'a pas sa place ici.

.. note::

   **À rédiger.** Reste à documenter, faute d'éléments de terrain :

   * pour le routeur de cluster : le matériel employé et sa version de référence ; la traduction
     de la redondance VRRP v3 dans sa configuration ;
   * pour les routeurs de datacentre et de bordure : tout — équipement, configuration de
     référence, ce qui est annoncé et filtré à chaque niveau, redondance ;
   * l'ordre de mise en service, et ce qui doit être opérationnel avant de préparer le premier
     hyperviseur.

Contraintes imposées par le reste du cluster
---------------------------------------------

Indépendamment des choix d'équipement, deux contraintes viennent de ce que fait l'agent.

.. warning::

   **MTU** — l'underlay est attendu à **9000** : l'agent refuse un subnet ``vxlan`` sur une
   interface à un autre MTU, sauf option, et les VM sont à 1500 plus 50 octets d'encapsulation.
   L'interface d'un routeur qui sert de gateway à un segment de VM doit être au MTU des VM
   (1500). Cf. :doc:`architecture-cluster`.

.. warning::

   **UDP 4789** doit passer entre hyperviseurs, dans les deux sens : c'est le port des tunnels
   VXLAN.

.. warning::

   **L'API de l'agent n'a aucune authentification.** Le filtrage réalisé ici est aujourd'hui
   l'une des rares barrières entre cette API et le reste du réseau : son port ne doit être
   joignable que depuis le réseau d'administration. Cf. :doc:`/exploitation/configuration`.
