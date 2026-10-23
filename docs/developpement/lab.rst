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
et ``evpn-3hv.yml`` (trois, celle de la suite). Le format est celui de ``labctl`` :

.. literalinclude:: ../../test/e2e/topologies/evpn-2hv.yml
   :language: yaml

Les ASN, la loopback du route reflector, le lien ``169.254.0.0/28`` et le subnet des hyperviseurs
de l'exemple sont **ceux de la production** (décision du 2026-10-04, #50) : les fichiers de
``test/e2e/topologies/`` restent ainsi au plus près de ce qui tourne réellement. Toutes les adresses y sont
**fixées** par ``addresses`` — les switchs en ``.2`` et ``.3``, les route reflectors en ``.4`` et ``.5``,
les hyperviseurs à partir de ``.11`` — pour que le modèle se lise sans le plan et ne dépende pas
de l'ordre de déclaration : le ``frr.conf`` d'un hyperviseur, écrit à la main, porte son adresse en
``router-id``. Les deux switchs ``sw1`` et ``sw2`` sont les routeurs du segment (rôle ``router`` de
labctl) : la passerelle, la première adresse du segment, est portée en VRRP v3 par celui qui est
maître, sans préemption — voir la section du guide de labctl sur les routeurs. Leurs
``frr.conf`` ne diffèrent que par ``hostname`` et ``router-id`` ; labctl y ajoute la strophe VRRP.

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
``agent/dnsmasq.yml`` met explicitement les autres sur dnsmasq (``dhcp.backend: dnsmasq``).

**Segment ``lan`` (mode ``bridge``).** hv1 et hv2 sont aussi reliés à un second segment,
``lan`` (``10.33.0.0/24``, MTU 1500, **sans routeur** : L2 pur, sans passerelle ni NAT),
déclaré après ``underlay`` pour ne changer ni ses adresses ni ses taps. Avant ``deploy.sh``,
``hypervisor.sh`` crée le bridge ``br-lan`` (STP coupé, MTU de l'interface), y rattache
l'interface ``lan`` et y déplace l'adresse que labctl lui a donnée (``10.33.0.11`` sur hv1,
``10.33.0.12`` sur hv2). Les deux fichiers ``agent/`` déclarent ``interfaces: {lan: br-lan}`` :
un subnet créé avec ``mode: bridge`` et ``iface_type: lan`` rattache son veth à ``br-lan``. Les
subnets ``vxlan`` (``iface_type: vms``, clé absente) restent sur ``default_interface`` : le VTEP
ne change pas. Le bridge n'est créé qu'au premier démarrage, comme le reste du provisionnement.

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

   **Les switchs du lab sont des routeurs Linux avec FRR, par choix.** Ils jouent le rôle générique
   de routeur de cluster : passerelle des hyperviseurs, sessions eBGP avec BFD vers les route
   reflectors, dont ils n'acceptent que les loopbacks (``test/e2e/topologies/frr/sw1.conf``). L'équipement réel dépend de qui
   déploie l'infrastructure (MikroTik aujourd'hui ; Cisco, Juniper, Arista… demain) : il n'a besoin
   que de BGP et d'EVPN, et sa configuration propre au constructeur n'a pas sa place dans le lab.

Suite de tests
--------------

