#!/bin/bash
set -e

SED_PARAM=""
unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)     SED_PARAM=" -i ";;
    Darwin*)    SED_PARAM=" -i '' ";;
    *)          exit 1
esac

SCRIPT_PATH="scripts/deploy.sh"
UNIT_DIR="/etc/systemd/system"
SCRIPTS_DIR="/opt/two/scripts"

info () { echo "== ${1}"; }
warn () { echo "!! ${1}" >&2; }
die  () { echo "!! ${1}" >&2; exit 1; }

exec_with_dry_run () {
  if [[ ${1} -eq ${FLAGS_TRUE} ]]; then
    echo "# ${2}"
  else
    eval "${2}" 2> /tmp/error || \
    {
      echo -e "failed with following error";
      output=$(cat /tmp/error | sed -e "s/^/ error -> /g");
      echo -e "${output}";
      return 1;
    }
  fi
  return 0
}

run () {
    exec_with_dry_run "${1}" "${2}" || die "échec : ${2}"
}

check_latest_script () {
    REMOTE_URL="${1}"
    LOCAL_PATH="${2}"

    REMOTE=$(curl --silent "${REMOTE_URL}" | sha256sum)
    LOCAL=$(cat ${LOCAL_PATH} | sha256sum)

    [[ "${REMOTE}" == "${LOCAL}" ]] || return 1
    return 0
}

list_active_units () {
    PATTERN="${1}"
    systemctl list-units --state=active --no-legend --plain "${PATTERN}" 2>/dev/null \
        | awk '{print $1}' || true
}

# Units non instanciables du profil (agent.service) : celles qu'on arrête et
# démarre nommément.
profile_main_units () {
    for unit in $(profile_units "${1}")
    do
        case "${unit}" in
            *@.service) continue ;;
            *)          echo "${unit}" ;;
        esac
    done
}

active_instances () {
    for unit in $(profile_units "${1}")
    do
        case "${unit}" in
            *@.service) list_active_units "${unit%@.service}@*" ;;
        esac
    done
}

stop_services () {
    DRY_RUN="${1}"
    PROFILE="${2}"
    INSTANCES="${3}"

    for unit in $(profile_main_units "${PROFILE}")
    do
        run "${DRY_RUN}" "systemctl stop '${unit}'"
    done

    for unit in ${INSTANCES}
    do
        run "${DRY_RUN}" "systemctl stop '${unit}'"
    done
}

start_services () {
    DRY_RUN="${1}"
    PROFILE="${2}"
    INSTANCES="${3}"

    for unit in ${INSTANCES}
    do
        exec_with_dry_run "${DRY_RUN}" "systemctl start '${unit}'" \
            || warn "démarrage de ${unit} en échec — poursuite"
    done

    for unit in $(profile_main_units "${PROFILE}")
    do
        run "${DRY_RUN}" "systemctl start '${unit}'"
    done
}

profile_units () {
    case "${1}" in
        kvm)   echo "agent.service dnsmasq@.service metadata@.service" ;;
        intel) echo "" ;;
        *)     return 1 ;;
    esac
}

profile_binaries () {
    case "${1}" in
        kvm)   echo "agent metadata run-dnsmasq-in-netns.sh" ;;
        intel) echo "" ;;
        *)     return 1 ;;
    esac
}

profile_bootstrap () {
    case "${1}" in
        kvm)   echo "bootstrap_kvm.sh" ;;
        intel) echo "" ;;
        *)     return 1 ;;
    esac
}

run_bootstrap () {
    DRY_RUN="${1}"
    PROFILE="${2}"
    GIT_SERVER="${3}"
    REPO_PATH="${4}"
    BRANCH="${5}"

    NAME=$(profile_bootstrap "${PROFILE}") || die "profil inconnu : ${PROFILE}"
    [[ -n "${NAME}" ]] || { info "bootstrap : rien à préparer pour le profil ${PROFILE}"; return 0; }

    BOOTSTRAP_URL="${GIT_SERVER}${REPO_PATH}raw/branch/${BRANCH}scripts/${NAME}"
    info "bootstrap du profil ${PROFILE} (${SCRIPTS_DIR}/${NAME} ou ${BOOTSTRAP_URL})"

    # shellcheck source=/dev/null
    [[ -f "${SCRIPTS_DIR}/${NAME}" ]] && . "${SCRIPTS_DIR}/${NAME}" || eval "$(curl --silent --fail "${BOOTSTRAP_URL}")"

    command -v bootstrap_host >/dev/null 2>&1 \
        || die "bootstrap_host() introuvable — ni ${SCRIPTS_DIR}/${NAME} ni ${BOOTSTRAP_URL} n'ont pu être chargés"

    bootstrap_host "${DRY_RUN}" "${FLAGS_packages}" "${FLAGS_network}" \
        "${FLAGS_uplink}" "${FLAGS_bridge}" "${FLAGS_pub_bridge}" "${FLAGS_rollback}"
}

