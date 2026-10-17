# 开发进度

每次会话结束前更新本文件。只保留对下一次会话有用的信息，过时的内容删掉，历史以 git 记录为准。

## 当前状态

- **阶段**：M1 节点管理。
- **当前任务**：无（M1-04 已完成）。
- **下一个任务**：M1-05 Reality 目标检测、常用列表、手动输入。

## 下次会话从这里开始

1. 做 M1-05（`feat/M1-05-reality-target`）。之后依次 M1-06、M1-07a、M1-07、M1-08、M1-09、M1-10；M1 全部完成后停下来让用户实机验收（D-050）。DESIGN 第 18 节待讨论 #3（常用目标列表）由 Claude 决定并记录。
   - 发布：GitHub 上有正式 Release `v0.0.1`（开发中测试版，`releases/latest` 指向它）。新版本发布时用 `git tag -a`，推送标签前先问用户。M6-02 更新脚本可复用 install.sh 的下载和校验方式（资产名 `litenode-linux-<架构>`，`sha256sums.txt` 为 `sha256sum` 标准格式）。
2. 提议条目中 P-001、P-002、P-006、P-008、P-010 已确认；其余 P-003 ~ P-005、P-007、P-009 在用到的任务开始前再与用户确认。

## 未解决的问题

- 2026-10-18 方案审查的 10 条已全部定案（D-040 ~ D-048），已写入 DESIGN 和 ROADMAP。
- 限时确认期间服务器重启，修改不会被回滚（DESIGN 第 6 节已记录为已知限制）。如需处理，可在 M2-02 讨论。
- 端口冲突检查和 ufw 同步（D-037、D-038）的通用模块在新任务 M1-07a；ufw 的实际行为（带备注的重复规则、停用时能否删除规则）需在测试 VPS 上实测后补充到 DESIGN 第 2 节。
- `docs/DESIGN.md` 第 18 节"待讨论事项"剩余 3 项（第 1~3 项），在对应任务开始前讨论。第 4 项已确定为界面文字集中管理（D-025）。

## 实现约定（本次会话确定）

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
- M1-07 要处理的两点：① Xray 对非 443 端口的 REALITY 会警告"可能更容易被 GFW 封锁 IP"，添加或修改节点端口时是否提示，需在 M1-07 与用户确认；② 在测试机上用真实客户端验证代理无法访问 127.0.0.1 上的服务（D-049 的实际效果）。
- 预设（M1-04）：`node.Preset.Build(node.BuildInput{NodeID, ClientID, Port, Remark, Target, Rand})` 生成完整节点（密钥钳位后编码、8 位 shortId、v4 UUID、chrome、默认回落限速、第一个客户端备注同节点备注）。
- 能力探测注册在 `internal/system/probe.go` 的 `probes`，目前只有 BBR；swap、sysctl、nftables 在对应任务中加入。
- `--dry-run` 参数的接入在 M0-07（子命令框架）完成，届时用 `system.NewDryRun(system.NewReal(nil), os.Stdout)`。

## 需要用户在测试 VPS 上验证的事项

（暂无）

## 测试 VPS

- Debian 12、KVM、x86_64、内存约 1G，BBR 已启用，已装 nftables，没有 Xray / Docker。
- 登录信息由用户在会话中提供，**不写入仓库**；用户会不定期重装系统和改密码。

## 最近一次会话

- **日期**：2026-10-18
- **内容**：确认 P-006、P-010；确定界面文字集中管理（D-025）；完成 M0-01、M0-02（dry-run 和日志细节见 D-026）、M0-03（系统检测细节见 D-027，VPS 验证已通过）、M0-04（状态文件细节见 D-028，确认 P-008）、M0-05（变更流程细节见 D-029；在测试 VPS 上验证了 systemd-run 定时器触发、同名重用和取消）、M0-06（菜单细节见 D-030；在测试 VPS 上验证了菜单显示、非法输入、颜色、SIGINT/SIGHUP 退出）、M0-07（子命令细节见 D-031）、M0-08（D-032；CI 通过，v0.0.1 已发布，校验值在本机和测试机上核对一致）、M0-09（D-033；用户在测试机上确认一行命令安装、litenode / lnode 打开菜单、终端中安装后自动进入菜单）。M0 全部完成。M1-01 已完成（确认 P-001、P-002；D-034 权限规则、D-035；集成测试结果经用户确认）；改用分支开发（D-036）；端口冲突与 ufw 同步的设计变更（D-037、D-038）；M0-10 移除共存检测；M1-02 节点模型（D-039）；方案审查（D-040 ~ D-048）；M0-11 CI arm64；M1-00 多类整体变更；M1-03 Xray 配置生成（D-049）；M1-04 Reality 预设；集成测试发现并修复：查询最新版本时响应大小上限过小、校验用临时文件名导致 Xray 无法识别格式、回退时被 systemd 启动频率限制拦住、恢复 xray.json 时丢失属组。
