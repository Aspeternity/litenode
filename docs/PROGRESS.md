# 开发进度

每次会话结束前更新本文件。只保留对下一次会话有用的信息，过时的内容删掉，历史以 git 记录为准。

## 当前状态

- **阶段**：M0 项目骨架。
- **当前任务**：无（M0-02 已完成）。
- **下一个任务**：M0-03 系统检测和能力探测。

## 下次会话从这里开始

1. 开始 M0-03。第一个界面提示（非 root、不支持的系统）的文字放进 `internal/text`。
2. 提议条目中 P-006、P-010 已确认；其余 P-001 ~ P-005、P-007 ~ P-009 在用到的任务开始前再与用户确认。

## 未解决的问题

- `docs/DESIGN.md` 第 18 节"待讨论事项"剩余 3 项（第 1~3 项），在对应任务开始前讨论。第 4 项已确定为界面文字集中管理（D-025）。

## 实现约定（本次会话确定）

- 每个 Go 包用 `doc.go` 写包注释作为占位；尚无文件的目录（`scripts/`、`.github/workflows/`）用 `.gitkeep`，有真实文件后删除。
- `go.mod` 使用 `go 1.27.2`（开发环境的 Go 版本）。
- `internal/text` 已创建（M0-02，dry-run 输出文字），`TestNoChineseOutsideText` 扫描其他包的非测试 Go 文件（含注释），发现中文即失败。
- 执行层用法：只读用 `Query` / `ReadFile`，修改用 `Exec` / `WriteFile` / `Remove` / `MkdirAll`；systemd 操作用 `system.Systemctl`、`DaemonReload`、`ServiceActive`、`ServiceEnabled`。命令执行时设置 `LC_ALL=C`，便于解析输出。
- 其他包的测试用 `systemtest.New()`：`Respond` 设定命令输出，`Files` 模拟文件，`WriteErr` 模拟写入失败，`Execs()` / `Mutations()` 断言。
- `--dry-run` 参数的接入在 M0-07（子命令框架）完成，届时用 `system.NewDryRun(system.NewReal(nil), os.Stdout)`。

## 需要用户在测试 VPS 上验证的事项

（暂无）

## 最近一次会话

- **日期**：2026-10-18
- **内容**：确认 P-006、P-010；确定界面文字集中管理（D-025）；完成 M0-01、M0-02（dry-run 和日志细节见 D-026）。
