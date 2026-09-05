Invariants et pièges
====================

Contraintes découvertes en production ou en corrigeant des bugs. Les enfreindre casse quelque
chose qui fonctionne, souvent en silence.

.. note::

   Cette page reprend la section « Invariants et pièges » de ``CLAUDE.md``, qui reste la
   référence de développement et fait foi en cas d'écart.

Réseau
------

* **Ne pas retirer la route ``/32`` vers ``169.254.169.254``** de l'option 121, même quand elle
  paraît redondante avec la route par défaut. La DNAT vers le serveur de metadata est posée dans
  le netns du VPC en ``PREROUTING`` : le paquet n'y est traité en L3 que si son next-hop est
  ``interface_ip``. Avec un autre next-hop, la trame est commutée en L2 sans traverser
  ``PREROUTING``, et le provisionnement cloud-init échoue.
* **RFC 3442** : un client qui lit l'option 121 **ignore l'option 3**. Toute route par défaut
  doit donc figurer dans la 121 ; l'option 3 ne sert que les clients qui n'implémentent pas la
  121.
* **La route vers le CIDR du VPC garde ``interface_ip`` comme next-hop** dans tous les modes sauf
  ``bridge`` : sur un subnet à IP publique, le trafic interne ne doit pas sortir par la gateway
  publique.
* ``169.254.169.254`` est centralisé dans ``metadata.ServiceIP`` — ne pas le réécrire en dur.

QEMU et VM
----------

* **Un seul disque ``vdX`` par VM** ; les disques additionnels passent par le SCSI (``sdX``). La
  carte PCI en dépend : NIC en ``0x03``, contrôleur SCSI en ``0x1e``, virtio-blk en ``0x1f``.
* ``bus=pci.0`` est explicite sur les trois ``-device`` : un passage de la machine en **q35**
  casserait le démarrage (``Bus 'pci.0' not found``).
* QEMU est lancé par ``systemd-run --scope``, **jamais** en unit transitoire : le scope est
  exécuté par le processus appelant et hérite du netns posé par ``netns.Call``. Une unit
  transitoire, forkée par PID 1, démarrerait dans le netns racine et ne verrait pas le tap.
* L'arrêt d'une VM ne touche **jamais** aux fichiers disque. En revanche, un ``quit`` brutal est
  envoyé à l'expiration de ``dispatcher.timeout_seconds``.

Arrêt de l'agent
----------------

* Ordre imposé : serveurs HTTP → drainage des workers → fermeture de la base. L'inverser crée une
  course.
* Budget d'arrêt dépassé ⇒ **la base n'est pas fermée** : fermer Badger sous un écrivain
  concurrent est pire qu'un rejeu du journal au démarrage suivant.
* Jamais de ``log.Fatal`` dans une goroutine : ``os.Exit`` n'exécute aucun ``defer``.

cloud-init
----------

* ``network-config.tmpl`` cible ``eth0`` alors que les guests sont en ``ens3`` : il ne s'applique
  donc à rien, et le réseau vient du DHCP. **Ne pas le « corriger » ni le supprimer** — le rendre
  opérant ferait remplacer par cloud-init la configuration réseau de l'image sur toutes les VM.
* Un document fourni par l'appelant est servi **verbatim** ; un document absent retombe sur le
  template ; un document explicitement vide est servi vide. Les trois cas sont distincts.
* ``metadata.password`` est un **hash**, pas un mot de passe en clair.
* ``instance-id`` vaut le nom de la VM : recréer une VM du même nom sur le même disque fait que
  cloud-init la reconnaît et **n'applique pas** le user-data.

Configuration
-------------

* Le chargement se fait par **viper** : tags ``mapstructure``, jamais ``yaml``.
* Un chemin configurable se propage par les signatures de fonction, jamais par une variable ou un
  setter de paquet.

Sécurité connue et acceptée
---------------------------

Ces points sont documentés parce qu'ils sont **assumés en l'état**, pas parce qu'ils sont sans
conséquence. Ils doivent être réévalués avant toute exposition élargie de l'API.

* **L'API n'a aucune authentification** et l'exemple de configuration l'expose sur
  ``0.0.0.0:8080``. Quiconque atteint ce port pilote le host KVM.
* ``vm/<name>/password`` est stocké tel quel (c'est un hash) et restitué par ``/db?prefix=vm/``
  du serveur d'administration — contenu par ``admin.enabled: false`` et l'écoute en boucle
  locale.
* ``/run/two/metadata/<vm>/vendor-data`` est en ``0644`` et contient ce hash : tout compte local
  du host peut le lire.
* ``pkg/systemd.New()`` n'a **pas de timeout** : si le socket D-Bus accepte sans répondre,
  l'appelant se fige. Concerne le watchdog, la création et la suppression de subnets, et le
  serveur de metadata.
* Il n'y a **pas de rollback** : un échec partiel de création laisse des objets réseau orphelins
  jusqu'à un ``DELETE`` explicite.
