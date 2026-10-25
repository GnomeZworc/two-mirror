set -u
API=http://127.0.0.1:8080
IMAGE_URL=https://cloud.debian.org/images/cloud/bookworm/latest
IMAGE_FILE=debian-12-genericcloud-amd64.qcow2
TWO_IMAGE=/var/tmp/deb-two.qcow2
VM_KEY=/root/.ssh/lab-vm
VOLUMES=/var/lib/two/volumes

check () {
    local desc="${1}" out
    shift
    if out=$("$@" 2>&1); then
        echo "RÉUSSI: ${desc}"
    else
        echo "ÉCHOUÉ: ${desc}${out:+ — $(printf '%s' "${out}" | tail -n 1)}"
    fi
}

known () {
    local ticket="${1}" desc="${2}" out
    shift 2
    if out=$("$@" 2>&1); then
        echo "INATTENDU: ${desc} (${ticket}) passe, retirer le marqueur known"
    else
        echo "CONNU: ${desc} (${ticket})${out:+ — $(printf '%s' "${out}" | tail -n 1)}"
    fi
}

known_flaky () {
    local ticket="${1}" desc="${2}" out
    shift 2
    if out=$("$@" 2>&1); then
        echo "RÉUSSI: ${desc} (${ticket}, intermittent : non reproduit cette fois)"
    else
        echo "CONNU: ${desc} (${ticket}, intermittent)${out:+ — $(printf '%s' "${out}" | tail -n 1)}"
    fi
}

info () { echo "INFO: ${1}"; }

api_post () {
    curl -fsS -o /dev/null -X POST -H 'Content-Type: application/json' -d "${2}" "${API}/${1}"
}

api_delete () {
    curl -fsS -o /dev/null -X DELETE "${API}/${1}"
}

api_code () {
    curl -s -o /dev/null -w '%{http_code}' -X "${1}" "${API}/${2}"
}

api_state () {
    curl -fsS "${API}/${1}" 2>/dev/null | jq -r .state
}

wait_state () {
    local path="${1}" want="${2}" i s code=""
    for i in $(seq 1 240); do
        s=$(api_state "${path}")
        [ "${s}" = "${want}" ] && return 0
        if [ "${want}" = deleted ] && [ -z "${s}" ]; then
            code=$(api_code GET "${path}")
            [ "${code}" = 404 ] && return 0
        fi
        [ "${s}" = error ] && { echo "${path}: error" >&2; return 1; }
        sleep 2
    done
    echo "${path}: ${s:-absent}${code:+ (HTTP ${code})}, ${want} attendu" >&2
    return 1
}

api_post_code () {
    curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d "${2}" "${API}/${1}"
}

expect_code () {
    local want="${1}" got
    shift
    got=$("$@")
    [ "${got}" = "${want}" ] || { echo "code ${got:-absent}, ${want} attendu"; return 1; }
}

api_field () {
    curl -fsS "${API}/${1}" | jq -r ".${2} // empty"
}
