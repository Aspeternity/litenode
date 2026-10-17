# 开发进度

每次会话结束前更新本文件。只保留对下一次会话有用的信息，过时的内容删掉，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 项目骨架。
- **当前任务**：无（M0-06 已完成）。
- **下一个任务**：M0-07 子命令框架。

## 下次会话从这里开始

1. 开始 M0-07。版本号变量已在 `cmd/litenode/main.go` 中声明为 `var version = "dev"`，M0-07 用 `-ldflags -X main.version=...` 注入；`internal rollback` 的临时路由在 `app.Run` 开头，改为正式结构。
2. 提议条目中 P-006、P-008、P-010 已确认；其余 P-001 ~ P-005、P-007、P-009 在用到的任务开始前再与用户确认。

## 未解决的问题

- 限时确认期间服务器重启，修改不会被回滚（DESIGN 第 6 节已记录为已知限制）。如需处理，可在 M2-02 讨论。
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
- 配置变更：`change.Manager.Apply(ctx, change.Change{Kind, Files, State})`；需要限时确认的用 `change.Confirmer.Apply`，然后询问用户，确认时调 `Disarm`（返回 `change.ErrRolledBack` 表示已超时回滚，菜单需重新加载状态）。各功能的 `change.Kind` 在对应任务中定义，并在 `cmd/litenode/main.go` 的 `Kinds` 注册表中登记（目前为空）。
- Runner 新增 `ReadDir`（只读）、`RemoveAll`、`Rename`（修改操作）。`Rename` 对不存在的源文件返回 `fs.ErrNotExist`，用于原子认领。
- `internal rollback <类型>` 目前在 `app.Run` 中做了最小路由，M0-07 改为正式的子命令结构。
- 能力探测注册在 `internal/system/probe.go` 的 `probes`，目前只有 BBR；swap、sysctl、nftables 在对应任务中加入。
- `--dry-run` 参数的接入在 M0-07（子命令框架）完成，届时用 `system.NewDryRun(system.NewReal(nil), os.Stdout)`。

## 需要用户在测试 VPS 上验证的事项

（暂无）

## 测试 VPS

- Debian 12、KVM、x86_64、内存约 1G，BBR 已启用，已装 nftables，没有 Xray / Docker。
- 登录信息由用户在会话中提供，**不写入仓库**；用户会不定期重装系统和改密码。

## 最近一次会话

- **日期**：2026-10-18
- **内容**：确认 P-006、P-010；确定界面文字集中管理（D-025）；完成 M0-01、M0-02（dry-run 和日志细节见 D-026）、M0-03（系统检测细节见 D-027，VPS 验证已通过）、M0-04（状态文件细节见 D-028，确认 P-008）、M0-05（变更流程细节见 D-029；在测试 VPS 上验证了 systemd-run 定时器触发、同名重用和取消）、M0-06（菜单细节见 D-030；在测试 VPS 上验证了菜单显示、非法输入、颜色、SIGINT/SIGHUP 退出）。
