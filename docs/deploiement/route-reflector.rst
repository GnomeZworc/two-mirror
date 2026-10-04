VM route reflector
==================

Le route reflector est le point de rendez-vous du plan de contrôle : plutôt que de maintenir une
session entre chaque paire d'hyperviseurs, chaque hyperviseur ouvre une session vers le route
reflector, qui redistribue.

Il tourne lui-même en machine virtuelle, ce qui crée une dépendance circulaire à traiter
explicitement : la VM qui porte le plan de contrôle du cluster est hébergée par le cluster.

Adressage
---------

Le route reflector est **sur le même segment L2 que les hyperviseurs**, avec trois adresses :

.. list-table::
   :header-rows: 1
   :widths: 30 25 45

   * - Adresse
     - Interface
     - Rôle
   * - une adresse de ``192.168.14.0/24``
     - interface principale
     - celle de n'importe quelle machine du segment ; les réponses aux hyperviseurs en partent
   * - ``169.254.0.3/28``
     - interface principale, **adresse secondaire**
     - le lien avec les routeurs de cluster (``169.254.0.1`` et ``.2``), sur le même L2
   * - ``10.255.255.1/32``
     - ``lo1``, interface ``dummy``
     - ``router-id``, ``cluster-id`` et source des sessions EVPN ; seule route annoncée aux routeurs

Les hyperviseurs ne connaissent que la loopback : ils ouvrent leur session vers ``10.255.255.1``
par leur passerelle (``192.168.14.1``), les routeurs l'ayant apprise du route reflector en eBGP.
Aucune adresse d'hyperviseur n'est déclarée côté route reflector : il accepte toute session venant
du segment (``bgp listen range``).

L'adresse du lien est une **adresse secondaire** de l'interface principale, pas une interface à
part : même L2, même MAC sur le fil, une interface de moins qu'avec un ``ipvlan``. Elle doit
survivre aux renouvellements DHCP de l'adresse principale : la déclarer dans la configuration
réseau du système, pas la poser à la main.

.. note::

   **Non vérifié sur l'image de production.** Avec NetworkManager, la forme attendue est :

   .. code-block:: bash

      nmcli connection modify <connexion> +ipv4.addresses 169.254.0.3/28
      nmcli connection add type dummy ifname lo1 con-name lo1 \
          ipv4.method manual ipv4.addresses 10.255.255.1/32 ipv6.method disabled

   Le lab, en Debian, pose ces adresses par cloud-init et un script rejoué au démarrage ; ces
   commandes restent à valider sur l'image golden.

Configuration de FRR
--------------------

``bgpd`` et ``bfdd`` activés dans ``/etc/frr/daemons``. La configuration ci-dessous est **celle du
lab** (``conf/lab/frr/rr1.conf``) : ASN, loopback et plages sont ceux de la production, seul le nom
d'hôte diffère. Le lab qualifie donc exactement ce fichier.

.. literalinclude:: ../../conf/lab/frr/rr1.conf
   :language: text

Ce qu'elle établit :

* **Routeurs de cluster** (groupe ``CLUSTER``) : eBGP vers l'AS 65100, avec BFD, en IPv4
  unicast. Le route reflector n'annonce que sa loopback (``RR-LOOPBACK-OUT``) et **n'accepte
  rien** (``NO-IN``) : il ne reçoit aucune route des routeurs.
* **Hyperviseurs** (groupe ``fabric``) : voisins dynamiques, toute session venant de
  ``192.168.14.0/24`` étant acceptée, jusqu'à 200. ``local-as 64600 no-prepend replace-as`` fait
  que, du point de vue des hyperviseurs, la session est en iBGP dans l'AS 64600 — celui de leur
  configuration — alors que le route reflector est en AS 65000 face aux routeurs.
* **EVPN** : seule famille activée vers les hyperviseurs, qui sont ses clients
  (``route-reflector-client``) : il réfléchit les routes EVPN de chacun vers tous les autres.

Vérifié dans le lab
~~~~~~~~~~~~~~~~~~~

Le 2026-10-04, FRR 10.7.1, route reflector en Debian 12 sur le segment des hyperviseurs, avec
l'adresse du lien en secondaire et la loopback sur ``lo1`` :

.. list-table::
   :header-rows: 1
   :widths: 55 45

   * - Vérification
     - Résultat
   * - session avec le routeur, IPv4 unicast
     - Established ; le routeur reçoit **un seul** préfixe, la loopback, et l'installe via
       ``169.254.0.3``
   * - BFD avec le routeur
     - up des deux côtés
   * - sessions des deux hyperviseurs vers ``10.255.255.1``
     - Established, voisins dynamiques, iBGP AS 64600, famille L2VPN EVPN négociée
   * - routes EVPN échangées, VM ↔ VM entre deux hyperviseurs
     - routes de type 3 échangées, ping et MTU 1500 de bout en bout — à partir de la release
       ``0.2.0rc003``, qui pose l'adresse VTEP locale des VXLAN
       (`#51 <https://git.g3e.fr/syonad/two/issues/51>`_)

.. warning::

   **Les tunnels ne survivent pas à la perte du route reflector.** Mesuré dans le lab
   (2026-10-04, FRR 10.7.1, un seul route reflector, sans ``graceful-restart``) : à l'arrêt de
   FRR sur le route reflector, la session EVPN des hyperviseurs tombe aussitôt, FRR retire les
   routes apprises et, avec elles, le VTEP distant et l'entrée d'inondation du VXLAN — **plus
   aucun paquet ne passe** entre hyperviseurs, à 30 s comme à 90 s. Au redémarrage de FRR sur le
   route reflector, VTEP distant et trafic reviennent **31 s** plus tard. Le trafic entre VM d'un
   même hyperviseur n'est pas concerné. La redondance (deux route reflectors) ou
   ``graceful-restart`` sont les deux leviers ; ni l'un ni l'autre n'est encore qualifié.

Le routeur du lab n'est qu'une configuration minimale écrite pour l'essai, pas celle des routeurs
de cluster (:doc:`routeurs`).

.. note::

   **À rédiger.** Reste à documenter :

   * la création de la VM : ressources, subnet et mode utilisés (``bridge``, pour pouvoir la
     lancer sur n'importe quel hyperviseur), et s'il s'agit d'une VM créée par l'agent comme les
     autres ou d'un cas particulier — l'image, elle, est l'image golden de :doc:`image-qcow2` ;
   * la redondance : une seule VM route reflector, ou deux, et sur quels hyperviseurs ;
   * la procédure de reconstruction — l'état du cluster pendant l'absence du route reflector est
     mesuré ci-dessus : plus de trafic entre hyperviseurs ;
   * la procédure d'amorçage : ce qui fonctionne, et dans quel ordre, quand on démarre un cluster
     entier depuis zéro — le premier hyperviseur n'a pas de session FRR établie tant que cette VM
     n'existe pas, cf. :doc:`premier-hyperviseur`.

Points de vigilance
-------------------

.. warning::

   L'agent **ne réattache pas** les VM existantes à son démarrage, et les processus QEMU ne
   survivent pas à un redémarrage de l'hyperviseur. Le redémarrage de l'hyperviseur qui héberge
   le route reflector est donc un événement à part entière : la procédure de remise en service
   doit être écrite, et testée.

.. warning::

   ``instance-id`` valant le nom de la VM, recréer la VM route reflector sous le même nom sur le
   même disque fait que cloud-init **ne rejoue pas** le user-data. Cf.
   :doc:`/concepts/metadata-cloud-init`.
