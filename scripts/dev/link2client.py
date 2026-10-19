# Development helper (not installed): turn a vless:// share link (REALITY or
# VLESS Encryption with security=none) or an ss:// Shadowsocks 2022 link
# into a minimal Xray client config with a local SOCKS inbound, to test
# links with the official Xray client:
#   python3 -I scripts/dev/link2client.py "<link>" 10808 [10853] > client.json
#   xray run -c client.json &  curl --socks5-hostname 127.0.0.1:10808 https://www.cloudflare.com/cdn-cgi/trace
# With a third port, UDP sent to 127.0.0.1:<port> goes through the node to
# 1.1.1.1:53 (a DNS query there tests UDP).
import json, sys, urllib.parse
u = urllib.parse.urlsplit(sys.argv[1])
q = dict(urllib.parse.parse_qsl(u.query))
cfg = {
  "log": {"loglevel": "warning"},
  "inbounds": [{"listen": "127.0.0.1", "port": int(sys.argv[2]), "protocol": "socks", "settings": {"udp": False}}]}
if len(sys.argv) > 3:
  cfg["inbounds"].append({"listen": "127.0.0.1", "port": int(sys.argv[3]), "protocol": "dokodemo-door",
    "settings": {"address": "1.1.1.1", "port": 53, "network": "udp"}})
if u.scheme == "ss":
  # SIP002 with SIP022 user info: "<method>:<percent-encoded password>".
  method, _, password = u.netloc.rsplit("@", 1)[0].partition(":")
  cfg["outbounds"] = [{"protocol": "shadowsocks", "settings": {"servers": [{"address": u.hostname, "port": u.port,
    "method": method, "password": urllib.parse.unquote(password)}]}}]
else:
  cfg["outbounds"] = [{"protocol": "vless",
    "settings": {"vnext": [{"address": u.hostname, "port": u.port,
      "users": [{"id": u.username, "encryption": q.get("encryption", "none"), "flow": q.get("flow", "")}]}]},
    "streamSettings": {"network": q.get("type", "tcp"), "security": q["security"]}}]
  if q["security"] == "reality":
    cfg["outbounds"][0]["streamSettings"]["realitySettings"] = {
      "serverName": q["sni"], "fingerprint": q["fp"], "publicKey": q["pbk"], "shortId": q["sid"]}
print(json.dumps(cfg))
