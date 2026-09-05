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

.. note::

   **À rédiger.** Cette page attend les éléments de terrain. Pour chacun des trois niveaux :

   * le matériel ou le logiciel employé, et la version de référence ;
   * la configuration de référence : interfaces, adressage, protocole de routage et numéros
     d'AS ;
   * la redondance : combien d'équipements, quel mécanisme de bascule, quel comportement attendu
     pendant une bascule ;
   * ce qui est annoncé et ce qui est filtré à chaque niveau ;
   * l'ordre de mise en service, et ce qui doit être opérationnel avant de préparer le premier
     hyperviseur.

Contraintes imposées par le reste du cluster
---------------------------------------------

Indépendamment des choix d'équipement, deux contraintes viennent de ce que fait l'agent.

.. warning::

   **MTU** — l'agent crée bridges, veth et interfaces VXLAN avec un MTU figé à 1500, et VXLAN
   ajoute 50 octets d'encapsulation. Les liens entre hyperviseurs doivent donc accepter au moins
   **1550 octets**. Cf. :doc:`architecture-cluster`.

.. warning::

   **UDP 4789** doit passer entre hyperviseurs, dans les deux sens : c'est le port des tunnels
   VXLAN.

.. warning::

   **L'API de l'agent n'a aucune authentification.** Le filtrage réalisé ici est aujourd'hui
   l'une des rares barrières entre cette API et le reste du réseau : son port ne doit être
   joignable que depuis le réseau d'administration. Cf. :doc:`/exploitation/configuration`.
