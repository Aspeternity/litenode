package text

import "strconv"

// System optimization menu (DESIGN section 15).
const (
	SysoptBBR         = "BBR + fq"
	SysoptBBRByOther  = "BBR 已由系统的其他配置开启，无需操作。"
	ConfirmBBREnable  = "将开启 BBR 拥塞控制和 fq 队列（写入 /etc/sysctl.d/99-litenode.conf，重启后保持）。确定开启？"
	ConfirmBBRDisable = "将关闭 BBR，恢复为系统默认的 cubic。确定关闭？"
	BBREnabledDone    = "BBR 已开启。"
	BBRDisabledDone   = "BBR 已关闭。"
	SysoptUnavailable = "部分功能在当前环境中不可用，已隐藏。"
	StateOn           = "✓"
	StateOff          = "✗"
)

// SysoptUnavailableIn names the environment that hides some options.
func SysoptUnavailableIn(virt string) string {
	return "部分功能在当前环境（" + virt + "）中不可用，已隐藏。"
}

// StatusBBR labels the BBR field of the system optimization status.
func StatusBBR(v string) string { return "BBR: " + v }

// Swap (DESIGN section 15).
const (
	SysoptSwap        = "swap"
	SysoptSwapHint    = "(创建 / 调整 / 删除)"
	SwapNone          = "无"
	SwapResize        = "调整大小"
	SwapDelete        = "删除"
	PromptSwapSize    = "swap 大小（如 512M、1G）"
	ConfirmSwapDelete = "将停用并删除 swap 文件。确定删除？"
	SwapDeleted       = "swap 已删除。"
	ErrSwapExists     = "swap 文件已存在。"
	ErrNoSwap         = "没有 LiteNode 创建的 swap。"
	ErrSwapInUse      = "swap 中的数据超过当前可用内存，停用可能导致程序被系统结束。请先减少内存占用再试。"
	SwapCreating      = "正在创建 swap 文件，可能需要一些时间……"
)

// StatusSwap labels the swap field of the system optimization status.
func StatusSwap(v string) string { return "swap: " + v }

// SwapLine describes one active swap area.
func SwapLine(path, size string, usedMB int, own bool) string {
	l := "  " + path + "  " + size + "，已用 " + strconv.Itoa(usedMB) + "M"
	if !own {
		l += "（非 LiteNode 创建）"
	}
	return l
}

// SwapMemory shows the memory size and, when small, the suggested swap.
func SwapMemory(mem, suggest string) string {
	if suggest == "" {
		return "内存 " + mem + "。"
	}
	return "内存 " + mem + "，建议创建 " + suggest + " swap。"
}

// SwapCreated reports a new swap file.
func SwapCreated(size string) string { return "已创建 " + size + " swap，重启后保持。" }

// SwapResized reports a resized swap file.
func SwapResized(size string) string { return "swap 已调整为 " + size + "。" }

// ErrSwapSize explains the allowed swap sizes.
func ErrSwapSize(min, max string) string { return "大小须在 " + min + " 到 " + max + " 之间。" }

// SwapMax shows the largest swap the disk can hold.
func SwapMax(max string) string {
	return "磁盘空间最多可创建 " + max + "（保留 256M 空闲空间）。"
}

// ErrNoDiskSpace reports that the disk cannot hold the swap file.
const ErrNoDiskSpace = "磁盘空间不足（创建后需保留 256M 空闲空间）。"
