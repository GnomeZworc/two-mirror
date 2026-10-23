#!/usr/bin/env bash

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAB_HOST="${LAB_HOST:-${LAB_DIR}/labctl.sh}"
SUITE_DIR="${SUITE_DIR:-${LAB_DIR}/suite}"
LIBS=(core vm net evpn fixture)
SCENARIO_LOG=""
KNOWN_FAIL=0

usage () {
    cat >&2 <<USAGE
usage: ${0##*/} [--continue] [--connus] all | --from <NN> | <couche N0>... | <test>...
       ${0##*/} --matrice

  tests : $(cd "${SUITE_DIR}" && ls -- *.sh 2>/dev/null | sed 's/\.sh$//' | tr '\n' ' ')

Chaque test s'exécute depuis le Mac sur le lab en cours (labctl.sh up, push, ./labctl up).
Une couche se désigne par son numéro (00, 10, …, 70), un test par son numéro à deux chiffres
ou son nom exact, avec ou sans numéro (21, 21-dhcp-two ou dhcp-two).
Avant un test, ses ressources t<NN> restantes (API, netns t<NN>-*, fichiers t<NN>-* et t<NN>.*
de /tmp et /var/tmp/e2e) sont supprimées et l'état de chaque hôte de l'en-tête « # hôtes: »
est relevé ; après, ses ressources sont supprimées et l'hôte doit être revenu à cet état.
Un test réussit s'il n'a aucun échec et au moins une vérification propre, hors fixtures. Avec all ou --from, une couche en échec arrête la suite (--continue : non).
Un contrôle marqué known (échec connu, rattaché à un ticket) donne CONNU sans compter comme
un échec, ou INATTENDU (compté comme un échec) s'il passe : retirer alors le marqueur. --connus
compte les CONNU comme des échecs. known_flaky (échec intermittent) donne RÉUSSI, annoté, s'il passe.
Une ligne RÉUSSI ou ÉCHOUÉ par vérification ; code de sortie 1 si une vérification échoue ou
si aucune n'a été faite.
USAGE
    exit 2
}

on () {
    local node="${1}" assignment lib
    shift
    {
        for assignment in "$@"; do
            printf 'export %q=%q\n' "${assignment%%=*}" "${assignment#*=}"
        done
        for lib in "${LIBS[@]}"; do
            cat "${LAB_DIR}/lib/${lib}.sh"
        done
        cat
    } | "${LAB_HOST}" ssh "./labctl ssh ${node} 'sudo bash -s'" 2>&1 | tee -a "${SCENARIO_LOG}"
}

header () {
    sed -n "s/^# ${2}: *//p" "${1}" | head -n 1
}

number () {
    local name
    name=$(basename "${1}")
    echo "${name%%-*}"
}

run_scenario () {
    local file="${1}" name passed failed known host hosts num start own
    name="$(basename "${file}" .sh)"
    num=$(number "${file}")
    hosts=$(header "${file}" hôtes)
    SCENARIO_LOG="$(mktemp)"
    echo "=== ${name}"
    for host in ${hosts}; do
        on "${host}" "T=${num}" "HOST=${host}" <<< 'prepare_test'
    done
    start=$(wc -l < "${SCENARIO_LOG}")
    ( . "${file}" )
    own=$(tail -n "+$(( start + 1 ))" "${SCENARIO_LOG}" | grep -cE '^(RÉUSSI|ÉCHOUÉ|CONNU|INATTENDU): ')
    for host in ${hosts}; do
        on "${host}" "T=${num}" "HOST=${host}" <<< 'finish_test'
    done
    passed=$(grep -c '^RÉUSSI: ' "${SCENARIO_LOG}")
    failed=$(grep -cE '^(ÉCHOUÉ|INATTENDU): ' "${SCENARIO_LOG}")
    known=$(grep -c '^CONNU: ' "${SCENARIO_LOG}")
    [[ "${KNOWN_FAIL}" -eq 1 ]] && failed=$(( failed + known ))
    rm -f "${SCENARIO_LOG}"
    echo "=== ${name} : ${passed} réussi(s), ${failed} échoué(s)$([[ "${known}" -gt 0 ]] && echo ", ${known} connu(s)")"
    [[ "${own}" -gt 0 ]] || echo "=== ${name} : aucune vérification propre au scénario n'a tourné"
    [[ "${failed}" -eq 0 && "${own}" -gt 0 ]]
}

matrix () {
    local file
    printf '%s\n' '.. list-table::' '   :header-rows: 1' '' \
        '   * - Test' '     - Fonctionnalité' '     - Hôtes' '     - Tickets'
    for file in "${SUITE_DIR}"/*.sh; do
        printf '   * - ``%s``\n     - %s\n     - %s\n     - %s\n' "$(basename "${file}" .sh)" \
            "$(header "${file}" fonctionnalité)" "$(header "${file}" hôtes)" "$(header "${file}" tickets)"
    done
}

select_files () {
    local arg file from=""
    for arg in "$@"; do
        if [[ -n "${from}" ]]; then
            [[ "${arg}" =~ ^[0-9]{2}$ ]] || { echo "--from : numéro à deux chiffres attendu" >&2; return 1; }
            for file in "${SUITE_DIR}"/*.sh; do
                [[ "$(number "${file}")" > "${arg}" || "$(number "${file}")" == "${arg}" ]] && echo "${file}"
            done
            from=""
        elif [[ "${arg}" == --from ]]; then
            from=1
        elif [[ "${arg}" == all ]]; then
            ls "${SUITE_DIR}"/*.sh
        elif [[ "${arg}" =~ ^[0-9]0$ ]]; then
            ls "${SUITE_DIR}/${arg:0:1}"[0-9]-*.sh 2>/dev/null || { echo "couche vide : ${arg}" >&2; return 1; }
        elif [[ "${arg}" =~ ^[0-9]{2}$ ]]; then
            file=$(ls "${SUITE_DIR}/${arg}"-*.sh 2>/dev/null | head -n 1)
            [[ -n "${file}" ]] || { echo "test inconnu : ${arg}" >&2; return 1; }
            echo "${file}"
        elif [[ -f "${SUITE_DIR}/${arg}.sh" ]]; then
            echo "${SUITE_DIR}/${arg}.sh"
        else
            file=$(ls "${SUITE_DIR}"/[0-9][0-9]-"${arg}".sh 2>/dev/null | head -n 1)
            [[ -n "${file}" ]] || { echo "test inconnu : ${arg} (numéro à deux chiffres ou nom exact)" >&2; return 1; }
            echo "${file}"
        fi
    done
    [[ -z "${from}" ]] || { echo "--from sans numéro" >&2; return 1; }
}

main () {
    local -a files=() args=()
    local arg file rc=0 layer="" layer_rc=0 stop=1 sweep=0 out
    for arg in "$@"; do
        case "${arg}" in
            --continue) stop=0 ;;
            --connus) KNOWN_FAIL=1 ;;
            --matrice) matrix; return 0 ;;
            all|--from) sweep=1; args+=("${arg}") ;;
            *) args+=("${arg}") ;;
        esac
    done
    [[ ${#args[@]} -gt 0 ]] || usage
    out=$(select_files "${args[@]}") || usage
    while IFS= read -r file; do files+=("${file}"); done <<< "${out}"
    [[ ${#files[@]} -gt 0 && -n "${files[0]}" ]] || usage
    for file in "${files[@]}"; do
        [[ "$(number "${file}")" =~ ^[0-9]{2}$ ]] || sweep=0
        if [[ "${sweep}" -eq 1 && "${stop}" -eq 1 && -n "${layer}" && "$(number "${file}" | cut -c1)" != "${layer}" && "${layer_rc}" -ne 0 ]]; then
            echo "=== couche ${layer}0 en échec : arrêt de la suite (--continue pour poursuivre)"
            return 1
        fi
        [[ "$(number "${file}" | cut -c1)" != "${layer}" ]] && { layer=$(number "${file}" | cut -c1); layer_rc=0; }
        run_scenario "${file}" || { rc=1; layer_rc=1; }
    done
    return "${rc}"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
