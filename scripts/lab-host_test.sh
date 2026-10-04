#!/usr/bin/env bash

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lab-host.sh"
PASS=0
FAIL=0
SECRET="secret-de-test-a1b2c3"
PROJECT="11111111-1111-1111-1111-111111111111"
OTHER_PROJECT="22222222-2222-2222-2222-222222222222"
HOURLY_ID="offer-hourly-0001"
MONTHLY_ID="offer-monthly-0001"

fail () { echo "FAIL: ${CURRENT}: ${1}"; FAIL=$(( FAIL + 1 )); CASE_OK=0; }

write_fakes () {
    local DIR="${1}"
    mkdir -p "${DIR}/bin"
    cat > "${DIR}/bin/curl" <<'PY'
#!/usr/bin/env python3
import json, os, sys, time, urllib.parse

d = os.environ["FAKE_DIR"]
args = sys.argv[1:]
with open(os.path.join(d, "argv.log"), "a") as f:
    f.write(json.dumps(args) + "\n")

method, body, headers, url = "GET", None, [], None
i = 0
while i < len(args):
    a = args[i]
    if a == "-X":
        method = args[i + 1]; i += 2; continue
    if a == "-H":
        h = args[i + 1]
        if h.startswith("@"):
            with open(h[1:]) as f:
                headers += [l.strip() for l in f if l.strip()]
        else:
            headers.append(h)
        i += 2; continue
    if a == "--data":
        body = args[i + 1]; i += 2; continue
    if a in ("-w", "--max-time", "--connect-timeout"):
        i += 2; continue
    if a.startswith("-"):
        i += 1; continue
    url = a; i += 1

state_path = os.path.join(d, "state.json")
with open(state_path) as f:
    st = json.load(f)

def reply(code, obj):
    with open(state_path, "w") as f:
        json.dump(st, f)
    sys.stdout.write(json.dumps(obj) + "\n" + str(code))
    sys.exit(0)

u = urllib.parse.urlparse(url)
path = u.path
with open(os.path.join(d, "calls.log"), "a") as f:
    f.write(method + " " + path + ("?" + u.query if u.query else "") + "\n")

if "X-Auth-Token: " + os.environ["FAKE_SECRET"] not in headers:
    reply(401, {"message": "unauthorized"})

zone = "/baremetal/v1/zones/fr-par-1"
if method == "GET" and path == zone + "/offers":
    reply(200, {"offers": st["offers"], "total_count": len(st["offers"])})
if method == "GET" and path == zone + "/os":
    reply(200, {"os": st["os"], "total_count": len(st["os"])})
if method == "GET" and path == "/iam/v1alpha1/ssh-keys":
    reply(200, {"ssh_keys": st["keys"], "total_count": len(st["keys"])})
if method == "GET" and path == zone + "/servers":
    if st["list_errors"] > 0:
        st["list_errors"] -= 1
        reply(503, {"message": "service unavailable"})
    listed = [v for v in st["servers"].values() if not v.get("hidden")]
    reply(200, {"servers": listed, "total_count": len(listed)})
if method == "POST" and path == zone + "/servers":
    req = json.loads(body)
    with open(os.path.join(d, "create.json"), "w") as f:
        json.dump(req, f)
    st["seq"] += 1
    sid = "srv-%d" % st["seq"]
    st["servers"][sid] = {"id": sid, "name": req["name"], "project_id": req["project_id"],
                          "tags": req["tags"], "offer_name": "EM-B212X-SSD", "status": "delivering",
                          "install": {"status": "to_install"}, "ips": [], "created_at": "now",
                          "hidden": st["hide_from_list"]}
    if st["post_fail_after_create"]:
        reply(502, {"message": "bad gateway"})
    if st["post_no_id"]:
        reply(200, {})
    reply(200, st["servers"][sid])
if path.startswith(zone + "/servers/"):
    sid = path.rsplit("/", 1)[1]
    if sid not in st["servers"]:
        reply(404, {"message": "not found"})
    srv = st["servers"][sid]
    if method == "GET" and st["get_errors"] > 0:
        st["get_errors"] -= 1
        reply(503, {"message": "service unavailable"})
    if method == "GET" and srv["status"] == "deleting":
        if st["linger"] > 0:
            st["linger"] -= 1
            reply(200, srv)
        del st["servers"][sid]
        reply(404, {"message": "not found"})
    if method == "GET" and st["vanish_after"] == 0:
        del st["servers"][sid]
        reply(404, {"message": "not found"})
    if method == "GET" and st["vanish_after"] > 0:
        st["vanish_after"] -= 1
    if method == "GET":
        if st["progress"]:
            status, install = st["progress"].pop(0)
            srv["status"] = status
            srv["install"]["status"] = install
            if status == "ready":
                srv["ips"] = [{"address": "2001:db8::1", "version": "IPv6"},
                              {"address": "203.0.113.7", "version": "IPv4"}]
        reply(200, srv)
    if method == "DELETE":
        if time.time() < st["refuse_until"]:
            reply(409, {"message": "server is installing"})
        if st["delete_refusals"] > 0:
            st["delete_refusals"] -= 1
            reply(409, {"message": "server is installing"})
        if st["linger"] > 0:
            srv["status"] = "deleting"
        else:
            del st["servers"][sid]
        reply(200, srv)
reply(500, {"message": "route inconnue du faux serveur : " + method + " " + path})
PY
    cat > "${DIR}/bin/ssh" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${FAKE_DIR}/ssh.log"
printf 'key=%s\n' "${SCW_SECRET_KEY:-}" >> "${FAKE_DIR}/ssh-env.log"
case "$*" in
    *BatchMode=yes*)
        N=$(cat "${FAKE_DIR}/probe_fails" 2>/dev/null || echo 0)
        if [ "${N}" -gt 0 ]; then
            echo $(( N - 1 )) > "${FAKE_DIR}/probe_fails"
            exit 255
        fi
        exit 0 ;;
    *"bash -s"*)
        cat > "${FAKE_DIR}/prepare.sh"
        exit "${FAKE_PREPARE_RC:-0}" ;;
    *"cat > "*)
        N=$(ls "${FAKE_DIR}" | grep -c '^pushed\.')
        cat > "${FAKE_DIR}/pushed.$(( N + 1 ))"
        exit "${FAKE_PUSH_RC:-0}" ;;
