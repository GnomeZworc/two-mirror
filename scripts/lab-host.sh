#!/usr/bin/env bash

API_URL="${SCW_API_URL:-https://api.scaleway.com}"
CREDENTIALS_FILE="${LAB_CREDENTIALS_FILE:-${HOME}/.config/two-lab/scaleway.env}"
OFFER_NAME="${LAB_OFFER:-EM-B212X-SSD}"
OS_NAME="${LAB_OS_NAME:-Debian}"
OS_VERSION="${LAB_OS_VERSION:-12}"
LAB_NAME="${LAB_NAME:-two-lab}"
LAB_TAG="${LAB_TAG:-two-lab}"
INSTALL_TIMEOUT="${LAB_INSTALL_TIMEOUT:-3600}"
DELETE_TIMEOUT="${LAB_DELETE_TIMEOUT:-900}"
SSH_TIMEOUT="${LAB_SSH_TIMEOUT:-600}"
POLL_INTERVAL="${LAB_POLL_INTERVAL:-20}"
HTTP_CONNECT_TIMEOUT="${LAB_HTTP_CONNECT_TIMEOUT:-10}"
HTTP_TIMEOUT="${LAB_HTTP_TIMEOUT:-60}"
STATE_DIR="${LAB_STATE_DIR:-${HOME}/.cache/two-lab}"
SSH_KEY="${LAB_SSH_KEY:-${HOME}/.config/two-lab/ssh/lab_ed25519}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CREATED_ID=""

info () { echo "== ${1}" >&2; }
warn () { echo "!! ${1}" >&2; }
die  () { echo "!! ${1}" >&2; exit 1; }

usage () {
    cat >&2 <<EOF
usage: ${0##*/} <commande> [arguments]

  plan              résout l'offre horaire, l'OS et les clés SSH, affiche la requête de création
                    et le prix ; ne crée rien
  up                crée le serveur de lab, attend la fin de son installation et son SSH,
                    puis le prépare (voir prepare)
  status            liste les serveurs de lab du projet
  ssh [commande]    se connecte au serveur de lab ; avec une commande, un terminal n'est demandé
                    que si l'entrée standard en est un
  prepare           installe sur le serveur ce dont lab a besoin (qemu, genisoimage), vérifie
                    /dev/kvm et la virtualisation imbriquée ; lancé aussi par up
  push <topologie>  compile cmd/lab pour linux/amd64 et dépose sur le serveur ~/lab et le
                    répertoire de la topologie dans ~/topology/ (avec les fichiers qu'elle
                    référence) ; ensuite : ssh './lab up topology/<topologie>'
  down              supprime tous les serveurs de lab du projet et attend leur disparition
  session [cmd]     up, puis la commande distante (ou un shell), puis down quoi qu'il arrive

environnement : SCW_SECRET_KEY et SCW_DEFAULT_PROJECT_ID (requis, à défaut lus dans ${CREDENTIALS_FILE},
                fichier en 0600, lignes CLÉ=valeur), SCW_DEFAULT_ZONE (${ZONE}),
                LAB_OFFER (${OFFER_NAME}), LAB_OS_NAME (${OS_NAME}), LAB_OS_VERSION (${OS_VERSION}),
                LAB_SSH_KEY (${SSH_KEY} ; à défaut, l'agent SSH)

ne jamais lancer sous 'bash -x' : la trace afficherait SCW_SECRET_KEY.
EOF
    exit 2
}

load_credentials () {
    local KEY VALUE
    if [[ -z "${SCW_SECRET_KEY:-}" || -z "${SCW_DEFAULT_PROJECT_ID:-}" ]] && [[ -f "${CREDENTIALS_FILE}" ]]; then
        [[ $(ls -l "${CREDENTIALS_FILE}" | cut -c5-10) == "------" ]] \
            || die "${CREDENTIALS_FILE} est lisible par d'autres que son propriétaire : chmod 600"
        while IFS='=' read -r KEY VALUE || [[ -n "${KEY}" ]]; do
            case "${KEY}" in
                SCW_SECRET_KEY)         [[ -n "${SCW_SECRET_KEY:-}" ]] || SCW_SECRET_KEY="${VALUE}" ;;
                SCW_DEFAULT_PROJECT_ID) [[ -n "${SCW_DEFAULT_PROJECT_ID:-}" ]] || SCW_DEFAULT_PROJECT_ID="${VALUE}" ;;
                SCW_DEFAULT_ZONE)       [[ -n "${SCW_DEFAULT_ZONE:-}" ]] || SCW_DEFAULT_ZONE="${VALUE}" ;;
            esac
        done < "${CREDENTIALS_FILE}"
    fi
    ZONE="${SCW_DEFAULT_ZONE:-fr-par-1}"
    PROJECT_ID="${SCW_DEFAULT_PROJECT_ID:-}"
}

