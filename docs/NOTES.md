# 开发备忘

实现约定和实机测试方法。不必每次会话都读：做相关任务时再读对应部分（如改菜单读"界面"，做 VPS 验证读"实机测试方法"）。新的约定写在这里，不写进 PROGRESS。

## 实机测试方法

- 远程执行：`scripts/dev/vps.sh`（`VPS_HOST`、`VPS_PORT`、`LN_PASS` 环境变量，密码不落盘）。
- `VPS_SSH_OPTS=-tt scripts/dev/vps.sh '...'` 以带终端的会话执行（模拟用户，断开时进程收到 SIGHUP）。
- 驱动真实菜单：`printf '1\n1\n...' | NO_COLOR=1 litenode`（菜单从标准输入逐行读取）。管道输入不会出现"按回车返回菜单"（D-097）；要测试终端行为，在测试机上用 pty 驱动脚本逐步输入：`scripts/dev/ptyx.py`（步骤文件每行"正则<Tab>输入[<Tab>超时]"，`@show` 打印到此为止的输出；限时确认可在提示处结束脚本来测试超时恢复，此时注意自己的登录方式会被临时修改）。
- **使用场景**：LiteNode 主要用作落地机。WSL 到 VPS 的直连经过中国，是用户在软路由上特意配置的模拟环境；直连被干扰不作为取舍依据，以 VPS 本机的协议检测为准（D-059）。
- 客户端连通性：在 WSL 下载官方 Xray（放在 scratchpad 的独立目录，校验 `.dgst`），`scripts/dev/link2client.py` 把链接转成客户端配置，经 SOCKS 访问 `https://www.cloudflare.com/cdn-cgi/trace`，出口 IP 为 VPS 即连通。
- **WSL 到 VPS 的直连经过 GFW（中国移动）**：所以"从中国大陆能连"和"协议上可用"要分开测；后者把 `scripts/dev/link2client.py`、`scripts/dev/vprobe.sh` 和 `scripts/dev/dnsprobe.py` 传到 VPS 的 `/tmp`，在 VPS 本机运行（注意测试机重启会清空 `/tmp`）。`link2client.py` 支持 vless:// 和 ss://；`/tmp/vprobe.sh '<链接>' udp` 另外经节点向 1.1.1.1 发一次 DNS 查询测试 UDP（M9-03）。
- **Debian 13（AWS）**：M9 测试时安全组没有放行 UDP，用户 2026-10-19 已打开。测试机上有用户开着的 `lnode` 时不要动它，可以用临时的独立 Xray（`xray run -c` 一份只含要测入站的配置）当落地机。
- 带 `vps` 构建标签的集成测试（`ports` 用 `VPS_SSH_PORT` 指定 sshd 端口，默认 22；`ssh` 会真实修改 sshd 配置，带 3 分钟自动恢复定时器）：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o x.test ./internal/<包>`，传到测试机以 root 运行 `./x.test -test.v -test.run TestVPS`。已有：`xray`、`change`、`share`、`reality`、`ufw`、`ports`、`firewall`、`ssh`、`forwardmgr`、`maint`。
- **测试防火墙时从另一台测试机探测端口**：WSL 到 VPS 的直连经过用户的中国代理，代理会自己完成 TCP 握手，被拦截的端口也显示"open"（M3-01 实测）。
- 测试"检查并修复"：用 `nft -a list chain inet litenode input` 找到规则的 handle，`nft delete rule inet litenode input handle <N>` 删除或 `nft insert rule ...` 加入带 `litenode:` 注释的规则，制造差异后再运行菜单。探测端口可在探测机上用 `timeout 4 bash -c "</dev/tcp/<IP>/<端口>"`，被测端口用 `python3 -m http.server <端口>` 监听。
- **测试端口转发**（M4-01）：Ubuntu 当线路机、Debian 当落地机。`forwardmgr` 的实机测试程序用 `LN_FWD=add|delete|fw-on|fw-off|link|golden` 直接修改测试机的真实状态（用法见 `internal/forwardmgr/vps_test.go` 开头）。在落地机上用 `LN_FWD=link LN_ADDR=<线路机 IP> LN_PORT=<转发端口>` 生成经线路机的链接，交给 `/tmp/vprobe.sh`（出口 IP 为落地机即通）；UDP 用落地机上的 Python 回显服务，回复中的来源地址为线路机 IP 说明 masquerade 生效。开 ufw 前先 `ufw allow 22/tcp`，并用 `systemd-run --on-active=20m ufw disable` 做保险。
- **测试域名转发的重新解析**（M4-02）：在线路机 `/etc/hosts` 写入测试域名（纯 Go 解析器读取 hosts，只有 A 记录时 AAAA 查询返回 `no suitable address found`），用 `LN_FWD=add` 添加指向该域名的转发，再修改 hosts 中的 IP，循环检查 `nft list table inet litenode` 中的 DNAT 地址并看 `journalctl -u litenode-resolve.service`。持锁跳过：`flock /run/litenode.lock sleep 3 & litenode internal resolve`。测试后删除 hosts 中的测试行。
- 测试防火墙重启后的状态：先装一个开机 10 分钟后 `nft delete table inet litenode` 的临时服务（`Type=simple`，oneshot 会拖住 `multi-user.target`）作为应急，验证后删除。远程 `pkill -f` 的模式会匹配执行它的 shell，按端口结束进程用 `fuser -k <端口>/tcp`。
- 测试机上用户可能开着 `lnode`（实例锁）：运行菜单前先 `pgrep -x lnode` 检查，不要结束用户的进程；远程 `pkill -f` 的匹配模式不能匹配到执行它的 shell 命令本身。
- 非 root 用户登录的测试机（如 AWS 的 Debian 13 用 `admin`）：`VPS_USER=admin scripts/dev/vps.sh 'sudo ...'`。
- **测试连通性检测**（M10-02）：落地机上用 `ptyx.py` 驱动菜单开始检测，几秒后在**另一台**测试机上运行客户端 `scripts/dev/m10cli.sh`（本机连自己不经过防火墙）。一次 DNS 查询只有 1 个 UDP 包，要连续查询 6 次才能达到"通"（5 个包）。LiteNode 每次打开菜单都会补回 ufw 规则（D-038），所以要在检测开始**之后**删除 ufw / LiteNode 防火墙的规则。`ptyx.py` 超时会 SIGKILL 程序，临时表留到下次启动时删除。非 root 用户的测试机上客户端配置不要写到 root 留下的 `/tmp/c.json`。
- 验证脚本只输出结论（PASS / FAIL、关键几行），不要把整屏菜单或完整日志拿回会话（节省 token，见 CLAUDE.md）。

## 资源占用（M7-11，2026-10-19）

测量对象：main 构建（与 release 相同：`CGO_ENABLED=0 -trimpath -ldflags "-s -w"`，Go 1.27.2）和 LiteNode 默认安装的 Xray v26.10.17，两台 1G 测试机，Xray 各有一个 Reality 节点、空闲（无客户端流量）。方法：菜单在 `script` 创建的 pty 中运行，读 `/proc/<pid>/status`（VmRSS、VmHWM、RssAnon、RssFile）和 `/proc/<pid>/stat` 的 CPU 时间，空闲 60 秒，再依次进入主菜单的 9 个页面并返回。

| 项目 | Debian 12（1 核，973M） | Ubuntu 24.04（2 核，937M） |
|---|---|---|
| litenode 程序大小 | 8.2 MiB（8573088 字节） | 同左 |
| Xray 程序（`/usr/local/lib/litenode`） | 33.4 MiB | 同左 |
| 菜单启动后 RSS | 9.5 MiB，7 线程 | 8.2 MiB，8 线程 |
| 进入全部页面后 RSS / 峰值 | 10.5 MiB | 9.5 MiB |
| 菜单空闲 60 秒 CPU | 0 ms | 0 ms |
| Xray RSS（运行约 30~55 分钟） | 38.8 MiB（匿名 12.0 MiB，文件映射 26.8 MiB） | 33.1 MiB（匿名 6.4 MiB，文件映射 26.7 MiB） |
| Xray cgroup 内存（MemoryCurrent） | 12.5 MiB | 6.8 MiB |
| Xray 空闲 60 秒 CPU | 10 ms | 0 ms |

- 不使用菜单时 LiteNode 没有常驻进程：`litenode-nft.service`、`litenode-rollback-boot.service` 是一次性服务；`litenode-resolve.timer` 只在有域名目标的转发规则时存在，每次运行几毫秒后退出。
- Xray 的 RSS 大部分是程序文件的映射页，内存紧张时可以被回收；实际独占内存看 RssAnon 或 cgroup 的值。
- 结论：没有需要单独修复的问题。1G 内存的机器上 LiteNode 菜单约 10M、Xray 空闲约 7~12M 独占内存，空闲 CPU 可以忽略。没有测量有流量时 Xray 的内存（取决于连接数，由 Xray 本身决定）。

## 实现约定

- 界面（D-069）：页面中的选项默认作为操作流程运行，打开其他页面的选项要标 `Page: true`；操作成功时调用 `s.Done(text.X)` 结束流程（之后输出的链接等不加前缀），多步操作用 `s.Step(n, total, 名称)` 标步骤；没有调用 `Done` 就返回 nil 显示为"已取消"。显示了需要阅读的内容（链接、日志、密钥、测试结果）的操作调用 `s.Hold()`；以警告结束用 `s.EndWarn(text.X)`（自动停住）；返回错误自动停住（D-097）。

- 每个 Go 包用 `doc.go` 写包注释作为占位；尚无文件的目录（`scripts/`、`.github/workflows/`）用 `.gitkeep`，有真实文件后删除。
- `go.mod` 使用 `go 1.27.2`（开发环境的 Go 版本）。
- `internal/text` 已创建（M0-02，dry-run 输出文字），`TestNoChineseOutsideText` 扫描其他包的非测试 Go 文件（含注释），发现中文即失败。
- 执行层用法：只读用 `Query` / `ReadFile`，修改用 `Exec` / `WriteFile` / `Remove` / `MkdirAll`；systemd 操作用 `system.Systemctl`、`DaemonReload`、`ServiceActive`、`ServiceEnabled`。命令执行时设置 `LC_ALL=C`，便于解析输出。
- 其他包的测试用 `systemtest.New()`：`Respond` 设定命令输出，`Files` 模拟文件，`WriteErr` 模拟写入失败，`Execs()` / `Mutations()` 断言。
- 启动流程在 `internal/app`：root 检查 → `system.Detect` → 版本判断 → 实例锁 → 加载状态 → 主菜单（`app/menu.go`，子菜单为"开发中"占位）。
- 菜单：`ui.Menu` 每次显示前重新构建（`Session.Run(build)`），`Hidden` 的选项不占编号；子菜单只有 `0` 上方一条分隔线，`Grouped`（仅主菜单）按 `Group` 分隔且 `0` 归入最后一组。确认用 `Session.Confirm`，输入用 `Session.Prompt`；返回 `ui.ErrQuit` 时原样向上返回。
- 菜单渲染测试用 golden file（`internal/ui/testdata/main_menu.golden`，与 DESIGN 第 8 节一致）。
- 状态文件：`state.Store` 的 `Load` / `Save` 经过 Runner；新 ID 用 `State.AllocID()`；迁移函数登记在 `internal/state/migrate.go` 的 `migrations`（目前为空）。功能任务新增字段时同步更新 DESIGN 第 5 节的结构示意。
- 迁移前的备份写在 `/etc/litenode/backups/state-v<N>-<时间>.json`，不参与按类型的数量清理。
- 配置变更（D-040，M1-00）：`change.Manager.Apply(ctx, change.Change{Parts: []change.Part{{Kind, Files}, ...}, State})`，一次变更可包含多类配置，按 Parts 顺序校验和应用，失败整体恢复；没有文件的类型（如 ufw 对账）只给 Kind。需要限时确认的用 `change.Confirmer.Apply`，然后询问用户，确认时调 `Disarm()`（返回 `change.ErrRolledBack` 表示已超时回滚，菜单需重新加载状态）。各功能的 `change.Kind` 在对应任务中定义，并在 `cmd/litenode/main.go` 的 `Kinds` 注册表中登记（目前为空，所以测试机上 `internal rollback` 会报告 unknown kind，属预期）。
- Runner 新增 `ReadDir`（只读）、`RemoveAll`、`Rename`（修改操作）。`Rename` 对不存在的源文件返回 `fs.ErrNotExist`，用于原子认领。
- 子命令路由在 `app.Run`（`internal/app/app.go`）；`internal` 子命令在 `runInternal` 中添加（M4-02 加 `resolve`）。Runner 在 `runMenu` 中按是否 dry-run 选择，Store、Confirmer 由 app 根据所选 Runner 构造。
- Xray：`xray.Installer`（`Install` / `Update` / `Uninstall` / `Installed` / `Latest`），路径常量在 `internal/xray/release.go`。安装不启动服务；M1-07 负责"有启用节点时 enable + start，没有时 stop + disable"。
- M1-03 / M1-07 定义 `xray` 配置类型（`change.Kind`）时：`xray.json` 用 `change.File{Mode: state.GroupFileMode, Group: xray.User}`；校验 `xray run -test -c <临时文件>`（临时文件名保留 `.json` 扩展名）；**Apply 必须根据磁盘上的实际情况行事**（恢复后可能没有配置，此时停止服务），可参考 `internal/xray/vps_test.go` 中的写法。
- 权限：`/etc/litenode` 下的目录用 `system.EnsureDir`，文件只能 600 或 640+属组（D-034），`change.Manager` 会拒绝其他权限。
- Runner 新增 `Chmod`。
- 共存检测已移除（M0-10）：`system.Info` 不再有 `Conflicts`；Docker 检测用 `system.DockerInstalled`（M3-02 启用防火墙时用）；主菜单警告行用 `ui.MainStatus.Warnings`（M3 用于"LiteNode 防火墙与 ufw 同时启用"）。
- 节点模型（M1-02）：`node.Node` / `node.Client` / `node.Reality` 存在 `state.State.Nodes`；组件、兼容性表、预设在 `internal/node/builtin` 显式注册（`builtin.Registry()`）；`Registry.CheckNode` 检查组合和流控；备注用 `node.NormalizeRemark`、`CheckNodeRemark`、`CheckClientRemark`（错误为 `ErrRemark*`，菜单映射为中文）。组件接口目前只有 `Name` / `Label`，M1-03 加生成 inbound 配置的方法。
- CI：main 的必须通过检查为 `test`（amd64）和 `test-arm64`（GitHub 原生 arm64），两个任务 ID 不要改名，否则 Ruleset 会卡住合并。
- Xray 配置生成（M1-03）：`xray.GenerateConfig(reg, nodes)`；组件接口有 `InboundSettings` / `Network` + `StreamSettings` / `StreamSecurity`。`xray` 配置类型（`change.Kind`：校验 `xray run -test`，应用按节点和 `xray.stopped` 启停，D-035 / D-045）在 M1-07 定义。
- 节点（M1-07）：业务在 `internal/nodemgr`（`Service.Add/SetPort/SetTarget/SetFallbackLimit/RegenerateKeys/SetSharePort/SetRemark/SetEnabled/Delete/SetShareAddress/SyncUFW`），菜单在 `internal/app/nodes*.go`（只做显示和输入，错误由 `app.describe` 映射成中文）。`xray.Kind` / `ufw.Kind` 的应用步骤读取磁盘上的状态文件决定行为；回滚子命令使用同样的 kinds（`Env.kinds`）。
- **实机测试方法**（M1-07 建立，后续沿用）：在测试机上用管道输入驱动真实菜单（`printf '...' | NO_COLOR=1 litenode`）；客户端连通性用 WSL 中的官方 Xray 客户端（scratchpad 的 `xclient/xray` + `tools/link2client.py` 把链接转成客户端配置），通过 SOCKS 访问 `https://www.cloudflare.com/cdn-cgi/trace`，出口 IP 为 VPS 即连通。**WSL 到 VPS 的直连经过 GFW（中国移动）**，所以"从中国大陆连通"和"协议可用"要分开测：协议用测试机本机的 `/tmp/vprobe.sh <链接>`（客户端在 VPS 上，绕开 GFW）。`link2client.py` 也支持 `hysteria2://` 链接（按 `pinSHA256` 固定证书）。Hysteria2 线路机实测用 `scripts/dev/m11relay.sh`（在线路机上运行，同时测 Xray / mihomo / sing-box 出站，加 `wrong` 参数测指纹错误）。
- D-049 实测：通过代理访问 VPS 的 127.0.0.1 被阻断，访问 VPS 公网 IP 与访问其他网站相同。
- 预设（M1-04）：`node.Preset.Build(node.BuildInput{NodeID, ClientID, Port, Remark, Target, Rand})` 生成完整节点（密钥钳位后编码、8 位 shortId、v4 UUID、chrome、默认回落限速、第一个客户端备注同节点备注）。
- Reality 目标（M1-05）：`reality.Checker{}.Check(ctx, target)` / `CheckAll(ctx, targets, 10)`，`Result.Usable()`、`LargeHomepage()`、`Keyword`；`reality.CommonTargets`；`reality.SplitTarget` 校验手动输入。选择菜单在 M1-07。
- 分享（M1-06）：`share.Link(reg, node, client, addr)`（端口优先 `share_port`）、`share.QR(text, color)`、`share.NewDetector().Detect(ctx)` → `Detected{IPv4, IPv6, Source}`，`Candidates()` IPv4 在前；`OnInterface(ip)` 为 false 时提示 NAT；`share.ValidAddress` 校验手动输入。测试机已安装 `zbar-tools`（`zbarimg`）用于验证二维码。
- 端口 / ufw / 已有拦截（M1-07a）：`ports.Listening(r)` + `ports.NodeClaims(nodes)` → `ports.Checker{}.Check(want, self, selfPorts)`，`RandomFree`；`ufw.Desired(nodes)` + `ufw.Reconcile(ctx, r, desired, liteNodeFirewall)`（ufw 未运行或 LiteNode 防火墙启用时不操作；`Sync` 无条件执行）；`firewall.DetectForeignFilter(ctx, r)`。
- 转发（M4-01）：业务在 `internal/forwardmgr`（`Service.Add/Update/SetEnabled/Delete`，错误 `PortTakenError`、`ErrNotFound`、`forward.Err*`），模型和校验在 `internal/forward`（`Check`、`CheckFields`、`IPv4()` / `IPv6()`、`Resolve`）。表由 `firewall.Config(st, sshPorts)` 生成（防火墙关闭时也包含转发链），`firewall.SyncKind` 无论防火墙是否启用都按状态应用；内核转发由 `sysctl.Desired(st, sysctl.ReadEnv(r))` 决定。防火墙条目 `Entry.ForwardIndex` 用于显示"转发 N"。端口冲突检查统一用 `ports.StateClaims(nodes, forwards)`。
- 菜单动作返回 `ui.ErrBack` 可退出当前菜单（如删除节点后离开该节点的子菜单）。
- 能力探测注册在 `internal/system/probe.go` 的 `probes`：`CapBBR`、`CapSwap`（M5-02）。
- 系统优化（M5）：业务在 `internal/sysopt`（`Service.SetBBR/CreateSwap/ResizeSwap/DeleteSwap/SetPreference/SetDNS/ResetDNS/SetTimezone/EnableNTP`），菜单在 `internal/app/sysoptmenu.go`。BBR 在状态文件（`sysopt.bbr`，sysctl 文件由 `sysctl.Desired` 整体生成）；其余从系统读取。文件修改走变更流程：`sysctl`、`swap`（fstab，应用步骤按 fstab 启停 swap）、`gai`、`dns`（应用步骤重启 systemd-resolved）。测试 DNS 服务器用 `Env.DNSProber`。
- **测试系统优化**（M5）：两台测试机的镜像都已由服务商开启 BBR（Ubuntu 在 `/etc/sysctl.d/99-custom.conf`），测试开启和重启保持前先去掉这两行（备份在 `/root/99-custom.conf.bak`）。Ubuntu 使用 systemd-resolved 且有 IPv6（测优先级：`curl https://www.cloudflare.com/cdn-cgi/trace` 的 `ip=`）；Debian 不用 resolved（resolv.conf 是 resolvconf 的链接，测试后 `ln -sf ../run/resolvconf/resolv.conf /etc/resolv.conf` 恢复），原本没有 NTP 服务，磁盘只剩约 550M。
- 维护（M6）：业务在 `internal/maint`（`Build` / `Encode` / `Decode` / `Check` / `Replace` / `Conflicts` 是纯函数；`Service.Export/Read/Import`），菜单在 `internal/app/maintmenu.go`。导出文件迁移用 `state.MigrateDocument(doc, table)`。nodemgr 的 `EnsureXray`、forwardmgr 的 `SyncTimer`、firewall 的 `CheckRule` 为此导出。
- **测试更新脚本**（M6-02）：用 `-ldflags "-X main.version=v0.0.1"` 编译旧版本装到测试机，再在菜单中更新（会装上 GitHub 上的最新 Release）；失败路径用 `maint` 的 `vps` 测试（篡改下载内容，检查程序未被替换）。测试后装回开发版本并删除 `litenode.bak`。
- **测试导入导出**（M6-01）：两台测试机先各自导出一份作为备份，测试后导入各自的备份恢复，再删除 `/root` 下的导出文件（含私钥）。
- `--dry-run` 参数的接入在 M0-07（子命令框架）完成，届时用 `system.NewDryRun(system.NewReal(nil), os.Stdout)`。
- 需要真终端的验证（如实时日志的 Ctrl+C）：在测试机上用 Python `pty.fork()` 驱动程序，按步骤发送按键和 `\x03`，只取关键行（M7-02 用的脚本在 scratchpad 的 `ptydrive.py`，可放 `/root/ptydrive.py`）。
