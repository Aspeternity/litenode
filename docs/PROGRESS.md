# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M2 已由用户验收通过；验收后的两项修改已合并（多个分享地址 D-068，PR #29；页面和操作流程分开显示 D-069，PR #30）。**M3 防火墙进行中**。
- **当前任务**：M3-01 已完成（PR 合并后）；下一个是 M3-02 启用、关闭、重置。

## 下次会话从这里开始

1. 从 main 切出 M3-02 分支，按 ROADMAP 做 M3-02 ~ M3-04（P-004 在 M3-03 前按 D-050 决定）。可复用 `internal/firewall` 的 `Config`、`SSHPorts` 和 `nft` 配置类型。
2. 打标签 `v0.0.3` 前先问用户（尚未同意）。
3. 实机测试前读 `docs/NOTES.md` 的"实机测试方法"。

## 测试 VPS 的当前状态（2026-10-18）

登录信息由用户在会话中提供，**不写入仓库或记忆**。用户授权两台都可以完全用于测试。

- **Ubuntu 24.04.5**（KVM、x86_64、约 1G 内存，SSH 端口 22，公网 IP 203.0.113.181，有 IPv6）：`/usr/local/bin/litenode` 是 `fix/ui-flow-style` 的编译版本（与合并后的 main `154181f` 相同）；有用户验收时添加的节点 `Reality-443`，Xray 运行中；分享地址有 IPv4 和 IPv6；`00-litenode.conf` 管理着 `Port 22`（带 `ListenAddress`）和 `PasswordAuthentication yes`；root 的 `authorized_keys` 中**已没有**开发机的公钥（用户验收时改过），只能用密码登录，不要在这台机器上关闭密码登录；装有 `litenode-rollback-boot.service`；ufw 已停用。
- **Debian 12**：KVM、x86_64、约 1G 内存，SSH 端口 61234（主配置中有商家写的 `Port 61234`），公网 IP 198.51.100.252。上面有用户的 2 个节点（`Test`、`Test2`）和 Xray；分享地址列表有 IPv4 和 `216-23-88-252.sslip.io`（M1-06 测试时添加）；没有 LiteNode 的 nftables 表（M3-01 重启测试后已移除，`/etc/litenode/nftables.nft` 只含删除表的两行，`litenode-nft.service` 已停用）；系统自带的 `table inet filter`（全部 accept）保持不变。`/usr/local/bin/litenode` 是 `fix/M1-06` 的编译版本（多个分享地址，早于流程样式）；`00-litenode.conf` 管理着 `Port 61234`（带 `ListenAddress`）。

## 未解决的问题

- 卸载 LiteNode（M6）时要删除 `litenode-rollback-boot.service` 和 `/etc/litenode/rollback/`（D-062）。
- **Xray 被外部删除后无法删除节点**：Xray 程序被手动删除（测试中出现）时，删除节点会因 `xray run -test` 无法执行而失败，恢复也失败。没有节点需要运行时应允许跳过校验，或提示先在 Xray 菜单重新安装。M2 结束前或 M6 维护功能中处理。

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在 M7 前讨论。
