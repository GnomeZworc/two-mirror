#!/bin/bash
#
# Préparation d'un host au profil kvm : paquets, kernel, réseau de base.
#
# Ce fichier n'est pas exécutable seul : il est *sourcé* par deploy.sh quand
# --bootstrap est demandé, et réutilise ses helpers (run, info, warn, die,
# exec_with_dry_run, FLAGS_TRUE). Conséquences :
#   - aucun effet de bord au chargement : uniquement des définitions,
#   - aucune variable globale de deploy.sh redéfinie ici (SCRIPT_PATH, TAG,
#     BIN_PATH, ASSETS…) : le sourcing les écraserait,
#   - pas de `set -e` ni de garde `BASH_SOURCE` en fin de fichier.
#
# Contrat : tout bootstrap_<profil>.sh expose bootstrap_host() avec cette
# signature. deploy.sh n'a rien à savoir du contenu du profil.
#
# L'host est stateless (root sur tmpfs) : rien de ce qui suit ne survit à un
# reboot, deploy.sh --bootstrap est rejoué à chaque démarrage.

KVM_PACKAGES="qemu-system-x86 ovmf dnsmasq ebtables iptables nfs-common jq curl"

kvm_packages () {
    DRY_RUN="${1}"
    WITH_PACKAGES="${2}"

    [[ ${WITH_PACKAGES} -eq ${FLAGS_TRUE} ]] || { info "paquets : ignorés (--nopackages)"; return 0; }

    info "paquets"
    run "${DRY_RUN}" "DEBIAN_FRONTEND=noninteractive apt-get update"
    run "${DRY_RUN}" "DEBIAN_FRONTEND=noninteractive apt-get install -y ${KVM_PACKAGES}"

    # Le dnsmasq système prendrait le port 53 en concurrence des instances
    # dnsmasq@ lancées par l'agent dans les netns.
    run "${DRY_RUN}" "systemctl disable --now dnsmasq.service || true"
    run "${DRY_RUN}" "systemctl mask dnsmasq.service"
}

kvm_kernel () {
    DRY_RUN="${1}"

    info "kernel"

    # modprobe AVANT sysctl : les clés net.bridge.* n'existent pas tant que
    # br_netfilter n'est pas chargé, et sysctl --system échouerait.
    run "${DRY_RUN}" "modprobe br_netfilter"
    run "${DRY_RUN}" "echo br_netfilter > /etc/modules-load.d/two.conf"

    # bridge-nf-call-iptables est requis par le DNAT metadata (169.254.169.254,
    # cf. internal/iptables) : sans lui, iptables ne voit pas le trafic bridgé
    # des VMs. Contrepartie : tout le trafic inter-VM traverse les tables nat.
    run "${DRY_RUN}" "printf 'net.ipv4.ip_forward = 1\nnet.bridge.bridge-nf-call-iptables = 1\n' > /etc/sysctl.d/90-two.conf"
    run "${DRY_RUN}" "sysctl --system >/dev/null"
}

# Bridge vide, up, sans STP.
kvm_ensure_bridge () {
    DRY_RUN="${1}"
    NAME="${2}"

    if ! ip link show dev "${NAME}" >/dev/null 2>&1
    then
        run "${DRY_RUN}" "ip link add name '${NAME}' type bridge"
    fi
    run "${DRY_RUN}" "ip link set dev '${NAME}' type bridge stp_state 0"
    run "${DRY_RUN}" "ip link set up dev '${NAME}'"
}

