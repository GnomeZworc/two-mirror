Lab de test multi-nœud
======================

Ce qui fait l'intérêt de two ne se voit qu'à partir de **deux hyperviseurs** : sur un nœud isolé,
le trafic reste sur le bridge local et l'absence de plan de contrôle passe inaperçue (voir
:doc:`/deploiement/architecture-cluster`). Le lab reproduit la topologie du cluster —
hyperviseurs, route reflector, switch L3 — sous forme de VM, sur un serveur physique loué à
l'heure.

Le pourquoi des choix (serveur physique plutôt que VM cloud, câbles QEMU, MTU 9000, versions) est
consigné sur le ticket `#50 <https://git.g3e.fr/syonad/two/issues/50>`_. Cette page décrit
comment s'en servir.

.. note::

   État actuel : étapes **E0** et **E1** livrées — le cycle de vie du serveur qui portera le lab
   (``scripts/lab-host.sh``), puis la description de la topologie et le calcul de son plan
   (``lab plan``). Le lancement des VM viendra avec les étapes suivantes, et cette page avec
   elles.

Le serveur de lab
-----------------

Un serveur **Scaleway Elastic Metal**, créé pour une campagne de tests puis supprimé.

.. list-table::
   :widths: 25 75

   * - Offre
     - ``EM-B212X-SSD``, zone ``fr-par-1``, **facturation horaire** : 0,321 € HT de l'heure, sans
       frais de mise en service
   * - Matériel
     - 2 × Xeon E5-2620 v4 *or equivalent*, 256 Go, 2 × 1 To SSD
   * - Système
     - Debian 12, installé par Scaleway à la création
   * - Pourquoi Intel
     - lab3 est en Intel : two lance ses VM en ``-cpu host``, et KVM a deux implémentations
       distinctes (``kvm_intel``, ``kvm_amd``)

*Or equivalent* n'est pas une clause de style : le premier serveur livré était un
**Xeon E5-2640 v3** (Haswell, la génération de lab3), pas le E5-2620 v4 annoncé. Relever
``lscpu`` au début de chaque campagne.

Prérequis côté Scaleway
-----------------------

#. **Un projet dédié au lab**, séparé de toute autre ressource. ``lab-host.sh down`` supprime
   tout serveur de lab du projet : il ne doit rien y avoir d'autre.
#. **Une clé d'API limitée à ce projet**, avec les droits Elastic Metal et la lecture des clés SSH
   du projet. Rien d'autre.
#. **Les clés SSH publiques enregistrées dans le projet**, injectées à l'installation : sans elle,
   le serveur serait facturé sans que personne puisse s'y connecter, et ``plan`` refuse de
   continuer.
#. **Le quota Elastic Metal.** L'``EM-B212X-SSD`` exige un compte dont le moyen de paiement *et*
   l'identité sont validés ; le quota est alors de 2. Vérifier dans la console : Organisation →
   Quotas → Elastic Metal. Voir `les quotas Scaleway
   <https://www.scaleway.com/en/docs/organizations-and-projects/organization/organization-quotas/>`_.

Fichiers locaux
---------------

Tout ce dont le script a besoin vit sous ``~/.config/two-lab/``, hors du dépôt.

``~/.config/two-lab/scaleway.env``
   Identifiants Scaleway, une ligne ``CLÉ=valeur`` chacun :

   .. code-block:: bash

      SCW_SECRET_KEY=<clé secrète>
      SCW_DEFAULT_PROJECT_ID=<identifiant du projet de lab>
      SCW_DEFAULT_ZONE=fr-par-1

   * le fichier doit être en ``0600`` : le script refuse de s'en servir s'il est lisible par
     d'autres que son propriétaire ;
   * il est **lu, jamais exécuté** — pas de ``source`` ; seules ces trois clés sont reconnues ;
   * une variable d'environnement du même nom l'emporte sur le fichier ;
   * la clé d'accès (``SCW…``) n'est pas nécessaire : l'API REST n'authentifie que par la clé
     secrète, dans l'en-tête ``X-Auth-Token``.

   Pour changer de clé, remplacer la ligne ``SCW_SECRET_KEY=`` ; rien d'autre à modifier.

