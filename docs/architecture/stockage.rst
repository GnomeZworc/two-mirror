Schéma des clés
===============

Toutes les valeurs stockées dans Badger sont des **chaînes plates** : une clé, une valeur, pas
de sérialisation structurée.

.. code-block:: text

   vpc/<name>/state             → creating | running | error | deleting | deleted
   vpc/<name>/cidr              → <cidr>
   vpc/<name>/l3vni             → <id>            (optionnel : VPC routé par L3VNI)

   subnet/<name>/state          → creating | running | error | deleting | deleted
   subnet/<name>/vpc            → <vpc-name>
   subnet/<name>/mode           → vxlan | bridge | public_ip
   subnet/<name>/vxlan_id       → <id>            (mode vxlan uniquement)
   subnet/<name>/cidr           → <cidr>
   subnet/<name>/interface_ip   → <ip>            (gateway ; calculée en vxlan : broadcast-1)
   subnet/<name>/relay_ip       → <ip>            (mode vxlan, calculée : broadcast-2)
   subnet/<name>/local_iface    → <bridge-name>
   subnet/<name>/default_route  → "true" | "false"
   subnet/<name>/gateway        → <ip>            (optionnel)

   vm/<name>/state              → creating | running | error | deleting | deleted
   vm/<name>/nic/<i>/subnet     → <subnet-name>   (une interface par index, ordre du tableau interfaces)
   vm/<name>/nic/<i>/ip         → <ip>
   vm/<name>/nic/<i>/mac        → <mac>           (fournie par l'appelant, conservée telle quelle)
   vm/<name>/nic/<i>/primary    → "true"          (exactement une interface ; absent sur les autres)
   vm/<name>/nic/<i>/tap_id     → <int>           (alloué au premier démarrage, puis conservé)
   vm/<name>/metadata_port      → <port>
   vm/<name>/disk/<dev>         → <path>          (une clé par disque : vda, xvda, xvdb, …)
   vm/<name>/memory             → <int>           (Mo)
   vm/<name>/cpus               → <int>
   vm/<name>/uefi               → "true"          (absent si SeaBIOS)
   vm/<name>/password           → <hash>          (optionnel — un hash, pas un mot de passe)
   vm/<name>/sshkey             → <pubkey>        (optionnel)
   vm/<name>/metadata/<document> → <contenu brut> (optionnel : user-data, vendor-data, …)

Règles
------

**Pas de duplication.** Une ressource ne stocke que ce qui lui est propre. Une interface de VM
garde le lien ``vm/<name>/nic/<i>/subnet`` ; le VPC, le mode, le bridge et l'``interface_ip`` sont
lus depuis le subnet, leur source canonique. De même, la L3VNI d'un VPC n'est stockée que sous
``vpc/<name>/l3vni`` : RT, RD, MAC de routeur et table de routage en sont calculés.

**Ancien schéma des VM.** Avant le multi-interfaces, une VM portait directement ``subnet``, ``ip``
et ``tap_id``. L'agent les migre au démarrage vers ``nic/0/…``, marquée ``primary``, avant
d'ouvrir son API ; une VM qui a déjà des clés ``nic/`` est laissée telle quelle.

**Ancienne table des MAC.** Jusqu'en 0.1.0, la MAC d'une interface était dérivée du rang de son IP
et préchargée pour tout le ``cidr`` sous ``subnet/<name>/dhcp/<ip>``. Au démarrage, l'agent
recopie la MAC de chaque interface existante dans ``vm/<name>/nic/<i>/mac``, puis supprime ces
clés. Les VM ``deleted`` sont ignorées. Une interface dont la MAC est introuvable fait échouer le
démarrage en la nommant, et la table reste en place tant que toutes les interfaces ne sont pas
migrées.

**Anciens noms de disques.** Jusqu'en 0.1.0, les disques additionnels s'appelaient ``sdX``. Au
démarrage, l'agent renomme chaque clé ``vm/<name>/disk/sdX`` en ``vm/<name>/disk/xvdX`` (même
lettre, même cible SCSI). Si ``xvdX`` existe déjà avec un autre chemin, le démarrage échoue en
nommant la clé. Le numéro de série du disque, et donc son lien ``by-id``, prend le nouveau nom au
prochain lancement de la VM.

**Les états passent par ``state``.** Toujours ``state.Set`` / ``state.Get`` : ``Set`` refuse une
valeur hors énumération, ``Get`` refuse de retourner une valeur non reconnue. ``error`` n'est
écrit que par ``Dispatcher.Dispatch``, via ``cmd.Key()``.

**Tout entier lu depuis la base peut être corrompu.** Les erreurs de conversion sont retournées,
jamais ignorées : une valeur absente ou illisible est un état d'erreur réel.

**Un seul ouvreur.** L'agent est le seul processus à ouvrir la base. Le serveur de metadata lit
des fichiers écrits par l'agent sous ``metadata.run_dir``, jamais Badger.
