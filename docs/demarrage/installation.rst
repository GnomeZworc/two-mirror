Installation d'un hyperviseur
=============================

Cette page installe l'agent sur **un** hyperviseur. Le réseau du cluster — routage entre nœuds,
plan de contrôle — est traité à part : voir :doc:`/deploiement/index`.

Prérequis
---------

Un host Linux avec KVM, sur lequel vous avez ``root``. Les opérations réseau (network
namespaces, netlink, VXLAN, ebtables, iptables) et QEMU ne fonctionnent que sous Linux.

Déploiement
-----------

.. code-block:: bash

   curl -O https://git.g3e.fr/syonad/two/raw/branch/main/scripts/deploy.sh
   bash ./deploy.sh -t 0.1.0 -i

``deploy.sh`` se met à jour lui-même depuis la branche avant toute action — s'il diffère, il se
réécrit et demande d'être relancé. Il télécharge ensuite binaires, units systemd et scripts
depuis la release, et les vérifie contre le manifeste ``SHA256SUMS``.

.. list-table::
   :header-rows: 1
   :widths: 26 54 20

   * - Option
     - Effet
     - Défaut
   * - ``-t <tag>``
     - déployer une release donnée
     - dernière
   * - ``-b <branche>``
     - branche utilisée pour l'auto-mise à jour du script
     - ``main``
   * - ``-p <profil>``
     - profil d'host ; seul ``kvm`` installe les units de l'agent
     - ``kvm``
   * - ``-i``
     - préparer l'host : paquets, noyau, réseau
     - désactivé
   * - ``-u <iface>``
     - interface physique d'uplink
     - ``eno1``
   * - ``-B <bridge>``
     - bridge principal, auquel l'uplink est rattaché
     - ``br-000000``
   * - ``-P <bridge>``
     - bridge supplémentaire, créé vide et réservé
     - ``br-public``
   * - ``-R <secondes>``
     - délai avant le redémarrage de secours pendant la migration réseau
     - ``120``
   * - ``-d``
     - dry-run : affiche les commandes sans les exécuter
     - désactivé

Les options booléennes actives par défaut se **désactivent** par leur forme longue négative :
``--nopackages``, ``--nonetwork``, ``--noverify``, ``--noup_script``.

.. warning::

   ``--noverify`` désactive la seule vérification d'intégrité des artefacts téléchargés. Ne
   l'utiliser que pour diagnostiquer un manifeste cassé, jamais en déploiement courant.

Ce que fait ``-i``
------------------

**Paquets** — ``qemu-system-x86``, ``ovmf``, ``dnsmasq``, ``ebtables``, ``iptables``,
``nfs-common``, ``jq``, ``curl``. Le service ``dnsmasq`` du système est ensuite désactivé et
**masqué** : il prendrait le port 53 en concurrence des instances ``dnsmasq@`` que l'agent lance
dans les netns.

**Noyau** — chargement de ``br_netfilter``, puis ``net.ipv4.ip_forward = 1`` et
``net.bridge.bridge-nf-call-iptables = 1``. Cette dernière clé est **requise** par la DNAT vers
le serveur de metadata : sans elle, iptables ne voit pas le trafic bridgé des VM et cloud-init
ne se provisionne pas. Contrepartie assumée : tout le trafic inter-VM traverse les tables NAT.

**Réseau** — création du bridge réservé, puis rattachement de l'uplink au bridge principal,
l'adresse et la route par défaut étant déplacées de l'interface physique vers le bridge.

.. danger::

   La migration réseau **coupe le réseau de l'host si elle échoue à mi-parcours**, sans console
   de secours. Deux garde-fous sont en place : un redémarrage de secours armé avant l'opération
   (``-R``, 120 s par défaut) qui ramène la configuration d'origine puisque rien n'est écrit sur
   disque, et l'exécution de la séquence sous systemd plutôt que dans la session SSH, pour
   qu'une coupure de SSH ne l'interrompe pas.

   Le désarmement n'a lieu **qu'après** un ping réussi vers la passerelle. Prévoir un accès
   physique ou console avant de lancer un ``-i`` à distance sur un host de production.

Host sans état
--------------

L'hyperviseur est **stateless** : sa racine est en tmpfs, rien de ce que pose ``-i`` ne survit à
un redémarrage. ``deploy.sh --bootstrap`` est donc rejoué à chaque démarrage — c'est le
mécanisme normal, pas une réparation.

Binaires installés
------------------

.. list-table::
   :header-rows: 1
   :widths: 20 60 20

   * - Binaire
     - Rôle
     - Drapeau de config
   * - ``agent``
     - processus principal : API, dispatcher, exécution, watchdog
     - ``-config``
   * - ``metadata``
     - serveur de metadata cloud-init, une instance par VM dans le netns du VPC
     - ``-conf``
   * - ``db``
     - inspection de la base clé-valeur en ligne de commande
     - ``-conf``

Les trois partagent le même fichier, ``/etc/two/agent.yml`` — voir
:doc:`/exploitation/configuration`.

Mise à jour
-----------

``deploy.sh`` relève les instances ``dnsmasq@`` et ``metadata@`` actives **avant** d'arrêter les
services, et les redémarre ensuite : c'est la seule façon de savoir lesquelles relancer. Arrêter
les services à la main avant de lancer le script fait perdre cette liste.

Vérifier l'installation
-----------------------

.. code-block:: bash

   systemctl status agent
   curl -s http://127.0.0.1:8080/vpcs

Une liste JSON — vide au premier démarrage — signifie que l'API répond. Passez à
:doc:`/demarrage/premier-vpc`.

.. important::

   L'API de l'agent **n'a aucune authentification**. Avant d'ouvrir le port au-delà de la boucle
   locale, lisez l'avertissement de :doc:`/exploitation/configuration` : quiconque atteint ce
   port pilote la totalité de l'hyperviseur.
