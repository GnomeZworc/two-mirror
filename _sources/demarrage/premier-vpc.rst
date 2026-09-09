Premier VPC, premier subnet, première VM
========================================

Ce tutoriel crée de bout en bout une VM joignable, sur un hyperviseur où l'agent est installé et
répond. Il suppose l'API sur ``127.0.0.1:8080`` et une image disque déjà présente sur l'host.

Tout se passe sur un **seul nœud** : un subnet ne s'étend à d'autres hyperviseurs qu'une fois le
plan de contrôle du cluster en place, cf. :doc:`/deploiement/architecture-cluster`.

Ce que l'on construit
---------------------

.. mermaid::

   graph LR
     subgraph netns vp-admin
       BR["br-sn000001<br/>10.1.1.1"]
       MD["metadata@i-web<br/>169.254.169.254"]
     end
     VM["VM i-web<br/>10.1.1.2"] --- BR
     BR --- MD
     BR --- VXLAN["VXLAN vni 1<br/>br-000000"]

Le VPC est un network namespace ; le subnet y pose un bridge porteur de la gateway ; la VM s'y
raccroche par un tap, reçoit son adresse en DHCP et son cloud-init depuis le serveur de metadata
du netns.

1. Le VPC
---------

.. code-block:: bash

   curl -X POST http://127.0.0.1:8080/vpcs \
     -H 'Content-Type: application/json' \
     -d '{"name": "vp-admin", "cidr": "192.168.0.0/16"}'

Le ``cidr`` est l'espace d'adressage global du VPC : c'est lui qui sera annoncé aux VM comme
route interne, quel que soit le mode du subnet.

La réponse est un **202** : la création est acceptée, pas terminée.

.. code-block:: bash

   curl -s http://127.0.0.1:8080/vpcs/vp-admin

Attendez ``"state": "running"`` avant l'étape suivante — un subnet dont le VPC parent n'est pas
prêt est refusé en **422**. Le modèle d'attente est décrit dans :doc:`/exploitation/api-agent/asynchronisme`.

2. Le subnet
------------

.. code-block:: bash

   curl -X POST http://127.0.0.1:8080/subnets \
     -H 'Content-Type: application/json' \
     -d '{"name": "sn-000001",
          "vpc": "vp-admin",
          "mode": "vxlan",
          "vxlan_id": 1,
          "iface_type": "vms",
          "interface_ip": "10.1.1.1",
          "cidr": "10.1.0.0/23"}'

``iface_type`` est une clé **logique** résolue dans la configuration de l'agent (section
``interfaces``) vers un bridge physique de l'host ; une clé inconnue retombe sur
``default_interface``. ``interface_ip`` est la gateway du subnet, portée par le bridge créé dans
le netns.

Les modes disponibles et leurs conséquences sur le routage sont détaillés dans
:doc:`/concepts/modes-reseau`.

Là encore, attendez ``running`` :

.. code-block:: bash

   curl -s http://127.0.0.1:8080/subnets/sn-000001

3. La VM
--------

.. code-block:: bash

   curl -X POST http://127.0.0.1:8080/vms \
     -H 'Content-Type: application/json' \
     -d '{"name": "i-web",
          "memory": 2048,
          "cpus": 2,
          "uefi": true,
          "metadata": {"sshkey": "ssh-ed25519 AAAA…",
                       "user_data": "'"$(base64 < user-data.yml | tr -d '\n')"'"},
          "interfaces": [{"subnet": "sn-000001", "ip": "10.1.1.2", "primary": true}],
          "storage": [{"path": "/var/lib/two/volumes/i-web.qcow2", "dev": "vda"}]}'

Quatre points qui coûtent du temps quand on les découvre en production :

``user_data`` est **encodé en base64**
   Un base64 invalide est rejeté en 400 plutôt que servi vide. L'agent n'interprète jamais ce
   contenu.

``password`` est un **hash**, pas un mot de passe
   Le champ attend la valeur de la clé ``passwd`` de cloud-config (``$6$…``). Sans ``password``
   ni ``sshkey``, aucun compte n'est créé.

Exactement une interface est ``primary``
   Elle porte la route par défaut et le serveur de metadata. L'ordre du tableau détermine le
   slot PCI (``0x03 + index``), donc le nom de l'interface dans le guest. Tous les subnets d'une
   VM doivent appartenir au même VPC.

Un seul disque ``vdX``
   Les disques supplémentaires passent par ``sdX``. La carte PCI en dépend — voir
   :doc:`/architecture/contraintes`.

4. Vérifier
-----------

.. code-block:: bash

   curl -s http://127.0.0.1:8080/vms/i-web

En ``running``, la VM est démarrée et le serveur de metadata est en place. Le provisionnement
cloud-init, lui, se déroule dans le guest ; on l'observe par la console série :

.. code-block:: bash

   socat -,raw,echo=0 UNIX-CONNECT:/run/two/vms/serial/i-web.sock

Puis, depuis l'host :

.. code-block:: bash

   ssh syonad@10.1.1.2

.. note::

   En mode ``vxlan``, la VM vit dans le netns du VPC : elle n'est pas joignable depuis l'host
   sans route explicite. En mode ``bridge``, elle l'est directement.

5. Supprimer
------------

Dans l'ordre inverse — un VPC dont il reste des subnets est refusé en **409** :

.. code-block:: bash

   curl -X DELETE http://127.0.0.1:8080/vms/i-web
   curl -X DELETE http://127.0.0.1:8080/subnets/sn-000001
   curl -X DELETE http://127.0.0.1:8080/vpcs/vp-admin

La suppression d'une VM ne touche **jamais** aux fichiers disque.

.. warning::

   ``instance-id`` vaut le nom de la VM. Recréer une VM du même nom sur le même disque fait que
   cloud-init la reconnaît et **n'applique pas** le user-data. Pour rejouer un provisionnement,
   changez de nom ou repartez d'un disque neuf.