``~/.config/two-lab/ssh/lab_ed25519``
   Clé SSH dédiée au lab, **sans phrase de passe**, pour que les sessions tournent sans
   intervention. Sa partie publique doit être enregistrée dans le projet. Quand elle existe, le
   script l'utilise **seule** (``IdentitiesOnly``, agent désactivé) ; sinon il retombe sur
   l'agent SSH.

   Elle ne doit ouvrir que les serveurs éphémères du projet de lab : **ne jamais l'installer sur
   lab3 ni sur une machine durable**. En cas de doute, la retirer du projet et en générer une
   autre :

   .. code-block:: bash

      mkdir -p ~/.config/two-lab/ssh && chmod 700 ~/.config/two-lab ~/.config/two-lab/ssh
      ssh-keygen -t ed25519 -N '' -C two-lab-automation -f ~/.config/two-lab/ssh/lab_ed25519

``~/.cache/two-lab/``
   État de la session en cours : adresse et utilisateur du serveur, ``known_hosts`` dédié. Vidé
   par ``down``.

Commandes
---------

.. code-block:: text

   usage: lab-host.sh <commande> [arguments]

     plan              résout l'offre horaire, l'OS et les clés SSH, affiche la requête de création
                       et le prix ; ne crée rien
     up                crée le serveur de lab, attend la fin de son installation et son SSH
     status            liste les serveurs de lab du projet
     ssh [commande]    se connecte au serveur de lab
     down              supprime tous les serveurs de lab du projet et attend leur disparition
     session [cmd]     up, puis la commande distante (ou un shell), puis down quoi qu'il arrive

``plan`` et ``status`` sont **gratuits** ; ``up`` et ``session`` **créent un serveur facturé**.

Toujours commencer par ``plan``. Il valide la clé d'API, le quota d'offre, l'OS et les clés SSH,
et montre exactement ce qui serait commandé :

.. code-block:: text

   $ scripts/lab-host.sh plan
   == offre   : EM-B212X-SSD (ddaf8ba6-b2b2-4279-8af3-51930fb602f8), facturation hourly, stock available
   == prix    : 0.321 EUR HT par heure, frais de mise en service 0 EUR
   == os      : Debian 12 (Bookworm) (83640d93-a0b8-45ad-9c9f-30cae48380a4), utilisateur root
   == clés    : 2 clé(s) SSH du projet
   == requête : POST /baremetal/v1/zones/fr-par-1/servers

``session`` est la forme normale d'usage : le serveur est supprimé à la fin, que la commande
réussisse, échoue, ou que la session soit interrompue (Ctrl-C, ``TERM``, fermeture du terminal).

.. code-block:: text

   $ scripts/lab-host.sh session 'uname -a; lscpu | grep -E "Model name|^CPU\(s\)|Virtualization"; free -g | head -2; echo "nested=$(cat /sys/module/kvm_intel/parameters/nested)"; ls -l /dev/kvm'
   == création de two-lab (EM-B212X-SSD, 0.321 EUR/h HT)
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 créé, facturé jusqu'à 'lab-host.sh down'
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ordered, installation to_install
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ready, installation installing
   …
   == serveur 2ecc1e6a-0de8-48c0-a198-93857eee5957 : ready, installation completed
   == SSH pas encore joignable, nouvel essai dans 20s
   …
   == prêt : root@<adresse>
   Linux two-lab 6.1.0-53-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.1.187-1 (2026-09-07) x86_64 GNU/Linux
   CPU(s):                                  32
   Model name:                              Intel(R) Xeon(R) CPU E5-2640 v3 @ 2.60GHz
   …
   Virtualization:                          VT-x
                  total        used        free      shared  buff/cache   available
   Mem:             251           1         250           0           0         249
   nested=Y
   crw-rw---- 1 root kvm 10, 232 Oct  3 17:23 /dev/kvm
   == session terminée (code 0), suppression du serveur
   == suppression de 2ecc1e6a-0de8-48c0-a198-93857eee5957
   == aucun serveur de lab ne reste dans le projet

Extrait de la première campagne (lignes répétées remplacées par ``…``). Compter **environ 15 minutes** entre la création et le SSH disponible : 13 min 30 à la première
campagne, suppression comprise. SSH ne répond pas tout de suite après la fin de l'installation —
environ 100 secondes la première fois — d'où l'attente intégrée à ``up``. Le code de sortie de
``session`` est celui de la commande distante.

``up``, ``ssh`` et ``down`` séparément servent au debug interactif — et laissent la suppression
à la charge de l'utilisateur.

Topologie
---------

Un lab est décrit par un fichier YAML : des **nœuds** (les VM) et des **segments** (des réseaux L2
portés par un switch). Exemple livré, ``conf/lab/evpn-2hv.yml`` :

