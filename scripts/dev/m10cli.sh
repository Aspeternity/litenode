#!/bin/sh
# Development helper (not installed): run ON ANOTHER TEST VPS as the client
# while the node connectivity check (M10) runs on the node machine. Needs
# /tmp/link2client.py, /tmp/dnsprobe.py and LiteNode's Xray on this machine.
#   /tmp/m10cli.sh '<link>'      -> "ip=<exit IP>" or "FAILED" (TCP)
#   /tmp/m10cli.sh '<link>' udp  -> also 6 DNS queries, e.g. "6 udp=ok"
python3 /tmp/link2client.py "$1" 10809 10853 > /tmp/m10c-$(id -u).json
/usr/local/lib/litenode/xray run -c /tmp/m10c-$(id -u).json >/dev/null 2>&1 &
pid=$!; sleep 2
ip=$(curl -s --max-time 10 --socks5-hostname 127.0.0.1:10809 https://www.cloudflare.com/cdn-cgi/trace | grep '^ip=')
echo "${ip:-FAILED}"
if [ "$2" = udp ]; then for i in 1 2 3 4 5 6; do python3 /tmp/dnsprobe.py 127.0.0.1 10853; done | sort | uniq -c | tr -s ' '; fi
kill $pid
