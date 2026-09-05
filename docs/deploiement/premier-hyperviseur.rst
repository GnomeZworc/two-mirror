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

.. note::

   **À rédiger.** À documenter :

   * la version de FRR de référence et son mode d'installation, sachant que l'hyperviseur est
     sans état : le paquet et la configuration doivent être posés à chaque démarrage, par le
     bootstrap ou par un mécanisme équivalent ;
   * les démons activés dans ``/etc/frr/daemons`` ;
   * la configuration de référence : numéro d'AS, session vers le route reflector, famille
     d'adresses utilisée pour annoncer les MAC et les VNI ;
   * l'articulation avec les interfaces créées par l'agent : comment FRR découvre une interface
     VXLAN qui apparaît à la création d'un subnet, et si une action est nécessaire ensuite ;
   * ce qui se passe au démarrage à froid, quand FRR démarre avant ou après l'agent ;
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