require_env () {
    [[ -n "${SCW_SECRET_KEY:-}" ]] || die "SCW_SECRET_KEY absent de l'environnement"
    [[ -n "${PROJECT_ID}" ]] || die "SCW_DEFAULT_PROJECT_ID absent de l'environnement"
    command -v jq >/dev/null 2>&1 || die "jq est requis"
    command -v curl >/dev/null 2>&1 || die "curl est requis"
}

http_code () {
    cat "${STATE_DIR}/last_http_code" 2>/dev/null
}

api () {
    local METHOD="${1}"
    local URL_PATH="${2}"
    local BODY="${3:-}"
    local OUT CODE
    local -a ARGS=(-sS -X "${METHOD}" -H "Content-Type: application/json" -w '\n%{http_code}'
                   --connect-timeout "${HTTP_CONNECT_TIMEOUT}" --max-time "${HTTP_TIMEOUT}")

    : > "${STATE_DIR}/last_http_code"
    [[ -n "${BODY}" ]] && ARGS+=(--data "${BODY}")
    OUT=$(curl "${ARGS[@]}" -H @<(printf 'X-Auth-Token: %s\n' "${SCW_SECRET_KEY}") "${API_URL}${URL_PATH}") \
        || { warn "${METHOD} ${URL_PATH} : échec réseau"; return 1; }
    CODE="${OUT##*$'\n'}"
    OUT="${OUT%$'\n'*}"
    printf '%s' "${CODE}" > "${STATE_DIR}/last_http_code"
    if [[ "${CODE}" -lt 200 || "${CODE}" -ge 300 ]]; then
        warn "${METHOD} ${URL_PATH} : HTTP ${CODE} ${OUT}"
        return 1
    fi
    printf '%s\n' "${OUT}"
}

http_failure_is_final () {
    local CODE
    CODE=$(http_code)
    [[ "${CODE}" =~ ^4[0-9][0-9]$ && "${CODE}" != "429" ]]
}

money () {
    jq -r "${1} | if . == null then \"?\" else \"\\(.units + .nanos / 1000000000) \\(.currency_code)\" end"
}

resolve_offer () {
    local JSON OFFER COUNT
    JSON=$(api GET "/baremetal/v1/zones/${ZONE}/offers?subscription_period=hourly&name=${OFFER_NAME}&page_size=100") \
        || die "impossible de lister les offres"
    OFFER=$(jq -c --arg n "${OFFER_NAME}" \
        '[.offers[] | select(.name == $n and .subscription_period == "hourly")]' <<< "${JSON}")
    COUNT=$(jq 'length' <<< "${OFFER}")
    [[ "${COUNT}" -eq 1 ]] || die "offre horaire ${OFFER_NAME} : ${COUNT} correspondance(s) en ${ZONE}, une seule attendue"
    OFFER=$(jq -c '.[0]' <<< "${OFFER}")
    [[ $(jq -r '.enable' <<< "${OFFER}") == "true" ]] || die "offre ${OFFER_NAME} désactivée"
    [[ $(jq -r '.stock' <<< "${OFFER}") != "empty" ]] || die "offre ${OFFER_NAME} en rupture de stock en ${ZONE}"
    [[ $(jq -r '((.fee.units // 0) == 0) and ((.fee.nanos // 0) == 0)' <<< "${OFFER}") == "true" ]] \
        || die "offre ${OFFER_NAME} : frais de mise en service non nuls ($(money '.fee' <<< "${OFFER}")), refus"
    printf '%s\n' "${OFFER}"
}

