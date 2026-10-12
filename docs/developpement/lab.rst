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

   État actuel : le lab de #50 est livré et l'outil en est sorti (#55) : c'est
   `labctl <https://git.g3e.fr/syonad/labctl>`_, un dépôt à part, dont two épingle une release.
   Ce dépôt garde ce qui est propre à two — ses topologies, l'installation de two sur les
   hyperviseurs, les scénarios et leurs résultats. La redondance du plan de contrôle — deux route
   reflectors, deux switchs — est l'objet de `#54 <https://git.g3e.fr/syonad/two/issues/54>`_.

L'outil
-------

``labctl`` monte un lab de VM QEMU à partir d'une topologie YAML et s'exécute sur un hôte Linux
avec KVM ; son ``lab-host.sh`` loue cet hôte à l'heure chez Scaleway (Elastic Metal), le prépare,
y dépose ``labctl`` et la topologie, puis le supprime. Prérequis Scaleway, fichiers locaux
(``~/.config/labctl/``), commandes, format de topologie, rendu des VM, garanties de facturation et
diagnostic : voir le `guide de labctl <https://git.g3e.fr/syonad/labctl/src/branch/main/docs/guide.md>`_.

two n'en garde aucune copie. ``test/e2e/labctl.sh`` épingle une release :

* au premier appel, il télécharge ``labctl_linux_amd64`` et ``lab-host.sh`` dans
  ``~/.cache/labctl/release/<version>/`` et les vérifie contre des sommes SHA-256 **écrites dans
  le script** — une release modifiée après coup est refusée ; un fichier altéré dans le cache est
  retéléchargé ;
* il lance ensuite ``lab-host.sh`` avec ses arguments et ``LABCTL_BIN`` pointé sur le binaire :
  ``push`` dépose ce binaire au lieu de compiler.

Changer de version : ``LABCTL_VERSION`` et les deux lignes de ``LABCTL_SUMS`` dans
``test/e2e/labctl.sh``, relevées dans le ``SHA256SUMS`` de la release **et** recalculées sur les
fichiers téléchargés.

Une campagne, depuis le Mac :

.. code-block:: text

   test/e2e/labctl.sh plan                                   # gratuit : offre, zone, prix
   test/e2e/labctl.sh up                                     # crée le serveur, facturé à l'heure
   test/e2e/labctl.sh push test/e2e/topologies/evpn-3hv.yml
   test/e2e/labctl.sh ssh './labctl up -timeout 25m topology/evpn-3hv.yml'
   test/e2e/run.sh all
   test/e2e/labctl.sh ssh './labctl ssh hv1'                 # shell interactif sur hv1
   test/e2e/labctl.sh down                                   # supprime le serveur

.. warning::

   Un serveur Elastic Metal est facturé **de sa création à sa suppression**, chaque heure
   entamée comme une heure pleine. En cas de doute (session interrompue, poste mis en veille) :
   ``test/e2e/labctl.sh status`` puis ``down``. **Ne jamais lancer** ``lab-host.sh`` **sous**
   ``bash -x`` : la trace afficherait la clé d'API.

Topologies
----------

Les topologies de two vivent dans ``test/e2e/topologies/`` : ``evpn-2hv.yml`` (deux hyperviseurs)
et ``evpn-3hv.yml`` (trois, pour ``s8`` et ``s9``). Le format est celui de ``labctl`` :

.. literalinclude:: ../../test/e2e/topologies/evpn-2hv.yml
   :language: yaml

Les ASN, la loopback du route reflector, le lien ``169.254.0.0/28`` et le subnet des hyperviseurs
de l'exemple sont **ceux de la production** (décision du 2026-10-04, #50) : les fichiers de
``test/e2e/topologies/`` restent ainsi au plus près de ce qui tourne réellement. Toutes les adresses y sont
**fixées** par ``addresses`` — les route reflectors en ``.2`` et ``.3``, les hyperviseurs à partir de ``.11`` —
pour que le modèle se lise sans le plan et ne dépende pas de l'ordre de déclaration : le
``frr.conf`` d'un hyperviseur, écrit à la main, porte son adresse en ``router-id``. Seul le switch
n'en déclare pas : il porte toujours la passerelle, la première adresse du segment.

