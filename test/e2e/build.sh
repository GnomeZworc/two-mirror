#!/usr/bin/env bash

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${E2E_DIR}/../.." && pwd)"
BIN_DIR="${BIN_DIR:-${E2E_DIR}/topologies/bin}"
BINARIES="agent dhcp metadata"

usage () {
    cat >&2 <<USAGE
usage: ${0##*/} [--clean]

  sans option : compile ${BINARIES} pour linux/amd64 depuis l'arbre courant dans ${BIN_DIR} ;
                le prochain lab les installe par-dessus la release épinglée dans
                topologies/provision/hypervisor.sh
  --clean     : vide ${BIN_DIR} ; le prochain lab teste la release seule
USAGE
    exit 2
}

clean () {
    local binary
    for binary in ${BINARIES}; do
        rm -f "${BIN_DIR}/${binary}" || return 1
    done
}

build () {
    local binary
    for binary in ${BINARIES}; do
        echo "== ${binary}" >&2
        (cd "${REPO_DIR}" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${BIN_DIR}/${binary}" "./cmd/${binary}") || return 1
    done
    (cd "${BIN_DIR}" && sha256sum ${BINARIES} 2>/dev/null || shasum -a 256 ${BINARIES})
}

main () {
    case "${1:-}" in
        "") build ;;
        --clean) clean ;;
        *) usage ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
