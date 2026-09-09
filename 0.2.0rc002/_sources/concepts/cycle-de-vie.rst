Cycle de vie des ressources
===========================

VPC, subnets et VM partagent le même jeu d'états.

.. mermaid::

   stateDiagram-v2
     [*] --> creating
     creating --> running
     creating --> error
     running --> deleting
     running --> error
     deleting --> deleted
     deleting --> error
     error --> deleting
     deleted --> [*]

.. list-table::
   :header-rows: 1
   :widths: 15 85

   * - État
     - Signification
   * - ``creating``
     - la demande est acceptée et enregistrée ; ``Execute`` n'a pas encore abouti
   * - ``running``
     - la ressource existe sur le système
   * - ``error``
     - ``Execute`` a échoué ; état **terminal**, il n'y a pas de reprise automatique
   * - ``deleting``
     - suppression en cours
   * - ``deleted``
     - suppression terminée

Suppression
-----------

Elle n'est autorisée que depuis ``running`` ou ``error`` — sinon **409**. Depuis ``error``, elle
est **best-effort** : les ressources système peuvent n'avoir été créées que partiellement.

Un VPC ne peut être supprimé qu'une fois tous ses subnets supprimés.

Pas de rollback
---------------

En cas d'échec partiel pendant une création, les ressources réseau déjà créées **ne sont pas
nettoyées**. C'est un choix délibéré : le nettoyage est déclenché explicitement par une
suppression, qui est justement autorisée depuis ``error``.

Conséquence pour l'appelant : après un passage en ``error``, émettre un ``DELETE`` avant toute
tentative de recréation, faute de quoi la recréation butera sur des objets système résiduels.

États transitoires au redémarrage de l'agent
--------------------------------------------

La file d'attente des workers est **en mémoire**. Une ressource restée en ``creating`` ou
``deleting`` au moment d'un arrêt de l'agent est donc nécessairement orpheline : plus personne
ne la traite.

Au démarrage, une migration idempotente bascule ces ressources en ``error``, et traduit
l'ancien vocabulaire d'états. Une ressource retrouvée en ``error`` après un redémarrage n'a donc
pas forcément échoué techniquement — elle peut simplement avoir été interrompue.
