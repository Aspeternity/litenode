package text

import (
	"fmt"
	"strings"
)

// SSH menu (DESIGN section 13).
const (
	SSHKeys         = "密钥管理"
	SSHKeyAdd       = "添加公钥"
	SSHKeyGen       = "生成密钥对"
	SSHLoginMethod  = "登录方式"
	SSHRootPolicy   = "root 登录"
	SSHPortChange   = "修改 SSH 端口"
	SSHSudoUser     = "创建 sudo 用户"
	SSHAccounts     = "账户管理"
	SSHPasswd       = "修改密码"
	SSHDeleteUser   = "删除 sudo 用户"
	SSHKeyDelete    = "删除公钥"
	SSHRollback     = "回滚到上次修改前"
	SSHKeysOnly     = "仅密钥"
	SSHPasswordOnly = "仅密码"
	SSHKeysPassword = "密钥 + 密码"
	SSHNoLogin      = "无法登录"

	SSHRootYes   = "允许"
	SSHRootKeys  = "仅密钥"
	SSHRootNo    = "禁止"
	SSHKeysTitle = "公钥:"
	SSHKeysNone  = "公钥: （无）"

	SSHPasteKey      = "粘贴公钥"
	SSHGitHubImport  = "从 GitHub 导入"
	PromptPublicKey  = "粘贴公钥（一行，0 返回）"
	PromptGitHubUser = "GitHub 用户名（0 返回）"
	PromptKeyChoice  = "导入哪些？输入序号（如 1,3），直接回车导入全部，0 返回"
	PromptKeyNumber  = "请输入公钥序号 (0 返回)"
	ChooseAccount    = "选择用户"
	SSHKeyExists     = "这个公钥已经存在。"
	PromptKeyComment = "公钥注释"
	PrivateKeyTitle  = "私钥如下（只显示这一次，服务器上不保留）："
	PrivateKeyHint   = "请把上面整段（含 BEGIN / END 两行）保存到本机文件，如 id_ed25519，权限设为 600；登录时用 ssh -i 指定。"
	PublicKeyTitle   = "对应的公钥（已加入 authorized_keys）："
	ConfirmGenerate  = "将生成新的 ed25519 密钥对，私钥只显示一次，请准备好保存。是否继续？"

	PromptNewPort     = "新的 SSH 端口 (0 返回)"
	PromptSudoName    = "用户名（小写字母、数字、_、-，0 返回）"
	SudoPasswordGen   = "自动生成强密码（推荐，只显示一次）"
	SudoPasswordInput = "手动输入密码"
	ChoosePassword    = "设置密码（sudo 时需要输入）"
	PromptPassword    = "输入密码（不显示）"
	PromptPassword2   = "再次输入密码"
	PasswordMismatch  = "两次输入的密码不一致。"
	ConfirmRollback   = "恢复为上次修改前的 SSH 设置？"
	RollbackTitle     = "上次修改前的 SSH 设置（当前 → 恢复后）："
	SSHSystemSetting  = "系统原有设置"
	SSHPortName       = "端口"
	SSHMethodName     = "登录方式"
	SSHRootName       = "root 登录"
	SSHOldRulesNone   = "防火墙中没有需要处理的旧端口规则。"
	PasswordWarn      = "⚠"
)

// SSHStatusLines are the status lines of the SSH menu (D-090).
func SSHStatusLines(ports, method, root string, keys, sudoUsers int) []string {
	return []string{
		fmt.Sprintf("端口: %s   登录方式: %s", ports, method),
		fmt.Sprintf("root 登录: %s   公钥: %d 个   sudo 用户: %d 个", root, keys, sudoUsers),
	}
}

// RollbackChange is one setting the rollback changes.
func RollbackChange(name, cur, prev string) string {
	return fmt.Sprintf("  %s: %s → %s", name, cur, prev)
}

// LoginMethod names a combination of key and password login.
func LoginMethod(pubkey, password bool) string {
	switch {
	case pubkey && password:
		return SSHKeysPassword
	case pubkey:
		return SSHKeysOnly
	case password:
		return SSHPasswordOnly
	}
	return SSHNoLogin
}

