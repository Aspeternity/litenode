# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **M0 ~ M11 全部完成并经用户验收**，最新版本 **v0.3.4**（2026-10-25，FIX-05、FIX-06；v0.3.3 为 FIX-04；v0.3.2 为 FIX-03 与 Go 1.27.3；v0.3.1 为 FIX-01、FIX-02；v0.3.0 为 M11 Hysteria2）。各阶段内容见 ROADMAP，发布记录见 GitHub Releases。
- **用户决定（2026-10-20）：暂时不增加新功能**。ROADMAP"未排期"中的项目保留，等用户提出再排期。
- **项目收尾（2026-10-20，D-121）**：测试用的真实 IP 换成文档专用地址；新增 Dependabot、Issue 模板、SECURITY.md、CI 的 govulncheck、README 徽章；调整 GitHub 仓库设置。

## 下次会话从这里开始

1. **M12 进行中**：M12-01（#105）、M12-02（#106）、M12-03（#107）已合并。**M12-04 WARP 管理**代码和单元测试已完成，在分支 `feat/M12-04-warp`（已推送，未开 PR）。实机已验证：纯 IPv6 开启 WARP 成功、IPv6 和 SSH 不受影响、重启后 IPv4 出口仍为 WARP。**待验证**：端点检查定时器自动换端点（测试机 conf 的 Endpoint 被故意改成 `[2001:db8::1]:2408`，看 `journalctl -u litenode-warp-check` 和 conf 是否已换回可用端点）；关闭 WARP 后恢复。然后更新 ROADMAP、开 PR、合并。
2. 之后 M12-05（Reality 解析/连接分开限时、DNS 预设与不可达标记、出站 IP 与转发用出口检测、`NetReason` 按地址族说明、扫描在纯 IPv6 下拒绝、全部卸载列出 DNS 和 WARP）、M12-06（README、从零实机走一遍），阶段结束请用户验收。

## 测试 VPS

- **纯 IPv6 测试机**（2026-10-27，Debian 13，425M，只有内网 IPv4）：访问方式在本机记忆中。WSL 没有 IPv6，经 Windows 上的中转程序作 ssh ProxyCommand 连接（见记忆）。当前状态：手动的 `wg-quick@warp` 已停用（disable），改由 LiteNode 开发版（/usr/local/bin/litenode 为 M12 开发构建）的 `wg-quick@litenode-warp` 提供 IPv4；之前曾手动开启 WARP（`wg-quick@warp`，只承载 IPv4，`/etc/wireguard/warp.conf`），DNS 为商家原配置（备份 `/root/resolv.conf.orig`），一行命令装了 LiteNode v0.3.4，**Xray 未装、无节点**（WARP 出口被 GitHub API 限流）；BBR 开启。测 M12 前可关闭 WARP（`systemctl disable --now wg-quick@warp`）还原纯 IPv6。
- **双栈测试机**（2026-10-27，Debian 11，用户说可随便使用）：作客户端 / 探测机，能经 IPv6 访问纯 IPv6 机；上面有 LiteNode v0.2.0-m11 和 Xray。访问方式在本机记忆中。
- 原 Debian 12 测试机（2026-10-23）已改为仅密钥登录，暂不可用。
- 测试机的 IP、密码只保存在本机记忆中，**不写入仓库**（测试数据用文档专用地址 192.0.2.0/24、198.51.100.0/24、203.0.113.0/24、2001:db8::/32）。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 纯 IPv6 实测问题见 D-129，由 M12 解决。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
