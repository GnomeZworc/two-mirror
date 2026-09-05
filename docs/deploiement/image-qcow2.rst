Construction de l'image qcow2
=============================

Toutes les VM du cluster — ``intel``, PostgreSQL, route reflector et les suivantes — partent
d'une même image qcow2 « golden », construite une fois puis réutilisée. Cette page décrit la
procédure en service.

.. important::

   Cette image est un **artefact redistribuable** : tout ce qui s'y trouve se retrouve dans
   chaque VM qui en dérive. Les étapes de nettoyage de la fin ne sont pas une commodité, ce sont
   des exigences.

Principe
--------

La construction se fait dans une **VM jetable**, et non par montage de l'image sur l'host : le
chroot a besoin d'un noyau et d'un espace utilisateur cohérents avec la distribution cible, ce
que l'host ne fournit pas nécessairement.

Cette VM de construction démarre sur un overlay de l'image du fournisseur et voit deux disques
supplémentaires : le futur disque « golden », et un espace de travail.

.. mermaid::

   graph LR
     ISO["seed.iso<br/>cloud-init NoCloud"] --> BVM
     OVL["&lt;os&gt;-tmp.qcow2<br/><i>overlay, jetable</i>"] --> BVM["VM de construction"]
     BVM --> ROOT["&lt;os&gt;-root.qcow2<br/><b>image golden</b>"]
     BVM --> WORK["tmp.qcow2<br/><i>espace de travail</i>"]
     BASE["image du fournisseur<br/>(qcow2)"] -.backing file.-> OVL

.. list-table::
   :header-rows: 1
   :widths: 26 20 54

   * - Disque
     - Vu dans la VM
     - Rôle
   * - ``<os>-tmp.qcow2``
     - ``vda`` (virtio-blk)
     - système de la VM de construction ; overlay de l'image du fournisseur, jeté à la fin
   * - ``<os>-root.qcow2``
     - ``sda`` (SCSI)
     - **le résultat** : l'image golden, écrite en brut depuis la VM
   * - ``tmp.qcow2``
     - ``sdb`` (SCSI)
     - espace de travail : téléchargement et conversion

Variables
---------

.. code-block:: bash

   export os=<nom_os>
   export os_link=<url_du_qcow2_fournisseur>
   export os_file=<nom_du_fichier_qcow2>
   export os_dir=<repertoire_de_telechargement>
   export disk_dir=<repertoire_des_disques>

Étape 1 — Le seed cloud-init de la VM de construction
------------------------------------------------------

Ce seed ne concerne **que la VM de construction**. Il n'a aucun rapport avec la configuration
cloud-init de l'image produite, qui est posée plus loin en chroot. Son seul rôle est de donner
un accès à la VM le temps du build.

.. code-block:: bash

   mkdir -p "${os_dir}" && cd "${os_dir}"
   mkdir -p /opt/seed/${os}

   cat << 'ENDFILE' > /opt/seed/${os}/meta-data
   instance-id: iid-local01
   local-hostname: my-vm-01
   ENDFILE

   cat << 'ENDFILE' > /opt/seed/${os}/network-config
   version: 2
   renderer: networkd
   ethernets:
     eth0:
       dhcp4: true
   ENDFILE

   cat << 'ENDFILE' > /opt/seed/${os}/user-data
   #cloud-config
   users:
     - name: <utilisateur>
       lock_passwd: false
       passwd: "<hash du mot de passe>"
       sudo: ALL=(ALL) NOPASSWD:ALL
       ssh_authorized_keys:
         - <clé publique ssh>
   ENDFILE

   mkisofs -o /opt/seed/${os}_seed.iso -V cidata -J -r /opt/seed/${os}/

Le label de volume ``cidata`` n'est pas décoratif : c'est ce qui fait reconnaître l'ISO comme une
source NoCloud par cloud-init.

.. warning::

   ``passwd`` attend un **hash**, et ``ssh_authorized_keys`` une clé publique personnelle : ces
   deux valeurs sont des données à ne pas recopier hors de l'host de construction. Elles ne
   figurent volontairement pas dans cette documentation.

   ``openssl passwd -5`` pour generer un hash

Étape 2 — Les disques
---------------------

.. code-block:: bash

   curl "${os_link}" -O

   qemu-img create -f qcow2 "${disk_dir}/${os}-root.qcow2" 10G
   qemu-img create -f qcow2 "${disk_dir}/tmp.qcow2" 50G
   qemu-img create -f qcow2 -b "${os_dir}/${os_file}" -F qcow2 "${disk_dir}/${os}-tmp.qcow2" 10G

