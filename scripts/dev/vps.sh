#!/bin/sh
# Development helper (not installed): run a command on the test VPS with
# password login, without storing the password anywhere.
#
#   VPS_HOST=1.2.3.4 VPS_PORT=22 LN_PASS='...' scripts/dev/vps.sh 'uname -a'
#   VPS_USER=admin ... (default root)
#   scripts/dev/vps.sh 'cat > /tmp/f' < local-file
#   VPS_SSH_OPTS=-tt scripts/dev/vps.sh 'cmd'   (with a terminal, like a user)
# Needs OpenSSH >= 8.4 (SSH_ASKPASS_REQUIRE). Host keys go to a file next
# to this script's caller ($TMPDIR), not ~/.ssh.
set -eu
dir=${TMPDIR:-/tmp}
askpass="$dir/litenode-askpass.sh"
# shellcheck disable=SC2016 # expanded later, by the generated helper
printf '#!/bin/sh\nprintf "%%s\\n" "$LN_PASS"\n' > "$askpass"
chmod 700 "$askpass"
exec env SSH_ASKPASS="$askpass" SSH_ASKPASS_REQUIRE=force LN_PASS="$LN_PASS" \
	ssh -p "${VPS_PORT:-22}" -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$dir/litenode-known_hosts" \
	-o PubkeyAuthentication=no -o ConnectTimeout=15 ${VPS_SSH_OPTS:-} "${VPS_USER:-root}@$VPS_HOST" "$@"