resolve_os () {
    local OFFER_ID="${1}"
    local JSON OS COUNT
    JSON=$(api GET "/baremetal/v1/zones/${ZONE}/os?offer_id=${OFFER_ID}&page_size=100") \
        || die "impossible de lister les OS"
    OS=$(jq -c --arg n "${OS_NAME}" --arg v "${OS_VERSION}" \
        '[.os[] | select(.enabled and .allowed and (.name | ascii_downcase | contains($n | ascii_downcase))
                         and (.version | split(" ")[0] | split(".")[0]) == $v)]' <<< "${JSON}")
    COUNT=$(jq 'length' <<< "${OS}")
    if [[ "${COUNT}" -ne 1 ]]; then
        warn "OS disponibles pour ${OFFER_NAME} :"
        jq -r '.os[] | "   \(.name) \(.version) (\(.id))"' <<< "${JSON}" >&2
        die "OS ${OS_NAME} ${OS_VERSION} : ${COUNT} correspondance(s), une seule attendue"
    fi
    jq -c '.[0]' <<< "${OS}"
}

resolve_ssh_keys () {
    local JSON
    JSON=$(api GET "/iam/v1alpha1/ssh-keys?project_id=${PROJECT_ID}&disabled=false&page_size=100") \
        || die "impossible de lister les clés SSH du projet"
    JSON=$(jq -c '[.ssh_keys[] | select(.disabled | not) | .id]' <<< "${JSON}")
    [[ $(jq 'length' <<< "${JSON}") -gt 0 ]] || die "aucune clé SSH active dans le projet ${PROJECT_ID}"
    printf '%s\n' "${JSON}"
}

build_request () {
    local OFFER="${1}"
    local OS="${2}"
    local KEYS="${3}"
    jq -nc --argjson offer "${OFFER}" --argjson os "${OS}" --argjson keys "${KEYS}" \
        --arg project "${PROJECT_ID}" --arg name "${LAB_NAME}" --arg tag "${LAB_TAG}" \
        '{offer_id: $offer.id, project_id: $project, name: $name, description: "lab two (#50)",
          tags: [$tag], install: {os_id: $os.id, hostname: $name, ssh_key_ids: $keys}}'
}

lab_servers () {
    local JSON
    JSON=$(api GET "/baremetal/v1/zones/${ZONE}/servers?project_id=${PROJECT_ID}&tags=${LAB_TAG}&page_size=100") \
        || return 1
    jq -c --arg p "${PROJECT_ID}" --arg t "${LAB_TAG}" \
        '[.servers[] | select(.project_id == $p and (.tags | index($t)))]' <<< "${JSON}"
}

server_ip () {
    jq -r '[.ips[] | select(.version == "IPv4") | .address][0] // empty'
}

