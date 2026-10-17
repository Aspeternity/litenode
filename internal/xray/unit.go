package xray

import "fmt"

// Unit returns the systemd unit for Xray. It runs as the dedicated user
// with only the capability to bind ports below 1024 (P-002).
func Unit() string {
	return fmt.Sprintf(`[Unit]
Description=LiteNode Xray service
Documentation=https://github.com/Aspeternity/litenode
After=network-online.target
Wants=network-online.target

[Service]
User=%[1]s
Group=%[1]s
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
ExecStart=%[2]s run -c %[3]s
Restart=on-failure
RestartPreventExitStatus=23
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`, User, BinPath, ConfigPath)
}
