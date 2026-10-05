#!/usr/bin/env bash

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS=0
FAIL=0

fail () { echo "FAIL: ${CURRENT}: ${1}"; FAIL=$(( FAIL + 1 )); CASE_OK=0; }

setup () {
    CURRENT="${1}"
    CASE_OK=1
    WORK=$(mktemp -d "${TMPDIR:-/tmp}/scenario.XXXXXX")
    mkdir -p "${WORK}/scenarios"
    cat > "${WORK}/lab-host" <<'FAKE'
#!/usr/bin/env bash
N=$(ls "${FAKE_DIR}" | grep -c '^payload\.')
cat > "${FAKE_DIR}/payload.$(( N + 1 ))"
printf '%s\n' "$*" >> "${FAKE_DIR}/calls.log"
if ! bash -n "${FAKE_DIR}/payload.$(( N + 1 ))" 2>"${FAKE_DIR}/syntax.$(( N + 1 ))"; then
    echo "ÉCHOUÉ: syntaxe du bloc $(( N + 1 ))"
    exit 0
fi
[ -n "${FAKE_OUT:-}" ] && printf '%b\n' "${FAKE_OUT}"
exit 0
FAKE
    chmod +x "${WORK}/lab-host"
    : > "${WORK}/calls.log"
}

run () {
    env LAB_HOST="${WORK}/lab-host" FAKE_DIR="${WORK}" SCENARIO_DIR="${SCENARIO_DIR:-${WORK}/scenarios}" \
        FAKE_OUT="${FAKE_OUT:-}" bash "${DIR}/run.sh" "$@" > "${WORK}/out.log" 2>&1
}

teardown () {
    [[ "${CASE_OK}" -eq 1 ]] && PASS=$(( PASS + 1 )) && echo "ok:   ${CURRENT}"
    [[ "${CASE_OK}" -eq 1 ]] || { echo "      sortie :"; sed 's/^/      | /' "${WORK}/out.log"; }
    rm -rf "${WORK}"
}

node () {
    bash -c ". '${DIR}/lib/node.sh'; ${1}" 2>&1
}

test_on_sends_assignments_library_and_block_to_the_node () {
    setup "on : variables, bibliothèque et bloc envoyés au bon nœud"
    printf '%s\n' "on hv2 KEY='ssh-ed25519 AAAA x' NAME=vm <<'NODE'" 'echo "${NAME}"' 'NODE' > "${WORK}/scenarios/t1.sh"
    FAKE_OUT='RÉUSSI: un' run t1 || fail "code de sortie $?"
    [[ $(cat "${WORK}/calls.log") == "ssh ./lab ssh hv2 'sudo bash -s'" ]] || fail "appel : $(cat "${WORK}/calls.log")"
    grep -q '^check () {' "${WORK}/payload.1" || fail "node.sh absent du bloc"
    [[ $(tail -n 1 "${WORK}/payload.1") == 'echo "${NAME}"' ]] || fail "bloc du scénario absent en fin d'envoi"
    [[ $(bash -c "$(grep '^export ' "${WORK}/payload.1"); printf '%s|%s' \"\${KEY}\" \"\${NAME}\"") == "ssh-ed25519 AAAA x|vm" ]] \
        || fail "variables mal transmises : $(grep '^export ' "${WORK}/payload.1")"
    teardown
}

test_scenario_result_counts_and_exit_codes () {
    local CASE
    for CASE in ok fail none; do
        setup "résultat d'un scénario (${CASE})"
        printf '%s\n' "on hv1 <<'NODE'" 'true' 'NODE' > "${WORK}/scenarios/t2.sh"
        case "${CASE}" in
            ok)   FAKE_OUT='RÉUSSI: a\nRÉUSSI: b' run t2; [[ $? -eq 0 ]] || fail "code non nul"
                  grep -q '=== t2 : 2 réussi(s), 0 échoué(s)' "${WORK}/out.log" || fail "bilan absent" ;;
            fail) FAKE_OUT='RÉUSSI: a\nÉCHOUÉ: b — détail' run t2; [[ $? -eq 1 ]] || fail "code différent de 1"
                  grep -q '=== t2 : 1 réussi(s), 1 échoué(s)' "${WORK}/out.log" || fail "bilan absent" ;;
            none) FAKE_OUT='INFO: rien' run t2; [[ $? -eq 1 ]] || fail "un scénario sans vérification a réussi" ;;
        esac
        teardown
    done
}

test_unknown_scenario_and_no_argument () {
    setup "scénario inconnu ou absent : usage, code 2"
    run nope; [[ $? -eq 2 ]] || fail "scénario inconnu : code $?"
    run; [[ $? -eq 2 ]] || fail "sans argument : code $?"
    [[ -s "${WORK}/calls.log" ]] && fail "un nœud a été appelé"
    teardown
}

