#!/usr/bin/env bash
# Build the web UI component set from components.yml.
#
#   ./build.sh           build (download remote components + generate components.json)
#   ./build.sh --check   validate manifest and entry files without downloading
#
# Reads components.yml, downloads each remote component into components/<name>/
# at the requested ref, then generates components.json (runtime load list) and
# components.lock.json (resolved commit pins).
#
# Dependencies: yq (mikefarah/yq v4), jq, curl
# Optional env:
#   GIT_TOKEN   forge token for private repos (sent as "Authorization: token …")

set -euo pipefail

WEB_DIR="$(cd "$(dirname "$0")" && pwd)"
MANIFEST="${WEB_DIR}/components.yml"
COMPONENTS_DIR="${WEB_DIR}/components"
OUTPUT="${WEB_DIR}/components.json"
LOCKFILE="${WEB_DIR}/components.lock.json"
GIT_TOKEN="${GIT_TOKEN:-}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log()  { echo -e "${GREEN}[+]${NC} $*"; }
warn() { echo -e "${YELLOW}[!]${NC} $*"; }
info() { echo -e "${BLUE}[i]${NC} $*"; }
die()  { echo -e "${RED}[-]${NC} $*" >&2; exit 1; }

CHECK_ONLY=false
[[ "${1:-}" == "--check" ]] && CHECK_ONLY=true

# ── Dependency + manifest guards ────────────────────────────────────────────────

command -v yq   &>/dev/null || die "yq is required (mikefarah/yq v4)"
command -v jq   &>/dev/null || die "jq is required"
command -v curl &>/dev/null || die "curl is required"
[[ -f "$MANIFEST" ]] || die "manifest not found: $MANIFEST"

# ── Forge API helpers ───────────────────────────────────────────────────────────

api_get() {
    local url="$1"
    local args=(-sf -H "Accept: application/json")
    [[ -n "$GIT_TOKEN" ]] && args+=(-H "Authorization: token ${GIT_TOKEN}")
    curl "${args[@]}" "$url" || die "API request failed: $url"
}

raw_dl() {
    local url="$1" out="$2"
    local args=(-sfL)
    [[ -n "$GIT_TOKEN" ]] && args+=(-H "Authorization: token ${GIT_TOKEN}")
    curl "${args[@]}" "$url" -o "$out" || die "download failed: $url"
}

# Split a repo URL into "server owner repo".
#   https://git.g3e.fr/team-reseau/vpc-panel → https://git.g3e.fr team-reseau vpc-panel
parse_repo_url() {
    local url="$1"
    url="${url%.git}"; url="${url%/}"
    local proto="${url%%://*}"
    local rest="${url#*://}"
    local host="${rest%%/*}"
    local path="${rest#*/}"
    [[ "$path" == "$rest" || -z "$path" ]] && die "invalid repo URL (need owner/repo): $1"
    local owner="${path%/*}" repo="${path##*/}"
    [[ -z "$owner" || -z "$repo" ]] && die "invalid repo URL (need owner/repo): $1"
    echo "${proto}://${host}" "$owner" "$repo"
}

resolve_commit() {
    local server="$1" owner="$2" repo="$3" ref="$4"
    api_get "${server}/api/v1/repos/${owner}/${repo}/commits?sha=${ref}&limit=1" \
        | jq -r '.[0].sha // empty'
}

# Recursively download a repo path (relative to repo root) into dest dir.
download_path() {
    local server="$1" owner="$2" repo="$3" ref="$4" rpath="$5" dest="$6"

    local api
    if [[ -z "$rpath" ]]; then
        api="${server}/api/v1/repos/${owner}/${repo}/contents?ref=${ref}"
    else
        api="${server}/api/v1/repos/${owner}/${repo}/contents/${rpath}?ref=${ref}"
    fi

    local listing
    listing="$(api_get "$api")"

    mkdir -p "$dest"
    while IFS= read -r entry; do
        local type name dl path
        type="$(jq -r '.type'              <<<"$entry")"
        name="$(jq -r '.name'              <<<"$entry")"
        dl="$(  jq -r '.download_url // ""' <<<"$entry")"
        path="$(jq -r '.path'              <<<"$entry")"

        if [[ "$type" == "dir" ]]; then
            download_path "$server" "$owner" "$repo" "$ref" "$path" "${dest}/${name}"
        else
            [[ -z "$dl" ]] && die "no download_url for ${path} in ${owner}/${repo}@${ref}"
            raw_dl "$dl" "${dest}/${name}"
        fi
    done < <(echo "$listing" | jq -c 'if type=="array" then .[] else . end')
}

