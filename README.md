<div align="center">

# LiteNode

**轻量的 VPS 节点部署与管理工具**

单个 Go 程序 · 数字菜单操作 · 为小内存、低配 CPU 的 VPS 设计

[![CI](https://github.com/Aspeternity/litenode/actions/workflows/ci.yml/badge.svg)](https://github.com/Aspeternity/litenode/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Aspeternity/litenode)](https://github.com/Aspeternity/litenode/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/Aspeternity/litenode)](go.mod)
[![License](https://img.shields.io/github/license/Aspeternity/litenode)](LICENSE)

[快速开始](#快速开始) · [功能](#功能) · [协议](#支持的协议) · [使用](#使用) · [常见场景](#常见场景) · [更新与卸载](#更新与卸载)

</div>

---

LiteNode 用一个命令行菜单完成 VPS 上的常见工作：部署 Xray 节点（落地机）、配置 nftables 端口转发（线路机），同时处理防火墙、SSH 安全和系统优化。没有 Web 面板、没有数据库、没有常驻进程，只占一个二进制文件。

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Aspeternity/litenode/main/scripts/install.sh)
```

## 功能

- **节点管理**：VLESS Reality、VLESS Encryption（后量子）、Shadowsocks 2022、Hysteria2；添加、修改、启用 / 禁用、连通性检测。
- **客户端管理**：每个节点下可建多个客户端，每个客户端有独立的凭据和分享链接。
- **分享**：分享链接和终端二维码，自动检测主机地址，支持 IPv4 / IPv6 和 NAT 机的外部端口。
- **端口转发**：nftables 内核转发 TCP / UDP，支持端口段和域名目标，不经过 Xray，几乎没有开销。
- **防火墙**：nftables 防火墙，默认拒绝入站，节点和转发端口自动放行。
- **SSH 安全**：登录方式（密钥 / 密码）、root 登录、SSH 端口、公钥和账户管理。
- **系统优化**：BBR + fq、swap、IPv4 / IPv6 优先级、DNS、时区。
- **维护**：配置导出 / 导入、在线更新（sha256 校验）、两种卸载方式。

**安全的变更方式**

- 所有系统修改都经过 *备份 → 生成 → 校验 → 应用*，失败自动恢复；系统文件原子写入。
- 修改 SSH 或防火墙后需要在新终端确认能登录，**60 秒内未确认自动回滚**，不会把自己锁在外面。
- `--dry-run` 模式只显示将要执行的修改，不实际执行。

## 支持的协议

| 协议 | 传输 | 用途 | 端口 |
|---|---|---|---|
| VLESS + TCP + Reality (Vision) | TCP | 客户端直连，伪装成目标网站 | 默认 443 |
| VLESS + TCP + Encryption (Vision) | TCP | 经中转连接的落地机，ML-KEM-768 + X25519 后量子加密 | 随机 |
| Shadowsocks 2022 (`2022-blake3-aes-128-gcm`) | TCP + UDP | 兼容不支持 VLESS Encryption 的客户端 | 随机 |
| Hysteria2 | UDP | 线路机出站连接落地机，证书指纹固定 | 随机 |

Xray 由 LiteNode 自动下载安装到独立路径，不影响机器上的其他程序。

## 系统要求

| 项目 | 要求 |
|---|---|
| 系统 | Debian 11 / 12 / 13，Ubuntu 22.04 / 24.04 及之后的 LTS |
| 架构 | amd64、arm64 |
| 其他 | systemd，root 权限 |

> 主要在 Debian 12 和 Ubuntu 24.04 上测试，Debian 11 / 13 做过冒烟测试。Debian 11 已于 2026-08-31 停止安全更新，建议使用 Debian 12 或 13。

## 快速开始

### 安装

以 root 身份执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Aspeternity/litenode/main/scripts/install.sh)
```

安装脚本从 GitHub Releases 下载最新版本，校验 sha256 后安装为 `/usr/local/bin/litenode`，并创建快捷命令 `lnode`，然后进入主菜单。再次执行同一条命令即可更新，节点和设置不受影响。

<details>
<summary>安装遇到问题</summary>

- 最小化安装的系统可能没有 curl：先执行 `apt update && apt install -y curl`。
- Debian 11 上 apt 从 `bullseye-security` 下载返回 404（该源的软件包已下架）时，可以暂时注释掉 `/etc/apt/sources.list` 中 `bullseye-security` 那一行再安装。
- 机器无法访问 GitHub：见[无法访问 GitHub 的机器](#无法访问-github-的机器)。

</details>

### 添加第一个节点

1. 执行 `lnode`，进入"节点管理 → 添加节点"。第一次添加时会自动下载安装 Xray。
2. 选择端口（默认 443）和 Reality 目标网站。可以从常用网站中选，也可以自己输入，程序会检测目标网站是否可用。
3. 完成后显示第一个客户端的分享链接和二维码，以后可以在"分享节点"中随时再取。

> 连不上时，先检查云厂商的安全组有没有放行节点端口。安全组在 VPS 外部，LiteNode 看不到也改不了。

## 使用

| 命令 | 作用 |
|---|---|
| `litenode` 或 `lnode` | 进入主菜单 |
| `litenode --dry-run` | 进入菜单，只显示将要执行的修改，不实际执行 |
| `litenode version` | 显示版本 |
| `litenode help` | 显示帮助 |

```
 1. 节点管理        添加、修改、启用 / 禁用、检测连通性、删除节点
 2. 客户端管理      每个节点下的客户端（每个客户端有自己的链接）
 3. 分享节点        导出链接和二维码，配置主机地址
 4. 端口转发        nftables 端口转发（中转机用）
 5. Xray 服务       启动 / 停止、日志、节点出站 IP、更新 Xray
 6. SSH 管理        登录方式、root 登录、SSH 端口、密钥、账户
 7. 防火墙管理      nftables 防火墙
 8. 系统优化        BBR + fq、swap、IPv4 / IPv6 优先级、DNS、时区
 9. 维护            导出 / 导入配置、更新、卸载
```

| 路径 | 内容 |
|---|---|
| `/etc/litenode/` | 配置文件 |
| `/var/log/litenode.log` | 操作日志 |
| `/usr/local/bin/litenode` | 程序（`lnode` 为软链接） |

## 协议说明

<details>
<summary><b>VLESS Encryption</b>：经中转连接的落地机</summary>

<br>

后量子加密（ML-KEM-768 + X25519）的 VLESS 节点，不需要证书和目标网站。线路机用端口转发（或其他中转）把流量转到落地机的这个节点。它不伪装成网站，不建议让客户端直接连接。

- 端口默认随机，备注默认 `Enc-<端口>`。需要 Xray v25.9.5 或更高版本（默认安装的版本满足）。
- 客户端兼容性（2026-10 实测）：

| 客户端 | 支持情况 |
|---|---|
| Xray ≥ v25.9.5 | 直接使用分享链接（链接中的 `encryption` 即客户端配置中的 `encryption`）。v2rayN、v2rayNG 等需要内核 ≥ v25.9.5，且能识别链接中的 `encryption` 参数 |
| mihomo ≥ v1.19.13 | 代理配置写 `encryption: "<链接中 encryption 的值>"`、`tls: false`、`flow: xtls-rprx-vision`；v1.19.12 及以下连不上 |
| sing-box v1.14.3 | 不支持 |

</details>

<details>
<summary><b>Shadowsocks 2022</b>：兼容选项</summary>

<br>

给不支持 VLESS Encryption 的客户端（sing-box 及以它为内核的客户端、Surge、Shadowrocket 等）使用，同样适合经中转连接的落地机。它没有前向保密，Xray 启动时会打印弃用警告，客户端支持时优先用 VLESS Encryption。

- 端口默认随机，备注默认 `SS-<端口>`。节点同时使用 **TCP 和 UDP**：LiteNode 防火墙和 ufw 会同时放行，云厂商安全组也要放行 UDP；经端口转发时协议选"TCP + UDP"。
- 每个客户端有自己的密钥。"重新生成密钥"会让所有客户端的链接失效；只换一个客户端用"客户端管理 → 修改客户端 → 重置密钥"。
- 分享链接为 SIP002 格式，用户信息不做 base64：`ss://2022-blake3-aes-128-gcm:<服务端密钥>%3A<用户密钥>@<地址>:<端口>#<备注>`。手动填写时密码为 `<服务端密钥>:<用户密钥>`。
- 2026-10 实测直连和经端口转发 TCP、UDP 都可用：Xray v26.10.17、mihomo v1.19.32（直接导入分享链接）、sing-box v1.14.3。

</details>

<details>
<summary><b>Hysteria2</b>：线路机出站连接落地机</summary>

<br>

线路机上的代理软件（Xray、mihomo、sing-box）把 Hysteria2 作为出站，连到 LiteNode 落地机。使用 Xray 内置的 Hysteria2，需要 Xray v26.3.27 或更高版本（默认安装的版本满足）。

- 端口默认随机，备注默认 `Hy2-<端口>`。节点**只使用 UDP**，云厂商安全组要放行这个 UDP 端口。
- 证书自动生成（自签，有效期 10 年，不需要域名），线路机用**证书指纹**固定证书，不要开启"跳过证书验证"。分享链接下方列出线路机出站参数：地址、端口、密码、SNI、证书 SHA-256 和公钥 SHA-256。"重新生成证书"会更换指纹，之后要更新线路机配置。
- 每个客户端（即每台线路机）有自己的密码，"重置密码"只影响一个。
- 线路机出站配置（2026-10 实测，TCP 和 UDP 都可用，指纹错误时连不上）：

| 线路机软件 | 配置 |
|---|---|
| Xray ≥ v26.2.6 | `tlsSettings` 写 `"serverName": "www.bing.com", "pinnedPeerCertSha256": "<证书 SHA-256>"` |
| mihomo v1.19.32 | `type: hysteria2`，`sni: www.bing.com`，`fingerprint: <证书 SHA-256>` |
| sing-box v1.14.3 | `tls` 中 `"server_name": "www.bing.com", "certificate_public_key_sha256": ["<公钥 SHA-256>"]` |

- 暂不支持 Salamander 混淆和端口跳跃。

</details>

## 常见场景

### 无法访问 GitHub 的机器

安装 LiteNode、安装或更新 Xray、更新 LiteNode 都需要访问 GitHub。国内线路机和纯 IPv6 的 VPS（github.com 没有 IPv6 地址）通常无法访问。LiteNode 不内置镜像或下载代理：校验文件也要经过代理下载，用代理就等于完全信任代理。

<details>
<summary>手动安装 LiteNode</summary>

<br>

在能访问 GitHub 的电脑上，从 [Releases](https://github.com/Aspeternity/litenode/releases/latest) 下载对应架构的程序和 `sha256sums.txt`（在 VPS 上执行 `uname -m`：`x86_64` 选 `litenode-linux-amd64`，`aarch64` 选 `litenode-linux-arm64`），上传到 VPS：

```bash
scp litenode-linux-amd64 sha256sums.txt root@<VPS 地址>:/root/
```

在 VPS 上校验，显示 `OK` 后再安装（arm64 把文件名中的 `amd64` 换成 `arm64`）：

```bash
cd /root && sha256sum -c --ignore-missing sha256sums.txt
```

```bash
install -m 0755 /root/litenode-linux-amd64 /usr/local/bin/litenode && { [ -e /usr/local/bin/lnode ] || ln -s litenode /usr/local/bin/lnode; } && litenode version
```

以后手动更新也是同样的步骤。

Xray 只能由 LiteNode 从 GitHub 下载，不能手动安装。只做中转的线路机不需要 Xray，手动安装 LiteNode 后就可以使用端口转发、防火墙、SSH 管理和系统优化。

</details>

<details>
<summary>纯 IPv6 的 VPS</summary>

<br>

先让机器能访问 IPv4 网站，再用一行命令安装：

- 开启 Cloudflare WARP，得到 IPv4 出口；或者
- 把 DNS 换成同时提供 NAT64 的公共 DNS64 服务（如 [nat64.net](https://nat64.net/)）。

节点本身用 IPv6 地址连接即可。开启 WARP 后，程序检测到的 IPv4 主机地址是 WARP 的出口地址，用它连接不上：在"分享节点 → 配置主机地址"中把 IPv6 地址设为默认，或删除这个 IPv4 地址。

</details>

### NAT 机

NAT 机没有独立的公网 IP，商家只分配一部分端口，外部端口和内部端口可能不同。

1. 添加节点时，端口填商家分配的**内部端口**。检测到公网 IP 不在本机网卡上时，程序会给出提示。
2. 外部端口和内部端口不同时，在"节点管理 → 修改节点 → 分享端口"中填外部端口。分享链接改用外部端口，Xray 仍然监听内部端口。
3. 检测到的地址不对时，在"分享节点 → 配置主机地址"中修改。

## 更新与卸载

| 操作 | 方式 | 说明 |
|---|---|---|
| 更新 | "维护 → 更新脚本"，或重新执行安装命令 | 旧版本保留为 `/usr/local/bin/litenode.bak` |
| 卸载脚本 | "维护 → 卸载脚本" | 只删除 LiteNode 程序，节点、转发、防火墙照常运行，重新安装后可继续管理 |
| 全部卸载 | "维护 → 全部卸载" | 删除 Xray、所有节点、转发规则和 LiteNode；防火墙如果开着会保持生效 |

两种卸载都不会撤销 SSH、防火墙、BBR、swap、DNS、时区的设置，避免机器恢复成密码登录、端口全开等不安全的状态。卸载完成后会列出保留下来的设置和文件位置。

## 反馈与安全

- 问题和建议：[Issues](https://github.com/Aspeternity/litenode/issues/new/choose)
- 安全漏洞请私密报告，见 [SECURITY.md](SECURITY.md)

## 致谢

- [Xray-core](https://github.com/XTLS/Xray-core)：节点的代理内核
- [nftables](https://netfilter.org/projects/nftables/)：端口转发与防火墙

## 许可证

[GPL-3.0](LICENSE)
