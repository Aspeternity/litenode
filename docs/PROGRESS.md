# 开发进度

每次会话结束前更新本文件。只保留对下一次会话有用的信息，过时的内容删掉，历史以 git 记录为准。

## 当前状态

- **阶段**：M1 已由用户实机验收通过（含验收修复 D-059、D-060）。M2 SSH 管理开始。
- **M2 前的测试机复验**（D-048，2026-10-18）：在 Ubuntu 24.04 测试机上用一行命令安装 v0.0.2，节点添加、常用目标、客户端增删均端到端连通；`vps` 集成测试 `reality`、`share`、`ports`、`firewall`、`ufw`、`change`（systemd-run 定时器回滚）、`xray`（安装 / 更新 / 回退 / 卸载、生成配置、密钥）全部通过。
- **当前任务**：M2-01 已完成；下一个 M2-02 SSH 的限时确认回滚。

## 下次会话从这里开始

1. 从 M2-01 开始按顺序做 M2。
2. M2 用到的待确认提议：P-003（root 设为禁止前要求有 sudo 用户）。P-004、P-005、P-007 分别在 M3、M6 用到。按 D-050 由 Claude 决定并记录。

## 测试 VPS 的当前状态（2026-10-18）

登录信息由用户在会话中提供，**不写入仓库或记忆**。用户授权两台都可以完全用于测试。

- **Ubuntu 24.04.5**（M2 起的主测试机，D-048）：KVM、x86_64、约 1G 内存，SSH 端口 22，公网 IP 62.84.164.181，有 IPv6。已用一行命令安装 `v0.0.2`，之后换成 main `75bf986` 的编译版本供用户复验；有一个测试节点 `Reality-443`（443 端口，目标 www.xbox.com，客户端 `phone`）；已安装 `python3`、`zbar-tools`；ufw 未启用。
- **Debian 12**：KVM、x86_64、约 1G 内存，SSH 端口 61234，公网 IP 216.23.88.252。上面有用户验收时添加的节点。

## 实机测试方法（M1 建立，后续沿用）

- 远程执行：`scripts/dev/vps.sh`（`VPS_HOST`、`VPS_PORT`、`LN_PASS` 环境变量，密码不落盘）。
- 驱动真实菜单：`printf '1\n1\n...' | NO_COLOR=1 litenode`（菜单从标准输入逐行读取）。
- **使用场景**：LiteNode 主要用作落地机。WSL 到 VPS 的直连经过中国，是用户在软路由上特意配置的模拟环境；直连被干扰不作为取舍依据，以 VPS 本机的协议检测为准（D-059）。
- 客户端连通性：在 WSL 下载官方 Xray（放在 scratchpad 的独立目录，校验 `.dgst`），`scripts/dev/link2client.py` 把链接转成客户端配置，经 SOCKS 访问 `https://www.cloudflare.com/cdn-cgi/trace`，出口 IP 为 VPS 即连通。
- **WSL 到 VPS 的直连经过 GFW（中国移动）**：所以"从中国大陆能连"和"协议上可用"要分开测；后者把 `scripts/dev/link2client.py` 和 `scripts/dev/vprobe.sh` 传到 VPS 的 `/tmp`，在 VPS 本机运行（注意测试机重启会清空 `/tmp`）。
- 带 `vps` 构建标签的集成测试（`ports` 用 `VPS_SSH_PORT` 指定 sshd 端口，默认 22；`ssh` 会真实修改 sshd 配置，带 3 分钟自动恢复定时器）：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags vps -o x.test ./internal/<包>`，传到测试机以 root 运行 `./x.test -test.v -test.run TestVPS`。已有：`xray`、`change`、`share`、`reality`、`ufw`、`ports`、`firewall`、`ssh`。

## 未解决的问题

- **M2-05 必须处理 `Port` 叠加**：`Port` 在所有文件中都生效（Debian 测试机主配置 `Include` 之后有商家写的 `Port 61234`）。修改端口前需要决定如何处理主配置和其他文件中的 `Port`（DESIGN 第 13 节要求不修改主配置）。Ubuntu 的 `ssh.socket` 也要在 M2-05 实测处理。
- **Xray 被外部删除后无法删除节点**：Xray 程序被手动删除（测试中出现）时，删除节点会因 `xray run -test` 无法执行而失败，恢复也失败。没有节点需要运行时应允许跳过校验，或提示先在 Xray 菜单重新安装。M2 结束前或 M6 维护功能中处理。

- 限时确认期间服务器重启，修改不会被回滚（DESIGN 第 6 节已记录为已知限制），M2-02 时决定是否处理。
- 常用目标列表（D-055）只在一个观测点（中国移动）验证过；附近网站扫描在测试机网段没找到可用网站（邻居多为代理服务器）。
- 默认安装的 Xray 目前是预发布版 v26.10.17（D-054）；正式版达到 v26.9.8 后会自动改用正式版。
- DESIGN 第 18 节待讨论事项只剩第 2 项（TLS 证书来源），在 M7 前讨论。

## 实现约定

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
- **实机测试方法**（M1-07 建立，后续沿用）：在测试机上用管道输入驱动真实菜单（`printf '...' | NO_COLOR=1 litenode`）；客户端连通性用 WSL 中的官方 Xray 客户端（scratchpad 的 `xclient/xray` + `tools/link2client.py` 把链接转成客户端配置），通过 SOCKS 访问 `https://www.cloudflare.com/cdn-cgi/trace`，出口 IP 为 VPS 即连通。**WSL 到 VPS 的直连经过 GFW（中国移动）**，所以"从中国大陆连通"和"协议可用"要分开测：协议用测试机本机的 `/tmp/vprobe.sh <链接>`（客户端在 VPS 上，绕开 GFW）。
- D-049 实测：通过代理访问 VPS 的 127.0.0.1 被阻断，访问 VPS 公网 IP 与访问其他网站相同。
- 预设（M1-04）：`node.Preset.Build(node.BuildInput{NodeID, ClientID, Port, Remark, Target, Rand})` 生成完整节点（密钥钳位后编码、8 位 shortId、v4 UUID、chrome、默认回落限速、第一个客户端备注同节点备注）。
- Reality 目标（M1-05）：`reality.Checker{}.Check(ctx, target)` / `CheckAll(ctx, targets, 10)`，`Result.Usable()`、`LargeHomepage()`、`Keyword`；`reality.CommonTargets`；`reality.SplitTarget` 校验手动输入。选择菜单在 M1-07。
- 分享（M1-06）：`share.Link(reg, node, client, addr)`（端口优先 `share_port`）、`share.QR(text, color)`、`share.NewDetector().Detect(ctx)` → `Detected{IPv4, IPv6, Source}`，`Candidates()` IPv4 在前；`OnInterface(ip)` 为 false 时提示 NAT；`share.ValidAddress` 校验手动输入。测试机已安装 `zbar-tools`（`zbarimg`）用于验证二维码。
- 端口 / ufw / 已有拦截（M1-07a）：`ports.Listening(r)` + `ports.NodeClaims(nodes)` → `ports.Checker{}.Check(want, self, selfPorts)`，`RandomFree`；`ufw.Desired(nodes)` + `ufw.Reconcile(ctx, r, desired, liteNodeFirewall)`（ufw 未运行或 LiteNode 防火墙启用时不操作；`Sync` 无条件执行）；`firewall.DetectForeignFilter(ctx, r)`。
- 菜单动作返回 `ui.ErrBack` 可退出当前菜单（如删除节点后离开该节点的子菜单）。
- 能力探测注册在 `internal/system/probe.go` 的 `probes`，目前只有 BBR；swap、sysctl、nftables 在对应任务中加入。
- `--dry-run` 参数的接入在 M0-07（子命令框架）完成，届时用 `system.NewDryRun(system.NewReal(nil), os.Stdout)`。

