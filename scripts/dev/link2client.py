# Development helper (not installed): turn a vless:// REALITY share link into
# a minimal Xray client config with a local SOCKS inbound, to test links
# with the official Xray client:
#   python3 -I scripts/dev/link2client.py "<link>" 10808 > client.json
#   xray run -c client.json &  curl --socks5-hostname 127.0.0.1:10808 https://www.cloudflare.com/cdn-cgi/trace
import json, sys, urllib.parse
u = urllib.parse.urlsplit(sys.argv[1])
q = dict(urllib.parse.parse_qsl(u.query))
cfg = {
  "log": {"loglevel": "warning"},
  "inbounds": [{"listen": "127.0.0.1", "port": int(sys.argv[2]), "protocol": "socks", "settings": {"udp": False}}],
  "outbounds": [{"protocol": "vless",
    "settings": {"vnext": [{"address": u.hostname, "port": u.port,
      "users": [{"id": u.username, "encryption": q.get("encryption", "none"), "flow": q.get("flow", "")}]}]},
    "streamSettings": {"network": q.get("type", "tcp"), "security": q["security"],
      "realitySettings": {"serverName": q["sni"], "fingerprint": q["fp"], "publicKey": q["pbk"], "shortId": q["sid"]}}}]}
print(json.dumps(cfg))
