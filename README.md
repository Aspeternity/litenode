# LiteNode

LiteNode 是一个轻量的 VPS 节点部署与管理工具，类似没有 Web 界面的 3x-ui。

- 单个 Go 程序，没有其他依赖，通过数字菜单操作，适合小内存、低配 CPU 的 VPS。
- 功能：Xray 节点（VLESS + TCP + Reality、VLESS + TCP + VLESS Encryption、Shadowsocks 2022）和客户端管理、分享链接和二维码、nftables 端口转发、nftables 防火墙、SSH 安全配置、系统优化（BBR、swap、DNS 等）、备份与更新。
- 支持系统：Debian 11 / 12 / 13，Ubuntu 22.04 / 24.04 及之后的 LTS；架构 amd64、arm64。需要 systemd 和 root 权限。
- Debian 11 已于 2026-08-31 停止安全更新，建议使用 Debian 12 或 13。在 Debian 11 上 apt 从 `bullseye-security` 下载时如果返回 404（该源的软件包已下架），可以暂时注释掉 `/etc/apt/sources.list` 中 `bullseye-security` 那一行再安装。

> 主要在 Debian 12 和 Ubuntu 24.04 上测试，Debian 11 / 13 做过冒烟测试。

## 安装

以 root 身份执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Aspeternity/litenode/main/scripts/install.sh)
```

最小化安装的系统可能没有 curl（如部分 Debian 11 镜像），先执行 `apt update && apt install -y curl`。

安装脚本从 GitHub Releases 下载最新版本，校验 sha256 后安装为 `/usr/local/bin/litenode`，同时创建快捷命令 `lnode`，然后进入主菜单。再次执行同一条命令会把程序更新到最新版本，节点和设置不受影响。

机器无法访问 GitHub 时，见下方[无法访问 GitHub 的机器](#无法访问-github-的机器)。

## 使用

```bash
lnode
```

| 命令 | 作用 |
|---|---|
| `litenode` 或 `lnode` | 进入主菜单 |
| `litenode --dry-run` | 进入菜单，但只显示将要执行的修改，不实际执行 |
| `litenode version` | 显示版本 |
| `litenode help` | 显示帮助 |

主菜单：

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

### 添加第一个节点

1. 进入"节点管理 → 添加节点"。第一次添加时会自动下载安装 Xray。
2. 按提示选择端口（默认 443）和 Reality 目标网站。可以从常用网站中选，也可以自己输入。程序会检测目标网站是否可用。
3. 添加完成后会显示第一个客户端的分享链接和二维码。以后在主菜单"分享节点"中可以随时再取。
4. 连不上时，先检查云厂商的安全组（防火墙）有没有放行节点端口。这在 VPS 外部，LiteNode 看不到也改不了。

### VLESS Encryption 节点

"VLESS + TCP + Encryption (Vision)"是后量子加密（ML-KEM-768 + X25519）的 VLESS 节点，不需要证书和目标网站，适合**经中转连接的落地机**：线路机用端口转发（或其他中转）把流量转到落地机的这个节点。它不伪装成网站，不建议让客户端直接连接。

- 添加时端口默认随机，备注默认 `Enc-<端口>`。需要 Xray v25.9.5 或更高版本（LiteNode 默认安装的版本满足）。
- 客户端必须支持 VLESS Encryption。实测（2026-10）：
  - Xray v25.9.5 及以上：可以直接使用分享链接（链接中的 `encryption` 参数就是客户端配置中的 `encryption`）。v2rayN、v2rayNG 等以 Xray 为内核的客户端需要内核不低于 v25.9.5，并且客户端本身能识别链接中的 `encryption` 参数。
  - mihomo（Clash Meta 内核）v1.19.13 及以上：在代理配置中写 `encryption: "<链接中 encryption 的值>"`，`tls: false`、`flow: xtls-rprx-vision`。v1.19.12 及以下连不上。
  - sing-box（v1.14.3）：不支持。

### Shadowsocks 2022 节点

"Shadowsocks 2022"（`2022-blake3-aes-128-gcm`）是**兼容选项**：给不支持 VLESS Encryption 的客户端（sing-box 及以它为内核的客户端、Surge、Shadowrocket 等）使用，同样适合经中转连接的落地机。它没有前向保密，Xray 启动时会打印弃用警告，客户端支持时优先用 VLESS Encryption。

- 添加时端口默认随机，备注默认 `SS-<端口>`。节点同时使用 **TCP 和 UDP**：LiteNode 防火墙和 ufw 会同时放行两种协议，云厂商的安全组也要放行 UDP（否则只有 TCP 可用）；经端口转发时转发规则选"TCP + UDP"。
- 每个客户端有自己的密钥。"重新生成密钥"会让所有客户端的链接失效；只换一个客户端用"客户端管理 → 修改客户端 → 重置密钥"。
- 分享链接为 SIP002 格式，2022 方法的用户信息不做 base64：`ss://2022-blake3-aes-128-gcm:<服务端密钥>%3A<用户密钥>@<地址>:<端口>#<备注>`。手动填写客户端时，密码为 `<服务端密钥>:<用户密钥>`（两段用英文冒号连接）。
- 实测（2026-10，直连和经 LiteNode 端口转发，TCP 和 UDP 都通）：Xray v26.10.17、mihomo v1.19.32（直接导入分享链接）、sing-box v1.14.3。

