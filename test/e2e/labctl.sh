#!/usr/bin/env bash

LABCTL_VERSION="0.1.0rc001"
LABCTL_URL="https://git.g3e.fr/syonad/labctl/releases/download/${LABCTL_VERSION}"
LABCTL_SUMS="e0c852a8f14f453dedb44e5ef70481293c72a63773dfef166cd298a4da4903e7  labctl_linux_amd64
3bb13f2088360e332c2c071c7cf070f54956677c6387665b6bef6a58f9e2a631  lab-host.sh"
LABCTL_DIR="${LABCTL_DIR:-${HOME}/.cache/labctl/release/${LABCTL_VERSION}}"

die () { echo "!! ${1}" >&2; exit 1; }

sha256 () {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$@"
    else
        shasum -a 256 "$@"
    fi
}

verify () {
    (cd "${LABCTL_DIR}" && sha256 -c --quiet - <<< "${LABCTL_SUMS}") >&2
}

fetch () {
    local asset part
    mkdir -p "${LABCTL_DIR}" || return 1
    part="$(mktemp -d "${LABCTL_DIR}/.part.XXXXXX")" || return 1
    for asset in labctl_linux_amd64 lab-host.sh; do
        echo "== téléchargement de ${asset} (labctl ${LABCTL_VERSION})" >&2
        curl -fsSL --proto '=https' -o "${part}/${asset}" "${LABCTL_URL}/${asset}" || { rm -rf "${part}"; return 1; }
    done
    (cd "${part}" && sha256 -c --quiet - <<< "${LABCTL_SUMS}") >&2 || { rm -rf "${part}"; return 1; }
    chmod 755 "${part}/labctl_linux_amd64" "${part}/lab-host.sh" || { rm -rf "${part}"; return 1; }
    mv "${part}/labctl_linux_amd64" "${part}/lab-host.sh" "${LABCTL_DIR}/" && rm -rf "${part}"
}

main () {
    if ! verify 2>/dev/null; then
        fetch || die "labctl ${LABCTL_VERSION} : téléchargement ou vérification des sommes échoué"
        verify || die "labctl ${LABCTL_VERSION} : sommes invalides dans ${LABCTL_DIR}"
    fi
    LABCTL_BIN="${LABCTL_DIR}/labctl_linux_amd64" exec "${LABCTL_DIR}/lab-host.sh" "$@"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" || -z "${BASH_SOURCE[0]}" ]]; then
    main "$@"
fi