// SSHMainStatus is the SSH value in the main menu, e.g. "22222 / 仅密钥".
func SSHMainStatus(port string, pubkey, password bool) string {
	mode := "密钥+密码"
	switch {
	case pubkey && !password:
		mode = "仅密钥"
	case !pubkey && password:
		mode = "仅密码"
	case !pubkey && !password:
		mode = "无法登录"
	}
	return port + " / " + mode
}

// SSHKeyLine lists one key; user is "" when only root has keys.
func SSHKeyLine(n int, user, typ, fingerprint, comment string) string {
	if user != "" {
		user += "  "
	}
	if r := []rune(comment); len(r) > 24 {
		comment = string(r[:23]) + "…"
	}
	return strings.TrimRight(fmt.Sprintf("  [%d] %s%s %s  %s", n, user, typ, fingerprint, comment), " ")
}

// GitHubKeysFound lists the keys found for a GitHub user.
func GitHubKeysFound(user string, n, skipped int) string {
	s := fmt.Sprintf("GitHub 用户 %s 有 %d 个可用公钥", user, n)
	if skipped > 0 {
		s += fmt.Sprintf("（另有 %d 个不受支持，已跳过）", skipped)
	}
	return s + "："
}

// KeysAdded reports how many keys were added.
func KeysAdded(added, total int) string {
	if added == total {
		return fmt.Sprintf("已添加 %d 个公钥。", added)
	}
	return fmt.Sprintf("已添加 %d 个公钥，其余 %d 个已经存在。", added, total-added)
}

// SSHCurrent marks the current login method or root policy.
func SSHCurrent(label string) string { return "当前: " + label }

// PortChangeNotice reminds about the provider's firewall before moving
// the SSH port.
func PortChangeNotice(port int) string {
	return fmt.Sprintf("修改前请确认云厂商的安全组或防火墙已放行 TCP %d，否则新端口将无法连接（未确认时 60 秒后自动恢复）。", port)
}

// ConfirmPortChange asks before moving the SSH port.
func ConfirmPortChange(old, port int) string {
	return fmt.Sprintf("SSH 端口将从 %d 改为 %d，是否继续？", old, port)
}

// PortChanged tells how to log in on the new port.
func PortChanged(port int) string {
	return fmt.Sprintf("SSH 端口已改为 %d。之后登录请使用：ssh -p %d 用户名@服务器地址", port, port)
}

// OldPortRules lists the user's firewall rules for the old port.
func OldPortRules(port int, rules []string, other int) string {
	s := fmt.Sprintf("ufw 中还有放行旧端口 %d 的规则：%s", port, strings.Join(rules, "、"))
	if other > 0 {
		s += fmt.Sprintf("；另有 %d 条带来源限制或多端口的规则，请自行检查", other)
	}
	return s + "。"
}

// OldPortComplexOnly reports rules for the old port that are left to the
// user.
func OldPortComplexOnly(port, other int) string {
	return fmt.Sprintf("ufw 中有 %d 条涉及旧端口 %d 的规则带来源限制或多端口，请自行检查。", other, port)
}

// ConfirmDeleteOldRules asks before deleting them (D-038: default no).
const ConfirmDeleteOldRules = "是否删除这些旧端口规则？"

// OldRulesDeleted reports the deletion.
const OldRulesDeleted = "旧端口规则已删除。"

// Account management page (D-096): one row per admin.
var AccountsHeader = []string{"用户", "sudo", "公钥", "密码"}

const (
	AccountSudo       = "是"
	AccountRoot       = "-"
	AccountPassword   = "有"
	AccountNoPassword = "无"
	PasswordLoginOff  = "SSH 密码登录当前已关闭，新密码用于 sudo 和 VNC 控制台登录。"
	PromptDeleteUser  = "请输入要删除的用户序号 (0 返回)"
	ErrNotAdmin       = "只能管理 root 和 sudo 用户。"
	ErrDeleteRoot     = "不能删除 root。"
	ErrUserInUse      = "该用户正在登录或有进程在运行。请先退出该用户的所有会话（包括用 sudo 运行 LiteNode 的会话），再用 root 删除。"
	ErrDeleteUserLock = "删除后 root 和其他 sudo 用户都无法登录，已取消。请先确认 root 或其他 sudo 用户能登录（如添加公钥、调整 root 登录）。"
)