.. important::

   ``-F qcow2`` est **obligatoire** sur qemu récent : sans lui, le format du backing file n'est
   pas figé dans l'en-tête de l'overlay.

La taille de ``<os>-root.qcow2`` (10 Gio ici) borne l'image produite : elle doit être au moins
égale à la taille **virtuelle** de l'image du fournisseur, pas à la taille de son fichier.

Étape 3 — Lancer la VM de construction
--------------------------------------

.. code-block:: bash

   qemu-system-x86_64 \
       -enable-kvm \
       -cpu host \
       -m 2048 \
       -smp 2 \
       -nographic \
       -serial mon:stdio \
       -monitor unix:/tmp/vm-build.mon-sock,server,nowait \
       -drive file=/opt/seed/${os}_seed.iso,media=cdrom,if=ide \
       \
       -drive file=${disk_dir}/${os}-tmp.qcow2,format=qcow2,if=none,id=vda \
       -device virtio-blk-pci,drive=vda,bootindex=0 \
       \
       -device virtio-scsi-pci,id=scsi0 \
       \
       -drive file=${disk_dir}/${os}-root.qcow2,if=none,id=hd0 \
       -device scsi-hd,drive=hd0,bus=scsi0.0 \
       \
       -drive file=${disk_dir}/tmp.qcow2,if=none,id=hd1 \
       -device scsi-hd,drive=hd1,bus=scsi0.0 \
       \
       -netdev tap,id=net0,ifname=tap0,script=no,downscript=no \
       -device virtio-net-pci,netdev=net0,mac=00:22:33:00:00:01

La répartition virtio-blk pour le système / SCSI pour les disques supplémentaires est la même que
celle qu'impose l'agent — voir :doc:`/architecture/contraintes`. Le tap ``tap0`` doit exister et
être raccordé à un réseau qui donne un accès sortant : la suite télécharge l'image du
fournisseur depuis la VM.

Étape 4 — Écrire l'image du fournisseur sur le disque cible
------------------------------------------------------------

Les commandes suivantes s'exécutent **dans la VM de construction**. Identifier d'abord les
disques : le disque de travail et le disque cible ne doivent pas être confondus.

.. danger::

   ``qemu-img convert`` écrase intégralement le disque cible. Vérifier les noms avant, avec
   ``lsblk``, plutôt que de supposer l'ordre d'énumération.

