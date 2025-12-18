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

    curl --silent "${GIT_SERVER}api/v1/repos/${REPO_PATH}releases/tags/${TAG}" | jq -c '.assets[]' | while read tmp
    do
        BINARY_NAME=$(echo "${tmp}" | jq -r '.name')
        BINARY_SHORT_NAME=$(echo "${BINARY_NAME}" | cut -d_ -f 1)
        BINARY_URL=$(echo "${tmp}" | jq -r '.browser_download_url')
        exec_with_dry_run "${DRY_RUN}" "curl --silent '${BINARY_URL}' -o '${BIN_PATH}${BINARY_NAME}'"
        exec_with_dry_run "${DRY_RUN}" "chmod +x '${BIN_PATH}${BINARY_NAME}'"
        exec_with_dry_run "${DRY_RUN}" "rm -f '${LN_PATH}${BINARY_SHORT_NAME}'"
        exec_with_dry_run "${DRY_RUN}" "ln -s '${BIN_PATH}${BINARY_NAME}' '${LN_PATH}${BINARY_SHORT_NAME}'"
    done
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