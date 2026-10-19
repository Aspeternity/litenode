# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 ~ M7 全部完成，用户 2026-10-19 验收 M7 通过，v0.1.0 已发布，Debian 12 测试机从 v0.0.3 升级验证通过。
- **当前阶段**：M8 VLESS Encryption 节点（D-106）开发完成（M8-01～03，D-108、D-109），**等待用户实机验收 M8**。

## 下次会话从这里开始

1. 用户验收 M8 后：是否发布 v0.2.0 先问用户；然后开始 M9-01（Shadowsocks 2022，DESIGN 9.8、D-110）。

## 测试 VPS 的状态（2026-10-19，M8 结束时）

M8 测试后三台的 `/usr/local/bin/litenode` 换成了 main 的构建（含 M8）。v0.1.0 不认识 VLESS Encryption 节点，Debian 12 上不要再用 v0.1.0 打开。

- **M8 新增**：Debian 12 有节点 `Enc-24680`（VLESS Encryption，端口 24680）；Debian 11 有转发规则 30080 → 216.23.88.252:24680（备注 enc-test）；Debian 11 `/tmp/mihomo` 有测试用的 mihomo，`/tmp/encver` 在 Debian 12 上有各版本 Xray。

以下为冒烟测试结束时的状态：

登录密码记在本机记忆中（用户同意），**不写入仓库**。三台都先用 install.sh 安装 v0.0.3；Debian 11 / 13 随后换成 main 的构建（`v0.1.0-rc`），Debian 12 已用更新脚本从 v0.0.3 升级到 v0.1.0。

- **Debian 12**（SSH 61234，IP 216.23.88.252）：节点 `Reality-443`（443，SNI www.microsoft.com）；没有转发、防火墙关闭；DNS 由 LiteNode 写入 `/etc/resolv.conf`（9.9.9.9 1.1.1.1）；时区 US/Eastern，时间同步开启；没有 swap。`/tmp` 有 `vprobe.sh`、`link2client.py`。
- **Debian 11**（SSH 22，root，IP 62.84.164.181，有 IPv6；原 Ubuntu 24.04 重装）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。为装 curl 临时注释过 `bullseye-security`，已恢复原 `sources.list`（备份 `/root/sources.list.orig`）。M8-02 测试装了 ufw（已关闭并重置规则）。
- **Debian 13**（SSH 22，用户 `admin`，sudo 免密，IP 54.178.218.30，AWS，442M，有 IPv6）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。分享链接时有 IPv4 / IPv6 两个地址可选。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在加入 TLS 类节点类型之前讨论。
