#!/usr/bin/env bash

E2E_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${E2E_DIR}/../.." && pwd)"
BIN_DIR="${BIN_DIR:-${E2E_DIR}/topologies/bin}"
SCRIPTS_DIR="${SCRIPTS_DIR:-${E2E_DIR}/topologies/scripts}"
BINARIES="agent dhcp metadata two-hostsvc"
ASSET_SCRIPTS="run-dnsmasq-in-netns.sh run-dhcp-in-netns.sh"
UNITS="two-hostsvc.service agent.service dnsmasq@.service dhcp@.service metadata@.service"
MARKERS="SHA256SUMS VERSION RELEASE"
SCRIPTS="deploy.sh bootstrap_kvm.sh"

usage () {
    cat >&2 <<USAGE
usage: ${0##*/} [--release <tag> | --clean]

  sans option     : prépare dans ${BIN_DIR} les assets de l'arbre courant au format d'une release
                    (${BINARIES} compilés pour linux/amd64, ${ASSET_SCRIPTS}, units, SHA256SUMS,
                    VERSION) et copie ${SCRIPTS} dans ${SCRIPTS_DIR} ; le prochain lab déploie
                    l'arbre courant en entier, sans rien télécharger
  --release <tag> : le prochain lab déploie la release <tag> telle que publiée (fichier RELEASE)
  --clean         : vide ${BIN_DIR} et ${SCRIPTS_DIR} ; le prochain lab refuse de démarrer
USAGE
    exit 2
}

clean () {
    local file
    for file in ${BINARIES} ${ASSET_SCRIPTS} ${UNITS} ${MARKERS}; do
        rm -f "${BIN_DIR}/${file}" || return 1
    done
    for file in ${SCRIPTS}; do
        rm -f "${SCRIPTS_DIR}/${file}" || return 1
    done
}

build_version () {
    local commit changes
    commit=$(git -C "${REPO_DIR}" rev-parse --short HEAD) || return 1
    changes=$(git -C "${REPO_DIR}" status --porcelain) || return 1
    if [ -n "${changes}" ]; then
        echo "${commit}-dirty"
    else
        echo "${commit}"
    fi
}

sha256 () {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$@"
    else
        shasum -a 256 "$@"
    fi
}

build () {
    local binary file version
    version=$(build_version) || return 1
    clean || return 1
    echo "== version ${version}" >&2
    for binary in ${BINARIES}; do
        echo "== ${binary}" >&2
        (cd "${REPO_DIR}" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=${version}" -o "${BIN_DIR}/${binary}" "./cmd/${binary}") || return 1
    done
    for file in ${ASSET_SCRIPTS}; do
        install -m 0755 "${REPO_DIR}/scripts/${file}" "${BIN_DIR}/${file}" || return 1
    done
    for file in ${UNITS}; do
        cp "${REPO_DIR}/systemd/${file}" "${BIN_DIR}/${file}" || return 1
    done
    for file in ${SCRIPTS}; do
        echo "== ${file}" >&2
        cp "${REPO_DIR}/scripts/${file}" "${SCRIPTS_DIR}/${file}" || return 1
    done
    (cd "${BIN_DIR}" && sha256 ${BINARIES} ${ASSET_SCRIPTS} ${UNITS} > SHA256SUMS) || return 1
    echo "${version}" > "${BIN_DIR}/VERSION" || return 1
    cat "${BIN_DIR}/SHA256SUMS"
}

release () {
    [ -n "${1}" ] || usage
    clean || return 1
    echo "${1}" > "${BIN_DIR}/RELEASE" || return 1
    echo "== le prochain lab déploie la release ${1}" >&2
}

main () {
    case "${1:-}" in
        "") build ;;
        --release) release "${2:-}" ;;
        --clean) clean ;;
        *) usage ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
