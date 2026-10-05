Premier hyperviseur
===================

Le premier nœud du cluster se déploie comme les suivants, mais il est le seul à devoir
fonctionner **avant** que le plan de contrôle existe : c'est lui qui hébergera la première VM
route reflector.

Installation
------------

L'installation de l'agent est identique à celle d'un nœud isolé et n'est pas reprise ici :
voir :doc:`/demarrage/installation` pour ``deploy.sh``, ses options, la préparation de l'host et
la migration réseau vers le bridge d'uplink.

Deux points à relire avant de lancer un ``-i`` sur un nœud de production : la migration réseau
coupe le réseau de l'host si elle échoue à mi-parcours, et l'hyperviseur est **sans état** —
tout ce qui est posé doit l'être par un mécanisme rejoué à chaque démarrage.

Plan de contrôle — FRR
----------------------

FRR tourne sur chaque hyperviseur et peuple la table de transfert (FDB) des interfaces VXLAN
créées par l'agent. C'est ce qui rend un subnet utilisable au-delà d'un seul nœud, puisque
l'agent désactive l'apprentissage et ne configure aucun voisin — cf.
:doc:`architecture-cluster`.

Configuration de FRR
~~~~~~~~~~~~~~~~~~~~

Seul ``bgpd`` est activé dans ``/etc/frr/daemons``. D'un hyperviseur à l'autre ne changent que
``hostname`` et ``router-id``, l'adresse de l'hyperviseur sur son segment.

.. literalinclude:: ../../test/e2e/topologies/frr/hv1.conf
   :language: text

Ce qu'elle établit :

* **Une seule session**, en iBGP dans l'AS 64600, vers la loopback du route reflector
  (``10.255.255.1``), jointe par la passerelle par défaut — le routeur de cluster l'a apprise du
  route reflector (:doc:`routeurs`). Aucune adresse d'un autre hyperviseur n'est écrite : ajouter
  un nœud ne touche pas la configuration des autres.
* **EVPN seulement** : l'IPv4 unicast n'est pas activé (``no bgp default ipv4-unicast``).
  ``advertise-all-vni`` annonce les VNI de toutes les interfaces VXLAN que FRR voit, et donc
  celles que l'agent crée.
* **Pas de BFD** sur cette session, contrairement à la session entre route reflector et routeur.

Articulation avec l'agent
~~~~~~~~~~~~~~~~~~~~~~~~~

Rien à configurer à la création d'un subnet : la VXLAN apparaît, FRR la voit et l'annonce, sans
action ni redémarrage. Il faut en revanche que la VXLAN porte une **adresse VTEP locale** : sans
elle, FRR voit la VNI mais n'annonce rien, et deux hyperviseurs gérés par two ne se joignent pas.
L'agent la pose à partir de la release ``0.2.0rc003``
(`#51 <https://git.g3e.fr/syonad/two/issues/51>`_) : l'adresse IPv4 primaire du ``local_iface``
du subnet.

.. note::

   **À rédiger.** Reste à documenter :

   * la version de FRR de référence et son installation sur l'hyperviseur **sans état** : paquet
     et configuration doivent être posés à chaque démarrage, par le bootstrap ou un mécanisme
     équivalent (`#52 <https://git.g3e.fr/syonad/two/issues/52>`_) ;
   * le démarrage à froid, selon que FRR démarre avant ou après l'agent ;
   * les subnets ``vxlan`` créés avant ``0.2.0rc003``, dont la VXLAN n'a pas d'adresse VTEP
     locale : les recréer ou poser l'adresse à chaud, à trancher dans
     `#51 <https://git.g3e.fr/syonad/two/issues/51>`_ ;
   * le cas particulier du **premier** hyperviseur, dont la session ne peut pas s'établir tant
     que le route reflector n'existe pas.

Vérifier le plan de données
---------------------------

Ces deux vérifications restent valables quelle que soit la configuration retenue, et méritent
d'être dans toute procédure de diagnostic :

.. code-block:: bash

   # La FDB du VXLAN doit contenir des entrées vers les autres hyperviseurs.
   # Vide, c'est le plan de contrôle qui ne fonctionne pas, pas l'agent.
   ip netns exec <vpc> bridge fdb show dev <interface-vxlan>

   # L'interface VXLAN telle que l'agent l'a créée : port 4789, learning off,
   # aucun groupe multicast, aucun remote.
   ip netns exec <vpc> ip -d link show <interface-vxlan>

.. important::

   Une FDB vide alors que le subnet est en ``running`` n'est **pas** un défaut de l'agent : il
   crée délibérément l'interface sans apprentissage ni voisin, et laisse le peuplement au plan
   de contrôle.

Valider le nœud
---------------

Avant de passer à la suite, le nœud doit savoir créer une VM de bout en bout à partir de l'image
golden — c'est exactement le parcours de :doc:`/demarrage/premier-vpc`, avec
``storage[0].path`` pointant sur une copie de l'image produite par :doc:`image-qcow2`.

Une VM qui démarre, obtient son adresse en DHCP et applique son user-data valide d'un coup
l'agent, le DHCP, la route vers le serveur de metadata et l'image. C'est le prérequis de
:doc:`route-reflector`.