## 需要用户在测试 VPS 上验证的事项

（暂无）

## 测试 VPS

- Debian 12、KVM、x86_64、内存约 1G，BBR 已启用，已装 nftables，没有 Xray / Docker。
- 登录信息由用户在会话中提供，**不写入仓库**；用户会不定期重装系统和改密码。

## 最近一次会话

- **日期**：2026-10-18
- **内容**：确认 P-006、P-010；确定界面文字集中管理（D-025）；完成 M0-01、M0-02（dry-run 和日志细节见 D-026）、M0-03（系统检测细节见 D-027，VPS 验证已通过）、M0-04（状态文件细节见 D-028，确认 P-008）、M0-05（变更流程细节见 D-029；在测试 VPS 上验证了 systemd-run 定时器触发、同名重用和取消）、M0-06（菜单细节见 D-030；在测试 VPS 上验证了菜单显示、非法输入、颜色、SIGINT/SIGHUP 退出）、M0-07（子命令细节见 D-031）、M0-08（D-032；CI 通过，v0.0.1 已发布，校验值在本机和测试机上核对一致）、M0-09（D-033；用户在测试机上确认一行命令安装、litenode / lnode 打开菜单、终端中安装后自动进入菜单）。M0 全部完成。M1-01 已完成（确认 P-001、P-002；D-034 权限规则、D-035；集成测试结果经用户确认）；改用分支开发（D-036）；端口冲突与 ufw 同步的设计变更（D-037、D-038）；M0-10 移除共存检测；M1-02 节点模型（D-039）；方案审查（D-040 ~ D-048）；M0-11 CI arm64；M1-00 多类整体变更；M1-03 Xray 配置生成（D-049）；M1-04 Reality 预设；M1-05 目标检测（D-051）；M1-06 分享链接和二维码（D-052）；M1-07a 端口 / ufw / 已有拦截检测；M1-07 节点菜单（D-053；实机发现并解决 ML-KEM 问题 D-054、常用列表重选 D-055）；M1-08 客户端管理（D-056）；M1-09 Xray 服务菜单（D-057，实机验证手动停止跨重启保持）；M1-10 附近网站扫描（D-058）。M1 全部完成。；集成测试发现并修复：查询最新版本时响应大小上限过小、校验用临时文件名导致 Xray 无法识别格式、回退时被 systemd 启动频率限制拦住、恢复 xray.json 时丢失属组。
