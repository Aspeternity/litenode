# 开发进度

每次会话结束前更新本文件。**只写当前状态和下一步，保持简短**（约 5000 字节以内）；实现约定和测试方法写在 `docs/NOTES.md`，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 ~ M6 已完成；M7 全部完成（M7-01 ~ M7-14），等待用户对 M7 实机验收。
- **发布顺序**（D-101）：Debian 11 / 13 冒烟测试已通过 → 用户验收 M7 → v0.1.0（打标签前先问用户）。

## 下次会话从这里开始

1. 用户验收 M7 后准备 v0.1.0（打标签前先问用户）。
2. 发布后在测试机上用"维护 › 更新脚本"从 v0.0.3 升级，验证更新功能。

## 测试 VPS 的状态（2026-10-19，冒烟测试结束时）

登录密码记在本机记忆中（用户同意），**不写入仓库**。三台都先用 install.sh 安装 v0.0.3；Debian 11 / 13 随后换成 main 的构建（`v0.1.0-rc`），Debian 12 仍是 v0.0.3（发布后用它验证从 v0.0.3 升级）。

- **Debian 12**（SSH 61234，IP 216.23.88.252）：节点 `Reality-443`（443，SNI www.microsoft.com）；没有转发、防火墙关闭；DNS 由 LiteNode 写入 `/etc/resolv.conf`（9.9.9.9 1.1.1.1）；时区 US/Eastern，时间同步开启；没有 swap。`/tmp` 有 `vprobe.sh`、`link2client.py`。
- **Debian 11**（SSH 22，root，IP 62.84.164.181，有 IPv6；原 Ubuntu 24.04 重装）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。为装 curl 临时注释过 `bullseye-security`，已恢复原 `sources.list`（备份 `/root/sources.list.orig`）。
- **Debian 13**（SSH 22，用户 `admin`，sudo 免密，IP 54.178.218.30，AWS，442M，有 IPv6）：节点 `Reality-443`（443，SNI www.microsoft.com），其余未设置。分享链接时有 IPv4 / IPv6 两个地址可选。

## 未解决的问题

- 附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在加入 TLS 类节点类型之前讨论。
