# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 ~ M6 已完成；M7-01 ~ M7-09、M7-12 ~ M7-14 已完成。
- **发布顺序**（D-101）：M7-10、M7-11 → v0.1.0（打标签前先问用户）。

## 下次会话从这里开始

1. M7-10（文字检查，含 M7-13 补充的两项）、M7-11（资源占用）。
2. v0.1.0 发布前要求在 Debian 11 / 13 上冒烟测试，需要向用户要测试机。发布后在测试机上用"维护 › 更新脚本"从 v0.0.3 升级，验证更新功能。

## 测试 VPS 的状态（2026-10-19 重装后，M7-14 结束时）

登录密码记在本机记忆中（用户同意），**不写入仓库**。两台都用 install.sh 安装了 v0.0.3（`/usr/local/bin/litenode`），root 都没有公钥，只能密码登录。`/root` 下有走查用的辅助脚本：`ptyx.py`（按提示文字驱动 pty，用法见文件开头）、`vprobe.sh`、`link2client.py`（`/tmp` 下的副本重启后消失）；Ubuntu 上 `/root/litenode-export-*.json`、`/root/u-backup.json` 是导出的配置。

- **Ubuntu 24.04.5**（SSH 22，IP 203.0.113.181，有 IPv6）：节点 `US-Reality`（443，SNI www.microsoft.com；M7-14 验证时重置过客户端 UUID，旧链接已失效）；转发规则 20443 → Debian 198.51.100.252:443（tcp+udp，备注 to-debian）；LiteNode 防火墙关闭（ufw 未启用）；DNS 由 LiteNode 设为 9.9.9.9 1.1.1.1（systemd-resolved drop-in）；`00-litenode.conf` 管理 `Port 22`；BBR 由镜像的 `99-custom.conf` 开启；没有 sudo 用户。
- **Debian 12**（SSH 61234，IP 198.51.100.252，磁盘约 770M 空闲）：节点 `Reality-443`（443，SNI www.microsoft.com）；没有转发、防火墙关闭；DNS 由 LiteNode 写入 `/etc/resolv.conf`（9.9.9.9 1.1.1.1）；安装了 systemd-timesyncd，时间同步开启；时区 US/Eastern；BBR 由镜像的 `99-bbr.conf` 开启；没有 swap。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在加入 TLS 类节点类型之前讨论。
