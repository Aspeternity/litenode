#!/bin/sh
# Development helper (not installed), M11-03: run ON THE RELAY test VPS to
# check Xray (as 3x-ui v3.10.2 imports the hysteria2:// link), mihomo and
# sing-box as Hysteria2 outbounds to a LiteNode landing node, certificate
# pinned. Prints "<socks port> ip=<exit IP> udp=ok" per client; "wrong"
# breaks both fingerprints, so every client must fail. Expects mihomo and
# sing-box binaries in /tmp/m9.
#   m11relay.sh <host> <port> <password> <cert sha256 hex> <spki sha256 base64> [wrong]
H=$1 P=$2 PW=$3 FP=$4 PK=$5
D=/tmp/m11; mkdir -p $D; cd $D
if [ "${6:-}" = wrong ]; then FP=00${FP#??}; PK=AA${PK#??}; fi
# Exactly what 3x-ui v3.10.2 builds from the hysteria2:// link.
cat > x.json <<J
{"log":{"loglevel":"warning"},"inbounds":[{"listen":"127.0.0.1","port":11080,"protocol":"socks","settings":{"udp":true}}],
"outbounds":[{"protocol":"hysteria","tag":"Hy2","settings":{"address":"$H","port":$P,"version":2},
"streamSettings":{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"auth":"$PW","udpIdleTimeout":60},
"tlsSettings":{"serverName":"www.bing.com","alpn":["h3"],"fingerprint":"","echConfigList":"","verifyPeerCertByName":"","pinnedPeerCertSha256":"$FP"}}}]}
J
cat > m.yaml <<J
mixed-port: 11081
allow-lan: false
mode: rule
log-level: warning
proxies:
  - {name: hy2, type: hysteria2, server: $H, port: $P, password: $PW, sni: www.bing.com, fingerprint: $FP}
rules: ["MATCH,hy2"]
J
cat > s.json <<J
{"log":{"level":"warn"},"inbounds":[{"type":"mixed","listen":"127.0.0.1","listen_port":11082}],
"outbounds":[{"type":"hysteria2","tag":"hy2","server":"$H","server_port":$P,"password":"$PW",
"tls":{"enabled":true,"server_name":"www.bing.com","certificate_public_key_sha256":["$PK"]}}]}
J
/usr/local/lib/litenode/xray run -c x.json >x.log 2>&1 & A=$!
/tmp/m9/mihomo -f m.yaml -d $D >m.log 2>&1 & B=$!
/tmp/m9/sing-box run -c s.json >s.log 2>&1 & C=$!
sleep 3
for port in 11080 11081 11082; do
  ip=$(curl -s --max-time 10 --socks5-hostname 127.0.0.1:$port https://www.cloudflare.com/cdn-cgi/trace | grep '^ip=')
  udp=$(python3 - $port <<'P' 2>&1
import socket,struct,sys
s=socket.create_connection(("127.0.0.1",int(sys.argv[1])));s.sendall(b"\x05\x01\x00");s.recv(2)
s.sendall(b"\x05\x03\x00\x01\x00\x00\x00\x00\x00\x00");r=s.recv(10)
port=struct.unpack(">H",r[8:10])[0]
u=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);u.settimeout(6)
q=b"\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01"
u.sendto(b"\x00\x00\x00\x01"+socket.inet_aton("1.1.1.1")+struct.pack(">H",53)+q,("127.0.0.1",port))
try: d=u.recv(2048);print("ok")
except Exception as e: print("FAIL")
P
)
  echo "$port ${ip:-FAILED} udp=$udp"
done
kill $A $B $C 2>/dev/null; sleep 1
grep -ihE "error|fail|fingerprint|certificate" x.log m.log s.log | cut -c1-200 | sort -u | head -6