.. literalinclude:: ../../conf/lab/evpn-2hv.yml
   :language: yaml

Chaque nœud non-switch est relié au switch de chacun de ses segments par un câble virtuel QEMU ;
le switch met ces câbles dans un bridge et porte la passerelle du segment.

Ce que le fichier déclare :

``images``
   ``url`` de l'image qcow2 et ``sums`` du fichier de sommes à vérifier, tous deux en ``https://``.

``segments``
   ``switch`` (un nœud de rôle ``switch``), ``cidr`` IPv4 entre ``/8`` et ``/30``, ``mtu``
   facultatif — 9000 par défaut, entre 1280 et 9000. Nom : 12 caractères au plus, minuscules et
   chiffres, parce qu'il devient le nom d'interface dans les VM et, préfixé de ``br-``, celui du
   bridge (15 caractères au plus sous Linux).

``nodes``
   ``role`` (``switch``, ``rr`` ou ``hypervisor``), ``image``, ``cpus``, ``memory`` en Mio
   (256 au moins), ``segments`` auxquels le nœud est relié, et ``addresses`` pour fixer
   l'adresse d'un nœud sur un segment (``addresses: {underlay: 10.250.0.50}``). Un switch ne
   déclare ni ``segments`` ni ``addresses`` : il porte ceux dont il est le ``switch``.

Ce que l'outil en déduit, de façon déterministe — même fichier, même plan :

.. list-table::
   :widths: 30 70

   * - Passerelle d'un segment
     - la première adresse du CIDR, portée par le switch sur ``br-<segment>``
   * - Adresse d'un nœud
     - les suivantes, **dans l'ordre de déclaration des nœuds** ; une adresse fixée par
       ``addresses`` est réservée d'abord et sautée par l'attribution automatique
   * - Câbles
     - un par couple (segment, nœud), segments puis nœuds dans l'ordre de déclaration ; le
       câble *i* utilise les ports UDP ``20000 + 2i`` (côté nœud) et ``20001 + 2i`` (côté switch)
   * - MAC
     - ``02:4c:<nœud>:<nœud>:<segment>:<côté>`` — préfixe localement administré, rang du nœud
       sur deux octets, rang du segment, ``00`` côté nœud et ``01`` côté switch
   * - Interfaces
     - côté nœud, le nom du segment ; côté switch, ``p<i>``, du rang du câble
   * - SSH d'administration
     - ``127.0.0.1:<2200 + rang du nœud>`` sur l'hôte du lab

.. warning::

   Réordonner les nœuds ou les segments dans le fichier **change les adresses, les MAC et les
   ports**. C'est assumé pour un lab ; ``lab plan`` montre le résultat avant tout lancement.

Limites : 1000 nœuds, 256 segments, et autant de câbles que la plage UDP le permet (22 768).

``lab plan`` valide le fichier et affiche le plan, sans rien lancer :

.. code-block:: text

   $ go run ./cmd/lab plan conf/lab/evpn-2hv.yml
   lab evpn-2hv: nodes 4, segments 1, cables 3

   nodes
     name  role        image     cpus  memory     ssh
     sw1   switch      debian12  2     1024 MiB   127.0.0.1:2200
     rr1   rr          debian12  1     1024 MiB   127.0.0.1:2201
     hv1   hypervisor  debian12  4     16384 MiB  127.0.0.1:2202
     hv2   hypervisor  debian12  4     16384 MiB  127.0.0.1:2203

   segment underlay: 10.250.0.0/24, mtu 9000, switch sw1, bridge br-underlay, gateway 10.250.0.1
     node  interface  address        mac                udp         switch port  mac                udp
     rr1   underlay   10.250.0.2/24  02:4c:00:01:00:00  20000  <->  sw1 p0       02:4c:00:01:00:01  20001
     hv1   underlay   10.250.0.3/24  02:4c:00:02:00:00  20002  <->  sw1 p1       02:4c:00:02:00:01  20003
     hv2   underlay   10.250.0.4/24  02:4c:00:03:00:00  20004  <->  sw1 p2       02:4c:00:03:00:01  20005

Un fichier invalide est refusé avec **toutes** ses erreurs à la fois, et un code de sortie 1. Les
champs inconnus et les clés en double sont refusés aussi :

.. code-block:: text

   $ lab plan cassee.yml
   lab: cassee.yml:
   segment underlay: rr1 is a rr, not a switch
   segment underlay: cidr 10.250.0.0/31 prefix length out of range [/8, /30]
   node sw1: switch carries no segment

