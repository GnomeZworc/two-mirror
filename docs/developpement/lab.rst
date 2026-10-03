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

   État actuel : seule l'étape **E0** est livrée — le cycle de vie du serveur qui portera le lab,
   avec ``scripts/lab-host.sh``. La description de la topologie et le lancement des VM viendront
   avec les étapes suivantes, et cette page avec elles.

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

Environ une minute et demie, sans réseau : la suite remplace ``curl`` par une fausse API Scaleway
qui se place dans le pire cas (offre mensuelle listée avant l'horaire, serveurs d'autres projets,
suppressions refusées, erreurs 503, serveur qui tarde à disparaître) et ``ssh`` par un faux client.
Elle tourne sous bash 5 comme sous le bash 3.2 de macOS.