kvm_network () {
    DRY_RUN="${1}"
    WITH_NETWORK="${2}"
    UPLINK="${3}"
    BRIDGE="${4}"
    PUBLIC_BRIDGE="${5}"
    ROLLBACK_DELAY="${6}"

    [[ ${WITH_NETWORK} -eq ${FLAGS_TRUE} ]] || { info "réseau : ignoré (--nonetwork)"; return 0; }

    info "réseau"

    # Bridge public : créé à vide, réservé pour un usage futur. Aucune adresse,
    # aucun esclave, pas référencé dans la config agent.
    kvm_ensure_bridge "${DRY_RUN}" "${PUBLIC_BRIDGE}"

    MASTER=""
    [[ ${DRY_RUN} -ne ${FLAGS_TRUE} ]] && MASTER=$(ip -j link show dev "${UPLINK}" | jq -r '.[0].master // ""')
    if [[ "${MASTER}" == "${BRIDGE}" ]]
    then
        info "  ${UPLINK} déjà esclave de ${BRIDGE} — migration ignorée"
        kvm_ensure_bridge "${DRY_RUN}" "${BRIDGE}"
        return 0
    fi

    # IP, préfixe et gateway dérivés de l'uplink : rien en dur. Le filtre
    # inet/global évite de tomber sur une IPv6 ou une adresse de lien.
    if [[ ${DRY_RUN} -eq ${FLAGS_TRUE} ]]
    then
        IP="<ip(${UPLINK})>"; PREFIX="<prefixlen>"; GW="<gateway>"
    else
        ADDR_JSON=$(ip -j addr show dev "${UPLINK}")
        IP=$(echo "${ADDR_JSON}"     | jq -r '[.[0].addr_info[] | select(.family=="inet" and .scope=="global")][0].local      // ""')
        PREFIX=$(echo "${ADDR_JSON}" | jq -r '[.[0].addr_info[] | select(.family=="inet" and .scope=="global")][0].prefixlen // ""')
        GW=$(ip -j route show default dev "${UPLINK}" | jq -r '.[0].gateway // ""')

        [[ -n "${IP}" && -n "${PREFIX}" ]] || die "aucune adresse IPv4 globale sur ${UPLINK}"
        [[ -n "${GW}" ]] || die "aucune route par défaut via ${UPLINK}"
    fi

    info "  ${UPLINK} : ${IP}/${PREFIX} gw ${GW} → ${BRIDGE}"

    # Cette étape coupe le réseau de l'host si elle échoue à mi-parcours, sans
    # console de secours. Deux garde-fous :
    #  - un rollback armé avant d'y toucher : rien n'étant écrit sur disque, un
    #    reboot ramène la configuration d'origine. Désarmé si le test passe.
    #  - la séquence tourne sous systemd et non dans la session SSH : une
    #    coupure de SSH ne l'interrompt plus à mi-chemin.
    run "${DRY_RUN}" "systemctl stop two-net-rollback.timer 2>/dev/null || true"
    run "${DRY_RUN}" "systemd-run --collect --unit=two-net-rollback --on-active=${ROLLBACK_DELAY} systemctl reboot"

    MIGRATE=$(cat <<EOF
set -e
ip link show dev '${BRIDGE}' >/dev/null 2>&1 || ip link add name '${BRIDGE}' type bridge
ip link set dev '${BRIDGE}' type bridge stp_state 0
ip link set up dev '${BRIDGE}'
ip link set '${UPLINK}' master '${BRIDGE}'
ip addr add '${IP}/${PREFIX}' dev '${BRIDGE}'
ip route replace default via '${GW}' dev '${BRIDGE}'
ip addr del '${IP}/${PREFIX}' dev '${UPLINK}'
pkill dhclient || true
EOF
)

    if [[ ${DRY_RUN} -eq ${FLAGS_TRUE} ]]
    then
        echo "# systemd-run --wait --collect --service-type=oneshot --unit=two-net-migrate /bin/bash <<'EOF'"
        echo "${MIGRATE}" | sed -e 's/^/# /'
        echo "# EOF"
    else
        MIGRATE_SCRIPT=$(mktemp /run/two-net-migrate.XXXXXX)
        printf '%s\n' "${MIGRATE}" > "${MIGRATE_SCRIPT}"
        systemctl reset-failed two-net-migrate.service 2>/dev/null || true
        systemd-run --wait --collect --service-type=oneshot --unit=two-net-migrate \
            /bin/bash "${MIGRATE_SCRIPT}" \
            || die "migration réseau échouée — rollback armé dans ${ROLLBACK_DELAY}s (reboot)"
        rm -f "${MIGRATE_SCRIPT}"

        # Vérification de connectivité avant de désarmer : seul critère qui
        # distingue un succès d'un host qu'on vient d'isoler.
        ping -c 2 -W 2 "${GW}" >/dev/null 2>&1 \
            || die "gateway ${GW} injoignable après migration — rollback armé dans ${ROLLBACK_DELAY}s (reboot)"
    fi

    run "${DRY_RUN}" "systemctl stop two-net-rollback.timer 2>/dev/null || true"
    info "  rollback désarmé"
}

# Point d'entrée appelé par deploy.sh --bootstrap.
bootstrap_host () {
    DRY_RUN="${1}"
    WITH_PACKAGES="${2}"
    WITH_NETWORK="${3}"
    UPLINK="${4}"
    BRIDGE="${5}"
    PUBLIC_BRIDGE="${6}"
    ROLLBACK_DELAY="${7}"

    kvm_packages "${DRY_RUN}" "${WITH_PACKAGES}"
    kvm_kernel   "${DRY_RUN}"
    kvm_network  "${DRY_RUN}" "${WITH_NETWORK}" "${UPLINK}" "${BRIDGE}" "${PUBLIC_BRIDGE}" "${ROLLBACK_DELAY}"
}
