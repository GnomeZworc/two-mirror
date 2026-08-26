FRR sur les hyperviseurs
========================

FRR tourne sur chaque hyperviseur et peuple la table de transfert (FDB) des interfaces VXLAN
créées par l'agent — c'est ce qui rend un subnet utilisable au-delà d'un seul nœud, puisque
l'agent désactive l'apprentissage et ne configure aucun voisin.

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
   * les commandes de vérification à utiliser en exploitation.

Vérifier le plan de données
---------------------------

Indépendamment de la configuration retenue, deux vérifications restent valables et méritent
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
   de contrôle. Cf. :doc:`architecture-cluster`.