La suite (`#70 <https://git.g3e.fr/syonad/two/issues/70>`_) vit dans ``test/e2e/suite/`` : **un
fichier par fonctionnalité**, rangés en couches de bas en haut. Chaque couche ne s'appuie que sur
les précédentes, et les tests de panne passent en dernier. Elle remplace les scénarios ``s1`` à
``s11``, écrits au fil des tickets : chacune de leurs 251 vérifications est reprise (tableau de
correspondance sur #70).

.. list-table::
   :header-rows: 1
   :widths: 18 82

   * - Couche
     - Contenu
   * - 00 socle
     - lab, prérequis d'environnement, image des VM
   * - 10 API
     - refus de l'API, machine d'états
   * - 20 VM
     - DHCP (backend ``two`` et dnsmasq), métadonnées, UEFI, multi-NIC, multi-disque, redémarrage
   * - 30 réseau local
     - gateway anycast, VXLAN sur un hôte, mode ``bridge``, isolation, filtrage au port
   * - 40 EVPN L2
     - subnet étendu entre hôtes, isolation, subnets partagés, fenêtre ARP
   * - 50 L3VNI
     - routage entre subnets, migration, watchdog L3
   * - 60 cycle de vie
     - redémarrage de l'agent, nettoyage global
   * - 70 résilience
     - perte d'un route reflector, perte d'un switch

Les tests se lancent **depuis le Mac**, sur un lab démarré (``up``, ``push``, ``./labctl up``) :

.. code-block:: text

   test/e2e/run.sh all              # toute la suite, dans l'ordre
   test/e2e/run.sh 40               # une couche
   test/e2e/run.sh 41 43            # des tests, par numéro ou par nom
   test/e2e/run.sh --from 30        # à partir d'un numéro
   test/e2e/run.sh --continue all   # sans s'arrêter sur une couche en échec
   test/e2e/run.sh --connus all     # les échecs connus comptent comme des échecs
   test/e2e/run.sh --matrice        # matrice de couverture, tirée des en-têtes

Chaque test affiche une ligne ``RÉUSSI`` ou ``ÉCHOUÉ`` par vérification (un échec porte la
dernière ligne de la commande en cause), des lignes ``INFO`` pour les mesures, puis son bilan. Le
code de sortie vaut 1 si une vérification échoue **ou si aucune n'a été faite**. Avec ``all`` ou
``--from``, une couche en échec arrête la suite à la fin de la couche.

**Tests autonomes.** Avant chaque test, le runner supprime les ressources ``tNN`` laissées par un
passage précédent, puis relève l'état de chaque hôte de l'en-tête ``# hôtes:``. Après le test, il
supprime ses VM, ses subnets puis ses VPC, et vérifie que l'hôte est revenu à l'état relevé
(``assert_clean`` : netns, liens, adresses, routes, règles ebtables et iptables, configuration FRR et
``frr.conf``, fichiers de ``/run/two``, ``dnsmasq.d`` et des volumes, unités ``two-vm-*``,
processus dnsmasq et QEMU). Aucun test ne dépend d'un autre : n'importe lequel se lance seul.

**Plages.** Le test ``NN`` nomme ses ressources ``vp-tNN*``, ``sn-tNN*`` et ``tNN-*``, et utilise
``10.NN.0.0/16`` et les VNI ``NN00`` à ``NN99`` : aucune collision entre tests.

**Échecs connus.** Un contrôle dont l'échec est connu et rattaché à un ticket s'écrit ``known
"#NN" "description" commande…`` au lieu de ``check``. En échec, il sort en ``CONNU`` sans faire
échouer le test (bilan : ``N connu(s)``). Dès qu'il passe, il sort en ``INATTENDU`` et fait
échouer le test : le marqueur est à retirer. Pour un échec **intermittent**, ``known_flaky`` sort
en ``RÉUSSI`` (annoté) quand l'échec ne se reproduit pas, sans rappel automatique : le retrait se
fait en fermant le ticket. Chaque ticket concerné liste ses marqueurs en commentaire.

Matrice de couverture
~~~~~~~~~~~~~~~~~~~~~

Générée par ``test/e2e/run.sh --matrice`` à partir des en-têtes ; à régénérer quand un test change.

.. list-table::
   :header-rows: 1

   * - Test
     - Fonctionnalité
     - Hôtes
     - Tickets
   * - ``01-lab``
     - socle du lab — underlay, passerelle VRRP, sessions EVPN, base BGP posée par l'agent, backend DHCP, watchdog silencieux
     - hv1 hv2 hv3
     - #50, #51, #54, #41
   * - ``02-prerequis``
     - hypothèses d'environnement de la suite — firmware UEFI, unités modèles de l'agent, journalctl, format de ebtables -L, br-lan, priorités VRRP lues dans FRR
     - hv1 hv2 hv3
     - #70
   * - ``03-image``
     - hypothèses sur l'image de VM — outils (curl, python3), nom d'interface, effet réel de network-config, forme du compte verrouillé
     - hv1
     - #70
   * - ``11-validation``
     - validation de l'API — refus synchrones (400, 409, 422) des VPC, subnets et VM, sans trace en base
     - hv1
     - #41, #57, #33, #62
   * - ``12-etats``
     - machine d'états — 409 pendant creating, suppression depuis error, dépendances VM → subnet → VPC, recréation sous le même nom
     - hv1
     - #41
   * - ``21-dhcp-two``
     - DHCP par le serveur intégré (backend two) — bail, server-id, renouvellement, option 121, un serveur par subnet, démarrages simultanés
     - hv1
     - #46, #57, #62
   * - ``22-dhcp-dnsmasq``
     - DHCP par dnsmasq (backend dnsmasq) — bail, server-id, renouvellement, option 121, une instance par subnet écoutant sur la gateway anycast
     - hv2
     - #46, #57
   * - ``23-metadonnees``
     - serveur de métadonnées et cloud-init — sshkey, password (hash), user_data servi tel quel, documents absents (templates), VM sans compte
     - hv1
     - #43, #46
   * - ``24-uefi``
     - démarrage UEFI (OVMF) — firmware et variables par VM, VM démarrée en UEFI, témoin BIOS
     - hv1
     - UEFI (sans ticket)
   * - ``25-multi-nic``
     - VM à plusieurs interfaces — une NIC par subnet, adresses posées, route par défaut sur la primaire seule
     - hv1
     - #33, #62, #73
   * - ``26-multi-disque``
     - VM à plusieurs disques — racine virtio (vda) et SCSI (xvdX), tailles et bus vus par la VM, liens ``/dev/xvdX`` et ``by-id`` par numéro de série, démarrage sur vda
     - hv1
     - #32, #74
   * - ``27-reboot``
     - redémarrage d'une VM depuis l'invité — même processus QEMU, même adresse, nouveau bail, user_data non rejoué
     - hv1
     - #46
   * - ``31-gateway``
     - gateway d'un subnet vxlan — gateway anycast et adresse d'émission calculées, default_route et gateway explicite, routes DHCP, macvlan gw-, FDB locale, sysctls ARP, règles de confinement, suppression et recréation
     - hv1
     - #57, #43
   * - ``32-vxlan-local``
     - routage entre deux subnets vxlan d'un même VPC sur un même hôte, sans et avec l3vni — ICMP, TCP, saut unique par la gateway
     - hv1
     - #57, #41
   * - ``33-bridge``
     - subnet en mode bridge rattaché au bridge br-lan de l'hôte — un hôte, puis le même subnet sur deux hôtes (une seule réponse ARP pour interface_ip, métadonnées servies par l'hôte de la VM)
     - hv1 hv2
     - #70
   * - ``34-isolation-locale``
     - isolation entre deux VPC sur un même hôte — ni ICMP ni TCP d'un VPC à l'autre
     - hv1
     - #50
   * - ``35-filtrage-port``
     - filtrage au port — en sortie vers le fabric (v-<id>-i) : ARP vers la gateway et l'adresse d'émission, DHCP émis depuis ces adresses, MAC anycast confinée dans les deux sens ; entre VM d'un même hôte : faux serveur DHCP et usurpation ARP de la gateway (comportement attendu, non filtré à ce jour)
     - hv1
     - #57
   * - ``41-evpn-l2``
     - subnet vxlan étendu entre deux hôtes par EVPN — VTEP local, VTEP distant et inondation appris, MAC des VM en type 2, trafic ICMP et TCP, MTU 1500 de bout en bout
     - hv1 hv2
     - #50, #51
   * - ``42-isolation-evpn``
     - isolation entre VPC à travers EVPN — deux VPC sur la même plage, la VM d'un VPC ne joint pas celle de l'autre sur un hôte distant
     - hv1 hv2
     - #50, #51
   * - ``43-subnets-partages``
     - subnets vxlan partagés par deux hôtes, deux VM par hôte, dans un VPC avec et un VPC sans l3vni — toutes les paires, gateway anycast vue par chaque VM et absente d'EVPN, aucune MAC en double
     - hv2 hv3
     - #57, #41
   * - ``44-fenetre-arp``
     - fenêtre ARP de la gateway anycast entre deux hôtes — relais L2 depuis l'adresse d'émission d'un hôte, gateway de la VM inchangée, métadonnées, renouvellement DHCP et TCP local sans perte pendant la fenêtre
     - hv1 hv2
     - #57
   * - ``51-l3vni``
     - routage inter-subnet entre hôtes par L3VNI (IRB symétrique) — VRF, RMAC, exports /24 et /32, RT-5 réfléchies, TTL, isolation par RT, démontage d'un VPC routé
     - hv1 hv2
     - #41
   * - ``52-migration``
     - migration d'une adresse de VM entre hôtes d'un VPC routé — /32 retiré en premier, /24 en filet puis en ECMP, séquence orchestrée, 40 sources, suppression du subnet refusée tant qu'une VM l'utilise, MAC anycast hors EVPN
     - hv1 hv2 hv3
     - #41, #57
   * - ``53-watchdog-l3``
     - watchdog d'un VPC routé — /32 orphelin signalé comme trou noir, /32 manquant signalé, aucune fausse alerte BGP ni L3VNI, trafic intact après les essais
     - hv1 hv3
     - #41
   * - ``61-redemarrage-agent``
     - redémarrage de l'agent avec des VM en marche — rien ne disparaît ni n'est recréé, trafic routé sans perte, DHCP et métadonnées servis, base BGP réappliquée à l'identique, ressources toujours gérées
     - hv1 hv2
     - #41, #46
   * - ``62-nettoyage-global``
     - nettoyage global en fin de suite — aucun hôte ne garde de ressource : API vide, ni netns, tap, VRF, unité, instance DHCP ou de métadonnées, processus QEMU, règle de port, volume ; FRR réduit à la base BGP
     - 
     - #70
   * - ``71-perte-rr``
     - perte d'un route reflector — l'autre garde la session EVPN, le VTEP distant reste connu, le trafic L2 entre hôtes passe sans perte mesurable, retour du RR arrêté
     - hv1 hv2
     - #54
   * - ``72-perte-switch``
     - perte d'un switch de la passerelle VRRP — bascule vers l'autre en moins de 5 s, sessions EVPN jamais coupées ; au retour, le plus prioritaire redevient maître, seconde bascule mesurée
     - 
     - #54, #69

