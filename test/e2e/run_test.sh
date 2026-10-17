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
    env LAB_HOST="${WORK}/lab-host" FAKE_DIR="${WORK}" SUITE_DIR="${SUITE_DIR:-${WORK}/scenarios}" \
        FAKE_OUT="${FAKE_OUT:-}" bash "${DIR}/run.sh" "$@" > "${WORK}/out.log" 2>&1
}

teardown () {
    [[ "${CASE_OK}" -eq 1 ]] && PASS=$(( PASS + 1 )) && echo "ok:   ${CURRENT}"
    [[ "${CASE_OK}" -eq 1 ]] || { echo "      sortie :"; sed 's/^/      | /' "${WORK}/out.log"; }
    rm -rf "${WORK}"
}

node () {
    bash -c "$(printf ". '%s'; " "${DIR}"/lib/{core,vm,net,evpn,fixture}.sh)${1}" 2>&1
}

test_on_sends_assignments_library_and_block_to_the_node () {
    setup "on : variables, bibliothèque et bloc envoyés au bon nœud"
    printf '%s\n' "on hv2 KEY='ssh-ed25519 AAAA x' NAME=vm <<'NODE'" 'echo "${NAME}"' 'NODE' > "${WORK}/scenarios/t1.sh"
    FAKE_OUT='RÉUSSI: un' run t1 || fail "code de sortie $?"
    [[ $(cat "${WORK}/calls.log") == "ssh ./labctl ssh hv2 'sudo bash -s'" ]] || fail "appel : $(cat "${WORK}/calls.log")"
    grep -q '^check () {' "${WORK}/payload.1" || fail "bibliothèque absente du bloc"
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
    local f dir
    for dir in scenarios suite; do
        [[ -d "${DIR}/${dir}" ]] || continue
        setup "chaque bloc envoyé par ${dir}/ est du bash valide"
        sleep () { :; }
        export -f sleep
        SUITE_DIR="${DIR}/${dir}" FAKE_OUT='RÉUSSI: x' run --continue all
        unset -f sleep
        for f in "${WORK}"/syntax.*; do
            [[ -s "${f}" ]] && fail "${f##*/} : $(cat "${f}")"
        done
        [[ $(ls "${WORK}" | grep -c '^payload\.') -ge 10 ]] || fail "trop peu de blocs envoyés : $(ls "${WORK}" | grep -c '^payload\.')"
        grep -q 'ÉCHOUÉ: syntaxe' "${WORK}/out.log" && fail "bloc invalide"
        teardown
    done
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

test_vrf_route_via_matches_the_next_hop_exactly () {
    setup "vrf_route_via : next-hop exact dans la bonne VRF"
    local stub='ip () { [ "$*" = "-4 route show vrf vrf-s7 10.241.1.10" ] && echo "10.241.1.10 nhid 12 via 192.168.14.11 dev brl3-s7 proto bgp metric 20 onlink"; }; '
    [[ $(node "${stub}vrf_route_via vp-s7 10.241.1.10 192.168.14.11 && echo OUI") == OUI ]] || fail "route présente refusée"
    [[ $(node "${stub}vrf_route_via vp-s7 10.241.1.10 192.168.14.1 || echo NON") == NON ]] || fail "192.168.14.1 pris pour 192.168.14.11"
    [[ $(node "${stub}vrf_route_via vp-s7b 10.241.1.10 192.168.14.11 || echo NON") == NON ]] || fail "mauvaise VRF acceptée"
    teardown
}

test_vrf_members_lists_interface_names () {
    setup "vrf_members : noms triés, sans le suffixe @ du veth"
    [[ $(node 'ip () { printf "%s\n" "15: l3-s7-o@if14: <UP> mtu 1500 master vrf-s7" "12: brl3-s7: <UP> mtu 1500 master vrf-s7"; }; vrf_members vp-s7') == "brl3-s7 l3-s7-o" ]] \
        || fail "lu : $(node 'ip () { printf "%s\n" "15: l3-s7-o@if14: <UP>" "12: brl3-s7: <UP>"; }; vrf_members vp-s7')"
    teardown
}

test_vm_create_is_idempotent () {
    setup "vm_create : VM déjà en running, rien n'est copié ni envoyé"
    [[ $(node 'api_state () { echo running; }; cp () { echo COPIE; }; api_post () { echo POST; }; vm_create v sn 10.0.0.10 && echo OK') == OK ]] \
        || fail "VM existante recréée"
    teardown
}

test_l3vni_ready_requires_type_state_and_rmac () {
    setup "l3vni_ready : type L3, état Up et RMAC exigés"
    local out='VNI: 2700\n  Type: L3\n  State: Up\n  Router MAC: 02:00:00:00:27:02'
    [[ $(node "vtysh () { printf '${out}\n'; }; l3vni_ready 2700 02:00:00:00:27:02 && echo OUI") == OUI ]] || fail "VNI prête refusée"
    [[ $(node "vtysh () { printf '${out}\n'; }; l3vni_ready 2700 02:00:00:00:27:01 || echo NON") == NON ]] || fail "mauvaise RMAC acceptée"
    [[ $(node "vtysh () { printf '${out/Up/Down}\n'; }; l3vni_ready 2700 02:00:00:00:27:02 || echo NON") == NON ]] || fail "état Down accepté"
    [[ $(node "vtysh () { printf '${out/L3/L2}\n'; }; l3vni_ready 2700 02:00:00:00:27:02 || echo NON") == NON ]] || fail "VNI L2 acceptée"
    teardown
}

test_vrf_nexthops_lists_every_ecmp_member () {
    setup "vrf_nexthops : un chemin, plusieurs chemins, aucun"
    [[ $(node 'ip () { echo "10.242.1.50 nhid 9 via 192.168.14.12 dev brl3-s8 proto bgp metric 20 onlink"; }; vrf_nexthops vp-s8 10.242.1.50') == 192.168.14.12 ]] || fail "un chemin"
    [[ $(node 'ip () { printf "%s\n" "10.242.1.0/24 nhid 90 proto bgp metric 20" "	nexthop via 192.168.14.12 dev brl3-s8 weight 1 onlink" "	nexthop via 192.168.14.11 dev brl3-s8 weight 1 onlink"; }; vrf_nexthops vp-s8 10.242.1.0/24') == "192.168.14.11 192.168.14.12" ]] || fail "ECMP"
    [[ -z $(node 'ip () { :; }; vrf_nexthops vp-s8 10.242.1.50') ]] || fail "aucun chemin"
    teardown
}

test_bgp_best_reports_next_hop_and_reason () {
    setup "bgp_paths / bgp_best : nombre de chemins, meilleur next-hop et raison"
    local json='{"prefix":"10.242.1.50/32","paths":[{"nexthops":[{"ip":"192.168.14.12"}]},{"nexthops":[{"ip":"192.168.14.11"}],"bestpath":{"overall":true,"selectionReason":"Router ID"}}]}'
    [[ $(node "vtysh () { echo '${json}'; }; bgp_paths vp-s8 10.242.1.50/32") == 2 ]] || fail "nombre de chemins"
    [[ $(node "vtysh () { echo '${json}'; }; bgp_best vp-s8 10.242.1.50/32") == "192.168.14.11 (Router ID)" ]] || fail "meilleur : $(node "vtysh () { echo '${json}'; }; bgp_best vp-s8 10.242.1.50/32")"
    [[ $(node "vtysh () { echo '{}'; }; bgp_paths vp-s8 10.242.1.50/32") == 0 ]] || fail "préfixe absent"
    teardown
}

test_vpc_create_sends_the_l3vni_only_when_given () {
    setup "vpc_create : l3vni transmise si donnée, absente sinon"
    local stub='api_state () { echo; }; wait_state () { :; }; api_post () { echo "$1 $2"; }; '
    [[ $(node "${stub}vpc_create vp-s7 10.241.0.0/16 2700") == 'vpcs {"name":"vp-s7","cidr":"10.241.0.0/16","l3vni":2700}' ]] \
        || fail "avec l3vni : $(node "${stub}vpc_create vp-s7 10.241.0.0/16 2700")"
    [[ $(node "${stub}vpc_create vp-s1 10.210.0.0/16") == 'vpcs {"name":"vp-s1","cidr":"10.210.0.0/16"}' ]] \
        || fail "sans l3vni : $(node "${stub}vpc_create vp-s1 10.210.0.0/16")"
    teardown
}

test_subnet_create_lets_the_agent_compute_the_gateway () {
    setup "subnet_create : vxlan sans interface_ip, champs supplémentaires ajoutés"
    local stub='api_state () { echo; }; wait_state () { :; }; api_post () { echo "$1 $2"; }; '
    [[ $(node "${stub}subnet_create sn-s1a vp-s1 2101 10.210.1.0/24") == 'subnets {"name":"sn-s1a","vpc":"vp-s1","mode":"vxlan","vxlan_id":2101,"iface_type":"vms","cidr":"10.210.1.0/24"}' ]] \
        || fail "sans extra : $(node "${stub}subnet_create sn-s1a vp-s1 2101 10.210.1.0/24")"
    [[ $(node "${stub}subnet_create sn-s2b vp-s2 2202 10.220.2.0/24 ',\"default_route\":true'") == 'subnets {"name":"sn-s2b","vpc":"vp-s2","mode":"vxlan","vxlan_id":2202,"iface_type":"vms","cidr":"10.220.2.0/24","default_route":true}' ]] \
        || fail "avec extra : $(node "${stub}subnet_create sn-s2b vp-s2 2202 10.220.2.0/24 ',\"default_route\":true'")"
    teardown
}

test_anycast_mac_encodes_the_vni_on_24_bits () {
    setup "anycast_mac : 02:01:00 suivi de la VNI sur 24 bits"
    [[ $(node 'anycast_mac 2911') == 02:01:00:00:0b:5f ]] || fail "2911 : $(node 'anycast_mac 2911')"
    [[ $(node 'anycast_mac 16777215') == 02:01:00:ff:ff:ff ]] || fail "VNI max : $(node 'anycast_mac 16777215')"
    teardown
}

test_api_code_returns_the_http_status () {
    setup "api_code : méthode, chemin et code HTTP"
    [[ $(node 'curl () { echo "$*" >&2; echo 409; }; api_code DELETE subnets/sn-a 2>/dev/null') == 409 ]] || fail "code"
    [[ $(node 'curl () { echo "$*"; }; api_code DELETE subnets/sn-a') == "-s -o /dev/null -w %{http_code} -X DELETE http://127.0.0.1:8080/subnets/sn-a" ]] \
        || fail "appel : $(node 'curl () { echo "$*"; }; api_code DELETE subnets/sn-a')"
    teardown
}

test_watchdog_reported_matches_only_watchdog_alerts () {
    setup "watchdog_reported : alerte du watchdog contenant le texte, rien d'autre"
    local alert='{"level":"ERROR","msg":"watchdog: inconsistency detected","kind":"vpc","name":"vp-s8","problem":"route 10.242.1.99/32 exported in vrf-s8 without a running owner: its traffic is black-holed"}'
    local other='{"level":"ERROR","msg":"command failed","error":"route 10.242.1.99/32 exported in vrf-s8 without a running owner"}'
    [[ $(node "journalctl () { echo '${alert}'; }; watchdog_reported 'route 10.242.1.99/32 exported in vrf-s8 without a running owner' && echo OUI") == OUI ]] || fail "alerte non trouvée"
    [[ $(node "journalctl () { echo '${other}'; }; watchdog_reported 'route 10.242.1.99/32 exported in vrf-s8' || echo NON") == NON ]] || fail "message hors watchdog accepté"
    [[ $(node "journalctl () { echo '${alert}'; }; watchdog_reported 'route 10.242.1.98/32' || echo NON") == NON ]] || fail "autre route acceptée"
    teardown
}

suite_file () {
    printf '%s\n' "# fonctionnalité: ${3:-f}" "# hôtes: ${2}" "# tickets: #70" "on hv1 <<'NODE'" 'true' 'NODE' > "${WORK}/scenarios/${1}.sh"
}

test_selection_by_layer_test_and_from () {
    setup "sélection : couche, test, --from, inconnu"
    suite_file 01-a ""; suite_file 11-b ""; suite_file 12-c ""; suite_file 21-d ""
    FAKE_OUT='RÉUSSI: x' run 10
    [[ $(grep '^=== [0-9a-z-]*$' "${WORK}/out.log" | tr '\n' ' ') == "=== 11-b === 12-c " ]] || fail "couche 10 : $(grep '^=== ' "${WORK}/out.log" | tr '\n' ' ')"
    FAKE_OUT='RÉUSSI: x' run 12
    [[ $(grep '^=== [0-9a-z-]*$' "${WORK}/out.log" | tr '\n' ' ') == "=== 12-c " ]] || fail "test 12"
    FAKE_OUT='RÉUSSI: x' run --from 12
    [[ $(grep '^=== [0-9a-z-]*$' "${WORK}/out.log" | tr '\n' ' ') == "=== 12-c === 21-d " ]] || fail "--from 12 : $(grep '^=== ' "${WORK}/out.log" | tr '\n' ' ')"
    run 30; [[ $? -eq 2 ]] || fail "couche vide acceptée"
    run --from; [[ $? -eq 2 ]] || fail "--from sans numéro accepté"
    teardown
}

test_failed_layer_stops_the_sweep_unless_continue () {
    setup "all : une couche en échec arrête la suite, sauf --continue"
    suite_file 01-a ""; suite_file 02-b ""; suite_file 11-c ""
    FAKE_OUT='ÉCHOUÉ: x' run all; [[ $? -eq 1 ]] || fail "code"
    [[ $(grep -c '^=== 0[12]-[ab] :' "${WORK}/out.log") -eq 2 ]] || fail "la couche 00 n'a pas été jouée en entier"
    grep -q '^=== 11-c' "${WORK}/out.log" && fail "couche 10 jouée après l'échec de la couche 00"
    grep -q 'couche 00 en échec' "${WORK}/out.log" || fail "arrêt non annoncé"
    FAKE_OUT='ÉCHOUÉ: x' run --continue all
    grep -q '^=== 11-c :' "${WORK}/out.log" || fail "--continue : couche 10 non jouée"
    FAKE_OUT='ÉCHOUÉ: x' run 01 11
    grep -q '^=== 11-c :' "${WORK}/out.log" || fail "tests nommés : pas d'arrêt attendu"
    teardown
}

test_fixtures_run_on_each_header_host () {
    setup "fixtures : préparation et fin sur chaque hôte de l'en-tête, avec T et HOST"
    suite_file 41-x "hv1 hv2"
    FAKE_OUT='RÉUSSI: x' run 41
    [[ $(tr '\n' '|' < "${WORK}/calls.log") == "ssh ./labctl ssh hv1 'sudo bash -s'|ssh ./labctl ssh hv2 'sudo bash -s'|ssh ./labctl ssh hv1 'sudo bash -s'|ssh ./labctl ssh hv1 'sudo bash -s'|ssh ./labctl ssh hv2 'sudo bash -s'|" ]] \
        || fail "appels : $(tr '\n' '|' < "${WORK}/calls.log")"
    [[ $(tail -n 1 "${WORK}/payload.1") == prepare_test && $(tail -n 1 "${WORK}/payload.5") == finish_test ]] || fail "fixture absente"
    grep -q "^export T=41$" "${WORK}/payload.1" && grep -q "^export HOST=hv2$" "${WORK}/payload.5" || fail "T ou HOST non transmis"
    teardown
    setup "fixtures : aucune sans en-tête hôtes"
    printf '%s\n' "on hv1 <<'NODE'" 'true' 'NODE' > "${WORK}/scenarios/s1.sh"
    FAKE_OUT='RÉUSSI: x' run s1
    [[ $(wc -l < "${WORK}/calls.log") -eq 1 ]] || fail "fixture envoyée à un scénario sans en-tête"
    teardown
}

test_matrix_lists_every_test_from_its_header () {
    setup "--matrice : une ligne par test, tirée des en-têtes"
    suite_file 41-x "hv1 hv2" "trafic EVPN"
    run --matrice
    grep -q '^   \* - ``41-x``$' "${WORK}/out.log" && grep -q '^     - trafic EVPN$' "${WORK}/out.log" && grep -q '^     - hv1 hv2$' "${WORK}/out.log" \
        || fail "matrice : $(cat "${WORK}/out.log")"
    [[ -s "${WORK}/calls.log" ]] && fail "un nœud a été appelé"
    teardown
}

test_owned_keeps_only_the_resources_of_the_test () {
    setup "owned : vp-tNN, sn-tNN, tNN- seulement"
    [[ $(node 'printf "%s\n" vp-t41a sn-t41b t41-vm vp-t42a t4-x vp-s4 sn-s41 | T=41 owned | tr "\n" " "') == "vp-t41a sn-t41b t41-vm " ]] \
        || fail "$(node 'printf "%s\n" vp-t41a sn-t41b t41-vm vp-t42a t4-x vp-s4 sn-s41 | T=41 owned | tr "\n" " "')"
    teardown
}

test_teardown_deletes_vms_then_subnets_then_vpcs () {
    setup "teardown_owned : VM, puis subnets, puis VPC, et rien d'autre"
    local stub='api_names () { case $1 in vms) [ -f /tmp/.t70.$$ ] || echo t41-a; echo t42-z ;; subnets) echo sn-t41a ;; vpcs) echo vp-t41a ;; esac; }; api_delete () { echo "DEL $1"; [ "$1" = vms/t41-a ] && touch /tmp/.t70.$$; :; }; wait_state () { :; }; rm () { :; }; '
    local out
    out=$(node "T=41; ${stub}teardown_owned; command rm -f /tmp/.t70.\$\$")
    [[ $(grep '^DEL' <<< "${out}" | tr '\n' ' ') == "DEL vms/t41-a DEL subnets/sn-t41a DEL vpcs/vp-t41a " ]] || fail "ordre : ${out}"
    grep -q 'encore présents : subnets/sn-t41a vpcs/vp-t41a' <<< "${out}" || fail "restes non signalés : ${out}"
    teardown
}

test_state_unchanged_reports_the_difference () {
    setup "state_unchanged : même état accepté, différence signalée"
    local dir out
    dir=$(mktemp -d)
    printf '%s\n' '## liens' lo underlay > "${dir}/t41.avant"
    [[ $(node "T=41; STATE_DIR='${dir}'; host_state () { printf '%s\n' '## liens' lo underlay; }; state_unchanged && echo OUI") == OUI ]] || fail "état identique refusé"
    out=$(node "T=41; STATE_DIR='${dir}'; sleep () { :; }; host_state () { printf '%s\n' '## liens' br-t41a lo underlay; }; state_unchanged || echo NON")
    [[ $(tail -n 1 <<< "${out}") == NON ]] || fail "différence acceptée"
    grep -q '^1 différence(s), dont : > br-t41a$' <<< "${out}" || fail "résumé : ${out}"
    rm -rf "${dir}"
    teardown
}

test_api_post_code_and_expect_code () {
    setup "api_post_code / expect_code : code HTTP d'un POST, comparaison et message"
    [[ $(node 'curl () { echo "$*"; }; api_post_code subnets "{\"a\":1}"') == '-s -o /dev/null -w %{http_code} -X POST -H Content-Type: application/json -d {"a":1} http://127.0.0.1:8080/subnets' ]] \
        || fail "appel : $(node 'curl () { echo "$*"; }; api_post_code subnets "{\"a\":1}"')"
    [[ $(node 'expect_code 422 echo 422 && echo OUI') == OUI ]] || fail "code attendu refusé"
    [[ $(node 'expect_code 422 echo 202 || echo NON') == $'code 202, 422 attendu\nNON' ]] || fail "écart : $(node 'expect_code 422 echo 202 || echo NON')"
    [[ $(node 'expect_code 422 true || echo NON') == $'code absent, 422 attendu\nNON' ]] || fail "code vide accepté"
    teardown
}

test_api_field_reads_one_field_or_nothing () {
    setup "api_field : un champ du GET, vide s'il manque"
    [[ $(node 'curl () { echo "{\"interface_ip\":\"10.11.1.254\"}"; }; api_field subnets/sn-t11a interface_ip') == 10.11.1.254 ]] || fail "champ présent"
    [[ -z $(node 'curl () { echo "{\"interface_ip\":\"10.11.1.254\"}"; }; api_field subnets/sn-t11a relay_ip') ]] || fail "champ absent non vide"
    teardown
}

test_vm_body_builds_the_create_request () {
    setup "vm_body : requête de création, disque par défaut, métadonnées et dev"
    [[ $(node 'vm_body t11-a sn-t11a 10.11.1.10') == '{"name":"t11-a","memory":1024,"cpus":1,"metadata":{},"interfaces":[{"subnet":"sn-t11a","ip":"10.11.1.10","primary":true}],"storage":[{"path":"/var/lib/two/volumes/t11-a.qcow2","dev":"vda"}]}' ]] \
        || fail "défaut : $(node 'vm_body t11-a sn-t11a 10.11.1.10')"
    [[ $(node 'vm_body t11-b sn-t11a 10.11.1.11 /x.qcow2 "{\"user_data\":\"YQ==\"}" sdb | jq -c "[.metadata.user_data, .storage[0].path, .storage[0].dev]"') == '["YQ==","/x.qcow2","sdb"]' ]] \
        || fail "options : $(node 'vm_body t11-b sn-t11a 10.11.1.11 /x.qcow2 "{\"user_data\":\"YQ==\"}" sdb')"
    teardown
}

test_vm_json_builds_nics_disks_metadata_and_uefi () {
    setup "vm_json : NIC (la première primaire), disques, métadonnées, uefi"
    local out
    out=$(node 'vm_json t25-m --disk vda=/v/a.qcow2 --nic sn-a=10.0.1.10 --nic sn-b=10.0.2.10 --disk sda=/v/b.qcow2 --sshkey "ssh-ed25519 AAAA x" --password "\$6\$s\$h" --user-data "I2Nsb3Vk" --uefi')
    [[ "${out}" == '{"name":"t25-m","memory":1024,"cpus":1,"metadata":{"sshkey":"ssh-ed25519 AAAA x","password":"$6$s$h","user_data":"I2Nsb3Vk"},"interfaces":[{"subnet":"sn-a","ip":"10.0.1.10","primary":true},{"subnet":"sn-b","ip":"10.0.2.10","primary":false}],"storage":[{"path":"/v/a.qcow2","dev":"vda"},{"path":"/v/b.qcow2","dev":"sda"}],"uefi":true}' ]] \
        || fail "complet : ${out}"
    out=$(node 'vm_json t21-a --nic sn-a=10.0.1.10')
    [[ "${out}" == '{"name":"t21-a","memory":1024,"cpus":1,"metadata":{},"interfaces":[{"subnet":"sn-a","ip":"10.0.1.10","primary":true}],"storage":[],"uefi":false}' ]] || fail "minimal : ${out}"
    [[ $(node 'vm_json t || echo NON' 2>&1) == '{"name":"t","memory":1024,"cpus":1,"metadata":{},"interfaces":[],"storage":[],"uefi":false}' ]] || fail "vide"
    [[ $(node 'vm_json t --inconnue x || echo NON') == *NON ]] || fail "option inconnue acceptée"
    teardown
}

test_vm_launch_copies_the_system_disk_first_and_waits () {
    setup "vm_launch : disque système vda copié en premier, POST, attente de running"
    local out
    out=$(node 'VOLUMES=/v; TWO_IMAGE=/img; api_state () { echo; }; mkdir () { :; }; cp () { echo "CP $*"; }; api_post () { echo "POST $1 $(jq -c "[.storage[].dev, .interfaces[0].ip]" <<<"$2")"; }; wait_state () { echo "WAIT $*"; }; vm_launch t26-d --nic sn=10.0.0.10 --disk sda=/v/s.qcow2')
    [[ "${out}" == $'CP /img /v/t26-d.qcow2\nPOST vms ["vda","sda","10.0.0.10"]\nWAIT vms/t26-d running' ]] || fail "${out}"
    [[ $(node 'api_state () { echo running; }; cp () { echo CP; }; api_post () { echo POST; }; vm_launch t --nic s=1.2.3.4 && echo OK') == OK ]] || fail "VM en running relancée"
    [[ $(node 'api_state () { echo; }; cp () { echo CP; }; vm_launch t --bad x || echo NON') == *NON ]] || fail "option invalide : disque copié ou POST envoyé"
    teardown
}

test_vm_lease_reads_one_field_of_the_interface_lease () {
    setup "vm_lease : champ du bail de l'interface demandée"
    local stub='vm_ssh () { echo "$3" >&2; printf "%s\n" "ADDRESS=10.0.1.10" "SERVER_ADDRESS=10.0.1.254"; }; '
    [[ $(node "${stub}vm_lease vp 10.0.1.10 SERVER_ADDRESS 2>/dev/null") == 10.0.1.254 ]] || fail "server-id"
    [[ $(node "${stub}vm_lease vp 10.0.1.10 SERVER_ADDRESS ens4 2>&1 >/dev/null") == *'/sys/class/net/ens4/ifindex'* ]] || fail "interface ignorée"
    [[ $(node "${stub}vm_lease vp 10.0.1.10 SERVER 2>/dev/null") == "" ]] || fail "préfixe de champ accepté"
    teardown
}

test_vm_has_ip_matches_the_exact_address () {
    setup "vm_has_ip : adresse exacte, sur n'importe quelle interface"
    local stub='vm_ssh () { echo "3: ens4    inet 10.25.2.10/24 brd 10.25.2.255 scope global ens4"; }; '
    [[ $(node "${stub}vm_has_ip vp 10.25.1.10 10.25.2.10 && echo OUI") == OUI ]] || fail "adresse présente refusée"
    [[ $(node "${stub}vm_has_ip vp 10.25.1.10 10.25.2.1 || echo NON") == NON ]] || fail "préfixe accepté"
    teardown
}

test_wait_reboot_needs_a_new_boot_id () {
    setup "wait_reboot : boot_id différent et lisible exigé"
    [[ $(node 'sleep () { :; }; vm_boot_id () { echo b2; }; wait_reboot vp ip b1 && echo OUI') == OUI ]] || fail "redémarrage non vu"
    [[ $(node 'sleep () { :; }; vm_boot_id () { echo b1; }; wait_reboot vp ip b1 2>&1 || echo NON') == *NON ]] || fail "même boot_id accepté"
    [[ $(node 'sleep () { :; }; vm_boot_id () { return 255; }; wait_reboot vp ip b1 2>&1 || echo NON') == *NON ]] || fail "VM injoignable prise pour redémarrée"
    teardown
}

test_lease_renewed_requires_a_newer_lease_file () {
    setup "lease_renewed : renouvellement demandé, fichier de bail modifié"
    local f
    f=$(mktemp)
    [[ $(node "sleep () { :; }; lease_mtime () { echo x >> '${f}.n'; wc -l < '${f}.n'; }; vm_ssh () { echo \"\$3\" >> '${f}'; }; lease_renewed vp ip && echo OUI") == OUI ]] || fail "renouvellement non vu"
    grep -qx 'sudo networkctl renew ens3' "${f}" || fail "renouvellement non demandé : $(cat "${f}")"
    [[ $(node "sleep () { :; }; lease_mtime () { echo 5; }; vm_ssh () { :; }; lease_renewed vp ip 2>&1 || echo NON") == *NON ]] || fail "bail inchangé accepté"
    rm -f "${f}" "${f}.n"
    teardown
}

test_ssh_denied_needs_a_permission_denied () {
    setup "ssh_denied : refus d'authentification exigé, pas une VM injoignable"
    [[ $(node 'sleep () { :; }; vm_ssh () { echo "syonad@10.0.0.1: Permission denied (publickey)." >&2; return 255; }; ssh_denied vp ip && echo OUI') == OUI ]] || fail "refus non reconnu"
    [[ $(node 'sleep () { :; }; vm_ssh () { echo "connect to host 10.0.0.1 port 22: Connection timed out" >&2; return 255; }; ssh_denied vp ip 2>&1 || echo NON') == *NON ]] || fail "injoignable pris pour un refus"
    [[ $(node 'sleep () { :; }; vm_ssh () { return 0; }; ssh_denied vp ip || echo NON') == *NON ]] || fail "SSH accepté pris pour un refus"
    teardown
}

test_subnet_create_bridge_sends_mode_iface_type_and_interface_ip () {
    setup "subnet_create_bridge : mode bridge, iface_type lan par défaut, interface_ip fournie"
    local stub='api_state () { echo; }; wait_state () { :; }; api_post () { echo "$1 $2"; }; '
    [[ $(node "${stub}subnet_create_bridge sn-t33a vp-t33 10.33.0.0/24 10.33.0.1") == 'subnets {"name":"sn-t33a","vpc":"vp-t33","mode":"bridge","iface_type":"lan","interface_ip":"10.33.0.1","cidr":"10.33.0.0/24"}' ]] \
        || fail "défaut : $(node "${stub}subnet_create_bridge sn-t33a vp-t33 10.33.0.0/24 10.33.0.1")"
    [[ $(node "${stub}subnet_create_bridge sn-x vp-x 10.1.0.0/24 10.1.0.1 vms") == *'"iface_type":"vms"'* ]] || fail "iface_type explicite ignoré"
    [[ $(node 'api_state () { echo running; }; api_post () { echo POST; }; subnet_create_bridge sn-x vp-x 10.1.0.0/24 10.1.0.1 && echo OK') == OK ]] || fail "subnet existant recréé"
    teardown
}

test_ebt_mac_drops_leading_zeros_like_ebtables () {
    setup "ebt_mac : octets sans zéro de tête, en minuscules, comme ebtables -L"
    [[ $(node 'ebt_mac 02:01:00:00:0C:1d') == 2:1:0:0:c:1d ]] || fail "$(node 'ebt_mac 02:01:00:00:0C:1d')"
    [[ $(node 'ebt_mac aa:10:00:ff:01:00') == aa:10:0:ff:1:0 ]] || fail "$(node 'ebt_mac aa:10:00:ff:01:00')"
    teardown
}

test_frames_builds_arp_dhcp_and_marker_frames () {
    setup "frames : requête ARP, DHCP en broadcast (somme IP juste) et trame marquée"
    local arp dhcp mark
    arp=$(node 'FRAMES_DRY=1 FRAMES_MAC=02:00:00:00:00:0a frames arp ens3 10.35.1.10 10.35.1.254 1')
    dhcp=$(node 'FRAMES_DRY=1 FRAMES_MAC=02:00:00:00:00:0a frames dhcp ens3 10.35.1.254 68 1')
    mark=$(node 'FRAMES_DRY=1 frames marker v-x-e 02:01:00:00:0d:ad t35-x 1')
    garp=$(node 'FRAMES_DRY=1 FRAMES_MAC=02:00:00:00:00:0a frames garp ens3 10.35.1.254 1')
    python3 - "${arp}" "${dhcp}" "${mark}" "${garp}" <<'PY' || fail "trames : ${arp} / ${dhcp} / ${mark} / ${garp}"
import socket, struct, sys
arp, dhcp, mark, garp = (bytes.fromhex(x) for x in sys.argv[1:])
assert garp[:6] == b'\xff' * 6 and garp[6:12] == bytes.fromhex('02000000000a'), 'garp mac'
assert garp[12:14] == b'\x08\x06' and garp[20:22] == b'\x00\x02', 'garp type/op'
assert garp[22:28] == bytes.fromhex('02000000000a') and garp[28:32] == socket.inet_aton('10.35.1.254'), 'garp émetteur'
assert garp[38:42] == socket.inet_aton('10.35.1.254'), 'garp cible'
assert arp[:6] == b'\xff' * 6 and arp[6:12] == bytes.fromhex('02000000000a'), 'arp mac'
assert arp[12:14] == b'\x08\x06' and arp[20:22] == b'\x00\x01', 'arp type/op'
assert arp[28:32] == socket.inet_aton('10.35.1.10') and arp[38:42] == socket.inet_aton('10.35.1.254'), 'arp ip'
ip = dhcp[14:34]
s = sum(struct.unpack('!10H', ip)); s = (s >> 16) + (s & 0xffff); s += s >> 16
assert s & 0xffff == 0xffff, 'somme IP'
assert dhcp[12:14] == b'\x08\x00' and dhcp[23] == 17 and dhcp[26:30] == socket.inet_aton('10.35.1.254'), 'dhcp ip'
assert struct.unpack('!HH', dhcp[34:38]) == (67, 68), 'dhcp ports'
assert mark[6:12] == bytes.fromhex('020100000dad'), 'marker mac'
assert mark[12:14] == b'\x88\xb5' and b't35-x' in mark[14:] and len(mark) == 60, 'marker'
PY
    teardown
}

test_vm_frames_ships_the_script_and_its_arguments () {
    setup "vm_frames : le script complet, décodé dans la VM, et ses arguments"
    local cmd
    cmd=$(node 'vm_ssh () { echo "$1|$2|$3"; }; vm_frames vp-t35 10.35.1.10 arp ens3 10.35.1.10 10.35.1.254 5')
    [[ "${cmd}" == "vp-t35|10.35.1.10|echo "*" | base64 -d > /tmp/frames.py && sudo python3 /tmp/frames.py arp ens3 10.35.1.10 10.35.1.254 5" ]] || fail "commande : ${cmd:0:80}…"
    local b64=${cmd#*echo }
    b64=${b64%% |*}
    [[ "$(printf '%s' "${b64}" | base64 -d)" == "$(node 'frames_py')" ]] || fail "script altéré en transit"
    teardown
}

test_evpn_mac_remote_requires_type_and_vtep () {
    setup "evpn_mac_remote : MAC distante, bon VTEP, casse ignorée"
    local json='{"macs":{"00:22:33:00:00:0b":{"type":"remote","remoteVtep":"192.168.14.12"},"00:22:33:00:00:0a":{"type":"local","intf":"tap1"}}}'
    local stub="vtysh () { [ \"\$*\" = '-c show evpn mac vni 4101 json' ] && echo '${json}'; }; "
    [[ $(node "${stub}evpn_mac_remote 4101 00:22:33:00:00:0B 192.168.14.12 && echo OUI") == OUI ]] || fail "MAC distante refusée"
    [[ $(node "${stub}evpn_mac_remote 4101 00:22:33:00:00:0b 192.168.14.11 || echo NON") == NON ]] || fail "mauvais VTEP accepté"
    [[ $(node "${stub}evpn_mac_remote 4101 00:22:33:00:00:0a 192.168.14.12 || echo NON") == NON ]] || fail "MAC locale acceptée"
    [[ $(node "${stub}evpn_mac_remote 4101 00:22:33:00:00:0c 192.168.14.12 || echo NON") == NON ]] || fail "MAC absente acceptée"
    [[ $(node "${stub}evpn_mac_remote 4102 00:22:33:00:00:0b 192.168.14.12 || echo NON") == NON ]] || fail "mauvaise VNI acceptée"
    teardown
}

test_vm_neigh_mac_reads_the_resolved_mac () {
    setup "vm_neigh_mac : MAC résolue, rien si incomplète"
    [[ $(node 'vm_ssh () { echo "10.41.1.11 dev ens3 lladdr 00:22:33:00:00:0b REACHABLE"; }; vm_neigh_mac v 10.41.1.10 10.41.1.11') == 00:22:33:00:00:0b ]] || fail "MAC non lue"
    [[ -z $(node 'vm_ssh () { echo "10.41.1.11 dev ens3 INCOMPLETE"; }; vm_neigh_mac v 10.41.1.10 10.41.1.11') ]] || fail "entrée incomplète lue comme une MAC"
    teardown
}

test_watchdog_l3_quiet_ignores_route_alerts_only () {
    setup "watchdog_l3_quiet : alertes BGP, L3VNI et interfaces L3 refusées, alertes de route ignorées"
    local route='{"msg":"watchdog: inconsistency detected","kind":"vpc","name":"vp-t53","problem":"route 10.53.1.99/32 exported in vrf-t53 without a running owner"}'
    local bgp='{"msg":"watchdog: inconsistency detected","kind":"bgp","name":"10.255.255.1","problem":"session down"}'
    local vni='{"msg":"watchdog: inconsistency detected","kind":"vpc","name":"vp-t53","problem":"l3vni 5300 is not Up"}'
    local link='{"msg":"watchdog: inconsistency detected","kind":"vpc","name":"vp-t53","problem":"interface brl3-t53 missing"}'
    local other='{"msg":"command failed","kind":"bgp"}'
    [[ $(node "journalctl () { echo '${route}'; echo '${other}'; }; watchdog_l3_quiet && echo OUI") == OUI ]] || fail "alerte de route ou message hors watchdog pris pour une alerte L3"
    local a
    for a in "${bgp}" "${vni}" "${link}"; do
        [[ $(node "journalctl () { echo '${a}'; }; watchdog_l3_quiet || echo NON" | tail -n 1) == NON ]] || fail "alerte acceptée : ${a}"
    done
    [[ $(node "journalctl () { echo \"\$*\" >&2; }; watchdog_l3_quiet 90 2>&1") == "-u agent --since -90s --no-pager -o cat" ]] || fail "fenêtre : $(node "journalctl () { echo \"\$*\" >&2; }; watchdog_l3_quiet 90 2>&1")"
    teardown
}

test_vrrp_expected_master_takes_the_highest_priority_then_address () {
    setup "vrrp_expected_master : plus haute priorité, puis plus haute adresse"
    [[ $(node 'vrrp_expected_master sw1:200:192.168.14.2 sw2:190:192.168.14.3') == sw1 ]] || fail "200 contre 190"
    [[ $(node 'vrrp_expected_master sw1:190:192.168.14.2 sw2:200:192.168.14.3') == sw2 ]] || fail "190 contre 200"
    [[ $(node 'vrrp_expected_master sw1:100:192.168.14.2 sw2:100:192.168.14.3') == sw2 ]] || fail "égalité : adresse la plus haute"
    [[ $(node 'vrrp_expected_master sw1:100:192.168.14.20 sw2:100:192.168.14.3') == sw1 ]] || fail "égalité : comparaison numérique des octets"
    [[ $(node 'vrrp_expected_master sw1:99:192.168.14.2 sw2:100:192.168.14.3') == sw2 ]] || fail "99 contre 100"
    teardown
}

test_log_gap_measures_the_longest_silence_in_a_window () {
    setup "log_gap : plus long silence, en dixièmes, dans une fenêtre"
    local log
    log=$(mktemp)
    printf '%s\n' '[100.0] 64 bytes from x: icmp_seq=1' '[100.2] 64 bytes from x: icmp_seq=2' '[103.0] 64 bytes from x: icmp_seq=3' \
        '[103.2] 64 bytes from x: icmp_seq=4' '[110.0] 64 bytes from x: icmp_seq=5' '[110.2] 64 bytes from x: icmp_seq=6' 'no answer yet for icmp_seq=7' > "${log}"
    [[ $(node "log_gap '${log}'") == 68 ]] || fail "toute la trace : $(node "log_gap '${log}'")"
    [[ $(node "log_gap '${log}' 0 105") == 28 ]] || fail "avant 105 : $(node "log_gap '${log}' 0 105")"
    [[ $(node "log_gap '${log}' 105 0") == 68 ]] || fail "silence à cheval sur 105 : $(node "log_gap '${log}' 105 0")"
    [[ $(node "log_gap '${log}' 110.1 0") == 2 ]] || fail "après 110.1 : $(node "log_gap '${log}' 110.1 0")"
    rm -f "${log}"
    teardown
}

test_ebt_matchers_read_the_ebtables_listing () {
    setup "ebt_confined / ebt_arp_drop / ebt_dhcp_drop : lecture de ebtables -L"
    local stub='ebt_rules () { printf "%s\n" "-s 2:1:0:0:c:27 -i v-x-i -j DROP" "-s 2:1:0:0:c:27 -o v-x-i -j DROP" "-p ARP -o v-x-i --arp-op Request --arp-ip-dst 10.2.1.254 -j DROP" "-p IPv4 -o v-x-i --ip-src 10.2.1.254 --ip-proto udp --ip-sport 67:68 --ip-dport 67:68 -j DROP" | grep -- "$2"; }; '
    [[ $(node "${stub}ebt_confined ns v-x-i 02:01:00:00:0c:27") == 2 ]] || fail "confinement : $(node "${stub}ebt_confined ns v-x-i 02:01:00:00:0c:27")"
    [[ $(node "${stub}ebt_confined ns v-x-i 02:01:00:00:0c:28") == 0 ]] || fail "autre MAC comptée"
    [[ $(node "${stub}ebt_arp_drop ns v-x-i 10.2.1.254 && echo OUI") == OUI ]] || fail "ARP non lu"
    [[ $(node "${stub}ebt_arp_drop ns v-x-i 10.2.1.25 || echo NON") == NON ]] || fail "10.2.1.25 pris pour 10.2.1.254"
    [[ $(node "${stub}ebt_dhcp_drop ns v-x-i 10.2.1.254 && echo OUI") == OUI ]] || fail "DHCP non lu"
    [[ $(node "${stub}ebt_dhcp_drop ns v-y-i 10.2.1.254 || echo NON") == NON ]] || fail "autre interface acceptée"
    [[ $(node 'ebt_rules () { echo "-p IPv4 -o v-z-i --ip-src 10.2.1.254 --ip-proto udp --ip-dport 53 -j DROP"; }; ebt_dhcp_drop ns v-z-i 10.2.1.254 || echo NON') == NON ]] || fail "règle UDP hors DHCP prise pour un blocage DHCP"
    teardown
}

test_wait_bgp_paths_waits_for_the_expected_count () {
    setup "wait_bgp_paths : nombre de chemins attendu, erreur explicite sinon"
    [[ $(node 'bgp_paths () { echo 0; }; sleep () { :; }; wait_bgp_paths vp-t53 10.53.1.50/32 0 && echo OUI') == OUI ]] || fail "0 attendu refusé"
    local out
    out=$(node 'bgp_paths () { echo 1; }; sleep () { :; }; wait_bgp_paths vp-t53 10.53.1.50/32 0 || echo NON')
    [[ $(tail -n 1 <<< "${out}") == NON ]] || fail "1 chemin accepté pour 0"
    grep -q 'vrf-t53 10.53.1.50/32 : 1 chemin(s) BGP, 0 attendu(s)' <<< "${out}" || fail "message : ${out}"
    teardown
}

test_section_unchanged_names_the_changed_entries () {
    setup "section_unchanged : une section comparée, entrées en cause nommées"
    local dir out
    dir=$(mktemp -d)
    printf '%s\n' '## liens' lo '## fichiers' /run/two/a /run/two/b '## unités' u1 > "${dir}/avant"
    [[ $(node "state_section '${dir}/avant' fichiers | tr '\n' ' '") == "/run/two/a /run/two/b " ]] || fail "section : $(node "state_section '${dir}/avant' fichiers")"
    [[ $(node "host_state () { printf '%s\n' '## liens' lo br-x '## fichiers' /run/two/a /run/two/b '## unités' u1; }; section_unchanged '${dir}/avant' fichiers && echo OUI") == OUI ]] || fail "autre section prise en compte"
    out=$(node "host_state () { printf '%s\n' '## liens' lo '## fichiers' /run/two/a /run/two/c '## unités' u1; }; section_unchanged '${dir}/avant' fichiers || echo NON")
    [[ $(tail -n 1 <<< "${out}") == NON ]] || fail "différence acceptée"
    grep -q '^fichiers en cause : -/run/two/b +/run/two/c $' <<< "${out}" || fail "message : ${out}"
    rm -rf "${dir}"
    teardown
}

test_evpn_mac_json_shape_requires_remote_vtep () {
    setup "evpn_mac_json_shape : MAC distante avec remoteVtep, champs listés sinon"
    local ok='{"numMacs":2,"macs":{"00:22:33:00:00:0a":{"type":"local","intf":"tap1"},"00:22:33:00:00:0b":{"type":"remote","remoteVtep":"192.168.14.12"}}}'
    local ko='{"macs":{"00:22:33:00:00:0b":{"type":"remote","vtep":"192.168.14.12"}}}'
    [[ $(node "vtysh () { echo '${ok}'; }; evpn_mac_json_shape 4101 && echo OUI") == OUI ]] || fail "forme attendue refusée"
    [[ $(node "vtysh () { echo '${ko}'; }; evpn_mac_json_shape 4101 || echo NON") == *'["type","vtep"]'*NON ]] || fail "champs non listés : $(node "vtysh () { echo '${ko}'; }; evpn_mac_json_shape 4101 || echo NON")"
    [[ $(node "vtysh () { echo '{\"macs\":{}}'; }; evpn_mac_json_shape 4101 || echo NON") == *NON ]] || fail "aucune MAC distante acceptée"
    teardown
}

for T in $(declare -F | awk '{print $3}' | grep '^test_'); do
    "${T}"
done
echo "${PASS} réussi(s), ${FAIL} échec(s)"
[[ "${FAIL}" -eq 0 ]]
