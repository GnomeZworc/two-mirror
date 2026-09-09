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

Chaque document suit la même règle :

* fourni par l'appelant → servi **verbatim**, l'agent n'interprète rien ;
* absent → le template par défaut est rendu ;
* fourni **vide** → servi vide.

Les deux derniers cas sont distincts, et c'est délibéré : fournir une chaîne vide est une façon
explicite de neutraliser un document.

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

.. warning::

   ``network-config.tmpl`` cible ``eth0`` alors que les guests utilisent ``ens3`` : il ne
   s'applique donc à rien, et le réseau des VM vient du DHCP. **Ne pas le « corriger » ni le
   supprimer.** Le rendre opérant ferait remplacer par cloud-init la configuration réseau de
   l'image, sur toutes les VM.

Sécurité
--------

``/run/two/metadata/<vm>/vendor-data`` est en ``0644`` et contient le hash de mot de passe. Tout
compte local du host peut le lire. C'est une exposition connue et acceptée en l'état ; elle
disqualifie l'usage de hashs faibles ou réutilisés.
