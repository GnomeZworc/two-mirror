import sys

LIGHT = dict(text="#1f2328", sub="#57606a", box_f="#ffffff", box_s="#8c959f", grp_f="#f6f8fa", grp_s="#d0d7de",
             pill_s="#57606a", link="#57606a", ctrl="#8250df", data="#0969da", made="#8c959f",
             orch="#1a7f37", net="#bc4c00")
DARK = dict(text="#e6edf3", sub="#9198a1", box_f="#22272e", box_s="#6e7781", grp_f="#1c2128", grp_s="#444c56",
            pill_s="#9198a1", link="#9198a1", ctrl="#c297ff", data="#4493f8", made="#6e7781",
            orch="#3fb950", net="#f0883e")

FONT = '-apple-system, "Segoe UI", Roboto, sans-serif'
W, H = 1500, 1210


def style(p):
    return f"""  <style>
    .lbl {{ font-family: {FONT}; font-size: 14px; fill: {p['text']}; }}
    .sub {{ font-family: {FONT}; font-size: 11px; fill: {p['sub']}; }}
    .grp {{ font-family: {FONT}; font-size: 12px; font-weight: 600; fill: {p['sub']}; letter-spacing: .04em; }}
    .ctrlt {{ font-family: {FONT}; font-size: 11px; fill: {p['ctrl']}; }}
    .datat {{ font-family: {FONT}; font-size: 11px; font-weight: 600; fill: {p['data']}; }}
    .orcht {{ font-family: {FONT}; font-size: 11px; font-weight: 600; fill: {p['orch']}; }}
    .nett {{ font-family: {FONT}; font-size: 11px; font-weight: 600; fill: {p['net']}; }}
    .box {{ fill: {p['box_f']}; stroke: {p['box_s']}; stroke-width: 1.5; }}
    .future {{ fill: {p['box_f']}; stroke: {p['box_s']}; stroke-width: 1.5; stroke-dasharray: 6 4; }}
    .group {{ fill: {p['grp_f']}; stroke: {p['grp_s']}; stroke-width: 1.5; }}
    .pill {{ fill: {p['box_f']}; stroke: {p['pill_s']}; stroke-width: 1.5; }}
    .bus {{ stroke: {p['link']}; stroke-width: 4; fill: none; stroke-linecap: round; }}
    .link {{ stroke: {p['link']}; stroke-width: 1.8; fill: none; }}
    .ctrl {{ stroke: {p['ctrl']}; stroke-width: 1.6; fill: none; stroke-dasharray: 5 4; }}
    .data {{ stroke: {p['data']}; stroke-width: 3; fill: none; }}
    .orch {{ stroke: {p['orch']}; stroke-width: 2; fill: none; stroke-dasharray: 8 4; }}
    .net {{ stroke: {p['net']}; stroke-width: 2.4; fill: none; }}
    .head-ctrl {{ fill: {p['ctrl']}; }}
    .head-data {{ fill: {p['data']}; }}
    .head-orch {{ fill: {p['orch']}; }}
    .head-net {{ fill: {p['net']}; }}
  </style>
  <defs>
    <marker id="a-ctrl" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path class="head-ctrl" d="M0 0 L10 5 L0 10 z"/></marker>
    <marker id="a-data" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse"><path class="head-data" d="M0 0 L10 5 L0 10 z"/></marker>
    <marker id="a-orch" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path class="head-orch" d="M0 0 L10 5 L0 10 z"/></marker>
    <marker id="a-net" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path class="head-net" d="M0 0 L10 5 L0 10 z"/></marker>
  </defs>"""


def esc(t):
    return t.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


out = []


def group(x, y, w, h, title):
    out.append(f'  <rect class="group" x="{x}" y="{y}" width="{w}" height="{h}" rx="8"/>')
    out.append(f'  <text class="grp" x="{x + 20}" y="{y + 24}">{esc(title)}</text>')


def box(x, y, w, h, label, subs=(), cls="box"):
    out.append(f'  <rect class="{cls}" x="{x}" y="{y}" width="{w}" height="{h}" rx="6"/>')
    n = 1 + len(subs)
    top = y + h / 2 - (n - 1) * 8 + 5
    out.append(f'  <text class="lbl" x="{x + w / 2}" y="{top}" text-anchor="middle">{esc(label)}</text>')
    for i, s in enumerate(subs):
        out.append(f'  <text class="sub" x="{x + w / 2}" y="{top + 17 + i * 15}" text-anchor="middle">{esc(s)}</text>')


def text(x, y, t, cls="sub", anchor="start"):
    out.append(f'  <text class="{cls}" x="{x}" y="{y}" text-anchor="{anchor}">{esc(t)}</text>')


def path(d, cls, start=False, end=False):
    m = cls.split()[0]
    ms = f' marker-start="url(#a-{m})"' if start else ""
    me = f' marker-end="url(#a-{m})"' if end else ""
    out.append(f'  <path class="{cls}" d="{d}"{ms}{me}/>')


