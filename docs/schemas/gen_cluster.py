import sys

import gen_ensemble as g

W, H = 1200, 900


def made_style(p):
    return (f"    .made {{ stroke: {p['made']}; stroke-width: 1.5; fill: none; stroke-dasharray: 2 3; }}\n"
            f"    .head-made {{ fill: {p['made']}; }}\n")


def hv(x0, name):
    g.group(x0, 250, 540, 450, name)
    g.box(x0 + 20, 280, 180, 52, "br-000000", ["IP de l'hyperviseur = VTEP"])
    g.box(x0 + 20, 350, 180, 62, "FRR", ["bgpd · AS 64600", "advertise-all-vni"])
    g.box(x0 + 20, 430, 180, 52, "agent", ["API REST · Badger"])
    g.box(x0 + 20, 510, 180, 66, "br-<subnet> de l'hôte", ["vxlan-<vni>, sans learning", "veth v-<id>-e"])
    g.text(x0 + 110, 596, "veth v-<id>-e ↔ v-<id>-i", "sub", "middle")

    g.group(x0 + 230, 280, 290, 300, "NETNS DU VPC")
    g.box(x0 + 245, 312, 125, 66, "gw-<subnet>", ["gateway .254", "MAC anycast"])
    g.box(x0 + 380, 312, 125, 66, "DHCP", ["métadonnées", "écoutent sur gw-"])
    g.box(x0 + 245, 400, 260, 52, "br-<subnet> du netns", ["adresse d'émission .253 · v-<id>-i"])
    g.box(x0 + 245, 478, 260, 52, "VM", ["QEMU / KVM · tap dans le netns"])
    g.text(x0 + 375, 552, "macvlan gw- posé sur br-, confiné à l'hôte (#57)", "sub", "middle")
    g.box(x0 + 230, 610, 290, 62, "vrf-<vpc> · L3VNI", ["brl3 · vxl3 · veth de transit", "routage entre hyperviseurs (#41)"])

    g.path(f"M{x0 + 307} 378 V400", "link")
    g.path(f"M{x0 + 200} 538 H{x0 + 215} V430 H{x0 + 245}", "link")
    g.path(f"M{x0 + 375} 452 V478", "link")
    g.text(x0 + 383, 469, "tap", "sub")
    g.path(f"M{x0 + 375} 580 V610", "link")
    g.text(x0 + 383, 600, "transit", "sub")
    g.path(f"M{x0 + 20} 538 H{x0 + 10} V306 H{x0 + 20}", "made", end=True)
    g.path(f"M{x0 + 200} 456 H{x0 + 230}", "made", end=True)
    g.path(f"M{x0 + 185} 280 V200", "link")
    g.path(f"M{x0 + 200} 365 H{x0 + 208} V200", "ctrl")


def draw():
    g.group(40, 20, 440, 140, "ROUTEURS DU CLUSTER · AS 65100")
    g.box(60, 52, 190, 56, "routeur 1", ["169.254.0.1/28"])
    g.box(270, 52, 190, 56, "routeur 2", ["169.254.0.2/28"])
    g.text(260, 132, "passerelle de l'underlay en VRRP", "sub", "middle")
    g.text(260, 148, "annoncent les loopbacks des RR, ne leur renvoient rien", "sub", "middle")

    g.group(620, 20, 540, 140, "ROUTE REFLECTORS · AS 65000")
    g.box(640, 52, 240, 56, "route reflector 1", ["lo 10.255.255.1/32 · 169.254.0.3/28"])
    g.box(900, 52, 240, 56, "route reflector 2", ["lo 10.255.255.2/32 · 169.254.0.4/28"])
    g.text(890, 132, "router-id, cluster-id et source des sessions EVPN : la loopback", "sub", "middle")
    g.text(890, 148, "au moins un, deux prévus · AS 64600 face aux hyperviseurs (local-as)", "sub", "middle")

    g.path("M480 80 H640", "ctrl", start=True, end=True)
    g.text(560, 72, "eBGP · BFD", "ctrlt", "middle")
    g.text(560, 98, "loopbacks seules", "ctrlt", "middle")

    g.out.append('  <line class="bus" x1="40" y1="200" x2="1160" y2="200"/>')
    g.text(600, 192, "segment underlay · L2 · lien routeurs ↔ RR en 169.254.0.0/28", "sub", "middle")
    g.path("M260 160 V200", "link")
    g.path("M890 160 V200", "link")

    g.path("M200 236 H780", "data")
    g.text(490, 230, "VXLAN · UDP 4789 · de VTEP à VTEP", "datat", "middle")
    g.path("M200 236 V280", "data")
    g.path("M780 236 V280", "data")

    hv(40, "HYPERVISEUR 1")
    hv(620, "HYPERVISEUR 2")
    g.text(1150, 230, "iBGP l2vpn evpn · AS 64600 (FRR → RR)", "ctrlt", "end")

    ly = 736
    g.out.append(f'  <line class="link" x1="60" y1="{ly}" x2="110" y2="{ly}"/>')
    g.text(118, ly + 4, "lien L2 / interface")
    g.path(f"M260 {ly} H310", "data")
    g.text(318, ly + 4, "plan de données : tunnels VXLAN")
    g.path(f"M520 {ly} H570", "ctrl")
    g.text(578, ly + 4, "plan de contrôle : sessions BGP")
    g.path(f"M790 {ly} H840", "made", end=True)
    g.text(848, ly + 4, "créé par l'agent")

    notes = [
        "Les hyperviseurs joignent les loopbacks des route reflectors par leur passerelle : les routeurs les ont apprises en eBGP, sans rien leur renvoyer.",
        "Les route reflectors acceptent toute session venant de l'underlay (bgp listen range) et réfléchissent les routes EVPN entre hyperviseurs ; chaque FRR en peuple la FDB des VXLAN locaux.",
        "La gateway d'un subnet a la même IP et la même MAC sur tous les hôtes ; chaque netns émet depuis son adresse d'émission. Les subnets d'un VPC se joignent par son L3VNI.",
        "ASN et loopbacks : valeurs de production (#50). Vue d'ensemble (bordures, Internet, orchestration, plusieurs clusters) : schéma précédent.",
    ]
    for k, n in enumerate(notes):
        g.text(60, 780 + k * 18, n)


def render(p):
    g.out.clear()
    draw()
    head = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="Détail d\'un cluster : routeurs, route reflectors, hyperviseurs, netns et VRF des VPC, plans de données et de contrôle">'
    st = g.style(p).replace("  </style>", made_style(p) + "  </style>").replace(
        "  </defs>",
        '    <marker id="a-made" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path class="head-made" d="M0 0 L10 5 L0 10 z"/></marker>\n  </defs>')
    return "\n".join([head, st] + g.out + ["</svg>", ""])


if __name__ == "__main__":
    d = sys.argv[1]
    open(f"{d}/architecture-cluster.svg", "w").write(render(g.LIGHT))
    open(f"{d}/architecture-cluster-dark.svg", "w").write(render(g.DARK))
