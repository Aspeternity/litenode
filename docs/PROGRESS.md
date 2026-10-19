# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 ~ M7 全部完成，用户 2026-10-19 验收 M7 通过，v0.1.0 已发布，Debian 12 测试机从 v0.0.3 升级验证通过。
- **M8** VLESS Encryption 节点：用户 2026-10-19 验收通过，**暂不发布 v0.2.0**（用户决定）。
- **M9** Shadowsocks 2022（D-110～D-113）：用户 2026-10-19 验收通过。
- **M10** 检测节点连通性（D-114～D-116，DESIGN 9.9）：用户 2026-10-19 验收通过（经 3x-ui 线路机实测，Vision 流控需改为 `xtls-rprx-vision-udp443` 才有 UDP 443）。
- **v0.2.0**（M8 ~ M10）2026-10-19 发布。
- **M11** Hysteria2 节点（D-117，DESIGN 9.10）：方案已确定，未开始。

## 下次会话从这里开始

1. 开始 M11-01：先在测试机实测 Xray hysteria2 入站。
2. v0.2.0 尚未在测试机上用"维护 › 更新脚本"从 v0.1.0 升级验证，可在 M11 测试时顺带做。

## 测试 VPS 的状态（2026-10-19）

- **M10 后**：Debian 11 的 `/usr/local/bin/litenode` 换成 `v0.1.0-m10`（main + M10），ufw 关闭（保留节点的 `litenode:` 规则），LiteNode 防火墙关闭；Debian 12 的转发测试规则已删除；Debian 12 / 13 的 `/tmp` 有 `m10cli.sh`（经节点 TCP 一次、DNS 查询 6 次）。

M8 测试后三台的 `/usr/local/bin/litenode` 换成了 main 的构建（含 M8）；M9 测试后 Debian 11 换成 `v0.1.0-m9`（ufw 和 LiteNode 防火墙测试后都已关闭），保留节点 `SS-42001`（Shadowsocks 2022，端口 42001）供验收；Debian 13 装上了 nftables（转发测试时 LiteNode 自动安装，转发已删除），`/tmp/m9` 有 mihomo v1.19.32 和 sing-box v1.14.3。Debian 12 上用户的 `lnode` 已关闭；Debian 13 的 AWS 安全组已放行 UDP（用户 2026-10-19）。v0.1.0 不认识 VLESS Encryption 节点，Debian 12 上不要再用 v0.1.0 打开。

- **M8 新增**：Debian 12 有节点 `Enc-24680`（VLESS Encryption，端口 24680）；Debian 11 有转发规则 30080 → 198.51.100.252:24680（备注 enc-test）；Debian 11 `/tmp/mihomo` 有测试用的 mihomo，`/tmp/encver` 在 Debian 12 上有各版本 Xray。

以下为冒烟测试结束时的状态：

登录密码记在本机记忆中（用户同意），**不写入仓库**。三台都先用 install.sh 安装 v0.0.3；Debian 11 / 13 随后换成 main 的构建（`v0.1.0-rc`），Debian 12 已用更新脚本从 v0.0.3 升级到 v0.1.0。

- **Debian 12**（SSH 61234，IP 198.51.100.252）：节点 `Reality-443`（443，SNI www.microsoft.com）；没有转发、防火墙关闭；DNS 由 LiteNode 写入 `/etc/resolv.conf`（9.9.9.9 1.1.1.1）；时区 US/Eastern，时间同步开启；没有 swap。`/tmp` 有 `vprobe.sh`、`link2client.py`。
- **Debian 11**（SSH 22，root，IP 203.0.113.181，有 IPv6；原 Ubuntu 24.04 重装）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。为装 curl 临时注释过 `bullseye-security`，已恢复原 `sources.list`（备份 `/root/sources.list.orig`）。M8-02 测试装了 ufw（已关闭并重置规则）。
- **Debian 13**（SSH 22，用户 `admin`，sudo 免密，IP 192.0.2.30，AWS，442M，有 IPv6）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。分享链接时有 IPv4 / IPv6 两个地址可选。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