resolve_tag () {
    TAG="${1}"
    GIT_SERVER="${2}"
    REPO_PATH="${3}"

    [[ -n "${TAG}" ]] && { echo "${TAG}"; return 0; }
    curl --silent "${GIT_SERVER}api/v1/repos/${REPO_PATH}releases/?limit=1" | jq -r '.[0].tag_name'
}

release_assets () {
    GIT_SERVER="${1}"
    REPO_PATH="${2}"
    RELEASE_TAG="${3}"

    curl --silent "${GIT_SERVER}api/v1/repos/${REPO_PATH}releases/tags/${RELEASE_TAG}" | jq -c '.assets[]'
}

asset_name () {
    echo "${1}" | jq -r --arg s "${2}" 'select(.name == $s or (.name | startswith($s + "_"))) | .name' | head -1
}

asset_url () {
    echo "${1}" | jq -r --arg n "${2}" 'select(.name == $n) | .browser_download_url' | head -1
}

fetch_assets () {
    DRY_RUN="${1}"
    PROFILE="${2}"
    ASSETS="${3}"

    info "assets de la release ${TAG} pour le profil ${PROFILE}"

    run "${DRY_RUN}" "mkdir -p '${BIN_PATH}'"
    run "${DRY_RUN}" "mkdir -p '${UNIT_PATH}'"
    run "${DRY_RUN}" "mkdir -p '${LN_PATH}'"

    for unit in $(profile_units "${PROFILE}")
    do
        UNIT_URL=$(asset_url "${ASSETS}" "${unit}")
        [[ -n "${UNIT_URL}" ]] || die "unit absente de la release ${TAG} : ${unit}"
        run "${DRY_RUN}" "curl --silent '${UNIT_URL}' -o '${UNIT_PATH}${unit}'"
    done

    for short in $(profile_binaries "${PROFILE}")
    do
        FULL_NAME=$(asset_name "${ASSETS}" "${short}")
        [[ -n "${FULL_NAME}" ]] || die "exécutable absent de la release ${TAG} : ${short}"
        BIN_URL=$(asset_url "${ASSETS}" "${FULL_NAME}")
        run "${DRY_RUN}" "curl --silent '${BIN_URL}' -o '${BIN_PATH}${FULL_NAME}'"
        run "${DRY_RUN}" "chmod +x '${BIN_PATH}${FULL_NAME}'"
    done
}

install_units () {
    DRY_RUN="${1}"
    PROFILE="${2}"

    UNITS=$(profile_units "${PROFILE}") || die "profil inconnu : ${PROFILE}"
    [[ -n "${UNITS}" ]] || { info "units : aucune pour le profil ${PROFILE}"; return 0; }

    info "units du profil ${PROFILE}"

    for unit in ${UNITS}
    do
        if [[ ${DRY_RUN} -ne ${FLAGS_TRUE} ]] && [[ ! -f "${UNIT_PATH}${unit}" ]]
        then
            die "unit absente des assets de la release ${TAG} : ${unit}"
        fi
        run "${DRY_RUN}" "install -m 0644 '${UNIT_PATH}${unit}' '${UNIT_DIR}/${unit}'"
    done

    run "${DRY_RUN}" "systemctl daemon-reload"

    for unit in ${UNITS}
    do
        case "${unit}" in
            *@.service) continue ;;
        esac
        run "${DRY_RUN}" "systemctl enable '${unit}'"
    done
}