esac
[ -n "${FAKE_SSH_SLEEP:-}" ] && sleep "${FAKE_SSH_SLEEP}"
exit "${FAKE_SSH_RC:-0}"
SH
    cat > "${DIR}/bin/go" <<'SH'
#!/usr/bin/env bash
printf '%s|%s|CGO_ENABLED=%s GOOS=%s GOARCH=%s\n' "${PWD}" "$*" "${CGO_ENABLED:-}" "${GOOS:-}" "${GOARCH:-}" >> "${FAKE_DIR}/go.log"
[ -n "${FAKE_GO_FAIL:-}" ] && exit 1
while [ $# -gt 0 ]; do
    [ "${1}" = "-o" ] && printf 'binaire-lab\n' > "${2}"
    shift
done
exit 0
SH
    chmod +x "${DIR}/bin/curl" "${DIR}/bin/ssh" "${DIR}/bin/go"
}

base_state () {
    jq -n --arg h "${HOURLY_ID}" --arg m "${MONTHLY_ID}" --arg p "${PROJECT}" --arg o "${OTHER_PROJECT}" '{
      seq: 0, progress: [], delete_refusals: 0, linger: 0, get_errors: 0, list_errors: 0,
      hide_from_list: false, refuse_until: 0, post_fail_after_create: false, post_no_id: false, vanish_after: -1,
      offers: [
        {id: $m, name: "EM-B212X-SSD", subscription_period: "monthly", enable: true, stock: "available",
         price_per_hour: null, price_per_month: {currency_code: "EUR", units: 115, nanos: 990000000},
         fee: {currency_code: "EUR", units: 115, nanos: 990000000}},
        {id: "offer-other", name: "EM-A610R-NVMe", subscription_period: "hourly", enable: true, stock: "available",
         price_per_hour: {currency_code: "EUR", units: 0, nanos: 110000000}, fee: {currency_code: "EUR", units: 0, nanos: 0}},
        {id: $h, name: "EM-B212X-SSD", subscription_period: "hourly", enable: true, stock: "available",
         price_per_hour: {currency_code: "EUR", units: 0, nanos: 321000000}, fee: {currency_code: "EUR", units: 0, nanos: 0}}
      ],
      os: [
        {id: "os-deb11", name: "Debian", version: "11 (Bullseye)", enabled: true, allowed: true, user: {default_value: "debian"}},
        {id: "os-deb12", name: "Debian", version: "12 (Bookworm)", enabled: true, allowed: true, user: {default_value: "debian"}},
        {id: "os-deb13", name: "Debian", version: "13 (Trixie)", enabled: true, allowed: true, user: {default_value: "debian"}},
        {id: "os-ubu", name: "Ubuntu", version: "24.04 LTS (Noble Numbat)", enabled: true, allowed: true, user: {default_value: "ubuntu"}}
      ],
      keys: [{id: "key-1", disabled: false}, {id: "key-off", disabled: true}],
      servers: {
        "foreign-1": {id: "foreign-1", name: "prod-db", project_id: $o, tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []},
        "foreign-2": {id: "foreign-2", name: "autre", project_id: $p, tags: ["autre"], status: "ready", install: {status: "completed"}, ips: []}
      }
    }'
}

setup () {
    CURRENT="${1}"
    CASE_OK=1
    WORK=$(mktemp -d "${TMPDIR:-/tmp}/labhost.XXXXXX")
    write_fakes "${WORK}"
    base_state > "${WORK}/state.json"
    : > "${WORK}/calls.log"
    : > "${WORK}/argv.log"
    : > "${WORK}/ssh.log"
    : > "${WORK}/ssh-env.log"
}

mutate () {
    local TMP
    TMP=$(jq "${1}" "${WORK}/state.json") && printf '%s\n' "${TMP}" > "${WORK}/state.json"
}

