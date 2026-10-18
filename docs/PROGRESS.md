# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 ~ M6 已完成；M7 打磨阶段的 M7-01 ~ M7-05 已完成并合并（D-091 ~ D-095）。
- **当前任务**：用户实机测试 M7-01 ~ 05 后的第二批反馈已确认（D-096），下一步依次做 M7-06 ~ M7-09。

## 下次会话从这里开始

1. 依次完成 M7-06 ~ M7-09（每个任务一个分支和 PR），全部完成后请用户再做实机测试。M7-07 依赖 M7-06。
2. 之后与用户确定 ROADMAP M7"之后"一节：全新安装走查、资源占用检查、文字检查、README。
3. 打标签前先问用户（v0.0.3 尚未同意，可能直接发 v0.1.0）。
4. 实机测试前读 `docs/NOTES.md` 的"实机测试方法"；用户可能正在测试机上测试，上机前先 `pgrep -x lnode`。

## 测试 VPS 的当前状态（2026-10-19）

登录密码记在本机记忆中（用户同意），**不写入仓库**。用户授权两台都可以完全用于测试。两台的 `/usr/local/bin/litenode` 都是 M6-03 分支的编译版本（M7 的测试版本放在 Debian 的 `/root/litenode.m7`（M7-05 分支），直接运行，不替换已安装的程序；Debian 的 `litenode-rollback-boot.service` 因此指向 `/root/litenode.m7`，下次用安装的程序做限时确认时自动改回）（`/root/litenode.dev` 为同一文件）（之前的版本在 `/root/litenode.before-m5`）。

- **Ubuntu 24.04.5**（KVM、x86_64、约 1G 内存，SSH 端口 22，公网 IP 203.0.113.181，有 IPv6）：有用户验收时添加的节点 `Reality-443`，Xray 运行中；`00-litenode.conf` 管理着 `Port 22`（带 `ListenAddress`）和 `PasswordAuthentication yes`；root 的 `authorized_keys` 中**没有**开发机的公钥，只能用密码登录，不要关闭密码登录；装有 `litenode-rollback-boot.service`；ufw 已停用；没有转发规则，LiteNode 防火墙关闭。M5：BBR 由 LiteNode 开启（`sysopt.bbr`，`99-litenode.conf` 只有 BBR 两行；镜像自带的 `99-custom.conf` 也开启 BBR，已恢复原样）；没有 swap；`gai.conf`、DNS（systemd-resolved，无 LiteNode drop-in）、时区 Europe/Berlin 均为测试前的状态；节点出站 IP 为默认。
- **Debian 12**（KVM、x86_64、约 1G 内存，SSH 端口 61234，公网 IP 198.51.100.252，磁盘只剩约 550M）：用户的 2 个节点（`Test`、`Test2`）和 Xray；没有 LiteNode 的 nftables 表，防火墙关闭；`00-litenode.conf` 管理着 `Port 61234`，M7-03 测试后还显式管理 `PubkeyAuthentication yes`、`PasswordAuthentication yes`、`PermitRootLogin yes`（与原生效值相同），root 没有公钥。M6-03 测试过全部卸载后已恢复（状态文件 + 导入备份，Xray 重新安装为 v26.10.17，`litenode-rollback-boot.service` 已删除，下次限时确认时自动重装）。M5：BBR 由镜像开启（LiteNode 未管理）；没有 swap；resolv.conf 已恢复为 resolvconf 链接（1.1.1.1）；**新装了 systemd-timesyncd，时间同步已开启**；时区 US/Eastern（原值）。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在加入 TLS 类节点类型之前讨论。
