Metadata et cloud-init
======================

Chaque VM dispose d'un serveur de metadata NoCloud, servi sur ``169.254.169.254`` dans le netns
de son VPC, sous la forme d'une instance systemd ``metadata@<vm>``.

Chaîne de production
--------------------

.. mermaid::

   graph LR
     A["agent<br/>WriteNoCloudFiles"] -->|"/run/two/metadata/&lt;vm&gt;/"| M["binaire metadata"]
     M -->|HTTP 169.254.169.254| G["guest<br/>cloud-init"]

L'agent écrit les fichiers cloud-init sur disque **avant** de démarrer le service ; le binaire
``metadata`` les lit et les sert. Le processus ``metadata`` n'ouvre **jamais** la base Badger :
deux processus ne doivent pas partager une même instance.

Documents servis
----------------

Quatre documents sont servis : ``meta-data``, ``user-data``, ``network-config`` et
``vendor-data``. Seul ``user-data`` vient de l'appelant :

* ``user_data`` fourni → servi **verbatim**, après décodage base64 ; l'agent n'interprète rien ;
* ``user_data`` absent ou **vide** → ``user-data`` servi vide : les deux cas sont équivalents.

``meta-data``, ``network-config`` et ``vendor-data`` sont **toujours** rendus depuis les templates
de l'agent ; ``vendor-data`` crée le compte ``syonad`` à partir de ``password`` et ``sshkey``.

Champs de ``metadata``
----------------------

``sshkey``
   Clé publique ajoutée au compte ``syonad``. Transmise telle quelle, non encodée.

``password``
   Un **hash**, tel qu'attendu par la clé ``passwd`` de cloud-config (``$6$…``) — jamais un mot
   de passe en clair. Omis, le compte est créé verrouillé ; sans ``password`` ni ``sshkey``,
   aucun compte n'est créé.

``user_data``
   Le user-data cloud-init, **encodé en base64**. L'encodage évite l'échappement JSON des
   documents multi-lignes et autorise les charges ``gzip+base64``. Un base64 invalide est rejeté
   en 400 plutôt que servi vide.

``instance-id``
------------------

``instance-id`` vaut le **nom de la VM**. Recréer une VM du même nom sur le même disque fait que
cloud-init la reconnaît comme déjà provisionnée et **n'applique pas** le user-data. Pour rejouer
un provisionnement : changer de nom, repartir d'un disque neuf, ou exécuter
``cloud-init clean --logs`` dans le guest avant l'extinction.

Configuration réseau
--------------------

Le ``network-config`` servi à une VM contient une entrée par interface, dans l'ordre de l'API,
visée par sa MAC et configurée en DHCP IPv4 :

.. code-block:: yaml

   version: 2
   ethernets:
     eth0:
       match:
         macaddress: "52:54:00:19:01:0a"
       dhcp4: true
     eth1:
       match:
         macaddress: "52:54:00:19:02:14"
       dhcp4: true

La correspondance se fait par la MAC, pas par le nom : l'entrée ``eth<index>`` s'applique à
``ens3`` comme à ``eth0``. La clé n'est qu'un identifiant pour netplan et NetworkManager ;
deux renderers de cloud-init s'en servent pourtant comme nom : ``sysconfig`` renomme
l'interface vers la clé, et ``networkd`` l'ajoute au ``[Match]``, qui ne correspond alors
qu'aux images démarrées avec ``net.ifnames=0``.

Le ``network-config`` ne pose **aucune métrique**. Toutes les interfaces reçoivent du serveur
DHCP les routes du VPC et des métadonnées (option 121) ; seule la primaire reçoit la route par
défaut. Départager les routes en double relève de l'utilisateur de la VM, par son ``user_data``.

.. warning::

   Le ``network-config`` ne suffit pas à configurer les interfaces secondaires. La source
   NoCloud en ``seedfrom`` HTTP n'est trouvée qu'à l'étape réseau de cloud-init :

   * cloud-init 22.4 (Debian 12) ne demande jamais ``network-config`` par ``seedfrom`` et
     applique sa configuration de repli, limitée à la première interface ;
   * cloud-init 25.1 (Debian 13) le lit et l'applique au premier démarrage, mais à chaque
     démarrage suivant l'étape locale réapplique le repli, et ``network-config`` n'est pas
     relu : les interfaces secondaires perdent leur configuration.

   L'image doit donc faire du DHCP IPv4 sur **toutes** ses interfaces Ethernet, sans compter
   sur cloud-init. Voir :doc:`/deploiement/image-qcow2`.

Sécurité
--------

``/run/two/metadata/<vm>/vendor-data`` est en ``0644`` et contient le hash de mot de passe. Tout
compte local du host peut le lire. C'est une exposition connue et acceptée en l'état ; elle
disqualifie l'usage de hashs faibles ou réutilisés.
