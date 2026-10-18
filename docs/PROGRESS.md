# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M5 系统优化**已完成**（M5-01 ~ M5-07），用户要求接着做 M6，M6 完成后与 M5 一起汇报验收（2026-10-19）。
- **当前任务**：M6-03 卸载（M6-01、M6-02 已完成）。

## 下次会话从这里开始

1. 按顺序做 M6-01 ~ M6-04，全部完成后与 M5 一起请用户实机验收（M5-01 ~ 06 的验收步骤见 PR #44 描述，M5-07 见其 PR）。
2. 打标签前先问用户（v0.0.3 尚未同意）。
3. 实机测试前读 `docs/NOTES.md` 的"实机测试方法"（含 M5 的测试注意事项）。

## 测试 VPS 的当前状态（2026-10-19）

登录密码记在本机记忆中（用户同意），**不写入仓库**。用户授权两台都可以完全用于测试。两台的 `/usr/local/bin/litenode` 分别是 M6-02（Ubuntu）和 M6-01（Debian）分支的编译版本（之前的版本在 `/root/litenode.before-m5`）。

- **Ubuntu 24.04.5**（KVM、x86_64、约 1G 内存，SSH 端口 22，公网 IP 203.0.113.181，有 IPv6）：有用户验收时添加的节点 `Reality-443`，Xray 运行中；`00-litenode.conf` 管理着 `Port 22`（带 `ListenAddress`）和 `PasswordAuthentication yes`；root 的 `authorized_keys` 中**没有**开发机的公钥，只能用密码登录，不要关闭密码登录；装有 `litenode-rollback-boot.service`；ufw 已停用；没有转发规则，LiteNode 防火墙关闭。M5：BBR 由 LiteNode 开启（`sysopt.bbr`，`99-litenode.conf` 只有 BBR 两行；镜像自带的 `99-custom.conf` 也开启 BBR，已恢复原样）；没有 swap；`gai.conf`、DNS（systemd-resolved，无 LiteNode drop-in）、时区 Europe/Berlin 均为测试前的状态；节点出站 IP 为默认。
- **Debian 12**（KVM、x86_64、约 1G 内存，SSH 端口 61234，公网 IP 198.51.100.252，磁盘只剩约 550M）：用户的 2 个节点（`Test`、`Test2`）和 Xray；没有 LiteNode 的 nftables 表，防火墙关闭；`00-litenode.conf` 管理着 `Port 61234`。M5：BBR 由镜像开启（LiteNode 未管理）；没有 swap；resolv.conf 已恢复为 resolvconf 链接（1.1.1.1）；**新装了 systemd-timesyncd，时间同步已开启**；时区 US/Eastern（原值）。

## 未解决的问题

- 卸载 LiteNode（M6）时要删除 `litenode-rollback-boot.service` 和 `/etc/litenode/rollback/`（D-062）。
- **Xray 被外部删除后无法删除节点**：Xray 程序被手动删除（测试中出现）时，删除节点会因 `xray run -test` 无法执行而失败，恢复也失败。没有节点需要运行时应允许跳过校验，或提示先在 Xray 菜单重新安装。M2 结束前或 M6 维护功能中处理。

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在 M7 前讨论。