def draw():
    dy = 205

    out.append('  <rect class="pill" x="600" y="20" width="300" height="56" rx="28"/>')
    text(750, 54, "Internet", "lbl", "middle")
    path("M750 76 V110", "net", start=True, end=True)
    text(758, 98, "agrégats publics", "nett")
    group(560, 110, 380, 130, "BORDURES · AS PUBLIC · #31")
    box(580, 140, 160, 52, "bordure 1", ["gateway anycast"], "future")
    box(760, 140, 160, 52, "bordure 2", ["gateway anycast"], "future")
    text(750, 214, "même IP et même MAC de gateway · sans état", "sub", "middle")
    text(750, 230, "VTEP · VNI 66001 (public)", "sub", "middle")

    group(1180, 20, 280, 100, "POSTE DE L'OPÉRATEUR")
    box(1200, 50, 240, 52, "Terraform / OpenTofu", ["provider two · #72"], "future")
    path("M1180 64 H900", "net", end=True)
    text(1040, 56, "API publique de l'orchestrateur", "nett", "middle")

    group(40, 280, 480, 130, "CLUSTER A")
    box(60, 312, 210, 56, "routeur 1", ["169.254.0.1/28"])
    box(290, 312, 210, 56, "routeur 2", ["169.254.0.2/28"])
    text(280, 392, "VRRP · VTEP · VNI 66000 · NAT vers le VNI 66000 · #72", "sub", "middle")

    group(1200, 280, 260, 130, "CLUSTER B")
    box(1215, 312, 110, 56, "routeur 1", ["169.254.0.1/28"])
    box(1335, 312, 110, 56, "routeur 2", ["169.254.0.2/28"])
    text(1330, 388, "VRRP · VTEP · VNI 66000", "sub", "middle")
    text(1330, 403, "NAT vers le VNI 66000 · #72", "sub", "middle")
    out.append('  <line class="bus" x1="1200" y1="460" x2="1460" y2="460"/>')
    text(1205, 478, "underlay B · VXLAN UDP 4789 · BGP", "sub")
    path("M1230 368 V460", "link")
    path("M1430 368 V460", "link")
    box(1215, 505, 220, 52, "route reflector 2", ["lo 10.255.255.2/32 · 169.254.0.4/28"])
    path("M1440 460 V505", "link")
    text(1434, 498, "iBGP EVPN", "ctrlt", "end")
    group(1200, 395 + dy, 260, 410, "HYPERVISEURS B (×N)")
    for k in range(3):
        box(1220, 440 + dy + k * 84, 220, 64, f"hyperviseur B{k + 1}", ["agent · FRR · VPC", "VTEP : son IP sur l'underlay B"])
    text(1330, 720 + dy, "…", "lbl", "middle")
    text(1330, 750 + dy, "même modèle que", "sub", "middle")
    text(1330, 765 + dy, "l'hyperviseur détaillé", "sub", "middle")
    path(f"M1452 460 V{395 + dy}", "link")

    path("M500 340 H1215", "link")
    text(860, 332, "eBGP · routeur 2 de A → routeur 1 de B", "sub", "middle")
    path("M660 240 V258 H210 V312", "link")
    path("M840 240 V258 H1315 V312", "link")
    text(420, 252, "eBGP", "sub", "middle")
    text(1060, 252, "eBGP", "sub", "middle")

    out.append('  <line class="bus" x1="40" y1="460" x2="680" y2="460"/>')
    text(690, 465, "underlay A · VXLAN UDP 4789 · BGP", "sub")
    path("M75 368 V460", "link")
    path("M485 368 V460", "link")
    box(60, 490, 230, 52, "route reflector 1", ["lo 10.255.255.1/32 · 169.254.0.3/28"])
    path("M175 460 V490", "link")
    text(183, 482, "iBGP EVPN", "ctrlt")
    group(740, 490, 440, 92, "SERVICES · VNI 66000 PARTAGÉ ENTRE CLUSTERS · #72")
    box(760, 520, 190, 52, "broker", ["MQTT · VNI 66000"], "future")
    box(970, 520, 190, 52, "orchestrateur", ["VNI 66000 et VNI 66001"], "future")
    path("M950 546 H970", "orch")
    path("M510 410 V546 H760", "orch", end=True)
    path("M1200 400 H1192 V560 H1180", "orch", end=True)

    group(40, 395 + dy, 1120, 410, "HYPERVISEUR A (détaillé)")
    box(60, 430 + dy, 260, 52, "br-000000", ["IP de l'hyperviseur = VTEP"])
    box(60, 496 + dy, 260, 52, "FRR · bgpd", ["sessions EVPN vers les RR"])
    box(60, 562 + dy, 260, 66, "agent", ["API locale 127.0.0.1:8080", "client du broker, en sortie"])
    box(60, 644 + dy, 260, 56, "IP flottante · NAT 1:1", ["hôte de la VM, sans état · #31"], "future")
    box(60, 716 + dy, 260, 56, "subnet public de l'hôte", ["VNI 66001 · hors VPC · #31"], "future")

    group(360, 430 + dy, 780, 355, "VPC · netns + VRF")
    box(380, 470 + dy, 240, 60, "netns du VPC", ["gateway .254 · DHCP · métadonnées"])
    box(380, 565 + dy, 240, 60, "VRF · L3VNI", ["routage entre subnets et hyperviseurs"])
    box(660, 565 + dy, 200, 60, "passerelle NAT du VPC", ["sur un des hyperviseurs · #42"], "future")
    box(380, 690 + dy, 240, 60, "subnet public du VPC", ["relié au subnet public de l'hôte"], "future")
    box(880, 470 + dy, 240, 60, "VM privée", ["adresse du VPC"])
    box(880, 690 + dy, 240, 60, "VM publique", ["+ NIC sur le subnet public"], "future")

    path(f"M880 {500 + dy} H620", "link")
    path(f"M500 {530 + dy} V{565 + dy}", "link")
    path(f"M880 {720 + dy} H620", "net")
    path(f"M380 {748 + dy} H320", "net", end=True)
    path(f"M380 {605 + dy} H340 V{672 + dy} H320", "net", end=True)
    path(f"M620 {595 + dy} H660", "net", end=True)
    path(f"M760 {625 + dy} V{658 + dy} H350 V{724 + dy} H320", "net", end=True)
    text(640, 558 + dy, "défaut type 5", "nett", "middle")
    path(f"M190 {700 + dy} V{716 + dy}", "net", end=True)
    path(f"M60 {744 + dy} H50 V{456 + dy} H60", "data")
    out.append(f'  <text class="datat" x="30" y="{600 + dy}" text-anchor="middle" transform="rotate(-90 30 {600 + dy})">VNI 66001</text>')
    path(f"M300 {430 + dy} V460", "link")

    path(f"M320 {580 + dy} H332 V410", "orch", end=True)
    text(340, 596, "register · ordres · événements (mTLS, sortant)", "orcht")

    path("M750 240 V300 H1140 V520", "net", end=True)
    text(1132, 430, "API publique", "nett", "end")
    text(1132, 446, "VNI 66001", "nett", "end")

    ly = 840 + dy
    path(f"M60 {ly} H110", "data")
    text(118, ly + 4, "plan de données : VXLAN")
    path(f"M290 {ly} H340", "ctrl")
    text(348, ly + 4, "plan de contrôle : BGP")
    path(f"M510 {ly} H560", "orch", end=True)
    text(568, ly + 4, "orchestration")
    path(f"M690 {ly} H740", "net", end=True)
    text(748, ly + 4, "trafic Internet")
    out.append(f'  <rect class="future" x="880" y="{ly - 10}" width="34" height="20" rx="4"/>')
    text(922, ly + 4, "à venir (ticket indiqué)")

    notes = [
        "Les agents ne sont jamais joints par l'orchestrateur : ils se connectent au broker en sortie, via leur passerelle ; les routeurs de leur cluster les traduisent (NAT) vers le VNI 66000.",
        "L'orchestrateur a une patte dans le VNI 66001 : son API est joignable depuis Internet, par les bordures.",
        "VM publique : un subnet public dans son VPC, relié au subnet public de l'hôte (VNI 66001), dont la gateway anycast est portée par toutes les bordures.",
        "VM privée : route par défaut de type 5 vers la passerelle NAT de son VPC (IP de sortie du VPC) si le VPC autorise Internet, ou IP flottante 1:1 sur son hôte.",
        "Les bordures restent sans état : tout NAT vit sur un hyperviseur. Broker et orchestrateur sont des VM du VNI 66000, partagé : elles tournent sur les hyperviseurs de n'importe quel cluster.",
        "Au moins un route reflector ; deux prévus, ici un par cluster : un événement d'un cluster ne coupe pas les deux sessions. Chaque lien routeurs ↔ RR est en 169.254.0.0/28, propre à son cluster.",
    ]
    for k, n in enumerate(notes):
        text(60, 884 + dy + k * 18, n)


def render(p):
    out.clear()
    draw()
    head = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}" role="img" aria-label="Vue d\'ensemble : Internet, bordures, deux clusters, route reflectors, services partagés et hyperviseurs">'
    return "\n".join([head, style(p)] + out + ["</svg>", ""])


if __name__ == "__main__":
    d = sys.argv[1]
    open(f"{d}/architecture-ensemble.svg", "w").write(render(LIGHT))
    open(f"{d}/architecture-ensemble-dark.svg", "w").write(render(DARK))