.. code-block:: bash

   work_disk=/dev/sdb
   os_disk=/dev/sda

   mkdir /work
   mkfs.xfs ${work_disk}
   mount ${work_disk} /work
   cd /work

   curl "${os_link}" -O
   qemu-img convert ./*.qcow2 -O raw ${os_disk}

L'image du fournisseur est écrite **en brut** directement sur le disque cible : le qcow2 obtenu
côté host contient donc une image disque complète et amorçable, sans backing file.

.. code-block:: bash

   partprobe
   echo 1 > /sys/block/sda/device/rescan
   sleep 2

   # La partition racine est la plus grande du disque
   root_partition=$(fdisk -lo device,size /dev/sda | grep -E '^\/dev\/' | tr -s ' ' \
                    | sort -rhk2 | head -n1 | cut -d ' ' -f1)

   mount -o nouuid $root_partition /mnt
   mount -o bind /dev  /mnt/dev
   mount -o bind /proc /mnt/proc
   mount -o bind /sys  /mnt/sys

   cp /etc/resolv.conf /mnt/etc/resolv.conf

``-o nouuid`` est nécessaire parce que le système de fichiers qui vient d'être écrit porte le
même UUID que celui déjà monté par la VM de construction. Le ``resolv.conf`` est copié pour que
les commandes en chroot aient la résolution DNS ; il est supprimé au nettoyage.

Étape 5 — Personnaliser l'image
-------------------------------

**Accès SSH**

.. code-block:: bash

   yum install -y augeas

   echo "The default user for Syonad VMs is 'syonad'." > /mnt/etc/banner

   augtool -r /mnt -s <<'EOF'
   set /files/etc/ssh/sshd_config/X11Forwarding no
   set /files/etc/ssh/sshd_config/PermitTunnel no
   set /files/etc/ssh/sshd_config/PermitRootLogin no
   set /files/etc/ssh/sshd_config/RSAAuthentication yes
   set /files/etc/ssh/sshd_config/PubkeyAuthentication yes
   set /files/etc/ssh/sshd_config/PasswordAuthentication no
   set /files/etc/ssh/sshd_config/UseDNS no
   set /files/etc/ssh/sshd_config/ChallengeResponseAuthentication no
   set /files/etc/ssh/sshd_config/GSSAPIAuthentication no
   set /files/etc/ssh/sshd_config/Match[1]/Condition/User "root,centos,ubuntu,debian,ec2-user"
   set /files/etc/ssh/sshd_config/Match[1]/Settings/Banner "/etc/banner"
   EOF

``PasswordAuthentication no`` vaut pour toutes les VM dérivées : l'accès se fait par clé, et le
champ ``password`` de l'API de l'agent ne sert donc **pas** à ouvrir une session SSH.

**Utilisateur par défaut et source de metadata**

.. code-block:: bash

   cat << 'ENDFILE' > /mnt/etc/cloud/cloud.cfg.d/20_user.cfg
   system_info:
     default_user:
       name: syonad
   ENDFILE

   cat << 'ENDFILE' > /mnt/etc/cloud/cloud.cfg.d/99_metadata.cfg
   datasource_list: [ NoCloud ]
   datasource:
     NoCloud:
       seedfrom: 'http://169.254.169.254:80'
       timeout: 5
       max_wait: 10
   ENDFILE

C'est ce second fichier qui raccorde l'image au serveur de metadata de l'agent : ``NoCloud`` est
la seule source retenue, et elle pointe sur ``169.254.169.254``. La route ``/32`` vers cette
adresse est indispensable côté agent — voir :doc:`/concepts/modes-reseau`.

**Services et durcissement**

.. code-block:: bash

   chroot /mnt/ systemctl enable fstrim.timer

   chroot /mnt/ systemctl disable rpcbind.service
   chroot /mnt/ systemctl disable rpcbind.socket

   augtool -r /mnt -s set /files/etc/selinux/config/SELINUX disabled

   chroot /mnt/ dnf remove -y 'cockpit*'
   chroot /mnt/ rm -rf /run/cockpit

Étape 6 — Nettoyer, puis éteindre
---------------------------------

.. code-block:: bash

   rm -f  /mnt/etc/resolv.conf
   rm -rf /mnt/var/cache/yum
   rm -rf /mnt/root/.ssh
   rm -rf /mnt/root/.bash_history
   rm -rf /mnt/tmp/*
   rm -rf /mnt/var/lib/dhcp/*
   rm -rf /mnt/var/tmp/*
   find /mnt/var/log ! -type d -exec rm '{}' \;
   rm -rf /mnt/var/lib/cloud/*

   poweroff

``/mnt/var/lib/cloud/*`` est le nettoyage le plus important : c'est lui qui fait que cloud-init
considère chaque VM dérivée comme une instance neuve. Sans lui, l'image embarque l'identité de
l'instance de construction et le user-data n'est pas appliqué — même mécanisme que la
recréation d'une VM sous un nom déjà utilisé, cf. :doc:`/concepts/metadata-cloud-init`.

Une fois la VM éteinte, ``${disk_dir}/${os}-root.qcow2`` est l'image golden.
``${os}-tmp.qcow2`` et ``tmp.qcow2`` sont jetables.

Points de vigilance
-------------------

.. warning::

   **SELinux est désactivé** dans l'image. C'est une couche de protection en moins sur toutes les
   VM qui en dérivent, y compris celles qui portent des fonctions sensibles comme le route
   reflector ou la base de données. Décision à assumer explicitement, et à réévaluer : le mode
   ``permissive`` permettrait au minimum de savoir ce qui serait bloqué.

.. note::

   ``fstrim.timer`` est activé dans l'image, mais l'agent lance QEMU **sans** ``discard=unmap``
   ni ``detect-zeroes=unmap`` sur les disques. Le ``fstrim`` du guest ne rend donc aujourd'hui
   aucun espace à l'host : les qcow2 ne se rétractent pas. L'activation reste utile pour le jour
   où l'option sera ajoutée côté agent, mais ne pas compter dessus pour la place disque.

.. note::

   **À vérifier** — ``seedfrom`` sans barre oblique finale. cloud-init construit l'URL des
   documents en concaténant ``seedfrom`` avec ``meta-data`` et ``user-data``. Confirmer sur une
   VM réelle que les deux documents sont bien récupérés, et corriger en
   ``http://169.254.169.254:80/`` si ce n'est pas le cas.

.. note::

   **Provenance de l'image du fournisseur** — tout ce qui est construit en hérite. Vérifier la
   somme de contrôle, et la signature quand elle existe, avant de construire dessus. La
   procédure actuelle télécharge l'image deux fois, une fois sur l'host et une fois dans la VM,
   sans vérification.

.. note::

   **À rédiger** — la procédure ne dit pas encore comment l'image produite est nommée, versionnée
   et distribuée aux hyperviseurs, ni quelles variantes existent par rôle (``intel``, PostgreSQL,
   route reflector) : image unique personnalisée au démarrage par cloud-init, ou images
   dérivées ?
