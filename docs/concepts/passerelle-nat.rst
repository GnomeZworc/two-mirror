Passerelle NAT
==============

Une VM sans IP publique sort vers Internet par la **passerelle NAT de son VPC**. C'est une
ressource de l'API de l'agent (``/nat-gateways``), créée sur un seul hyperviseur, choisi par
l'appelant. Elle porte l'IP de sortie du VPC sur le segment public de l'hôte (``br-public``,
``vxlan-public``) et annonce une route par défaut de type 5 dans la VRF du VPC : toutes les VM du
VPC, sur tous les hyperviseurs, sortent par elle, avec cette IP pour source.

**L'autorisation Internet d'un VPC, c'est l'existence de sa passerelle** : le VPC ne porte aucun
champ pour cela. Le VPC doit avoir un ``l3vni``, et une seule passerelle existe par VPC. L'IP de
sortie, son préfixe et la gateway des bordures sont fournis par l'appelant ; l'agent ne les
vérifie pas.

Chemin d'un paquet
------------------

.. mermaid::

   graph LR
     V["VM sur l'hôte B"] --> NB["netns du VPC, hôte B<br/>route par défaut vers la VRF"]
     NB --> RB["vrf-&lt;id&gt;, hôte B<br/>route par défaut de type 5"]
     RB -->|"L3VNI"| RA["vrf-&lt;id&gt;, hôte A<br/>route par défaut vers 169.254.0.2"]
     RA --> NA["netns du VPC, hôte A<br/>SNAT vers l'IP de sortie"]
     NA -->|"ng-&lt;id&gt;, br-public, vxlan-public"| B["bordures"]

Le netns de chaque VPC routé envoie sa route par défaut vers la VRF. Sans passerelle, la route
``unreachable default`` de la VRF rejette ce trafic (voir :doc:`vpc-subnet-vm`) ; avec une
passerelle, la route par défaut de type 5 l'emporte, l'``unreachable`` ayant la métrique maximale.
Créer ou déplacer une passerelle ne demande donc rien aux autres hyperviseurs du VPC. Sur l'hôte
de la passerelle, les VM sortent directement par le netns, sans passer par la VRF.

Les réponses reviennent à l'IP de sortie : les bordures la résolvent vers la MAC de la
passerelle, le netns défait la traduction et renvoie le paquet vers la VM par la VRF.

Ce que l'agent pose
-------------------

Dans le netns du VPC ``vp-<id>``, sur l'hôte de la passerelle :

* un ``macvlan`` ``ng-<id>`` sur le bridge ``br-public`` du netns, déjà relié au bridge public de
  l'hôte par ``vp-<id>-i`` / ``vp-<id>-e``. Sa MAC est **calculée** à partir du ``l3vni`` :
  ``02:04:00`` suivi du ``l3vni`` sur 24 bits (``4600`` → ``02:04:00:00:11:f8``). Elle est la
  même quel que soit l'hôte : c'est ce qui permet de déplacer la passerelle ;
* une entrée FDB locale de cette MAC sur ``br-public`` : un ``macvlan`` n'en crée pas dans son
  bridge parent, et sans elle les trames vers la passerelle seraient inondées au lieu de lui être
  remises (même mécanisme que la MAC anycast des subnets ``vxlan``). Elle est posée par
  remplacement : après une bascule, le bridge a appris cette MAC sur le segment, vers l'ancien
  hôte ;
* ``arp_ignore=1``, ``arp_announce=2`` et ``arp_notify=1`` sur ``ng-<id>`` : la passerelle ne
  répond à l'ARP que pour son IP de sortie, et annonce celle-ci par un ARP gratuit à sa mise en
  route ;
* l'IP de sortie sur ``ng-<id>``, puis une route par défaut vers la gateway des bordures par
  ``ng-<id>``, à la place de celle vers la VRF ; le ``cidr`` du VPC reste routé vers la VRF ;
* trois chaînes dans la table nftables ``ip two`` du netns :

  * ``snat-ng-<id>`` : ``snat`` vers l'IP de sortie de tout ce qui sort par ``ng-<id>`` ;
  * ``fwd-ng-<id>`` et ``in-ng-<id>`` : rien n'entre par ``ng-<id>`` — ni vers les VM, ni vers le
    netns lui-même (DNS et NTP, métadonnées) — hors réponse à une connexion établie
    (``ct state established,related``). L'ARP n'est pas filtré ;

* en dernier, sur l'hôte, une route par défaut vers ``169.254.0.2`` dans la table de la VRF :
  l'annonce n'est jamais faite avant que la traduction soit en place.

La suppression retire d'abord la route par défaut de la VRF, puis les règles nftables, remet la
route par défaut du netns vers la VRF, retire l'entrée FDB et le ``macvlan``. Chaque étape tolère
ce qui est déjà absent : une suppression interrompue se relance depuis ``error``. Le retrait de la
route de la VRF désigne sa gateway et son interface, de sorte que l'``unreachable default`` reste
en place.

Annonce de la route par défaut
------------------------------

Le route-map ``EXPORT-<id>`` de la VRF porte une entrée ``permit 15`` : ``0.0.0.0/0``, à
condition que son next-hop soit ``169.254.0.2`` (prefix-lists ``PL-<id>-default`` et
``PL-<id>-transit``). Sans cette condition, ``redistribute kernel`` exporterait aussi
l'``unreachable default`` de chaque hôte (constaté sur FRR 8.4, hors VRF), et tous les
hyperviseurs annonceraient une sortie qu'ils n'ont pas.

Ces entrées sont posées à la création du VPC et retirées à sa suppression ; seule la route du
noyau dépend de la passerelle.

Déplacer la passerelle
----------------------

L'agent ne détecte aucune panne et ne coordonne rien avec les autres hyperviseurs. Pour déplacer
la passerelle, l'appelant la supprime sur l'ancien hôte s'il répond, puis la crée sur le nouveau
avec la même IP. La MAC étant la même, la bascule se réduit à un ARP gratuit et à la mobilité MAC
d'EVPN. Les connexions en cours sont perdues.

Si l'hôte de la passerelle tombe, ses routes restent annoncées jusqu'à l'expiration des sessions
BGP vers les route reflectors (180 s), et le VPC n'a plus de sortie jusqu'à la création d'une
nouvelle passerelle.

Une VM qui a une :doc:`IP flottante <ip-flottante>` ne sort pas par la passerelle : elle sort par
son IP flottante, y compris sur l'hôte de la passerelle.

Limites
-------

* Seuls les VPC créés par la 0.3.0 ou après envoient leur route par défaut vers la VRF et
  exportent la route de la passerelle : un VPC plus ancien doit être recréé pour s'en servir.
* IPv4 seulement.
