#!/usr/bin/env bash

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB_HOST="${LAB_HOST:-${LAB_DIR}/../lab-host.sh}"
SCENARIO_DIR="${SCENARIO_DIR:-${LAB_DIR}/scenarios}"
SCENARIO_LOG=""

usage () {
    cat >&2 <<USAGE
usage: ${0##*/} <scénario>... | all

  scénarios : $(cd "${SCENARIO_DIR}" && ls -- *.sh | sed 's/\.sh$//' | tr '\n' ' ')

Chaque scénario s'exécute depuis le Mac sur le lab en cours (lab-host.sh up, push, ./lab up).
Une ligne RÉUSSI ou ÉCHOUÉ par vérification ; code de sortie 1 si une vérification échoue ou
si aucune n'a été faite.
USAGE
    exit 2
}

on () {
    local node="${1}" assignment
    shift
    {
        for assignment in "$@"; do
            printf 'export %q=%q\n' "${assignment%%=*}" "${assignment#*=}"
        done
        cat "${LAB_DIR}/node.sh"
        cat
    } | "${LAB_HOST}" ssh "./lab ssh ${node} 'sudo bash -s'" 2>&1 | tee -a "${SCENARIO_LOG}"
}

run_scenario () {
    local file="${1}" name passed failed
    name="$(basename "${file}" .sh)"
    SCENARIO_LOG="$(mktemp)"
    echo "=== ${name}"
    ( . "${file}" )
    passed=$(grep -c '^RÉUSSI: ' "${SCENARIO_LOG}")
    failed=$(grep -c '^ÉCHOUÉ: ' "${SCENARIO_LOG}")
    rm -f "${SCENARIO_LOG}"
    echo "=== ${name} : ${passed} réussi(s), ${failed} échoué(s)"
    [[ "${failed}" -eq 0 && "${passed}" -gt 0 ]]
}

main () {
    local -a files=()
    local arg file rc=0
    [[ $# -gt 0 ]] || usage
    for arg in "$@"; do
        if [[ "${arg}" == all ]]; then
            files+=("${SCENARIO_DIR}"/*.sh)
            continue
        fi
        file=$(ls "${SCENARIO_DIR}/${arg}"*.sh 2>/dev/null | head -n 1)
        [[ -n "${file}" ]] || { echo "scénario inconnu : ${arg}" >&2; usage; }
        files+=("${file}")
    done
    for file in "${files[@]}"; do
        run_scenario "${file}" || rc=1
    done
    return "${rc}"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
