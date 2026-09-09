Vue d'ensemble
==============

Cycle d'une requête
-------------------

.. code-block:: text

   HTTP → internal/api/agent → Dispatcher.Prepare() → Dispatcher.Dispatch() → worker.Queue → Command.Execute()

**Prepare** (synchrone, dans le handler HTTP)
   valide l'état, écrit l'état initial (``creating`` / ``deleting``) en base, et retourne 202 ou
   une erreur.

**Dispatch** (asynchrone)
   place la commande sur un canal bufferisé ; une goroutine worker appelle ``Execute``. C'est
   ``Dispatch``, et lui seul, qui marque la ressource en ``error`` si ``Execute`` échoue.

**Execute**
   effectue le travail réseau (netns, netif, VXLAN, veth, bridge, DHCP), puis met l'état à
   ``running`` / ``deleted``.

Paquets
-------

.. list-table::
   :header-rows: 1
   :widths: 32 68

   * - Chemin
     - Rôle
   * - ``internal/api/agent``
     - handlers HTTP de ``/vpcs``, ``/subnets`` et ``/vms``
   * - ``internal/dispatcher/agent``
     - interface ``Command`` (``Prepare``/``Execute``/``Key``) et commandes concrètes
   * - ``internal/state``
     - énumération des états, ``CanDelete``/``IsTransient``, seul point d'écriture des états
   * - ``internal/migration``
     - migrations idempotentes jouées au démarrage de l'agent
   * - ``internal/vpc``, ``internal/subnet``
     - création et suppression bas niveau (netns + netif)
   * - ``internal/netns``
     - network namespaces : create/enter/delete/call
   * - ``internal/netif``
     - netlink : bridge, veth, vxlan, tap, routes, adresses
   * - ``internal/ebtables``, ``internal/iptables``
     - wrappers dédiés ; ne pas appeler ces binaires ailleurs
   * - ``internal/qemu``, ``internal/qmp``
     - lancement de QEMU et client QMP sur socket Unix
   * - ``internal/vm``
     - cycle de vie d'une VM : tap, iptables, metadata, qemu
   * - ``internal/dhcp``
     - génération des configurations dnsmasq et entrées ip → mac
   * - ``internal/metadata``
     - serveur de metadata cloud-init et ses templates
   * - ``internal/watchdog``
     - vérification périodique en lecture seule
   * - ``internal/config/agent``
     - chargement par viper — tags ``mapstructure``, jamais ``yaml``
   * - ``internal/prometheus/agent``
     - collector des métriques ``syonad_*``
   * - ``pkg/db/kv``
     - wrapper Badger ; toutes les valeurs sont des chaînes plates
   * - ``pkg/worker``
     - pool de goroutines sur canal
   * - ``pkg/systemd``
     - client D-Bus systemd
   * - ``pkg/logger``, ``pkg/prometheus``
     - journalisation ``slog`` et serveur de métriques

Ajouter un type de ressource
----------------------------

#. ajouter les helpers KV dans ``pkg/db/kv`` si nécessaire ;
#. définir ``Create<X>`` / ``Delete<X>`` dans un nouveau paquet ``internal/<x>/`` ;
#. ajouter ``Create<X>Command`` / ``Delete<X>Command`` dans ``internal/dispatcher/agent/``, dont
   ``Key()`` qui retourne ``<x>/<name>`` et le contrôle ``state.CanDelete`` dans
   ``Delete<X>Command.Prepare`` ;
#. ajouter les handlers HTTP dans ``internal/api/agent/`` et les routes dans ``server.go``.

Stubs de plateforme
-------------------

Les fichiers ``_linux.go`` portent l'implémentation netlink/netns réelle ; les ``_other.go``
correspondants retournent une erreur « not supported on this platform ». Tous les paquets
**compilent** sur macOS, ce qui permet d'y tester la logique qui ne touche ni netlink ni netns.

Deux exceptions à connaître : les stubs de ``netns`` exécutent ``fn`` **sans changer de
namespace** — ``netns.Call`` réussit donc hors Linux — et ``netif`` compile partout parce que
netlink fournit une implémentation « unspecified ».
