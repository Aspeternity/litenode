# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M2 已由用户验收通过；验收后的两项修改已合并（多个分享地址 D-068，PR #29；页面和操作流程分开显示 D-069，PR #30）。**M3 防火墙进行中**。
- **当前任务**：M3-01 nftables 表结构和规则生成，在分支 `feat/M3-01-nft`（已推送，未开 PR）。
  - 已完成：状态文件 `firewall` 结构；`internal/nft` 规则生成（golden 测试）、`nft` 配置类型（`nft -c` 校验，按磁盘上的文件加载并启用 / 停用 `litenode-nft.service`）；`internal/firewall` 从状态生成规则（`Config`、`SSHPorts`）；回滚子命令登记 `nft`；D-070。
  - 已实测：Debian 12 上 `nft -c -f` 通过；加载后测试端口被拦截，SSH、节点、ping 正常（从 Ubuntu 测试机直连探测；从 WSL 探测不可靠，见下）；之后已移除表，测试机恢复原状。
- **用户要求暂停开发（2026-10-18）**，等用户说继续再开始。

## 下次会话从这里开始

1. 切到 `feat/M3-01-nft`，完成 M3-01 剩余部分：
   - [VPS] 重启后规则仍在：在 Debian 上用 `firewall` 包的 vps 测试 `LN_NFT=on` 加载（先设置应急定时器），确认新的 SSH 连接正常后重启，检查表已加载、SSH 和节点正常，最后 `LN_NFT=off` 移除。
   - DESIGN 第 5 节状态示意中的 `firewall` 结构改为实际字段（`enabled`、`block_ping`、`rules[].sources`、`auto_sources`），第 14 节补充 D-070 的基础规则。
   - 更新 ROADMAP 后开 PR，CI 通过后合并；然后按顺序做 M3-02 ~ M3-04（P-004 在 M3-03 前按 D-050 决定）。
2. 打标签 `v0.0.3` 前先问用户（尚未同意）。
3. `feat/M3-01-nft` 分支上的 PROGRESS 是拆分前的旧版：继续 M3-01 时先 `git merge main`，PROGRESS 冲突以 main 的版本为准，再更新 M3-01 的进度。
4. 实机测试前读 `docs/NOTES.md` 的"实机测试方法"。

## 测试 VPS 的当前状态（2026-10-18）

登录信息由用户在会话中提供，**不写入仓库或记忆**。用户授权两台都可以完全用于测试。

- **Ubuntu 24.04.5**（KVM、x86_64、约 1G 内存，SSH 端口 22，公网 IP 203.0.113.181，有 IPv6）：`/usr/local/bin/litenode` 是 `fix/ui-flow-style` 的编译版本（与合并后的 main `154181f` 相同）；有用户验收时添加的节点 `Reality-443`，Xray 运行中；分享地址有 IPv4 和 IPv6；`00-litenode.conf` 管理着 `Port 22`（带 `ListenAddress`）和 `PasswordAuthentication yes`；root 的 `authorized_keys` 中**已没有**开发机的公钥（用户验收时改过），只能用密码登录，不要在这台机器上关闭密码登录；装有 `litenode-rollback-boot.service`；ufw 已停用。
- **Debian 12**：KVM、x86_64、约 1G 内存，SSH 端口 61234（主配置中有商家写的 `Port 61234`），公网 IP 198.51.100.252。上面有用户的 2 个节点（`Test`、`Test2`）和 Xray；分享地址列表有 IPv4 和 `216-23-88-252.sslip.io`（M1-06 测试时添加）；没有 LiteNode 的 nftables 表（`/etc/litenode/nftables.nft` 只含删除表的两行，`litenode-nft.service` 已停用）；系统自带的 `table inet filter`（全部 accept）保持不变。`/usr/local/bin/litenode` 是 `fix/M1-06` 的编译版本（多个分享地址，早于流程样式）；`00-litenode.conf` 管理着 `Port 61234`（带 `ListenAddress`）。

## 未解决的问题

- 卸载 LiteNode（M6）时要删除 `litenode-rollback-boot.service` 和 `/etc/litenode/rollback/`（D-062）。
- **Xray 被外部删除后无法删除节点**：Xray 程序被手动删除（测试中出现）时，删除节点会因 `xray run -test` 无法执行而失败，恢复也失败。没有节点需要运行时应允许跳过校验，或提示先在 Xray 菜单重新安装。M2 结束前或 M6 维护功能中处理。

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在 M7 前讨论。