Les plages d'adresses de l'exemple sont des valeurs de travail : le plan d'adressage du lab reste à
définir (#50).

Facturation
-----------

.. warning::

   Un serveur Elastic Metal est facturé **de sa création à sa suppression, éteint compris**.
   Éteindre ne suffit pas : il faut supprimer. La granularité n'est pas documentée par Scaleway —
   compter chaque heure entamée comme une heure pleine.

Ce que le script garantit :

* il ne commande **jamais** d'offre mensuelle : il exige une seule offre au nom demandé, en
  facturation horaire, en stock et sans frais de mise en service, sinon il refuse avant toute
  création. La CLI ``scw`` n'est pas utilisée pour cette raison : son ``server create type=…``
  choisit l'offre par son seul nom et peut tomber sur la mensuelle, qui engage un mois ;
* il refuse de créer un second serveur si un serveur de lab existe déjà ;
* ``down`` agit sur **tous** les serveurs portant le tag ``two-lab`` dans le projet, et
  ``session`` y ajoute l'identifiant reçu à la création : un serveur créé juste avant une
  interruption est rattrapé ;
* une suppression refusée pendant la livraison ou l'installation est réessayée tant qu'elle dure,
  dans la limite du délai d'installation augmenté du délai de suppression ;
* le serveur n'est déclaré supprimé qu'au 404 de l'API, jamais sur une erreur passagère ;
* un échec de suppression se termine par ``SERVEUR(S) DE LAB TOUJOURS FACTURÉ(S)`` et un code
  d'erreur.

Ce qu'il ne peut pas garantir : un ``SIGKILL``, une coupure de courant ou une mise en veille du
poste qui lance la session. En cas de doute, toujours :

.. code-block:: bash

   scripts/lab-host.sh status
   scripts/lab-host.sh down

Diagnostic
----------

``aucune clé SSH active dans le projet``
   Aucune clé SSH n'est enregistrée dans le projet de lab. En ajouter une dans la console (projet
   → Clés SSH).

``offre horaire EM-B212X-SSD : 0 correspondance(s)``
   L'offre n'existe pas dans la zone en facturation horaire. Vérifier ``SCW_DEFAULT_ZONE``.

``création incertaine``
   La création a échoué ou n'a pas rendu d'identifiant. Le serveur a pu être créé malgré tout —
   cas typique : le quota (identité non validée), ou une réponse perdue. Le script indique s'il
   voit un serveur de lab ; dans tous les cas, lancer ``status`` puis ``down``.

``SSH injoignable``
   L'installation est terminée mais SSH ne répond pas après 10 minutes. Avec une clé matérielle
   (Yubikey), chaque connexion demande un PIN ou un toucher : utiliser la clé dédiée du lab. Le
   serveur est toujours facturé : ``down``.

``SERVEUR(S) DE LAB TOUJOURS FACTURÉ(S)``
   La suppression n'a pas abouti dans les délais. Relancer ``down`` ; si l'erreur persiste,
   supprimer depuis la console Scaleway.

``HTTP 403 insufficient permissions``
   La clé d'API est authentifiée mais n'a pas le droit demandé — en général la lecture des clés
   SSH du projet. Compléter la politique de la clé, limitée au projet.

Sécurité
--------

* La clé secrète n'apparaît ni dans les arguments des processus (elle est passée à ``curl`` par
  un descripteur de fichier), ni dans les journaux, ni dans l'environnement de ``ssh``.
* **Ne jamais lancer le script sous** ``bash -x`` : la trace afficherait la clé.
* Le serveur n'expose que SSH, par clé. Le lab n'a aucune donnée personnelle ni secret de
  production.
* Une clé secrète qui a circulé ailleurs que dans ``scaleway.env`` (conversation, terminal
  partagé, capture d'écran) se régénère.

Tests
-----

.. code-block:: bash

   bash scripts/lab-host_test.sh
   go test ./internal/lab/... ./cmd/lab/

Environ une minute et demie, sans réseau : la suite remplace ``curl`` par une fausse API Scaleway
qui se place dans le pire cas (offre mensuelle listée avant l'horaire, serveurs d'autres projets,
suppressions refusées, erreurs 503, serveur qui tarde à disparaître) et ``ssh`` par un faux client.
Elle tourne sous bash 5 comme sous le bash 3.2 de macOS.
