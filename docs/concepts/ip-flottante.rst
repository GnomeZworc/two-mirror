IP flottante
============

Une IP flottante rend une interface de VM **joignable par une IP publique qu'elle ne porte pas**.
C'est une ressource de l'API de l'agent (``/floating-ips``), créée sur l'hyperviseur de la VM :
la VM garde son adresse privée, et le netns de son VPC traduit l'IP publique vers cette adresse,
dans les deux sens, sans suivi de connexion propre à la traduction.

.. code-block:: text

   POST /floating-ips
   { "name": "fip-7", "vm": "vm-7", "interface": "nic-a1b2",
     "ip": "203.0.113.20/24", "gateway": "203.0.113.1" }

``interface`` est le **nom** d'une interface de la VM (``name`` de l'interface, voir
:doc:`vpc-subnet-vm`), pas une interface Linux. L'IP publique, son préfixe et la gateway des
bordures sont fournis par l'appelant ; l'agent ne les vérifie pas. Une interface a au plus une IP
flottante, et une IP publique n'est portée que par une IP flottante de l'hôte. Le VPC n'a pas
besoin de ``l3vni``.

On attache et détache une IP flottante sans toucher à la VM. Le ``GET`` d'une VM l'affiche sur
l'interface concernée (``floating_ip`` : nom, IP, état), en lecture seule ; la suppression d'une
VM est refusée (409) tant qu'une IP flottante y est attachée.

Ce que l'agent pose
-------------------

Dans le netns du VPC de l'interface, sur l'hôte de la VM :

* un ``macvlan`` ``fip-<IP publique en hexadécimal>`` (``203.0.113.20`` → ``fip-cb007114``) sur
  le bridge ``br-public`` du netns. Sa MAC est ``02:05`` suivi des quatre octets de l'IP publique
  (``02:05:cb:00:71:14``) : unique dans l'infrastructure, la même sur tout hôte ;
* une entrée FDB locale de cette MAC, posée par remplacement (le bridge a pu l'apprendre sur le
  segment tant que l'IP flottante vivait sur un autre hôte), et ``arp_ignore=1``,
  ``arp_announce=2``, ``arp_notify=1``, comme pour la :doc:`passerelle NAT <passerelle-nat>` ;
* deux chaînes dans la table nftables ``ip two`` :

  * ``pre-fip-…``, en ``prerouting`` à la priorité ``raw`` : ce qui arrive par ``fip-…`` vers
    l'IP publique est réécrit vers l'adresse privée, **avant** conntrack ;
  * ``post-fip-…``, en ``postrouting`` : ce que la VM envoie par ``fip-…`` repart avec l'IP
    publique pour source ;

  conntrack ne voit que l'échange entre l'adresse privée et l'extérieur. Il n'y a pas de
  ``notrack`` : la redirection des métadonnées et le relais DNS et NTP, qui reposent sur
  conntrack, restent servis à la VM. L'entrée est ouverte : c'est le but d'une IP flottante ;
* l'IP publique en ``/32`` sur ``fip-…`` : aucune route du segment public n'est ajoutée à la table
  ``main`` du netns ;
* une table de routage propre, numérotée par l'adresse privée lue comme un entier sur 32 bits
  (``10.47.1.10`` → ``170852618``), qui porte la route du segment public par ``fip-…`` puis la
  route par défaut vers la gateway des bordures ;
* deux règles de routage pour l'adresse privée :

  * ``100: from <privée> to <cidr du VPC> lookup main`` : le trafic interne au VPC suit les routes
    habituelles ;
  * ``101: from <privée> lookup <table propre>`` : tout le reste sort par l'IP flottante, y compris
    dans un VPC qui a une passerelle NAT, et sur l'hôte de celle-ci.

  La table ``local`` passant avant ces règles, le trafic vers les adresses du netns (gateway,
  métadonnées, DNS et NTP) n'est pas concerné.

Les règles nftables sont posées avant l'adresse et les routes : rien ne sort par ``fip-…`` sans
traduction. La suppression retire d'abord les règles de routage, puis les règles nftables,
l'entrée FDB et le ``macvlan``, dont les routes partent avec lui. Chaque étape tolère ce qui est
déjà absent : une suppression interrompue se relance depuis ``error``.

Le segment public n'apparaît pas dans ``main`` du fait de l'IP flottante : sinon, une VM sans IP
flottante du même netns pourrait sortir par ``fip-…`` sans traduction, et la réponse d'une VM à
IP flottante vers une adresse du segment (la bordure elle-même) sortirait par la passerelle NAT
avec son adresse privée.

Déplacer une IP flottante
-------------------------

Quand la VM change d'hôte, l'appelant supprime l'IP flottante sur l'ancien hôte, puis la crée sur
le nouveau. La MAC étant la même, le déplacement se réduit à un ARP gratuit et à la mobilité MAC
d'EVPN. Les connexions en cours sont perdues.

Limites
-------

* IPv4 seulement.
* Refusée (409) sur une interface d'un subnet ``public_ip`` : la VM y porte déjà son IP publique.
* Une VM qui joint l'IP flottante d'une autre VM passe par les bordures.