server_is_installing () {
    jq -e '.status == "delivering" or .status == "ordered"
           or (.install.status // "") == "to_install" or (.install.status // "") == "installing"' >/dev/null
}

cmd_plan () {
    local OFFER OS KEYS
    OFFER=$(resolve_offer) || exit 1
    OS=$(resolve_os "$(jq -r '.id' <<< "${OFFER}")") || exit 1
    KEYS=$(resolve_ssh_keys) || exit 1
    info "offre   : ${OFFER_NAME} ($(jq -r '.id' <<< "${OFFER}")), facturation $(jq -r '.subscription_period' <<< "${OFFER}"), stock $(jq -r '.stock' <<< "${OFFER}")"
    info "prix    : $(money '.price_per_hour' <<< "${OFFER}") HT par heure, frais de mise en service $(money '.fee' <<< "${OFFER}")"
    info "os      : $(jq -r '"\(.name) \(.version) (\(.id))"' <<< "${OS}"), utilisateur $(jq -r '.user.default_value // "?"' <<< "${OS}")"
    info "clés    : $(jq -r 'length' <<< "${KEYS}") clé(s) SSH du projet"
    info "requête : POST /baremetal/v1/zones/${ZONE}/servers"
    build_request "${OFFER}" "${OS}" "${KEYS}" | jq .
}

wait_installed () {
    local ID="${1}"
    local DEADLINE=$(( SECONDS + INSTALL_TIMEOUT ))
    local JSON STATUS INSTALL
    while (( SECONDS < DEADLINE )); do
        if ! JSON=$(api GET "/baremetal/v1/zones/${ZONE}/servers/${ID}"); then
            if http_failure_is_final; then
                warn "erreur définitive HTTP $(http_code) en attendant l'installation"
                return 1
            fi
            sleep "${POLL_INTERVAL}"
            continue
        fi
        STATUS=$(jq -r '.status' <<< "${JSON}")
        INSTALL=$(jq -r '.install.status // "none"' <<< "${JSON}")
        info "serveur ${ID} : ${STATUS}, installation ${INSTALL}"
        case "${STATUS}/${INSTALL}" in
            ready/completed) printf '%s\n' "${JSON}"; return 0 ;;
            error/*|*/error|out_of_stock/*|locked/*|deleting/*|stopped/*|stopping/*)
                warn "état terminal en échec : ${STATUS}/${INSTALL}"; return 1 ;;
        esac
        sleep "${POLL_INTERVAL}"
    done
    warn "installation non terminée après ${INSTALL_TIMEOUT}s"
    return 1
}

ssh_run () {
    local -a OPTS=()
    while [[ $# -gt 0 && "${1}" != "--" ]]; do
        OPTS+=("${1}")
        shift
    done
    [[ "${1:-}" == "--" ]] && shift
    [[ -f "${SSH_KEY}" ]] && OPTS+=(-i "${SSH_KEY}" -o IdentitiesOnly=yes -o IdentityAgent=none)
    env -u SCW_SECRET_KEY ssh -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="${STATE_DIR}/known_hosts" \
        "${OPTS[@]}" "$(cat "${STATE_DIR}/user")@$(cat "${STATE_DIR}/ip")" "$@"
}

wait_ssh () {
    local DEADLINE=$(( SECONDS + SSH_TIMEOUT ))
    while (( SECONDS < DEADLINE )); do
        ssh_run -o BatchMode=yes -o ConnectTimeout=5 -- true >/dev/null 2>&1 && return 0
        info "SSH pas encore joignable, nouvel essai dans ${POLL_INTERVAL}s"
        sleep "${POLL_INTERVAL}"
    done
    return 1
}

require_no_lab_server () {
    local EXISTING
    EXISTING=$(lab_servers) || die "impossible de lister les serveurs de lab"
    [[ $(jq 'length' <<< "${EXISTING}") -eq 0 ]] \
        || die "un serveur de lab existe déjà dans le projet ; '${0##*/} status' puis '${0##*/} down'"
}

report_uncertain_creation () {
    local FOUND
    if FOUND=$(lab_servers) && [[ $(jq 'length' <<< "${FOUND}") -gt 0 ]]; then
        warn "un serveur de lab existe bien : $(jq -r '[.[].id] | join(", ")' <<< "${FOUND}")"
    fi
    die "création incertaine : le serveur a pu être créé et être facturé ; '${0##*/} status' puis '${0##*/} down'"
}

cmd_up () {
    local OFFER OS KEYS BODY JSON ID IP SSH_USER
    require_no_lab_server
    OFFER=$(resolve_offer) || exit 1
    OS=$(resolve_os "$(jq -r '.id' <<< "${OFFER}")") || exit 1
    KEYS=$(resolve_ssh_keys) || exit 1
    BODY=$(build_request "${OFFER}" "${OS}" "${KEYS}")
    info "création de ${LAB_NAME} (${OFFER_NAME}, $(money '.price_per_hour' <<< "${OFFER}")/h HT)"
    JSON=$(api POST "/baremetal/v1/zones/${ZONE}/servers" "${BODY}") || report_uncertain_creation
    ID=$(jq -r '.id // empty' <<< "${JSON}" 2>/dev/null)
    [[ -n "${ID}" ]] || report_uncertain_creation
    CREATED_ID="${ID}"
    info "serveur ${ID} créé, facturé jusqu'à '${0##*/} down'"
    JSON=$(wait_installed "${ID}") || die "serveur ${ID} inutilisable — il est toujours facturé : '${0##*/} down'"
    IP=$(server_ip <<< "${JSON}")
    [[ -n "${IP}" ]] || die "serveur ${ID} sans IPv4 publique — il est toujours facturé : '${0##*/} down'"
    SSH_USER=$(jq -r '.user.default_value // empty' <<< "${OS}")
    printf '%s\n' "${IP}" > "${STATE_DIR}/ip"
    printf '%s\n' "${SSH_USER:-root}" > "${STATE_DIR}/user"
    : > "${STATE_DIR}/known_hosts"
    wait_ssh || die "SSH injoignable après ${SSH_TIMEOUT}s — le serveur est toujours facturé : '${0##*/} down'"
    cmd_prepare || die "préparation du serveur échouée — il est toujours facturé : '${0##*/} down'"
    info "prêt : ${SSH_USER:-root}@${IP}"
}