switch_binaries () {
    DRY_RUN="${1}"
    PROFILE="${2}"
    ASSETS="${3}"

    BINARIES=$(profile_binaries "${PROFILE}")
    [[ -n "${BINARIES}" ]] || { info "bascule : aucun exécutable pour le profil ${PROFILE}"; return 0; }

    info "bascule des binaires"

    INSTANCES=$(active_instances "${PROFILE}")

    stop_services "${DRY_RUN}" "${PROFILE}" "${INSTANCES}"

    for short in ${BINARIES}
    do
        FULL_NAME=$(asset_name "${ASSETS}" "${short}")
        run "${DRY_RUN}" "rm -f '${LN_PATH}${short}'"
        run "${DRY_RUN}" "ln -s '${BIN_PATH}${FULL_NAME}' '${LN_PATH}${short}'"
    done

    start_services "${DRY_RUN}" "${PROFILE}" "${INSTANCES}"
}

main () {
    [[ -f ./libs/shflags ]] && . ./libs/shflags || eval "$(curl --silent https://git.g3e.fr/H6N/tools/raw/branch/main/libs/shflags)"

    DEFINE_boolean 'dryrun'     false                 'Enable dry-run mode'                            'd'
    DEFINE_boolean 'up_script'  true                  'Upgrade script'                                 's'
    DEFINE_string  'git_server' 'https://git.g3e.fr/' 'Git Server'                                     'g'
    DEFINE_string  'repo_path'  'syonad/two/'         'Path of repository'                             'r'
    DEFINE_string  'branch'     'main/'               'Branch name'                                    'b'
    DEFINE_string  'tag'        ''                    'Tag name'                                       't'
    DEFINE_string  'profile'    'kvm'                 'Host profile: kvm'                              'p'
    DEFINE_boolean 'bootstrap'  false                 'Prepare the host (packages, kernel, network)'   'i'
    DEFINE_boolean 'packages'   true                  'Install profile packages during --bootstrap'    'k'
    DEFINE_boolean 'network'    true                  'Configure bridges during --bootstrap'           'n'
    DEFINE_string  'uplink'     'eno1'                'Physical uplink interface'                      'u'
    DEFINE_string  'bridge'     'br-000000'           'Main bridge, uplink is enslaved to it'          'B'
    DEFINE_string  'pub_bridge' 'br-public'           'Reserved empty bridge'                          'P'
    DEFINE_integer 'rollback'   120                   'Rollback reboot delay in seconds'               'R'

    FLAGS "$@" || exit $?
    eval set -- "${FLAGS_ARGV}"

    profile_units "${FLAGS_profile}" >/dev/null || die "profil inconnu : ${FLAGS_profile}"

    if [[ ${FLAGS_up_script} -eq ${FLAGS_TRUE} ]]
    then
        SCRIPT_URL="${FLAGS_git_server}${FLAGS_repo_path}raw/branch/${FLAGS_branch}${SCRIPT_PATH}"
        if ! check_latest_script "${SCRIPT_URL}" "${0}"
        then
            run "${FLAGS_dryrun}" "curl --silent '${SCRIPT_URL}' -o '${0}'"
            die "script local différent de la branche ${FLAGS_branch} — mis à jour, relancer"
        fi
    fi

    [[ ${FLAGS_bootstrap} -eq ${FLAGS_TRUE} ]] && \
        run_bootstrap "${FLAGS_dryrun}" "${FLAGS_profile}" "${FLAGS_git_server}" "${FLAGS_repo_path}" "${FLAGS_branch}"

    TAG=$(resolve_tag "${FLAGS_tag}" "${FLAGS_git_server}" "${FLAGS_repo_path}")
    [[ -n "${TAG}" && "${TAG}" != "null" ]] || die "impossible de déterminer la release à déployer"

    BIN_PATH="/opt/two/${TAG}/bin/"
    UNIT_PATH="/opt/two/${TAG}/units/"
    LN_PATH="/opt/two/bin/"

    ASSETS=$(release_assets "${FLAGS_git_server}" "${FLAGS_repo_path}" "${TAG}")
    [[ -n "${ASSETS}" ]] || die "aucun asset dans la release ${TAG}"

    fetch_assets "${FLAGS_dryrun}" "${FLAGS_profile}" "${ASSETS}"

    install_units   "${FLAGS_dryrun}" "${FLAGS_profile}"
    switch_binaries "${FLAGS_dryrun}" "${FLAGS_profile}" "${ASSETS}"
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && (main "$@" || exit 1)
[[ "${BASH_SOURCE[0]}" == "" ]] && (main "$@"  || exit 1)
