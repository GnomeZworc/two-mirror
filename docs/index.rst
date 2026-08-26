two
===

**two** est un orchestrateur de virtualisation et de réseau : il pilote un parc d'hyperviseurs,
le réseau qui les relie, et les machines virtuelles qui y tournent.

Il se compose de plusieurs éléments, déployés et versionnés séparément.

.. list-table::
   :header-rows: 1
   :widths: 22 58 20

   * - Composant
     - Rôle
     - État
   * - **agent**
     - un par hyperviseur : expose une API HTTP qui crée des VPC — isolés par network
       namespace —, des subnets — VXLAN ou bridge — et des VM QEMU/KVM raccordées à ces
       subnets, avec DHCP, routage et metadata cloud-init
     - livré (0.1.0)
   * - *à venir*
     - les composants de niveau supérieur — ordonnancement sur le parc, API d'orchestration,
       interface d'administration — sont à documenter au fur et à mesure de leur livraison
     - à venir

À ce stade, la totalité de cette documentation porte donc sur l'**agent** et sur le réseau du
cluster qui l'entoure.

Par où commencer
----------------

:doc:`/demarrage/index`
   Installer l'agent sur un hyperviseur et créer un premier VPC, un subnet et une VM. C'est le
   parcours court, sur un nœud isolé.

:doc:`/deploiement/index`
   L'architecture complète : réseau du cluster, routage, et ce qu'il faut mettre en place avant
   qu'un parc d'hyperviseurs fonctionne ensemble.

:doc:`/exploitation/index`
   Configuration, services, API de l'agent, métriques et diagnostic sur un nœud en service.

:doc:`/concepts/index`
   Comment les éléments fonctionnent entre eux : modèle de données, modes réseau, cycle de vie,
   metadata. À lire avant de diagnostiquer un comportement inattendu.

.. toctree::
   :hidden:
   :caption: Mise en œuvre

   demarrage/index
   deploiement/index

.. toctree::
   :hidden:
   :caption: Exploitation

   exploitation/configuration
   exploitation/services
   exploitation/api-agent/index
   exploitation/observabilite
   exploitation/diagnostic

.. toctree::
   :hidden:
   :caption: Interne

   concepts/index
   architecture/index
   versions/index
