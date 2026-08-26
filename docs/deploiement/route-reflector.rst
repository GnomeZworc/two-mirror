VM route reflector
==================

Le route reflector est le point de rendez-vous du plan de contrôle : plutôt que de maintenir une
session entre chaque paire d'hyperviseurs, chaque hyperviseur ouvre une session vers le route
reflector, qui redistribue.

Il tourne lui-même en machine virtuelle, ce qui crée une dépendance circulaire à traiter
explicitement : la VM qui porte le plan de contrôle du cluster est hébergée par le cluster.

.. note::

   **À rédiger.** À documenter :

   * la création de la VM : image de base, ressources, subnet et mode utilisés, s'il s'agit d'une
     VM créée par l'agent comme les autres ou d'un cas particulier ;
   * son adressage, et comment les hyperviseurs le connaissent ;
   * la configuration du démon de routage qu'elle héberge ;
   * la redondance : une seule VM route reflector, ou deux, et sur quels hyperviseurs ;
   * la procédure de reconstruction, et l'état du cluster pendant que le route reflector est
     absent — les tunnels déjà établis continuent-ils de fonctionner, et pendant combien de
     temps ;
   * la procédure d'amorçage : ce qui fonctionne, et dans quel ordre, quand on démarre un cluster
     entier depuis zéro.

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