Rôles
~~~~~

Les configurations FRR du lab vivent dans ``test/e2e/topologies/frr/``, une par nœud, **écrites à la main** :
ce sont les mêmes fichiers que la documentation de déploiement inclut, pour que le lab qualifie
exactement ce qu'elle prescrit. Celles des hyperviseurs se réduisent à ``hostname`` et ``log`` :
la configuration BGP est posée par l'agent (section ``bgp`` de ``agent.yml``). Celle du route
reflector :

.. literalinclude:: ../../test/e2e/topologies/frr/rr1.conf
   :language: text

FRR est installé par ``labctl`` au premier démarrage (``frr-stable`` de ``deb.frrouting.org``,
clé du dépôt embarquée dans l'outil) : voir la section FRR du guide de ``labctl``.

**Hyperviseurs.** Ce sont des nœuds ``host`` dont la provision installe two comme en
production. ``test/e2e/topologies/provision/hypervisor.sh`` dépose ``agent.yml`` (fichier
``agent.yml`` du nœud) dans ``/etc/two/agent.yml``, télécharge ``deploy.sh`` et
``bootstrap_kvm.sh`` **au tag de la release testée** dans ``/opt/two/scripts/``, où ``deploy.sh``
cherche d'abord ``bootstrap_kvm.sh``, puis lance :

.. code-block:: text

   deploy.sh --noup_script -i -u "$LAB_UPLINK" -t <release>

La release est fixée dans le script (``RELEASE=``) : sans elle, ``deploy.sh`` prendrait la dernière,
et le lab ne serait plus reproductible. ``--noup_script`` empêche l'auto-mise à jour de remplacer le
script par celui de ``main``. L'uplink ``-u`` est l'interface qui porte la route par défaut, parce
que ``deploy.sh`` y lit l'adresse et la passerelle qu'il déplace sur ``br-000000``. ``deploy.sh``
télécharge la release sur ``git.g3e.fr`` sans réessayer : le script attend d'abord que le serveur
réponde, à travers le switch. Les deux fichiers ``agent/`` donnent à l'agent la section ``bgp``
(AS 64600, route reflectors ``10.255.255.1`` et ``10.255.255.2``) et un watchdog toutes les 10 s ;
``agent/two.yml`` met en plus hv1 sur le serveur DHCP intégré (``dhcp.backend: two``),
``agent/bgp.yml`` laisse les autres sur dnsmasq.

**Tester l'arbre courant plutôt que la release.** ``test/e2e/build.sh`` compile ``agent``,
``dhcp`` et ``metadata`` pour linux/amd64 dans ``test/e2e/topologies/bin/`` (ignoré par git),
que les hyperviseurs reçoivent par ``files: {bin: bin}``. Après ``deploy.sh``,
``hypervisor.sh`` installe chaque binaire présent dans ``/opt/two/local/bin/`` et y fait pointer
``/opt/two/bin/<nom>`` ; le reste (units, scripts) vient de la release. ``build.sh --clean`` vide
le répertoire : le lab suivant teste la release seule.

.. code-block:: text

   test/e2e/build.sh                                   # binaires de l'arbre courant
   test/e2e/labctl.sh push test/e2e/topologies/evpn-3hv.yml
   test/e2e/labctl.sh ssh './labctl up topology/evpn-3hv.yml'
   test/e2e/build.sh --clean                           # retour à la release seule

FRR est installé **après** la provision : il démarre sur le réseau final. Démarré
avant la migration, zebra peut perdre la route par défaut quand elle passe de l'uplink à
``br-000000`` — constaté le 2026-10-11 sur un hyperviseur sur deux : route reflector
``unresolved``, session BGP jamais établie. L'agent, configuré avec une section ``bgp``, refuse de
démarrer tant que FRR n'est pas là : son premier démarrage par ``deploy.sh`` échoue, et
``provision/hypervisor-post.sh`` (``post_provision``) le **redémarre** une fois FRR installé.

.. warning::

   **Un hyperviseur du lab ne survit pas à un redémarrage.** En production, la racine est en
   tmpfs et ``deploy.sh --bootstrap`` est rejoué à chaque démarrage ; dans le lab, ``-i`` n'est
   exécuté qu'au premier, et la migration réseau, qui n'est pas persistée, est perdue. Recréer le
   lab : ``labctl down`` puis ``labctl up``.

Vérifié le 2026-10-04 sur le serveur de lab, topologie ``evpn-2hv``, release ``0.2.0rc002`` :

.. code-block:: text

   $ test/e2e/labctl.sh ssh './labctl up -timeout 25m topology/evpn-2hv.yml'
   sw1: started
   rr1: started
   hv1: started
   hv2: started
   sw1: ready
   rr1: ready
   hv1: ready
   hv2: ready

.. list-table::
   :header-rows: 1
   :widths: 55 45

   * - Vérification
     - Résultat
   * - ``labctl up`` complet : FRR, ``deploy.sh`` et vérification des services de chaque rôle
     - 6 min 15
   * - session switch ↔ route reflector, IPv4 unicast, BFD
     - Established, BFD up ; le switch reçoit la seule loopback du route reflector
   * - sessions EVPN des deux hyperviseurs vers la loopback du route reflector
     - Established, voisins dynamiques, stables
   * - réseau d'un hyperviseur après ``deploy.sh``
     - adresse sur ``br-000000``, MTU 9000, API de l'agent qui répond
   * - VPC, subnet ``vxlan`` et VM Debian ``genericcloud`` créés par l'API de hv1
     - ``login:`` en 20 s, en KVM imbriqué
   * - DHCP et routes (option 121) servis par two à la VM
     - conformes, route ``/32`` vers ``169.254.169.254`` comprise
   * - métadonnées, image configurée selon :doc:`/deploiement/image-qcow2`
     - ``DataSourceNoCloudNet``, nom d'hôte appliqué — avec la barre oblique finale de
       ``seedfrom`` (voir cette page)
   * - VM ↔ VM entre les deux hyperviseurs, même subnet ``vxlan``
     - **échec** : les VXLAN de two n'ont pas d'adresse VTEP locale, rien n'est annoncé en EVPN —
       `#51 <https://git.g3e.fr/syonad/two/issues/51>`_ ; avec l'adresse posée, ping et MTU 1500
       passent

.. note::

   **Le switch du lab est un routeur Linux avec FRR, par choix.** Il joue le rôle générique de
   routeur de cluster : passerelle des hyperviseurs, session eBGP avec BFD vers le route reflector,
   dont il n'accepte que la loopback (``test/e2e/topologies/frr/sw1.conf``). L'équipement réel dépend de qui
   déploie l'infrastructure (MikroTik aujourd'hui ; Cisco, Juniper, Arista… demain) : il n'a besoin
   que de BGP et d'EVPN, et sa configuration propre au constructeur n'a pas sa place dans le lab.

