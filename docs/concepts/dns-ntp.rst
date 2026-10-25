DNS et NTP des VM
=================

Les VM résolvent les noms et lisent l'heure par leur gateway : le DHCP annonce l'``interface_ip``
du subnet comme serveur DNS (option 6) et NTP (option 42), dans les deux backends. Un relais
unique par hyperviseur, ``two-hostsvc``, répond derrière cette adresse dans le netns de chaque
VPC. Une VM sans sortie Internet garde ainsi DNS et NTP, et aucune route n'est ouverte entre la
VRF du VPC et l'hôte.

Chemin d'une requête
--------------------

.. mermaid::

   graph LR
     V["VM<br/>vers interface_ip:53 ou :123"] --> N["netns du VPC<br/>chaîne TWO-HOSTSVC<br/>DNAT vers 169.254.169.253"]
     N --> H["two-hostsvc<br/>socket dans le netns<br/>169.254.169.253 sur la dummy hostsvc"]
     H -->|"depuis l'hôte"| U["serveurs amont<br/>puis résolveur ou horloge de l'hôte"]

À la création d'un VPC sur l'hyperviseur, l'agent pose dans son netns :

* une interface ``dummy`` ``hostsvc`` portant ``169.254.169.253/32`` ;
* une chaîne iptables ``TWO-HOSTSVC`` (table ``nat``), appelée depuis ``PREROUTING`` pour le seul
  trafic à destination d'une adresse locale du netns (``addrtype LOCAL`` : les gateways des
  subnets, anycast compris). Elle renvoie d'abord ce qui arrive par ``br-public`` (le segment
  public de l'hôte) et par ``l3-+`` (le lien vers la VRF), puis redirige 53 en UDP et TCP et 123
  en UDP vers ``169.254.169.253`` ;

puis demande à ``two-hostsvc`` d'ouvrir ses sockets dans ce netns. La réponse repart par
conntrack, avec l'adresse de la gateway pour source. Une VM qui interroge un résolveur extérieur
(par une passerelle NAT) n'est pas redirigée : seules les adresses locales du netns le sont.

À la suppression du VPC, l'agent demande la fermeture des sockets ; la ``dummy`` et la chaîne
disparaissent avec le netns.

**Si ``two-hostsvc`` ne répond pas, la création du VPC échoue** (``error``) plutôt que de créer
un VPC sans DNS ni NTP ; la suppression échoue de même si la fermeture échoue, et se relance
depuis ``error``. ``two-hostsvc`` doit donc tourner sur tout hyperviseur avant la création d'un
VPC.

Le relais
---------

``two-hostsvc`` est un seul processus par hyperviseur (``two-hostsvc.service``). Il entre dans le
netns de chaque VPC le temps d'y ouvrir trois sockets sur ``169.254.169.253`` (53/UDP, 53/TCP,
123/UDP), qui restent attachées à ce netns ; les requêtes vers l'amont partent de l'espace
réseau de l'hôte.

DNS
   La requête est transmise telle quelle aux serveurs de ``hostsvc.dns_upstream``, puis à ceux de
   ``/etc/resolv.conf`` de l'hôte. Seule une question standard est acceptée ; une réponse amont
   dont l'identifiant ne correspond pas est ignorée. Pas de cache : rien de ce qu'un locataire
   demande n'est gardé pour un autre.

NTP
   La requête est relayée aux serveurs de ``hostsvc.ntp_upstream`` ; une réponse dont
   l'horodatage d'origine ne correspond pas est ignorée. Sans serveur amont, ou sans réponse,
   ``two-hostsvc`` répond depuis l'horloge de l'hôte (SNTP, stratum 3) ; si l'hôte n'est pas
   synchronisé, la réponse porte l'indicateur d'alarme (LI = 3, stratum 16) et les clients
   l'ignorent.

Limite de débit
   ``hostsvc.rate_limit_per_vpc`` requêtes par seconde, DNS et NTP confondus, par VPC ; au-delà,
   les requêtes sont abandonnées. Au plus 16 connexions TCP simultanées par VPC.

Le relais garde la liste des VPC servis dans ``/run/two/hostsvc/hostsvc.state`` : à son
redémarrage, il rouvre seul les sockets des VPC dont le netns existe encore. Sa socket de
contrôle, ``/run/two/hostsvc/hostsvc.sock`` (mode 600), n'est utilisée que par l'agent.

Isolation
---------

* Le relais n'écoute que sur une adresse link-local, et seulement dans les netns des VPC ; il ne
  crée aucune route.
* Une réponse ne repart que vers l'adresse qui a posé la question, par la socket du VPC.
* Le trafic du segment public (``br-public``) et du lien vers la VRF n'atteint pas le relais.
* L'unité systemd ne garde que trois capacités : ``CAP_SYS_ADMIN`` (entrer dans un netns),
  ``CAP_NET_BIND_SERVICE`` (ports 53 et 123) et ``CAP_DAC_READ_SEARCH`` (les fichiers de
  ``/run/netns`` sont en mode 000). Le système de fichiers lui est en lecture seule, hors
  ``/run/two/hostsvc``.
* Les netns créés après le démarrage du service ne lui sont visibles que si ``/`` est en
  propagation ``shared``, le réglage par défaut de systemd.

Si ``two-hostsvc`` s'arrête, DNS et NTP s'arrêtent pour tous les VPC de l'hyperviseur ;
systemd le relance (``Restart=on-failure``) et le watchdog le signale.