test_all_runs_every_scenario_and_fails_if_one_fails () {
    setup "all : chaque scénario, échec si l'un échoue"
    printf '%s\n' "on hv1 <<'NODE'" 'true' 'NODE' > "${WORK}/scenarios/a.sh"
    printf '%s\n' "on hv1 <<'NODE'" 'true' 'NODE' > "${WORK}/scenarios/b.sh"
    FAKE_OUT='ÉCHOUÉ: x' run all; [[ $? -eq 1 ]] || fail "code différent de 1"
    [[ $(grep -c '^=== [ab] :' "${WORK}/out.log") -eq 2 ]] || fail "les deux scénarios n'ont pas tourné"
    teardown
}

test_every_shipped_block_is_valid_bash () {
    local f
    setup "chaque bloc envoyé par les scénarios livrés est du bash valide"
    SCENARIO_DIR="${DIR}/scenarios" FAKE_OUT='RÉUSSI: x' run all
    for f in "${WORK}"/syntax.*; do
        [[ -s "${f}" ]] && fail "${f##*/} : $(cat "${f}")"
    done
    [[ $(ls "${WORK}" | grep -c '^payload\.') -ge 10 ]] || fail "trop peu de blocs envoyés : $(ls "${WORK}" | grep -c '^payload\.')"
    grep -q 'ÉCHOUÉ: syntaxe' "${WORK}/out.log" && fail "bloc invalide"
    teardown
}

test_check_reports_success_and_failure_with_reason () {
    setup "check : RÉUSSI, ou ÉCHOUÉ avec la dernière ligne de sortie"
    [[ $(node 'check "a" true') == "RÉUSSI: a" ]] || fail "succès : $(node 'check "a" true')"
    [[ $(node 'check "b" sh -c "echo un; echo deux >&2; exit 3"') == "ÉCHOUÉ: b — deux" ]] || fail "échec : $(node 'check "b" sh -c "echo un; echo deux >&2; exit 3"')"
    [[ $(node 'check "c" false') == "ÉCHOUÉ: c" ]] || fail "échec muet : $(node 'check "c" false')"
    teardown
}

test_vm_fails_only_when_ssh_worked_and_command_failed () {
    setup "vm_fails : réussit seulement si SSH marche et la commande échoue"
    [[ $(node 'vm_ssh () { echo rc=1; }; vm_fails v 1.2.3.4 x && echo OUI') == "OUI" ]] || fail "commande échouée non reconnue"
    [[ $(node 'vm_ssh () { echo rc=0; }; vm_fails v 1.2.3.4 x || echo NON') == *NON ]] || fail "commande réussie prise pour un échec"
    [[ $(node 'vm_ssh () { return 255; }; vm_fails v 1.2.3.4 x || echo NON') == *NON ]] || fail "SSH en panne pris pour une isolation"
    [[ $(node 'vm_ssh () { echo bizarre; }; vm_fails v 1.2.3.4 x || echo NON') == *NON ]] || fail "sortie inattendue acceptée"
    teardown
}

test_route_via_matches_the_next_hop_exactly () {
    setup "route_via : le next-hop exact, pas un préfixe"
    [[ $(node 'vm_ssh () { echo "default via 10.220.1.1 dev ens3 proto dhcp"; }; route_via v ip default 10.220.1.1 && echo OUI') == OUI ]] || fail "next-hop exact refusé"
    [[ $(node 'vm_ssh () { echo "default via 10.220.1.10 dev ens3"; }; route_via v ip default 10.220.1.1 || echo NON') == NON ]] || fail "10.220.1.10 pris pour 10.220.1.1"
    teardown
}

test_wait_state () {
    setup "wait_state : état atteint, erreur, suppression"
    [[ $(node 'api_state () { echo running; }; wait_state vms/x running && echo OUI') == OUI ]] || fail "running"
    local start=${SECONDS}
    [[ $(node 'api_state () { echo error; }; wait_state vms/x running 2>&1 || echo NON') == *NON ]] || fail "error accepté"
    (( SECONDS - start < 5 )) || fail "une ressource en error n'arrête pas l'attente immédiatement ($(( SECONDS - start )) s)"
    [[ $(node 'api_state () { echo; }; wait_state vms/x deleted && echo OUI') == OUI ]] || fail "absence = supprimé"
    teardown
}

for T in $(declare -F | awk '{print $3}' | grep '^test_'); do
    "${T}"
done
echo "${PASS} réussi(s), ${FAIL} échec(s)"
[[ "${FAIL}" -eq 0 ]]
