Déploiement d'un cluster
========================

Le :doc:`/demarrage/index` couvre un hyperviseur isolé : un VPC, un subnet, une VM, tout sur le
même nœud. Cette section couvre ce qu'il faut mettre en place pour qu'un **parc** d'hyperviseurs
fonctionne ensemble — c'est-à-dire pour qu'un subnet s'étende à plusieurs nœuds.

Ce n'est pas une extension du quickstart : le réseau du cluster doit exister **avant** que
l'agent serve à quelque chose au-delà d'un nœud.

Ordre de mise en place
----------------------

#. :doc:`architecture-cluster` — la topologie cible et ce que l'agent suppose déjà en place
#. :doc:`routeurs-cluster` — le routage entre hyperviseurs et vers l'extérieur
#. :doc:`route-reflector` — la VM route reflector, point de rendez-vous du plan de contrôle
#. :doc:`frr-hyperviseur` — FRR sur chaque hyperviseur, qui peuple le plan de données

.. toctree::
   :hidden:
   :maxdepth: 1

   architecture-cluster
   routeurs-cluster
   route-reflector
   frr-hyperviseur