lab_env () {
    local EXEC=""
    [[ "${1}" == "exec" ]] && { EXEC="exec"; shift; }
    ${EXEC} env -i PATH="${WORK}/bin:${PATH}" HOME="${WORK}" TMPDIR="${TMPDIR:-/tmp}" \
        FAKE_DIR="${WORK}" FAKE_SECRET="${SECRET}" FAKE_SSH_RC="${FAKE_SSH_RC:-0}" \
        FAKE_SSH_SLEEP="${FAKE_SSH_SLEEP:-}" FAKE_PREPARE_RC="${FAKE_PREPARE_RC:-0}" \
        FAKE_PUSH_RC="${FAKE_PUSH_RC:-0}" FAKE_GO_FAIL="${FAKE_GO_FAIL:-}" \
        SCW_API_URL="https://api.example.invalid" SCW_SECRET_KEY="${SECRET}" \
        SCW_DEFAULT_PROJECT_ID="${PROJECT}" LAB_POLL_INTERVAL="${LAB_POLL_INTERVAL:-0}" \
        LAB_INSTALL_TIMEOUT="${LAB_INSTALL_TIMEOUT:-60}" LAB_DELETE_TIMEOUT="${LAB_DELETE_TIMEOUT:-60}" \
        LAB_SSH_TIMEOUT="${LAB_SSH_TIMEOUT:-60}" \
        "$@"
}

run_lab () {
    lab_env bash "${SCRIPT}" "$@" > "${WORK}/out.log" 2>&1
}

remaining () {
    jq -r '.servers | keys | sort | join(",")' "${WORK}/state.json"
}

teardown () {
    [[ "${CASE_OK}" -eq 1 ]] && PASS=$(( PASS + 1 )) && echo "ok:   ${CURRENT}"
    [[ "${CASE_OK}" -eq 1 ]] || { echo "      sortie :"; sed 's/^/      | /' "${WORK}/out.log"; }
    rm -rf "${WORK}"
}

test_plan_picks_hourly_offer_even_when_monthly_is_listed_first () {
    setup "plan : offre horaire choisie même si la mensuelle est listée d'abord"
    run_lab plan || fail "code de sortie $?"
    grep -q "\"offer_id\": \"${HOURLY_ID}\"" "${WORK}/out.log" || fail "offre horaire absente de la requête"
    grep -q "${MONTHLY_ID}" "${WORK}/out.log" && fail "offre mensuelle présente dans la sortie"
    grep -q "^POST" "${WORK}/calls.log" && fail "plan a créé quelque chose"
    teardown
}

test_plan_selects_debian_12_only () {
    setup "plan : Debian 12 et rien d'autre"
    run_lab plan || fail "code de sortie $?"
    grep -q '"os_id": "os-deb12"' "${WORK}/out.log" || fail "os-deb12 non retenu"
    teardown
}

test_plan_passes_only_enabled_ssh_keys () {
    setup "plan : seules les clés SSH actives sont transmises"
    run_lab plan || fail "code de sortie $?"
    grep -q '"key-1"' "${WORK}/out.log" || fail "key-1 absente"
    grep -q '"key-off"' "${WORK}/out.log" && fail "clé désactivée transmise"
    teardown
}

test_refuses_offer_with_setup_fee () {
    setup "refus d'une offre horaire avec frais de mise en service"
    mutate '(.offers[] | select(.id == "'"${HOURLY_ID}"'") | .fee.units) = 30'
    run_lab up && fail "up a réussi malgré des frais"
    grep -q "^POST" "${WORK}/calls.log" && fail "serveur créé malgré des frais"
    teardown
}

test_refuses_when_hourly_offer_missing () {
    setup "refus quand seule l'offre mensuelle existe"
    mutate 'del(.offers[] | select(.id == "'"${HOURLY_ID}"'"))'
    run_lab up && fail "up a réussi sans offre horaire"
    grep -q "^POST" "${WORK}/calls.log" && fail "serveur créé sur une offre mensuelle"
    teardown
}

test_refuses_ambiguous_os () {
    setup "refus d'un OS ambigu"
    mutate '.os += [{id: "os-deb12b", name: "Debian", version: "12 (Bookworm)", enabled: true, allowed: true}]'
    run_lab plan && fail "plan a réussi avec deux Debian 12"
    teardown
}

test_refuses_out_of_stock () {
    setup "refus d'une offre en rupture"
    mutate '(.offers[] | select(.id == "'"${HOURLY_ID}"'") | .stock) = "empty"'
    run_lab up && fail "up a réussi en rupture de stock"
    grep -q "^POST" "${WORK}/calls.log" && fail "serveur créé en rupture de stock"
    teardown
}

test_up_creates_and_waits_for_installation () {
    setup "up : création puis attente de la fin d'installation"
    mutate '.progress = [["delivering","to_install"],["ready","installing"],["ready","completed"]]'
    run_lab up || fail "code de sortie $?"
    [[ $(jq -r '.offer_id' "${WORK}/create.json") == "${HOURLY_ID}" ]] || fail "offre créée incorrecte"
    [[ $(jq -r '.tags | join(",")' "${WORK}/create.json") == "two-lab" ]] || fail "tag absent"
    [[ $(jq -r '.project_id' "${WORK}/create.json") == "${PROJECT}" ]] || fail "projet incorrect"
    [[ $(cat "${WORK}/.cache/two-lab/ip" 2>/dev/null) == "203.0.113.7" ]] || fail "IPv4 non retenue"
    [[ $(cat "${WORK}/.cache/two-lab/user" 2>/dev/null) == "debian" ]] || fail "utilisateur non retenu"
    teardown
}

test_up_refuses_when_lab_server_exists () {
    setup "up : refus si un serveur de lab existe déjà"
    mutate '.servers["srv-old"] = {id: "srv-old", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    run_lab up && fail "up a réussi"
    grep -q "^POST" "${WORK}/calls.log" && fail "second serveur créé"
    teardown
}

