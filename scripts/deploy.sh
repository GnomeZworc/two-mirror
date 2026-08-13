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

check_latest_script () {
    REMOTE_URL="${1}"
    LOCAL_PATH="${2}"

    REMOTE=$(curl --silent "${REMOTE_URL}" | sha256sum)
    LOCAL=$(cat ${LOCAL_PATH} | sha256sum)

    [[ "${REMOTE}" == "${LOCAL}" ]] || return 1
    return 0
}

# Liste les units actives correspondant à un motif, une par ligne.
# Sortie vide si aucune ne tourne — ce n'est pas une erreur.
list_active_units () {
    PATTERN="${1}"
    systemctl list-units --state=active --no-legend --plain "${PATTERN}" 2>/dev/null \
        | awk '{print $1}' || true
}

stop_services () {
    DRY_RUN="${1}"
    METADATA_UNITS="${2}"
    DNSMASQ_UNITS="${3}"

    # L'agent en premier : il pilote les autres units, on ne veut pas qu'il
    # observe leur disparition.
    exec_with_dry_run "${DRY_RUN}" "systemctl stop agent.service"

    for unit in ${METADATA_UNITS} ${DNSMASQ_UNITS}
    do
        exec_with_dry_run "${DRY_RUN}" "systemctl stop '${unit}'"
    done
}

start_services () {
    DRY_RUN="${1}"
    METADATA_UNITS="${2}"
    DNSMASQ_UNITS="${3}"

    # Ordre inverse de l'arrêt : les dépendances d'abord, l'agent en dernier.
    for unit in ${DNSMASQ_UNITS} ${METADATA_UNITS}
    do
        exec_with_dry_run "${DRY_RUN}" "systemctl start '${unit}'"
    done

    exec_with_dry_run "${DRY_RUN}" "systemctl start agent.service"
}

download_binaries () {
    DRY_RUN="${1}"
    TAG="${2}"
    GIT_SERVER="${3}"
    REPO_PATH="${4}"

    #'.[0].assets.[].browser_download_url'
    [[ "${TAG}" == "" ]] && TAG=$(curl --silent "${GIT_SERVER}api/v1/repos/${REPO_PATH}releases/?limit=1" | jq -r '.[0].tag_name')
    echo "Deploy ${TAG} binaries"

    BIN_PATH="/opt/two/${TAG}/bin/"
    LN_PATH="/opt/two/bin/"

    exec_with_dry_run "${DRY_RUN}" "mkdir -p \"${BIN_PATH}\""
    exec_with_dry_run "${DRY_RUN}" "mkdir -p \"${LN_PATH}\""

    ASSETS=$(curl --silent "${GIT_SERVER}api/v1/repos/${REPO_PATH}releases/tags/${TAG}" | jq -c '.assets[]')

    # Téléchargement d'abord, dans le répertoire versionné : aucun impact sur
    # les services en cours, donc aucune raison de les arrêter pendant ce temps.
    while read -r tmp
    do
        [[ -z "${tmp}" ]] && continue
        BINARY_NAME=$(echo "${tmp}" | jq -r '.name')
        BINARY_URL=$(echo "${tmp}" | jq -r '.browser_download_url')
        exec_with_dry_run "${DRY_RUN}" "curl --silent '${BINARY_URL}' -o '${BIN_PATH}${BINARY_NAME}'"
        exec_with_dry_run "${DRY_RUN}" "chmod +x '${BIN_PATH}${BINARY_NAME}'"
    done <<< "${ASSETS}"

    # Les units actives sont relevées avant l'arrêt : c'est la seule façon de
    # savoir lesquelles redémarrer ensuite (instances dnsmasq@ et metadata@).
    METADATA_UNITS=$(list_active_units 'metadata@*')
    DNSMASQ_UNITS=$(list_active_units 'dnsmasq@*')

    stop_services "${DRY_RUN}" "${METADATA_UNITS}" "${DNSMASQ_UNITS}"

    # Bascule des liens symboliques, services arrêtés : c'est la seule fenêtre
    # d'indisponibilité réelle.
    while read -r tmp
    do
        [[ -z "${tmp}" ]] && continue
        BINARY_NAME=$(echo "${tmp}" | jq -r '.name')
        BINARY_SHORT_NAME=$(echo "${BINARY_NAME}" | cut -d_ -f 1)
        exec_with_dry_run "${DRY_RUN}" "rm -f '${LN_PATH}${BINARY_SHORT_NAME}'"
        exec_with_dry_run "${DRY_RUN}" "ln -s '${BIN_PATH}${BINARY_NAME}' '${LN_PATH}${BINARY_SHORT_NAME}'"
    done <<< "${ASSETS}"

    start_services "${DRY_RUN}" "${METADATA_UNITS}" "${DNSMASQ_UNITS}"
}

main () {
    [[ -f ./libs/shflags ]] && . ./libs/shflags || eval "$(curl --silent https://git.g3e.fr/H6N/tools/raw/branch/main/libs/shflags)"

    DEFINE_boolean 'dryrun'     false                 'Enable dry-run mode' 'd'
    DEFINE_boolean 'up_script'  true                  'Upgrade script'      's'
    DEFINE_string  'git_server' 'https://git.g3e.fr/' 'Git Server'          'g'
    DEFINE_string  'repo_path'  'syonad/two/'         'Path of repository'  'r'
    DEFINE_string  'branch'     'main/'               'Branch name'         'b'
    DEFINE_string  'tag'        ''                    'Tag name'            't'

    FLAGS "$@" || exit $?
    eval set -- "${FLAGS_ARGV}"

    SCRIPT_URL="${FLAGS_git_server}${FLAGS_repo_path}raw/branch/${FLAGS_branch}${SCRIPT_PATH}"
    check_latest_script "${SCRIPT_URL}" "${0}" || (
        [[ ${FLAGS_up_script} -eq ${FLAGS_TRUE} ]] && \
            exec_with_dry_run "${FLAGS_dryrun}" "curl --silent '${SCRIPT_URL}' -o '${0}'"
        exit 1
    )

    download_binaries "${FLAGS_dryrun}" "${FLAGS_tag}" "${FLAGS_git_server}" "${FLAGS_repo_path}"
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && (main "$@" || exit 1)
[[ "${BASH_SOURCE[0]}" == "" ]] && (main "$@"  || exit 1)