### 改 SSH 和防火墙时的保护

修改 SSH 登录方式、SSH 端口、密码，或者启用 / 修改防火墙后，LiteNode 会请你**新开一个终端**测试能否登录，确认无误后回到原来的窗口输入 `y`。60 秒内没有确认（包括连接断开、机器重启），修改会自动撤销，所以设置错了也不会把自己锁在外面。程序还会拒绝明显会锁死的操作，比如没有公钥时切换为"仅密钥"登录。

## 更新与卸载

- **更新**："维护 → 更新脚本"，或者重新执行安装命令。旧版本保留为 `/usr/local/bin/litenode.bak`。
- **卸载脚本**：只删除 LiteNode 程序，节点、转发、防火墙照常运行，重新安装后可以继续管理。
- **全部卸载**：删除 Xray、所有节点、转发规则和 LiteNode。防火墙如果开着会保持生效。

两种卸载都不会撤销 SSH、防火墙、BBR、swap、DNS、时区的设置，避免机器恢复成密码登录、端口全开等不安全的状态。卸载完成后会列出保留下来的设置和文件位置。

配置文件在 `/etc/litenode/`，操作日志在 `/var/log/litenode.log`。

## 无法访问 GitHub 的机器

安装 LiteNode、安装或更新 Xray、更新 LiteNode 都需要访问 GitHub。国内线路机和纯 IPv6 的 VPS（github.com 没有 IPv6 地址）通常无法访问。LiteNode 不内置镜像或下载代理：校验文件也要经过代理下载，用代理就等于完全信任代理。

### 手动安装

在能访问 GitHub 的电脑上，从 [Releases](https://github.com/Aspeternity/litenode/releases/latest) 下载对应架构的程序（`litenode-linux-amd64` 或 `litenode-linux-arm64`。不知道是哪种时，在 VPS 上执行 `uname -m`：`x86_64` 选 amd64，`aarch64` 选 arm64）和 `sha256sums.txt`，上传到 VPS：

```bash
scp litenode-linux-amd64 sha256sums.txt root@<VPS 地址>:/root/
```

在 VPS 上校验并安装（arm64 把文件名中的 `amd64` 换成 `arm64`）：

```bash
cd /root && sha256sum -c --ignore-missing sha256sums.txt
```

```bash
install -m 0755 /root/litenode-linux-amd64 /usr/local/bin/litenode && { [ -e /usr/local/bin/lnode ] || ln -s litenode /usr/local/bin/lnode; } && litenode version
```

校验结果显示 `OK` 后再安装。以后手动更新也是同样的步骤。

### Xray

Xray 只能由 LiteNode 从 GitHub 下载，不能手动安装。只做中转（端口转发）的线路机不需要 Xray，手动安装 LiteNode 后就可以使用端口转发、防火墙、SSH 管理和系统优化。

### 纯 IPv6 的 VPS

先让机器能访问 IPv4 网站，再用一行命令安装：

- 开启 Cloudflare WARP，得到 IPv4 出口；或者
- 把 DNS 换成同时提供 NAT64 的公共 DNS64 服务（如 [nat64.net](https://nat64.net/)）。

节点本身用 IPv6 地址连接就可以。开启 WARP 后，程序检测到的 IPv4 主机地址是 WARP 的出口地址，用它连接不上：在"分享节点 → 配置主机地址"中把 IPv6 地址设为默认，或删除这个 IPv4 地址。

## NAT 机

NAT 机没有独立的公网 IP，商家只分配一部分端口，外部端口和机器内部的端口可能不同。

1. 添加节点时，端口填商家分配给你的**内部端口**。检测到公网 IP 不在本机网卡上时，程序会给出提示。
2. 如果外部端口和内部端口不同，在"节点管理 → 修改节点 → 分享端口"中填外部端口。分享链接会改用外部端口，Xray 仍然监听内部端口。
3. 如果检测到的地址不对，在"分享节点 → 配置主机地址"中修改。

## 协议

[GPL-3.0](LICENSE)
