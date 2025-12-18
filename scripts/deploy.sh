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

check_latest_script () {
    REMOTE_URL="${1}"
    LOCAL_PATH="${2}"

    REMOTE=$(curl --silent "${REMOTE_URL}" | sha256sum)
    LOCAL=$(cat ${LOCAL_PATH} | sha256sum)

    [[ "${REMOTE}" == "${LOCAL}" ]] \
        && echo "both are the same" \
        || echo "not the same"
}

main () {
    [[ -f ./libs/shflags ]] && . ./libs/shflags || eval "$(curl --silent https://git.g3e.fr/H6N/tools/raw/branch/main/libs/shflags)"

    DEFINE_boolean 'dryrun'     false                 'Enable dry-run mode' 'd'
    DEFINE_string  'git_server' 'https://git.g3e.fr/' 'Git Server'          'g'
    DEFINE_string  'repo_path'  'syonad/two/'         'Path of repository'  'r'
    DEFINE_string  'branch'     'main/'               'Branch name'         'b'

    check_latest_script "${FLAGS_git_server}${FLAGS_repo_path}raw/branch/${FLAGS_branch}${SCRIPT_PATH}" "${0}"
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && (main "$@" || exit 1)
[[ "${BASH_SOURCE[0]}" == "" ]] && (main "$@"  || exit 1)