# ── Download WASM libs ───────────────────────────────────────────────────────────

# ── Read manifest ────────────────────────────────────────────────────────────────

mapfile -t LOCALS < <(yq '(.local // [])[]' "$MANIFEST")
mapfile -t REMOTES < <(yq '(.components // [])[] | [.name, .repo, (.ref // "main")] | join("|")' "$MANIFEST")

# Ordered list of load paths for components.json
declare -a LOAD_PATHS=()

# ── Validate locals ──────────────────────────────────────────────────────────────

for name in "${LOCALS[@]}"; do
    [[ -z "$name" ]] && continue
    entry="${COMPONENTS_DIR}/${name}/${name}.js"
    if [[ ! -f "$entry" ]]; then
        die "local component '${name}' missing entry file: components/${name}/${name}.js"
    fi
    LOAD_PATHS+=("components/${name}/${name}.js")
done

# ── Download remotes ─────────────────────────────────────────────────────────────

LOCK_ENTRIES="[]"

for spec in "${REMOTES[@]}"; do
    [[ -z "$spec" ]] && continue
    IFS='|' read -r name repo ref <<<"$spec"
    [[ -z "$name" || -z "$repo" ]] && die "manifest entry missing name or repo: '$spec'"

    read -r server owner gitrepo <<<"$(parse_repo_url "$repo")"
    dest="${COMPONENTS_DIR}/${name}"

    if [[ "$CHECK_ONLY" == true ]]; then
        info "would fetch ${name} from ${repo}@${ref}"
        commit="$(resolve_commit "$server" "$owner" "$gitrepo" "$ref" || true)"
        [[ -z "$commit" ]] && warn "  could not resolve ref '${ref}' in ${owner}/${gitrepo}"
        LOAD_PATHS+=("components/${name}/${name}.js")
        continue
    fi

    log "fetching ${name} ← ${owner}/${gitrepo}@${ref}"
    commit="$(resolve_commit "$server" "$owner" "$gitrepo" "$ref")"
    [[ -z "$commit" ]] && die "cannot resolve ref '${ref}' in ${owner}/${gitrepo}"

    rm -rf "$dest"
    download_path "$server" "$owner" "$gitrepo" "$ref" "" "$dest"

    entry="${dest}/${name}.js"
    [[ -f "$entry" ]] || die "component '${name}' has no ${name}.js at repo root"

    LOAD_PATHS+=("components/${name}/${name}.js")
    LOCK_ENTRIES="$(jq \
        --arg name "$name" --arg repo "$repo" --arg ref "$ref" --arg commit "$commit" \
        '. + [{name:$name, repo:$repo, ref:$ref, commit:$commit}]' <<<"$LOCK_ENTRIES")"
    info "  pinned ${commit:0:12}"
done

# ── Generate components.json ─────────────────────────────────────────────────────

if [[ "$CHECK_ONLY" == true ]]; then
    log "check passed: ${#LOAD_PATHS[@]} components, manifest valid"
    exit 0
fi

printf '%s\n' "${LOAD_PATHS[@]}" | jq -R . | jq -s . > "$OUTPUT"
echo "$LOCK_ENTRIES" | jq '.' > "$LOCKFILE"

log "wrote ${OUTPUT} (${#LOAD_PATHS[@]} components)"
log "wrote ${LOCKFILE} ($(jq 'length' <<<"$LOCK_ENTRIES") remote pins)"
