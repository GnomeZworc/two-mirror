Routeurs de cluster
===================

Les routeurs de cluster raccordent les hyperviseurs entre eux et au monde extérieur. Ils
préexistent à l'agent : celui-ci ne les configure pas et n'en a aucune connaissance.

.. note::

   **À rédiger.** Cette page attend les éléments de terrain. À documenter :

   * le rôle exact des routeurs dans la topologie : passerelle du réseau d'hyperviseurs,
     terminaison du routage externe, ou les deux ;
   * le matériel ou le logiciel employé, et la version de référence ;
   * la configuration de référence : interfaces, adressage, protocole de routage employé avec
     les hyperviseurs, numéros d'AS si BGP ;
   * la redondance : combien de routeurs, quel mécanisme de bascule, quel comportement attendu
     pendant une bascule ;
   * le MTU configuré sur les liens vers les hyperviseurs — il doit tenir compte des 50 octets
     d'encapsulation VXLAN, cf. :doc:`architecture-cluster` ;
   * le filtrage : ce qui est autorisé entre hyperviseurs (au minimum UDP 4789), et ce qui est
     autorisé depuis l'extérieur.

Points de vigilance
-------------------

.. warning::

   L'API de l'agent n'a **aucune authentification**. Le filtrage réalisé par les routeurs de
   cluster est aujourd'hui l'une des rares barrières entre cette API et le reste du réseau : le
   port de l'API ne doit être joignable que depuis le réseau d'administration.
   Cf. :doc:`/exploitation/configuration`.