cmd_status () {
    local SERVERS
    SERVERS=$(lab_servers) || die "impossible de lister les serveurs de lab"
    if [[ $(jq 'length' <<< "${SERVERS}") -eq 0 ]]; then
        info "aucun serveur de lab dans le projet ${PROJECT_ID} (${ZONE})"
        return 0
    fi
    jq -r '.[] | "\(.id)  \(.name)  \(.offer_name)  \(.status)  installation \(.install.status // "none")  \(.created_at)  \([.ips[] | select(.version == "IPv4") | .address] | join(","))"' <<< "${SERVERS}"
}

require_known_server () {
    [[ -s "${STATE_DIR}/ip" ]] || die "aucun serveur de lab connu ; '${0##*/} up' d'abord"
}

cmd_ssh () {
    require_known_server
    if [[ $# -gt 0 && -t 0 ]]; then
        ssh_run -t -- "$@"
    else
        ssh_run -- "$@"
    fi
}

prepare_script () {
    cat <<'EOF'
set -eu
SUDO=
[ "$(id -u)" -eq 0 ] || SUDO=sudo
$SUDO env DEBIAN_FRONTEND=noninteractive apt-get update -qq
$SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends qemu-system-x86 qemu-utils genisoimage
[ "$(id -u)" -eq 0 ] || $SUDO usermod -aG kvm "$(id -un)"
[ -c /dev/kvm ] || { echo "/dev/kvm absent" >&2; exit 1; }
NESTED=$(cat /sys/module/kvm_intel/parameters/nested 2>/dev/null || cat /sys/module/kvm_amd/parameters/nested 2>/dev/null || true)
case "${NESTED}" in
    Y|1) ;;
    *) echo "virtualisation imbriquée désactivée (nested=${NESTED:-absent})" >&2; exit 1 ;;
esac
echo "qemu $(qemu-system-x86_64 --version | head -n 1), nested=${NESTED}"
EOF
}

cmd_prepare () {
    require_known_server
    info "préparation du serveur : qemu, genisoimage, KVM imbriqué"
    prepare_script | ssh_run -- bash -s
}

push_file () {
    local SOURCE="${1}" TARGET="${2}" MODE="${3}"
    ssh_run -- "cat > '${TARGET}.part' && chmod ${MODE} '${TARGET}.part' && mv '${TARGET}.part' '${TARGET}'" < "${SOURCE}"
}

push_dir () {
    local SOURCE="${1}"
    COPYFILE_DISABLE=1 tar --no-xattrs -C "${SOURCE}" -cf - . \
        | ssh_run -- "rm -rf topology.part && mkdir topology.part && tar -C topology.part --no-same-owner -xf - && rm -rf topology && mv topology.part topology"
}

