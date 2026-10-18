#!/bin/sh
# Development helper (not installed): run ON THE TEST VPS to check that a
# share link works without the GFW in the path. The client runs on the VPS
# itself and connects to its own public address.
#
#   scp scripts/dev/link2client.py scripts/dev/vprobe.sh root@vps:/tmp/
#   /tmp/vprobe.sh '<vless link>'      -> "ip=<exit IP>" or "FAILED"
python3 /tmp/link2client.py "$1" 10809 > /tmp/c.json
/usr/local/lib/litenode/xray run -c /tmp/c.json >/dev/null 2>&1 &
pid=$!
sleep 2
ip=$(curl -s --max-time 10 --socks5-hostname 127.0.0.1:10809 https://www.cloudflare.com/cdn-cgi/trace | grep '^ip=')
kill "$pid" 2>/dev/null
echo "${ip:-FAILED}"