test_secret_never_in_argv () {
    setup "la clé d'API n'apparaît jamais dans les arguments de curl"
    mutate '.progress = [["ready","completed"]]'
    run_lab up || fail "code de sortie $?"
    grep -q "${SECRET}" "${WORK}/argv.log" && fail "secret visible dans argv"
    grep -q "${SECRET}" "${WORK}/out.log" && fail "secret affiché"
    teardown
}

test_down_deletes_only_lab_servers_of_project () {
    setup "down : ne supprime que les serveurs de lab du projet"
    mutate '.servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    run_lab down || fail "code de sortie $?"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_down_retries_refused_deletion () {
    setup "down : réessaie une suppression refusée"
    mutate '.delete_refusals = 2 | .servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "installing"}, ips: []}'
    run_lab down || fail "code de sortie $?"
    [[ $(grep -c "^DELETE" "${WORK}/calls.log") -eq 3 ]] || fail "$(grep -c "^DELETE" "${WORK}/calls.log") DELETE au lieu de 3"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_down_fails_loudly_when_deletion_keeps_failing () {
    setup "down : échec bruyant si la suppression n'aboutit jamais"
    mutate '.delete_refusals = 1000 | .servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "installing"}, ips: []}'
    LAB_DELETE_TIMEOUT=1 run_lab down && fail "down a réussi"
    grep -q "TOUJOURS FACTURÉ" "${WORK}/out.log" || fail "pas d'alerte de facturation"
    teardown
}

test_down_waits_until_server_is_gone () {
    setup "down : attend que le serveur ait disparu"
    mutate '.linger = 3 | .servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    run_lab down || fail "code de sortie $?"
    [[ $(grep -c "^GET /baremetal/v1/zones/fr-par-1/servers/srv-lab$" "${WORK}/calls.log") -ge 4 ]] || fail "disparition non attendue"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_down_fails_loudly_when_server_never_disappears () {
    setup "down : échec bruyant si le serveur ne disparaît jamais"
    mutate '.linger = 100000 | .servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    LAB_DELETE_TIMEOUT=1 run_lab down && fail "down a réussi"
    grep -q "TOUJOURS FACTURÉ" "${WORK}/out.log" || fail "pas d'alerte de facturation"
    teardown
}

test_down_does_not_mistake_api_error_for_disappearance () {
    setup "down : une erreur de l'API n'est pas prise pour une disparition"
    mutate '.linger = 100000 | .get_errors = 1 | .servers["srv-lab"] = {id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    LAB_DELETE_TIMEOUT=1 run_lab down && fail "down a réussi alors que le serveur existe encore"
    grep -q "TOUJOURS FACTURÉ" "${WORK}/out.log" || fail "pas d'alerte de facturation"
    teardown
}

test_session_deletes_server_after_remote_command () {
    setup "session : serveur supprimé après la commande distante"
    mutate '.progress = [["ready","completed"]]'
    run_lab session uname -a || fail "code de sortie $?"
    grep -q "uname -a" "${WORK}/ssh.log" || fail "commande distante non lancée"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_session_deletes_server_and_propagates_failure_of_remote_command () {
    setup "session : serveur supprimé et code d'échec propagé"
    mutate '.progress = [["ready","completed"]]'
    FAKE_SSH_RC=7 run_lab session false
    [[ $? -eq 7 ]] || fail "code de sortie différent de 7"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_session_deletes_server_when_installation_fails () {
    setup "session : serveur supprimé quand l'installation échoue"
    mutate '.progress = [["ready","installing"],["ready","error"]]'
    run_lab session true && fail "session a réussi"
    grep -q "true" "${WORK}/ssh.log" && fail "commande lancée sur un serveur en échec"
    grep -q "état terminal en échec : ready/error" "${WORK}/out.log" || fail "échec d'installation non reconnu comme terminal"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_session_deletes_server_when_installation_times_out () {
    setup "session : serveur supprimé quand l'installation n'aboutit pas"
    mutate '.progress = [range(0; 100000) | ["delivering","to_install"]]'
    LAB_INSTALL_TIMEOUT=1 run_lab session true && fail "session a réussi"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_session_does_not_touch_existing_lab_server () {
    setup "session : ne supprime pas un serveur de lab lancé à côté"
    mutate '.servers["srv-old"] = {id: "srv-old", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
    run_lab session true && fail "session a réussi"
    grep -q "^DELETE" "${WORK}/calls.log" && fail "suppression lancée"
    [[ $(remaining) == "foreign-1,foreign-2,srv-old" ]] || fail "restants : $(remaining)"
    teardown
}

test_missing_credentials_are_refused_before_any_call () {
    setup "refus sans clé d'API, avant tout appel"
    env -i PATH="${WORK}/bin:${PATH}" HOME="${WORK}" FAKE_DIR="${WORK}" SCW_DEFAULT_PROJECT_ID="${PROJECT}" bash "${SCRIPT}" plan > "${WORK}/out.log" 2>&1 \
        && fail "plan a réussi sans clé"
    [[ -s "${WORK}/calls.log" ]] && fail "appel réseau sans clé"
    teardown
}

LAB_SERVER='{id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "completed"}, ips: []}'
INSTALLING_LAB_SERVER='{id: "srv-lab", name: "two-lab", project_id: "'"${PROJECT}"'", tags: ["two-lab"], status: "ready", install: {status: "installing"}, ips: []}'

test_down_keeps_retrying_while_server_is_installing () {
    setup "down : réessaie au-delà du délai tant que le serveur s'installe"
    mutate ".servers[\"srv-lab\"] = ${INSTALLING_LAB_SERVER} | .refuse_until = (now + 5)"
    LAB_DELETE_TIMEOUT=2 LAB_POLL_INTERVAL=0.2 run_lab down || fail "code de sortie $?"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    grep -q "pendant la livraison ou l'installation" "${WORK}/out.log" || fail "installation non reconnue"
    teardown
}

test_down_gives_up_on_installing_server_after_hard_deadline () {
    local RC
    setup "down : abandonne quand même après délai d'installation + délai de suppression"
    mutate ".servers[\"srv-lab\"] = ${INSTALLING_LAB_SERVER} | .refuse_until = (now + 100000)"
    LAB_DELETE_TIMEOUT=2 LAB_INSTALL_TIMEOUT=2 LAB_POLL_INTERVAL=0.2 lab_env timeout 30 bash "${SCRIPT}" down > "${WORK}/out.log" 2>&1
    RC=$?
    [[ "${RC}" -ne 124 ]] || fail "aucun plafond : down tourne encore après 30 s"
    [[ "${RC}" -ne 0 ]] || fail "down a réussi"
    grep -q "TOUJOURS FACTURÉ" "${WORK}/out.log" || fail "pas d'alerte de facturation"
    teardown
}

test_down_retries_listing_servers () {
    setup "down : réessaie la liste des serveurs"
    mutate ".servers[\"srv-lab\"] = ${LAB_SERVER} | .list_errors = 2"
    run_lab down || fail "code de sortie $?"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_down_fails_loudly_when_listing_never_works () {
    setup "down : échec bruyant si la liste reste indisponible"
    mutate ".servers[\"srv-lab\"] = ${LAB_SERVER} | .list_errors = 100000"
    LAB_DELETE_TIMEOUT=1 run_lab down && fail "down a réussi"
    grep -q "TOUJOURS FACTURÉ" "${WORK}/out.log" || fail "pas d'alerte de facturation"
    teardown
}

test_session_deletes_created_server_even_if_missing_from_list () {
    setup "session : supprime le serveur créé même absent de la liste"
    mutate '.hide_from_list = true | .progress = [["ready","completed"]]'
    run_lab session true || fail "code de sortie $?"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_every_api_call_has_timeouts () {
    setup "chaque appel à l'API a un délai de connexion et un délai total"
    mutate '.progress = [["ready","completed"]]'
    run_lab up || fail "code de sortie $?"
    [[ -s "${WORK}/argv.log" ]] || fail "aucun appel"
    [[ $(jq -s '[.[] | select((index("--max-time") | not) or (index("--connect-timeout") | not))] | length' "${WORK}/argv.log") -eq 0 ]] \
        || fail "appel sans délai"
    teardown
}

test_up_reports_uncertain_creation_when_post_fails () {
    setup "up : création incertaine signalée quand le POST échoue"
    mutate '.post_fail_after_create = true'
    run_lab up && fail "up a réussi"
    grep -q "création incertaine" "${WORK}/out.log" || fail "pas de message de création incertaine"
    grep -q "un serveur de lab existe bien : srv-1" "${WORK}/out.log" || fail "serveur créé non signalé"
    teardown
}

test_session_deletes_server_when_post_fails_after_creation () {
    setup "session : serveur supprimé quand le POST échoue après création"
    mutate '.post_fail_after_create = true'
    run_lab session true && fail "session a réussi"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_up_refuses_response_without_id () {
    setup "up : réponse de création sans identifiant refusée"
    mutate '.post_no_id = true'
    run_lab up && fail "up a réussi"
    grep -q "création incertaine" "${WORK}/out.log" || fail "pas de message de création incertaine"
    grep -q "servers/null" "${WORK}/calls.log" && fail "appel sur un identifiant null"
    teardown
}

test_up_stops_at_once_when_server_vanishes () {
    setup "up : arrêt immédiat si le serveur disparaît pendant l'installation"
    mutate '.vanish_after = 1 | .progress = [["delivering","to_install"]]'
    LAB_INSTALL_TIMEOUT=5 run_lab up && fail "up a réussi"
    grep -q "erreur définitive HTTP 404" "${WORK}/out.log" || fail "404 non traité comme définitif"
    teardown
}

test_up_stops_at_once_when_server_is_stopped_during_installation () {
    setup "up : arrêt immédiat si le serveur passe à l'arrêt pendant l'installation"
    mutate '.progress = [["delivering","to_install"],["stopped","installing"]]'
    LAB_INSTALL_TIMEOUT=5 run_lab up && fail "up a réussi"
    grep -q "état terminal en échec : stopped/installing" "${WORK}/out.log" || fail "arrêt non reconnu comme terminal"
    teardown
}

test_session_cleanup_survives_a_second_signal () {
    local PID RC I
    setup "session : un second signal n'interrompt pas le nettoyage"
    mutate '.progress = [["ready","completed"]] | .linger = 15'
    set -m
    FAKE_SSH_SLEEP=5 LAB_POLL_INTERVAL=0.2 lab_env exec bash "${SCRIPT}" session sleep-long > "${WORK}/out.log" 2>&1 &
    PID=$!
    set +m
    for I in $(seq 1 100); do
        grep -q "sleep-long" "${WORK}/ssh.log" && break
        sleep 0.1
    done
    kill -TERM -- "-${PID}"
    for I in $(seq 1 100); do
        grep -q "suppression de srv-1" "${WORK}/out.log" && break
        sleep 0.1
    done
    grep -q "suppression de srv-1" "${WORK}/out.log" || fail "nettoyage jamais commencé"
    kill -INT -- "-${PID}" 2>/dev/null
    kill -TERM -- "-${PID}" 2>/dev/null
    wait "${PID}"
    RC=$?
    [[ "${RC}" -eq 143 ]] || fail "code ${RC} au lieu de 143"
    grep -q "aucun serveur de lab ne reste" "${WORK}/out.log" || fail "nettoyage interrompu"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_ssh_never_receives_api_key () {
    setup "ssh ne reçoit jamais la clé d'API dans son environnement"
    mutate '.progress = [["ready","completed"]]'
    run_lab session uname || fail "code de sortie $?"
    [[ -s "${WORK}/ssh-env.log" ]] || fail "ssh jamais lancé"
    grep -qv '^key=$' "${WORK}/ssh-env.log" && fail "clé transmise à ssh"
    teardown
}

test_session_waits_for_ssh_before_running_command () {
    setup "session : attend que SSH réponde avant la commande"
    mutate '.progress = [["ready","completed"]]'
    echo 3 > "${WORK}/probe_fails"
    run_lab session uname || fail "code de sortie $?"
    [[ $(grep -c "BatchMode=yes" "${WORK}/ssh.log") -eq 4 ]] || fail "$(grep -c "BatchMode=yes" "${WORK}/ssh.log") sondes au lieu de 4"
    [[ $(tail -n 1 "${WORK}/ssh.log") == *" uname" ]] || fail "commande non lancée en dernier"
    teardown
}

test_session_deletes_server_when_ssh_never_answers () {
    setup "session : serveur supprimé si SSH ne répond jamais"
    mutate '.progress = [["ready","completed"]]'
    echo 100000 > "${WORK}/probe_fails"
    LAB_SSH_TIMEOUT=1 LAB_POLL_INTERVAL=0.2 run_lab session uname && fail "session a réussi"
    grep -qv "BatchMode=yes" "${WORK}/ssh.log" && fail "commande lancée sans SSH joignable"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_ssh_uses_dedicated_lab_key_only_when_present () {
    setup "ssh : clé dédiée du lab utilisée seule quand elle existe"
    mutate '.progress = [["ready","completed"]]'
    mkdir -p "${WORK}/.config/two-lab/ssh"
    : > "${WORK}/.config/two-lab/ssh/lab_ed25519"
    run_lab session uname || fail "code de sortie $?"
    grep -q " uname" "${WORK}/ssh.log" || fail "commande non lancée"
    [[ $(grep -c -- "-i ${WORK}/.config/two-lab/ssh/lab_ed25519 -o IdentitiesOnly=yes -o IdentityAgent=none" "${WORK}/ssh.log") -eq $(wc -l < "${WORK}/ssh.log") ]] \
        || fail "une connexion n'a pas utilisé la clé dédiée seule"
    teardown
}

test_ssh_falls_back_to_agent_without_lab_key () {
    setup "ssh : agent SSH utilisé quand la clé dédiée est absente"
    mutate '.progress = [["ready","completed"]]'
    run_lab session uname || fail "code de sortie $?"
    grep -q -- "-i " "${WORK}/ssh.log" && fail "clé imposée alors qu'elle n'existe pas"
    grep -q "IdentityAgent=none" "${WORK}/ssh.log" && fail "agent désactivé sans clé dédiée"
    teardown
}

known_server () {
    mkdir -p "${WORK}/.cache/two-lab"
    echo 203.0.113.7 > "${WORK}/.cache/two-lab/ip"
    echo debian > "${WORK}/.cache/two-lab/user"
    : > "${WORK}/.cache/two-lab/known_hosts"
}

test_up_prepares_the_server_after_ssh () {
    setup "up : prépare le serveur (qemu, genisoimage, KVM imbriqué) une fois SSH joignable"
    mutate '.progress = [["ready","completed"]]'
    run_lab up || fail "code de sortie $?"
    [[ $(tail -n 1 "${WORK}/ssh.log") == *"debian@203.0.113.7 bash -s" ]] || fail "préparation non lancée en dernier : $(tail -n 1 "${WORK}/ssh.log")"
    grep -q "apt-get install -y -qq --no-install-recommends qemu-system-x86 qemu-utils genisoimage" "${WORK}/prepare.sh" || fail "paquets absents du script"
    grep -q 'usermod -aG kvm' "${WORK}/prepare.sh" || fail "groupe kvm absent du script"
    grep -q -- '-c /dev/kvm' "${WORK}/prepare.sh" || fail "contrôle de /dev/kvm absent"
    grep -q 'kvm_intel/parameters/nested' "${WORK}/prepare.sh" || fail "contrôle de nested absent"
    teardown
}

test_up_reports_a_billed_server_when_preparation_fails () {
    setup "up : préparation échouée signalée, serveur toujours facturé"
    mutate '.progress = [["ready","completed"]]'
    FAKE_PREPARE_RC=1 run_lab up && fail "up a réussi"
    grep -q "préparation du serveur échouée — il est toujours facturé" "${WORK}/out.log" || fail "message absent"
    grep -q "prêt :" "${WORK}/out.log" && fail "serveur annoncé prêt"
    teardown
}

test_session_deletes_server_when_preparation_fails () {
    setup "session : serveur supprimé si la préparation échoue, commande jamais lancée"
    mutate '.progress = [["ready","completed"]]'
    FAKE_PREPARE_RC=1 run_lab session uname && fail "session a réussi"
    grep -q " uname" "${WORK}/ssh.log" && fail "commande lancée malgré la préparation échouée"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_prepare_script_is_valid_shell () {
    setup "prepare : le script distant est du shell valide"
    known_server
    run_lab prepare || fail "code de sortie $?"
    bash -n "${WORK}/prepare.sh" || fail "bash -n refuse le script"
    sh -n "${WORK}/prepare.sh" || fail "sh -n refuse le script"
    teardown
}

test_prepare_without_known_server_is_refused () {
    setup "prepare : refusé sans serveur connu"
    run_lab prepare && fail "prepare a réussi"
    [[ -s "${WORK}/ssh.log" ]] && fail "ssh lancé"
    teardown
}

test_push_builds_for_linux_and_sends_binary_and_topology () {
    setup "push : compile lab pour linux/amd64, envoie le binaire et la topologie"
    known_server
    printf 'name: evpn-2hv\n' > "${WORK}/evpn-2hv.yml"
    run_lab push "${WORK}/evpn-2hv.yml" || fail "code de sortie $?"
    local REPO
    REPO="$(cd "$(dirname "${SCRIPT}")/.." && pwd)"
    [[ $(cat "${WORK}/go.log") == "${REPO}|build -o ${WORK}/.cache/two-lab/lab ./cmd/lab|CGO_ENABLED=0 GOOS=linux GOARCH=amd64" ]] \
        || fail "compilation : $(cat "${WORK}/go.log")"
    grep -q "debian@203.0.113.7 cat > 'lab.part' && chmod 755 'lab.part' && mv 'lab.part' 'lab'" "${WORK}/ssh.log" || fail "envoi de lab absent"
    grep -q "debian@203.0.113.7 cat > 'evpn-2hv.yml.part' && chmod 644 'evpn-2hv.yml.part' && mv 'evpn-2hv.yml.part' 'evpn-2hv.yml'" "${WORK}/ssh.log" || fail "envoi de la topologie absent"
    [[ $(cat "${WORK}/pushed.1") == "binaire-lab" ]] || fail "contenu de lab : $(cat "${WORK}/pushed.1")"
    [[ $(cat "${WORK}/pushed.2") == "name: evpn-2hv" ]] || fail "contenu de la topologie : $(cat "${WORK}/pushed.2")"
    teardown
}

test_push_stops_when_the_build_fails () {
    setup "push : rien n'est envoyé si la compilation échoue"
    known_server
    printf 'name: x\n' > "${WORK}/x.yml"
    FAKE_GO_FAIL=1 run_lab push "${WORK}/x.yml" && fail "push a réussi"
    [[ -s "${WORK}/ssh.log" ]] && fail "ssh lancé"
    grep -q "compilation de lab échouée" "${WORK}/out.log" || fail "message absent"
    teardown
}

test_push_fails_when_a_transfer_fails () {
    setup "push : un envoi échoué fait échouer push"
    known_server
    printf 'name: x\n' > "${WORK}/x.yml"
    FAKE_PUSH_RC=1 run_lab push "${WORK}/x.yml" && fail "push a réussi"
    grep -q "envoi de lab échoué" "${WORK}/out.log" || fail "message absent"
    teardown
}

test_push_refusals () {
    local CASE
    for CASE in absent quote nothing; do
        setup "push : refus (${CASE})"
        known_server
        case "${CASE}" in
            absent)  run_lab push "${WORK}/absent.yml" && fail "push a réussi" ;;
            quote)   printf 'x\n' > "${WORK}/a'b.yml"; run_lab push "${WORK}/a'b.yml" && fail "push a réussi" ;;
            nothing) run_lab push && fail "push a réussi" ;;
        esac
        [[ -s "${WORK}/go.log" ]] && fail "compilation lancée"
        [[ -s "${WORK}/ssh.log" ]] && fail "ssh lancé"
        teardown
    done
}

test_ssh_without_terminal_does_not_ask_for_one () {
    setup "ssh : pas de -t quand l'entrée n'est pas un terminal"
    known_server
    run_lab ssh './lab status' < /dev/null || fail "code de sortie $?"
    [[ $(cat "${WORK}/ssh.log") == *"debian@203.0.113.7 ./lab status" ]] || fail "commande : $(cat "${WORK}/ssh.log")"
    grep -q -- " -t " "${WORK}/ssh.log" && fail "-t demandé sans terminal"
    teardown
}

run_lab_without_env_credentials () {
    env -i PATH="${WORK}/bin:${PATH}" HOME="${WORK}" TMPDIR="${TMPDIR:-/tmp}" \
        FAKE_DIR="${WORK}" FAKE_SECRET="${SECRET}" SCW_API_URL="https://api.example.invalid" \
        LAB_POLL_INTERVAL=0 "$@" bash "${SCRIPT}" plan > "${WORK}/out.log" 2>&1
}

write_credentials () {
    mkdir -p "${WORK}/.config/two-lab"
    printf '%s\n' "$@" > "${WORK}/.config/two-lab/scaleway.env"
    chmod 600 "${WORK}/.config/two-lab/scaleway.env"
}

test_credentials_file_is_used_when_environment_is_empty () {
    setup "identifiants : fichier lu quand l'environnement est vide"
    write_credentials "SCW_SECRET_KEY=${SECRET}" "SCW_DEFAULT_PROJECT_ID=${PROJECT}" "SCW_DEFAULT_ZONE=fr-par-1"
    run_lab_without_env_credentials || fail "code de sortie $?"
    grep -q "\"project_id\": \"${PROJECT}\"" "${WORK}/out.log" || fail "projet du fichier non utilisé"
    grep -q "${SECRET}" "${WORK}/out.log" && fail "secret affiché"
    teardown
}

test_credentials_file_without_final_newline_is_read () {
    setup "identifiants : dernière ligne sans retour à la ligne lue quand même"
    mkdir -p "${WORK}/.config/two-lab"
    printf 'SCW_DEFAULT_PROJECT_ID=%s\nSCW_SECRET_KEY=%s' "${PROJECT}" "${SECRET}" > "${WORK}/.config/two-lab/scaleway.env"
    chmod 600 "${WORK}/.config/two-lab/scaleway.env"
    run_lab_without_env_credentials || fail "code de sortie $?"
    teardown
}

test_environment_wins_over_credentials_file () {
    setup "identifiants : l'environnement l'emporte sur le fichier"
    write_credentials "SCW_SECRET_KEY=mauvais-secret" "SCW_DEFAULT_PROJECT_ID=${OTHER_PROJECT}"
    run_lab_without_env_credentials SCW_SECRET_KEY="${SECRET}" SCW_DEFAULT_PROJECT_ID="${PROJECT}" || fail "code de sortie $?"
    grep -q "\"project_id\": \"${PROJECT}\"" "${WORK}/out.log" || fail "projet de l'environnement non retenu"
    teardown
}

test_environment_value_kept_when_file_completes_the_rest () {
    setup "identifiants : une valeur de l'environnement n'est pas écrasée quand le fichier complète"
    write_credentials "SCW_SECRET_KEY=${SECRET}" "SCW_DEFAULT_PROJECT_ID=${OTHER_PROJECT}"
    run_lab_without_env_credentials SCW_DEFAULT_PROJECT_ID="${PROJECT}" || fail "code de sortie $?"
    grep -q "\"project_id\": \"${PROJECT}\"" "${WORK}/out.log" || fail "projet de l'environnement écrasé par le fichier"
    teardown
}

test_credentials_file_readable_by_others_is_refused () {
    setup "identifiants : fichier lisible par d'autres refusé, sans aucun appel"
    write_credentials "SCW_SECRET_KEY=${SECRET}" "SCW_DEFAULT_PROJECT_ID=${PROJECT}"
    chmod 644 "${WORK}/.config/two-lab/scaleway.env"
    run_lab_without_env_credentials && fail "plan a réussi"
    grep -q "chmod 600" "${WORK}/out.log" || fail "pas de consigne de droits"
    [[ -s "${WORK}/calls.log" ]] && fail "appel réseau avec un fichier exposé"
    teardown
}

test_credentials_file_is_never_executed () {
    setup "identifiants : le fichier est lu, jamais exécuté"
    write_credentials "SCW_SECRET_KEY=\$(touch ${WORK}/pwned)" "touch ${WORK}/pwned2" "SCW_DEFAULT_PROJECT_ID=${PROJECT}"
    run_lab_without_env_credentials && fail "plan a réussi avec un secret invalide"
    [[ -e "${WORK}/pwned" || -e "${WORK}/pwned2" ]] && fail "contenu du fichier exécuté"
    teardown
}

signal_during_session () {
    local SIGNAL="${1}"
    local EXPECTED="${2}"
    local PID RC I
    setup "session : ${SIGNAL} pendant la commande distante, serveur supprimé, code ${EXPECTED}"
    mutate '.progress = [["ready","completed"]]'
    set -m
    FAKE_SSH_SLEEP=5 lab_env exec bash "${SCRIPT}" session sleep-long > "${WORK}/out.log" 2>&1 &
    PID=$!
    set +m
    for I in $(seq 1 100); do
        grep -q "sleep-long" "${WORK}/ssh.log" && break
        sleep 0.1
    done
    grep -q "sleep-long" "${WORK}/ssh.log" || fail "commande distante jamais lancée"
    kill "-${SIGNAL}" -- "-${PID}"
    wait "${PID}"
    RC=$?
    [[ "${RC}" -eq "${EXPECTED}" ]] || fail "code ${RC} au lieu de ${EXPECTED}"
    [[ $(remaining) == "foreign-1,foreign-2" ]] || fail "restants : $(remaining)"
    teardown
}

test_session_handles_term () { signal_during_session TERM 143; }
test_session_handles_hup () { signal_during_session HUP 129; }
test_session_handles_int () { signal_during_session INT 130; }

for T in $(declare -F | awk '{print $3}' | grep '^test_'); do
    "${T}"
done
echo "${PASS} réussi(s), ${FAIL} échec(s)"
[[ "${FAIL}" -eq 0 ]]
