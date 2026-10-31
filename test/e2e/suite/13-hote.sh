# fonctionnalité: connaissance de l'hôte — /register (hostname, CPU, RAM, OS, noyau) lu au démarrage, /ping (version, démarrages de l'hôte et de l'agent, uptime, load) mesuré périodiquement, lecture seule, redémarrage de l'agent distingué de celui de l'hôte
# hôtes: hv1
# tickets: #39

on hv1 <<'NODE'
reg () { api_field register "${1}"; }
ping_field () { api_field ping "${1}"; }
lscpu_field () { lscpu | awk -F: -v k="${1}" '$1 == k {gsub(/ /, "", $2); print $2}'; }
os_field () { (. /etc/os-release && eval "echo \"\${${1}}\""); }
near () { awk -v a="${1}" -v b="${2}" -v d="${3}" 'BEGIN { exit !((a - b) <= d && (b - a) <= d) }' || { echo "${1} et ${2} à plus de ${3} s"; return 1; }; }
ping_ready () { local i; for i in $(seq 1 30); do [ "$(api_code GET ping)" = 200 ] && return 0; sleep 1; done; return 1; }

check "/register : 200" expect_code 200 api_code GET register
check "/register : hostname = hostname" test "$(reg hostname)" = "$(hostname)"
check "/register : threads = nproc" test "$(reg cpu.threads)" = "$(nproc --all)"
check "/register : sockets = lscpu" test "$(reg cpu.sockets)" = "$(lscpu_field 'Socket(s)')"
check "/register : cœurs = sockets × cœurs par socket (lscpu)" test "$(reg cpu.cores)" = "$(( $(lscpu_field 'Socket(s)') * $(lscpu_field 'Core(s) per socket') ))"
check "/register : modèle = lscpu" test "$(reg cpu.model)" = "$(lscpu | sed -n 's/^Model name: *//p')"
check "/register : fabricant = lscpu" test "$(reg cpu.vendor)" = "$(lscpu | sed -n 's/^Vendor ID: *//p')"
check "/register : RAM = MemTotal" test "$(reg memory_bytes)" = "$(awk '/^MemTotal:/ {printf "%.0f\n", $2 * 1024}' /proc/meminfo)"
check "/register : noyau = uname -r" test "$(reg kernel)" = "$(uname -r)"
check "/register : OS = os-release (ID, VERSION_ID, PRETTY_NAME)" test "$(reg os.id)|$(reg os.version_id)|$(reg os.pretty_name)" = "$(os_field ID)|$(os_field VERSION_ID)|$(os_field PRETTY_NAME)"

check "/ping : 200" expect_code 200 api_code GET ping
VERSION=$(ping_field agent_version)
info "version de l'agent : ${VERSION}"
check "/ping : version injectée au build (commit ou tag, pas undefined)" sh -c "echo '${VERSION}' | grep -Eq '^([0-9a-f]{7,}(-dirty)?|[0-9]+\.[0-9]+\.[0-9]+.*)$'"
check "/ping : boot_time = btime" test "$(ping_field boot_time)" = "$(awk '/^btime/ {print $2}' /proc/stat)"
check "/ping : boot_time + uptime_seconds = collected_at (horloge de l'hôte)" near "$(awk -v b="$(ping_field boot_time)" -v u="$(ping_field uptime_seconds)" 'BEGIN { printf "%d", b + u }')" "$(ping_field collected_at)" 2
check "/ping : mesure de moins de 15 s" near "$(ping_field collected_at)" "$(date +%s)" 15
check "/ping : trois valeurs de load" test "$(api_field ping 'load | length')" = 3
check "/ping : agent_start_time = démarrage de l'unité agent" near "$(ping_field agent_start_time)" "$(date -d "$(systemctl show agent -p ActiveEnterTimestamp --value)" +%s)" 2
for path in register ping; do
    check "POST /${path} : 405" expect_code 405 api_code POST "${path}"
done

BOOT=$(ping_field boot_time)
START=$(ping_field agent_start_time)
sleep 1
check "redémarrage de l'agent" systemctl restart agent
check "/ping de nouveau servi après le redémarrage" ping_ready
check "après redémarrage : boot_time inchangé" test "$(ping_field boot_time)" = "${BOOT}"
check "après redémarrage : agent_start_time plus récent" test "$(ping_field agent_start_time)" -gt "${START}"
NODE
