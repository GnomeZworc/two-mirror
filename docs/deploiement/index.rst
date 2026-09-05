Déploiement d'un cluster
========================

Le :doc:`/demarrage/index` couvre un hyperviseur isolé : un VPC, un subnet, une VM, tout sur le
même nœud. Cette section couvre la mise en place d'un **cluster** complet, dans l'ordre où les
étapes se font.

Cet ordre n'est pas indifférent : chaque étape a besoin de la précédente. L'image doit exister
avant qu'on puisse démarrer quoi que ce soit ; le réseau doit être en place avant le premier
hyperviseur ; le route reflector est lui-même une VM, il lui faut donc un hyperviseur qui
fonctionne.

Étapes
------

#. :doc:`architecture-cluster` — la topologie cible, à lire avant tout le reste
#. :doc:`image-qcow2` — l'image golden dont dérivent toutes les VM du cluster
#. :doc:`routeurs` — le matériel : routeurs de cluster, de datacentre et de bordure
#. :doc:`premier-hyperviseur` — le premier nœud, agent et plan de contrôle
#. :doc:`route-reflector` — les VM route reflector

D'autres étapes viendront à mesure que les composants d'orchestration seront livrés.

.. toctree::
   :hidden:
   :maxdepth: 1

   architecture-cluster
   image-qcow2
   routeurs
   premier-hyperviseur
   route-reflector
