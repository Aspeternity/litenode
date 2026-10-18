package text

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