Scénarios
---------

Les scénarios se lancent **depuis le Mac**, sur un lab démarré (``up``, ``push``, ``./labctl up``) :

.. code-block:: text

   test/e2e/run.sh s1          # un scénario
   test/e2e/run.sh s1 s3       # plusieurs
   test/e2e/run.sh all         # tous, dans l'ordre

Chacun affiche une ligne ``RÉUSSI`` ou ``ÉCHOUÉ`` par vérification — un échec porte la dernière
ligne de la commande en cause —, des lignes ``INFO`` pour les mesures, puis son bilan. Le code de
sortie vaut 1 si une vérification échoue **ou si aucune n'a été faite**.

.. list-table::
   :header-rows: 1
   :widths: 22 58 20

   * - Scénario
     - Ce qui doit être vrai
     - Hyperviseurs
   * - ``s1-dhcp-two``
     - backend DHCP ``two``, une VPC et deux subnets : chaque VM reçoit l'adresse de **son**
       subnet, démarrée seule ou en même temps qu'une autre ; le bail est tenu par
       systemd-networkd ; route par défaut, route vers la VPC et ``/32`` vers les métadonnées
       via ``interface_ip`` ; l'état de chaque serveur DHCP ne connaît que les MAC de son subnet
       (#46)
     - hv1
   * - ``s2-gateway``
     - route par défaut via ``interface_ip``, ou via ``gateway`` avec ``default_route`` ; route
       vers la VPC toujours via ``interface_ip`` ; même résultat après recréation du subnet (#31)
     - hv1
   * - ``s3-isolation-local``
     - deux VPC sur le même hyperviseur ne se joignent pas, en ICMP comme en TCP ; chaque VM
       joint sa passerelle (témoin)
     - hv1
   * - ``s4-evpn``
     - adresse VTEP locale sur les VXLAN (#51), VTEP distant appris par EVPN, ping VM ↔ VM
       entre hyperviseurs, trame de 1472 octets en ``-M do``, 1473 refusés
     - hv1, hv2
   * - ``s5-isolation-evpn``
     - deux VPC de même plage, VNI différentes, sur deux hyperviseurs : la VM joint celle de sa
       VPC sur l'autre hyperviseur (témoin) et pas celle de l'autre VPC — aucune résolution ARP
     - hv1, hv2
   * - ``s6-rr-loss``
     - FRR arrêté sur rr1, puis sur rr2, ping continu entre les VM
       de ``s4`` : la session vers le route reflector arrêté tombe, celle vers l'autre reste
       établie, le VTEP distant reste connu et **le trafic continue** (au moins 45 réponses sur 50
       à 30 s) ; session rétablie au redémarrage, délai mesuré ; au plus 1 % de pertes sur tout le
       scénario (#54)
     - hv1, hv2, rr1, rr2
   * - ``s7-l3vni``
     - qualification de `#41 <https://git.g3e.fr/syonad/two/issues/41>`_ **par l'API** : VPC créées
       avec une ``l3vni``, l'agent pose VRF, L3VNI, transit, FRR et les routes de type 5 (``/24``
       des subnets, ``/32`` des VM). Deux subnets d'une même VPC, chacun sur un seul hyperviseur,
       se joignent par le L3VNI à travers le route reflector inchangé ; 1472 octets en ``-M do`` ;
       une seconde VPC de même plage sur un autre L3VNI n'est ni importée ni jointe ; L3VNI déjà
       prise refusée ; forwarding de l'hôte limité aux interfaces de la VRF ; suppression d'un
       subnet refusée tant qu'une VM l'utilise ; démontage complet d'une VPC sans redémarrage de
       bgpd
     - hv1, hv2, rr1
   * - ``s8-l3vni-migration``
     - suite de ``s7``, topologie ``evpn-3hv.yml`` : migration d'une VM de hv1 vers hv2 par l'API,
       observée depuis hv3 qui n'a pas le subnet : migration simple (VM supprimée puis recréée,
       ``/24`` comme filet), séquence orchestrée (deux VM de même IP, deux ``/32`` en ECMP, 40
       sources sur 40) ; l'ancien « pire cas » devient un refus (409) ; MAC de gateway propre à
       chaque hôte, sans doublon EVPN ; watchdog : ``/32`` orphelin signalé comme trou noir, ``/32`` manquant
       signalé
     - hv1, hv2, hv3, rr1
   * - ``s9-subnets-partages``
     - deux VPC, avec et sans ``l3vni``, chacune avec deux subnets présents sur **les deux**
       hyperviseurs et une VM par subnet et par hôte : ping entre toutes les paires (même subnet ou
       non, même hôte ou non) ; chaque VM voit la MAC de gateway de son propre hôte ; aucune MAC en
       double sur les quatre VNI
     - hv2, hv3, rr1

``s6`` réutilise les VM de ``s4`` : le lancer après.

**Comment c'est fait.** ``test/e2e/run.sh`` exécute chaque scénario sur le Mac ; un
scénario envoie des blocs de shell aux nœuds par ``on <nœud> [VAR=valeur…] <<'NODE'``, précédés de
``test/e2e/lib/node.sh`` — appels à l'API de l'agent, attente des états, image Debian compatible two
(préparée une fois par hyperviseur, ``seedfrom`` avec barre oblique finale), clé SSH des VM,
``check`` et ``vm_fails`` ; pour #41, ``vpc_create`` avec une ``l3vni``, ``api_code`` (code HTTP
d'un refus attendu), l'observation des VRF et de BGP (``vrf_nexthops``, ``bgp_paths``,
``wait_l3vni``…) et ``wait_watchdog`` (alerte du watchdog dans le journal de l'agent). Une vérification négative (« ne joint pas ») passe par ``vm_fails`` :
elle n'est réussie que si le SSH vers la VM a fonctionné **et** que la commande y a échoué — un
SSH en panne ne passe jamais pour une isolation.

Résultats du 2026-10-04 sur le serveur de lab, release ``0.2.0rc003``, hv1 sur le DHCP intégré et
hv2 sur dnsmasq — toute la série en 8 minutes :

.. code-block:: text

   $ test/e2e/run.sh all | grep -E '^(=== s[0-9].* : |INFO)'
   INFO: MAC de sn-s1a : 00:22:33:00:00:0a 00:22:33:00:00:0b ; de sn-s1b : 00:22:33:00:00:0a 00:22:33:00:00:0b
   === s1-dhcp-two : 37 réussi(s), 0 échoué(s)
   === s2-gateway : 22 réussi(s), 0 échoué(s)
   === s3-isolation-local : 12 réussi(s), 0 échoué(s)
   === s4-evpn : 14 réussi(s), 0 échoué(s)
   === s5-isolation-evpn : 13 réussi(s), 0 échoué(s)
   INFO: 30 s après l'arrêt du route reflector : 0 réponses dans les 10 dernières secondes (50 si le trafic passe intégralement)
   INFO: VTEP distant encore connu : 0 ; entrée d'inondation : 0
   INFO: 90 s après l'arrêt : 0 réponses dans les 10 dernières secondes
   INFO: retour du VTEP distant et du trafic 31 s après le redémarrage de FRR
   === s6-rr-loss : 8 réussi(s), 0 échoué(s)

Dans ce run, le contrôle « uniquement les MAC de son subnet » de ``s1`` ne prouvait rien : two
dérive la MAC du rang de l'IP, et les VM ``.10``/``.11`` des deux subnets avaient les mêmes MAC.
Vérifié à la main sur le lab, chaque serveur DHCP ne connaissait que les couples MAC/IP de son
subnet ; le scénario compare désormais ces couples.

``s7-l3vni`` le 2026-10-05, même lab, FRR 10.7.1 : **46 réussi(s), 0 échoué(s)** au dernier run.
Quatre constats pour #41, au-delà du routage inter-subnet qui fonctionne avec le route reflector
inchangé :

* **bgpd 10.7.1 plante** (SIGSEGV dans ``bgp_redistribute_add``) quand la VRF du noyau est
  supprimée puis recréée sous une instance ``router bgp … vrf`` encore configurée — FRR refuse de
  supprimer l'instance tant que ``vni`` est déclaré dans la VRF (« Please unconfigure l3vni »).
  Le plantage a touché les deux hyperviseurs à une seconde d'intervalle. Avec le démontage dans
  l'ordre — ``no redistribute``, ``no vni``, ``no router bgp … vrf``, puis le noyau —, trois runs
  consécutifs passent sans plantage.
* **La configuration posée par vtysh est perdue** au redémarrage de bgpd par watchfrr, qui
  relit ``frr.conf`` seul.
* **Le transit** ``169.254.0.0/30``, **identique sur tous les hôtes**, fait rejeter comme
  « martiens » les ICMP émis par les sauts distants (VRF et netns de l'hôte de sortie) : le
  noyau de l'hôte d'entrée possède la même adresse. Quatre routeurs sur le chemin, seuls les
  deux premiers répondent. Même effet attendu sur un ICMP « fragmentation nécessaire ».
* **La route du supernet n'existe pas dans le netns** de la VPC avant la pose manuelle (premier
  run, lab neuf) : two ne pose que la route connectée du subnet.

``s8-l3vni-migration`` le 2026-10-06, ``evpn-3hv.yml`` : **61 réussi(s), 0 échoué(s)**. Le filet du
``/24`` porte le trafic tant que la destination n'a pas annoncé son ``/32`` ; deux ``/32``
coexistent sans perte tant que la source garde le subnet (40 sources sur 40, relais en L2). Si la
source supprime le subnet avant de retirer son ``/32``, le trou noir est **stable mais partiel** :
hv3 installe les deux chemins en ECMP, et seuls les flux hachés vers la source échouent (17 sources
sur 40) — invisible avec un seul émetteur.

Le 2026-10-11, les mêmes qualifications **par l'API**, l'agent posant toute la configuration
(FRR 10.7.1) :

* ``s1`` à ``s6`` sur ``evpn-2hv.yml``, release ``0.2.0rc004``, base BGP des hyperviseurs posée par
  l'agent : 37, 22, 12, 14, 13 et 8 réussis, aucun échec.
* Puis, release ``0.2.0rc005`` (correction du démontage FRR, ci-dessous), ``evpn-3hv.yml`` :
* ``s7-l3vni`` : **52 réussi(s), 1 échoué(s)**. Routage inter-subnet par le
  L3VNI, MAC de routeur dérivée du VTEP, exports ``/24`` et ``/32`` automatiques, forwarding par
  interface, L3VNI en double et subnet utilisé refusés (409), démontage complet d'une VPC sans
  redémarrage de bgpd. L'échec — « TTL 1 », un seul paquet — ne s'est pas reproduit au run
  précédent : probablement le débit limité des ICMP du noyau ; la vérification envoie désormais
  trois paquets.
* ``s8-l3vni-migration`` : **48 réussi(s), 1 échoué(s)**. Migration simple (``/32`` de la
  destination installé sur hv3 sans délai mesurable, 50/50), séquence orchestrée (deux ``/32`` en
  ECMP, 40 sources sur 40), ex-pire cas refusé (409), watchdog : ``/32`` orphelin signalé comme
  trou noir, ``/32`` manquant signalé. L'échec est un constat : **la MAC de gateway anycast est
  annoncée en EVPN** (route de type 2 sur rr1) et FRR la déclare en double sur la VNI du subnet
  porté par deux hyperviseurs — apprise sur le veth du subnet, numéros de séquence croissants.

Trois défauts trouvés et corrigés pendant la campagne :

* **FRR installé avant la migration réseau** : zebra a perdu la route par défaut sur un
  hyperviseur sur deux (route reflector ``unresolved``). Retour à l'ordre « FRR après
  ``deploy.sh`` », puis redémarrage de l'agent (voir `Rôles`_).
* **Remarques de vtysh prises pour des erreurs** : ``no vni`` répond
  ``% Configuration applied with notes:`` avec un code de retour 0. Une vraie erreur (commande
  inconnue, refus) sort en 1.
* **L3VNI libérée de façon asynchrone** : juste après ``no vni``, ``no router bgp … vrf`` peut être
  refusé (« Please unconfigure l3vni ») puis accepté un instant plus tard ; l'agent réessaie.

Le 2026-10-12, release ``0.2.0rc006``, ``evpn-3hv.yml`` : ``s1`` à ``s7`` sans échec,
``s8-l3vni-migration`` **50 réussi(s), 0 échoué(s)** (MAC de gateway propre à chaque hôte, aucun
doublon EVPN), ``s9-subnets-partages`` **75 réussi(s), 1 échoué(s)**. L'échec est une session SSH
coupée (``Broken pipe``), pas un ping perdu : quand un hôte route vers une VM d'un autre hôte, son
netns émet une requête ARP au nom de la gateway, partagée par tous les hôtes ; la VM met son cache à
jour et, pendant ~8 s, envoie à la gateway de l'autre hôte ce qui était destiné à la sienne.
Détails et suite : `#57 <https://git.g3e.fr/syonad/two/issues/57>`_.

.. note::

   Un ``Broken pipe`` (``client_loop: send disconnect``) ou une session SSH figée dans un scénario
   n'est **pas forcément un aléa du lab** : c'est la signature de ce problème. Ne pas le classer
   comme test instable ni ajouter un nouvel essai qui le masquerait ; regarder d'abord le cache ARP
   de la VM (``ip neigh show <gateway>``) et l'état de la session côté VM.

Les VM sont accessibles depuis le netns de leur VPC, sur l'hyperviseur, avec l'utilisateur
``syonad`` créé par les métadonnées de two :

.. code-block:: text

   test/e2e/labctl.sh ssh "./labctl ssh hv1 'sudo ip netns exec vp-s4 ssh -i /root/.ssh/lab-vm syonad@10.240.1.10'"

Écrire un scénario
~~~~~~~~~~~~~~~~~~

Le lab sert à qualifier des comportements qui ne se voient qu'à plusieurs hyperviseurs — la
campagne L3VNI de `#41 <https://git.g3e.fr/syonad/two/issues/41>`_ en est le prochain exemple. Un
scénario est un fichier ``test/e2e/scenarios/<n>-<nom>.sh``, exécuté par ``run.sh`` sur le
Mac ; il envoie des blocs aux nœuds :

.. code-block:: bash

   on hv1 <<'NODE'
   two_image || { echo "ÉCHOUÉ: image compatible two"; exit 0; }
   KEY=$(vm_key)
   check "VPC vp-x" vpc_create vp-x 10.250.0.0/16
   check "subnet sn-x" subnet_create sn-x vp-x 2601 10.250.1.1 10.250.1.0/24
   check "VM x1" vm_create x1 sn-x 10.250.1.10 "${KEY}"
   check "x1 joignable" vm_wait vp-x 10.250.1.10
   check "x1 joint sa passerelle" vm_ssh vp-x 10.250.1.10 'ping -c 2 -W 2 10.250.1.1'
   check "x1 ne joint pas 10.251.1.10" vm_fails vp-x 10.250.1.10 'ping -c 2 -W 2 10.251.1.10'
   info "mesure : $(vtysh -c 'show evpn vni 2601' | grep -c 'flood')"
   NODE

Les règles qui ont fait leurs preuves en E5 :

* **une vérification par ligne**, avec ``check`` — jamais un ``echo RÉUSSI`` écrit à la main ;
* **toute vérification négative a son témoin** : avant « ne joint pas », une ligne qui prouve que
  la cible est vivante et que le chemin du test fonctionne ;
* **``vm_fails`` pour le négatif**, jamais ``!`` devant un ``vm_ssh`` : un SSH en panne doit
  échouer, pas passer pour une isolation ;
* **une donnée qui distingue réellement les cas** : two dérivant la MAC du rang de l'IP, deux
  subnets ont les mêmes MAC — comparer des couples MAC/IP, pas des MAC ;
* **les mesures en ``INFO``**, les attentes en ``check`` : un temps de reconvergence se mesure,
  il ne se décrète pas ;
* chaque scénario crée ses propres VPC, plages et VNI, distinctes de celles des autres, pour que
  ``all`` les enchaîne sur le même lab ; un scénario qui dépend d'un autre le vérifie en tête
  (``check "prérequis : …"``) ;
* variables vers un nœud : ``on hv1 NOM=valeur <<'NODE'`` (valeurs échappées par
  ``run.sh``) ;
* ``bash test/e2e/run_test.sh`` vérifie la syntaxe de chaque bloc réellement envoyé — à
  lancer avant toute session.

Sécurité
--------

Propre à two — celle de l'outil (clé d'API, clés SSH des VM, images) est décrite dans le guide de
``labctl`` :

* ``labctl`` et ``lab-host.sh`` sont vérifiés contre des sommes épinglées dans
  ``test/e2e/labctl.sh``, pas contre le ``SHA256SUMS`` téléchargé avec eux.
* **Code exécuté sans épinglage par** ``deploy.sh`` : la bibliothèque ``shflags`` est récupérée
  par ``curl`` sur la branche ``main`` d'un autre dépôt (``H6N/tools``) et exécutée par ``eval``,
  sans vérification d'intégrité — dans le lab comme en production. Les artefacts de la release
  sont, eux, vérifiés contre ``SHA256SUMS``.
* Le lab n'a aucune donnée personnelle ni secret de production.

Tests
-----

.. code-block:: bash

   bash test/e2e/run_test.sh

Sans réseau : vérifie la syntaxe de chaque bloc réellement envoyé aux nœuds par les scénarios et
les fonctions de ``test/e2e/lib/node.sh``. Les tests de l'outil sont dans le dépôt ``labctl``.