// AccountKeys is the key count column.
func AccountKeys(n int) string { return fmt.Sprint(n) }

// PasswordChanged reports a kept password change.
func PasswordChanged(name string) string {
	return fmt.Sprintf("%s 的密码已修改。", name)
}

// ConfirmDeleteUser states what deleting a sudo user removes.
func ConfirmDeleteUser(name, home string) string {
	return fmt.Sprintf("删除用户 %s：主目录 %s（包括其中的公钥和文件）和邮件一并删除，不能恢复。继续吗？", name, home)
}

// SudoUserDeleted reports the deleted user.
func SudoUserDeleted(name string) string {
	return fmt.Sprintf("用户 %s 已删除。", name)
}

// SudoUserCreated reports the new user.
func SudoUserCreated(name string) string {
	return fmt.Sprintf("用户 %s 已创建并加入 sudo 组，使用 sudo 时需要输入密码。", name)
}

// GeneratedPassword shows the generated password once.
func GeneratedPassword(name, password string) string {
	return fmt.Sprintf("%s 的密码（只显示这一次，请保存）：%s", name, password)
}

// ConfirmCopyRootKeys asks whether the new user gets root's keys.
func ConfirmCopyRootKeys(n int, name string) string {
	return fmt.Sprintf("把 root 的 %d 个公钥复制给 %s？", n, name)
}

// ConfirmDeleteKey asks before deleting a key.
func ConfirmDeleteKey(user, fingerprint string) string {
	return fmt.Sprintf("删除 %s 的公钥 %s？", user, fingerprint)
}

// SSH errors.
const (
	ErrSSHNoInclude      = "/etc/ssh/sshd_config 没有读取 sshd_config.d 目录，LiteNode 的设置不会生效。LiteNode 不修改主配置，请在该文件开头加入一行：Include /etc/ssh/sshd_config.d/*.conf"
	ErrSSHIncludeLate    = "/etc/ssh/sshd_config 在 Include 之前设置了端口、登录方式或 root 策略，会覆盖 LiteNode 的设置。请把 Include /etc/ssh/sshd_config.d/*.conf 移到文件开头，或删除这些设置。"
	ErrSSHNotEffective   = "sshd 没有使用新的设置（可能被其他配置文件覆盖），修改已自动恢复。"
	ErrNeedPasswordLogin = "密码登录已关闭，不能再关闭密钥登录，否则将无法登录。"
	ErrNeedKeyLogin      = "请先开启密钥登录，再关闭密码登录。"
	ErrNeedKey           = "请先添加公钥（并确认可以用密钥登录），再关闭密码登录。"
	ErrNeedSudoUser      = "请先创建一个可以登录的 sudo 用户（有公钥或密码），再禁止 root 登录。"
	ErrSSHLockout        = "这样修改后，root 和 sudo 用户都无法登录，已取消。"
	ErrLastKey           = "密码登录已关闭，不能删除这个用户的最后一个公钥。"
	ErrKeyFormat         = "不是有效的 OpenSSH 公钥（应为 \"ssh-ed25519 AAAA... 注释\" 这样的一行）。"
	ErrKeyType           = "不支持的公钥类型（支持 ed25519、ecdsa、rsa）。"
	ErrKeyWeak           = "RSA 公钥太短，至少需要 2048 位。"
	ErrGitHubUser        = "GitHub 用户名格式不正确。"
	ErrGitHubNone        = "这个 GitHub 用户没有可用的公钥。"
	ErrSSHPortRange      = "端口必须在 1 到 65535 之间。"
	ErrSSHPortSame       = "SSH 已经在使用这个端口。"
	ErrUserName          = "用户名格式不正确：小写字母或 _ 开头，只含小写字母、数字、_、-，最多 32 个字符。"
	ErrUserExists        = "这个用户已经存在。"
	ErrPasswordShort     = "密码至少需要 8 个字符。"
	ErrPasswordChars     = "密码不能包含冒号或控制字符。"
	ErrNothingToRollBack = "没有可以回滚的 SSH 修改。"
)
