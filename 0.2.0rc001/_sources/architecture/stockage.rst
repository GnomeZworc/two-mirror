Schéma des clés
===============

Toutes les valeurs stockées dans Badger sont des **chaînes plates** : une clé, une valeur, pas
de sérialisation structurée.

.. code-block:: text

   vpc/<name>/state             → creating | running | error | deleting | deleted
   vpc/<name>/cidr              → <cidr>

   subnet/<name>/state          → creating | running | error | deleting | deleted
   subnet/<name>/vpc            → <vpc-name>
   subnet/<name>/mode           → vxlan | bridge | public_ip
   subnet/<name>/vxlan_id       → <id>            (mode vxlan uniquement)
   subnet/<name>/cidr           → <cidr>
   subnet/<name>/interface_ip   → <ip>            (gateway, portée par br-<subnetID>)
   subnet/<name>/local_iface    → <bridge-name>
   subnet/<name>/default_route  → "true" | "false"
   subnet/<name>/gateway        → <ip>            (optionnel)
   subnet/<name>/dhcp/<ip>      → <mac>

   vm/<name>/state              → creating | running | error | deleting | deleted
   vm/<name>/subnet             → <subnet-name>
   vm/<name>/tap_id             → <int>
   vm/<name>/ip                 → <ip>
   vm/<name>/metadata_port      → <port>
   vm/<name>/disk/<dev>         → <path>          (une clé par disque : sda, vda, …)
   vm/<name>/memory             → <int>           (Mo)
   vm/<name>/cpus               → <int>
   vm/<name>/uefi               → "true"          (absent si SeaBIOS)
   vm/<name>/password           → <hash>          (optionnel — un hash, pas un mot de passe)
   vm/<name>/sshkey             → <pubkey>        (optionnel)
   vm/<name>/metadata/<document> → <contenu brut> (optionnel : user-data, vendor-data, …)

Règles
------

**Pas de duplication.** Une ressource ne stocke que ce qui lui est propre. Une VM garde le lien
``vm/<name>/subnet`` ; le VPC, le bridge et l'``interface_ip`` sont lus depuis le subnet, leur
source canonique.

**Les états passent par ``state``.** Toujours ``state.Set`` / ``state.Get`` : ``Set`` refuse une
valeur hors énumération, ``Get`` refuse de retourner une valeur non reconnue. ``error`` n'est
écrit que par ``Dispatcher.Dispatch``, via ``cmd.Key()``.

**Tout entier lu depuis la base peut être corrompu.** Les erreurs de conversion sont retournées,
jamais ignorées : une valeur absente ou illisible est un état d'erreur réel.

**Un seul ouvreur.** L'agent est le seul processus à ouvrir la base. Le serveur de metadata lit
des fichiers écrits par l'agent sous ``metadata.run_dir``, jamais Badger.