Campagne du 2026-10-17
~~~~~~~~~~~~~~~~~~~~~~

Première campagne de la suite, ``evpn-3hv.yml``, labctl ``0.2.0``, binaires de l'arbre courant
(code de l'agent identique à ``main``). ``run.sh --continue all`` en 76 min, dont environ 15 min
d'arrêts de VM à 300 s et 15 min d'attentes sur des tests en échec. Les hypothèses fausses des
tests ont été corrigées sur le serveur et les tests concernés rejoués. Il reste :

* tout vert, sauf les échecs réels, marqués ``known`` et rattachés à leur ticket : multi-NIC
  (#73, #62), noms des disques SCSI (#74), mode ``bridge`` sur plusieurs hôtes (#75), filtrage au
  port (#76), socket DHCP (#77), fausse alerte du watchdog (#78) ;
* #69 vérifié : au retour de ``sw1`` (priorité 200), il redevient maître, seconde bascule en
  1,2 s, sessions EVPN jamais coupées ;
* une suppression de VM peut durer 300 s quand l'invité ignore l'arrêt ACPI (#79) : ``wait_state``
  attend jusqu'à 360 s.

Hypothèses vérifiées en lab, à garder en tête en écrivant un test :

* systemd-networkd **ne réécrit pas** un bail renouvelé à l'identique : un renouvellement se
  vérifie par le DHCPACK dans le journal du serveur (``lease_renewed``), pas par le fichier de bail ;
* l'agent n'accepte **qu'un seul disque virtio** (le disque racine, ``vda``) ; les autres sont des ``xvdX`` ;
* sur Debian 12, ``ebtables -L`` (nft) garde les zéros de tête des MAC ;
* ``interface_ip`` est posée en ``/32`` sur le bridge du netns en mode ``bridge`` ;
* depuis le netns d'un hôte, une VM d'un autre hôte ne répond pas : la réponse repart vers la
  gateway de son propre hôte. Une vérification entre hôtes passe par une VM.

Les VM sont accessibles depuis le netns de leur VPC, sur l'hyperviseur, avec l'utilisateur
``syonad`` créé par les métadonnées de two :

.. code-block:: text

   test/e2e/labctl.sh ssh "./labctl ssh hv1 'sudo ip netns exec vp-t41 ssh -i /root/.ssh/lab-vm syonad@10.41.1.10'"

Écrire un test
~~~~~~~~~~~~~~

Un test est un fichier ``test/e2e/suite/NN-nom.sh``, exécuté par ``run.sh`` sur le Mac. Il
commence par un en-tête, puis envoie des blocs aux nœuds par ``on <nœud> [VAR=valeur…] <<'NODE'``.
Chaque bloc est précédé des bibliothèques de ``test/e2e/lib/`` :

* ``core.sh`` : ``check``, ``known``, ``info``, appels à l'API, attente des états ;
* ``vm.sh`` : image Debian compatible two, clé SSH, ``vm_launch`` (plusieurs NIC et disques, UEFI,
  ``user_data``, mot de passe), SSH, bail, métadonnées. La MAC de chaque NIC est fournie à l'API :
  ``52:54:00`` suivi des trois derniers octets de l'IP (``mac_for_ip``), unique dans la suite
  puisque chaque test a sa plage ; ``--nic sn=ip=mac`` en impose une autre ;
* ``net.sh`` : VPC, subnets, MAC anycast, trames brutes, règles ebtables, VRRP ;
* ``evpn.sh`` : VRF, BGP, L3VNI, watchdog ;
* ``fixture.sh`` : nettoyage et ``assert_clean``, appelés par le runner.

.. code-block:: bash

   # fonctionnalité: trafic entre deux VM d'un même subnet sur un hôte
   # hôtes: hv1
   # tickets: #NN

   on hv1 <<'NODE'
   KEY=$(vm_key)
   check "VPC vp-t99" vpc_create vp-t99 10.99.0.0/16
   check "subnet sn-t99" subnet_create sn-t99 vp-t99 9901 10.99.1.0/24
   check "VM t99-a" vm_launch t99-a --nic sn-t99=10.99.1.10 --sshkey "${KEY}"
   check "VM t99-b" vm_launch t99-b --nic sn-t99=10.99.1.11 --sshkey "${KEY}"
   check "t99-a joignable" vm_wait vp-t99 10.99.1.10
   check "t99-a joint t99-b" vm_ssh vp-t99 10.99.1.10 'ping -c 2 -W 2 10.99.1.11'
   check "t99-a ne joint pas 10.98.1.10" vm_fails vp-t99 10.99.1.10 'ping -c 2 -W 2 10.98.1.10'
   NODE

Les règles :

* **une vérification par ligne**, avec ``check`` : jamais un ``echo RÉUSSI`` écrit à la main ;
* **toute vérification négative a son témoin** : avant « ne joint pas », une ligne qui prouve que
  la cible est vivante et que le chemin du test fonctionne ;
* **``vm_fails`` pour le négatif**, jamais ``!`` devant un ``vm_ssh`` : un SSH en panne doit
  échouer, pas passer pour une isolation ;
* **une donnée qui distingue réellement les cas** : rien n'empêche deux subnets de porter les
  mêmes MAC (c'était le cas quand two les dérivait du rang de l'IP) ; comparer des couples MAC/IP,
  pas des MAC ;
* **les mesures en ``INFO``**, les attentes en ``check`` : un temps de reconvergence se mesure,
  il ne se décrète pas ;
* **l'en-tête ``# hôtes:`` liste exactement les hôtes où le test crée des ressources** : ce sont
  ceux que le runner nettoie et compare ;
* **un échec connu se marque** ``known`` (ou ``known_flaky``) avec son ticket, et le ticket liste
  le marqueur : jamais un contrôle retiré ou affaibli pour faire passer la suite ;
* variables vers un nœud : ``on hv1 NOM=valeur <<'NODE'`` (valeurs échappées par ``run.sh``) ;
* ``bash test/e2e/run_test.sh`` vérifie le runner, les bibliothèques et la syntaxe de chaque bloc
  réellement envoyé : à lancer avant toute session.

Campagnes précédentes
~~~~~~~~~~~~~~~~~~~~~

Résultats obtenus avec les anciens scénarios ``s1`` à ``s11``, conservés pour leurs constats.
Leurs vérifications sont reprises dans la suite.

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
dérivait alors la MAC du rang de l'IP, et les VM ``.10``/``.11`` des deux subnets avaient les mêmes MAC.
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
* **La route du supernet n'existe pas dans le netns** du VPC avant la pose manuelle (premier
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
  interface, L3VNI en double et subnet utilisé refusés (409), démontage complet d'un VPC sans
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

Le 2026-10-13, labctl ``0.2.0``, release ``0.2.0rc008`` et binaires de l'arbre courant,
``evpn-3hv.yml`` avec deux switchs et deux route reflectors (#54) : ``s1`` à ``s10`` **313
réussi(s), 0 échoué(s)** ; lab monté en 8 min. Segments sur un bridge de l'hôte, sept taps ;
``sw2`` maître VRRP au démarrage, ``sw1`` en attente avec sa passerelle coupée. ``s6`` : 0
requête perdue sur 510 pendant l'arrêt de rr1 puis de rr2, session rétablie 14 s après le
redémarrage de FRR. ``s10`` : ``sw2`` coupé, ``sw1`` maître, 14 requêtes perdues, plus long
silence 2,8 s, sessions EVPN de hv1 jamais coupées ; ``sw2`` rétabli reste en attente.
``s9`` sans ``Broken pipe`` cette fois : #57 n'est pas corrigé pour autant.

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

Sans réseau : vérifie le runner, les fonctions de ``test/e2e/lib/`` et la syntaxe de chaque bloc
réellement envoyé aux nœuds par la suite. Les tests de l'outil sont dans le dépôt ``labctl``.