cmd_push () {
    local TOPOLOGY="${1:-}"
    local NAME="${TOPOLOGY##*/}"
    local BINARY="${STATE_DIR}/lab"
    [[ $# -eq 1 && -n "${TOPOLOGY}" ]] || usage
    [[ -f "${TOPOLOGY}" ]] || die "topologie introuvable : ${TOPOLOGY}"
    [[ "${NAME}" =~ ^[A-Za-z0-9._-]+$ ]] || die "nom de topologie refusé : ${NAME} (lettres, chiffres, '.', '_', '-')"
    require_known_server
    info "compilation de lab (linux/amd64)"
    (cd "${REPO_DIR}" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${BINARY}" ./cmd/lab) \
        || die "compilation de lab échouée"
    push_file "${BINARY}" lab 755 || die "envoi de lab échoué"
    push_dir "$(dirname "${TOPOLOGY}")" || die "envoi du répertoire de ${NAME} échoué"
    info "déposés sur le serveur : ~/lab, ~/topology/ — ensuite : ${0##*/} ssh './lab up topology/${NAME}'"
}

delete_server () {
    local ID="${1}"
    local DEADLINE=$(( SECONDS + DELETE_TIMEOUT ))
    local HARD_DEADLINE=$(( SECONDS + DELETE_TIMEOUT + INSTALL_TIMEOUT ))
    local JSON
    while (( SECONDS < DEADLINE )); do
        api DELETE "/baremetal/v1/zones/${ZONE}/servers/${ID}" >/dev/null && return 0
        [[ $(http_code) == "404" ]] && return 0
        if JSON=$(api GET "/baremetal/v1/zones/${ZONE}/servers/${ID}" 2>/dev/null) && server_is_installing <<< "${JSON}"; then
            DEADLINE=$(( SECONDS + DELETE_TIMEOUT ))
            (( DEADLINE > HARD_DEADLINE )) && DEADLINE="${HARD_DEADLINE}"
            warn "suppression de ${ID} refusée pendant la livraison ou l'installation, nouvel essai dans ${POLL_INTERVAL}s"
        else
            warn "suppression de ${ID} refusée, nouvel essai dans ${POLL_INTERVAL}s"
        fi
        sleep "${POLL_INTERVAL}"
    done
    return 1
}

wait_gone () {
    local ID="${1}"
    local DEADLINE=$(( SECONDS + DELETE_TIMEOUT ))
    while (( SECONDS < DEADLINE )); do
        if ! api GET "/baremetal/v1/zones/${ZONE}/servers/${ID}" >/dev/null 2>&1; then
            [[ $(http_code) == "404" ]] && return 0
        fi
        sleep "${POLL_INTERVAL}"
    done
    return 1
}

lab_server_ids () {
    local DEADLINE=$(( SECONDS + DELETE_TIMEOUT ))
    local SERVERS
    while (( SECONDS < DEADLINE )); do
        if SERVERS=$(lab_servers); then
            jq -r '.[].id' <<< "${SERVERS}"
            return 0
        fi
        warn "liste des serveurs de lab indisponible, nouvel essai dans ${POLL_INTERVAL}s"
        sleep "${POLL_INTERVAL}"
    done
    return 1
}

cmd_down () {
    local IDS ID FAILED=0
    if ! IDS=$(lab_server_ids); then
        warn "impossible de lister les serveurs de lab après ${DELETE_TIMEOUT}s"
        FAILED=1
        IDS=""
    fi
    IDS=$(printf '%s\n%s\n' "${IDS}" "${CREATED_ID}" | grep -v '^$' | sort -u)
    for ID in ${IDS}; do
        info "suppression de ${ID}"
        if ! delete_server "${ID}"; then
            warn "suppression de ${ID} toujours refusée"
            FAILED=1
            continue
        fi
        if ! wait_gone "${ID}"; then
            warn "${ID} toujours présent après ${DELETE_TIMEOUT}s"
            FAILED=1
        fi
    done
    rm -f "${STATE_DIR}/ip" "${STATE_DIR}/user" "${STATE_DIR}/known_hosts"
    [[ "${FAILED}" -eq 0 ]] || die "SERVEUR(S) DE LAB TOUJOURS FACTURÉ(S) — '${0##*/} status', puis la console Scaleway"
    info "aucun serveur de lab ne reste dans le projet"
}

cmd_session () {
    local RC
    require_no_lab_server
    trap 'trap - EXIT; trap "" INT TERM HUP; cmd_down' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    trap 'exit 129' HUP
    cmd_up
    cmd_ssh "$@"
    RC=$?
    info "session terminée (code ${RC}), suppression du serveur"
    return "${RC}"
}

main () {
    local COMMAND="${1:-}"
    [[ $# -gt 0 ]] && shift
    load_credentials
    [[ -n "${COMMAND}" ]] || usage
    mkdir -p "${STATE_DIR}" || die "impossible de créer ${STATE_DIR}"
    chmod 700 "${STATE_DIR}"
    case "${COMMAND}" in
        plan)    require_env; cmd_plan ;;
        up)      require_env; cmd_up ;;
        status)  require_env; cmd_status ;;
        ssh)     cmd_ssh "$@" ;;
        prepare) cmd_prepare || die "préparation du serveur échouée" ;;
        push)    cmd_push "$@" ;;
        down)    require_env; cmd_down ;;
        session) require_env; cmd_session "$@" ;;
        *)       usage ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
