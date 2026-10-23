package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
	"github.com/Aspeternity/litenode/internal/text"
)

const testPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJM6hmNMYG91sPhBXixk2IaSolPl/BTm0oP6H+4Febf+ laptop"

// sshSystem is a supported Debian 12 whose sshd reports effective.
func sshSystem(effective string) *systemtest.Fake {
	f := supported()
	f.Files[ssh.MainConfigPath] = []byte("Include /etc/ssh/sshd_config.d/*.conf\n")
	f.Files[ssh.PasswdPath] = []byte("root:x:0:0:root:/root:/bin/bash\n")
	f.Files[ssh.GroupPath] = []byte("root:x:0:\nsudo:x:27:\n")
	f.Files[ssh.ShadowPath] = []byte("root:$y$hash:19000:0:99999:7:::\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: effective}}, ssh.SshdPath, "-T")
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 3}}, "systemctl", "is-active", "--quiet", ssh.SocketUnit)
	return f
}

const effective22 = "port 22\nlistenaddress 0.0.0.0:22\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"

func runSSH(t *testing.T, f *systemtest.Fake, input ...string) result {
	t.Helper()
	return runEnv(t, Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"),
		TargetChecker: fakeChecker{}, AddressDetector: fakeDetector{}}, strings.Join(input, "\n")+"\n")
}

func TestSSHMenu(t *testing.T) {
	f := sshSystem(effective22)
	// main: 6 SSH; back; quit.
	r := runSSH(t, f, "6", "0", "0")
	for _, want := range []string{
		" SSH: 22 / 密钥+密码",
		"═════════════ 主菜单 › SSH 管理 ══════════════",
		" 端口: 22   登录方式: 密钥 + 密码 ⚠\n root 登录: 允许   公钥: 0 个   sudo 用户: 0 个\n",
		" 1. 登录方式\n 2. root 登录\n 3. 修改 SSH 端口\n 4. 密钥管理\n 5. 账户管理\n 6. 回滚到上次修改前\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	// Key management: no keys, so "delete" is hidden.
	r = runSSH(t, f, "6", "4", "0", "0", "0")
	if !strings.Contains(r.stdout, " 公钥: （无）\n──────────────────────────────────────────────\n 1. 生成密钥对\n 2. 添加公钥\n──") || strings.Contains(r.stdout, "删除公钥") {
		t.Errorf("key menu:\n%s", r.stdout)
	}
}

func TestSSHMenuInterlockAndKeys(t *testing.T) {
	f := sshSystem(effective22)
	// Key only without a key: refused with an explanation.
	r := runSSH(t, f, "6", "1", "1", "0", "0")
	if !strings.Contains(r.stdout, "当前: 密钥 + 密码") || !strings.Contains(r.stdout, "1. 仅密钥") || !strings.Contains(r.stdout, "2. 仅密码") || !strings.Contains(r.stdout, "3. 密钥 + 密码") ||
		!strings.Contains(r.stdout, "请先添加公钥（并确认可以用密钥登录），再关闭密码登录。") {
		t.Errorf("interlock:\n%s", r.stdout)
	}
	// Choosing the current method does nothing.
	before := len(f.Mutations())
	runSSH(t, f, "6", "1", "3", "0", "0")
	if m := f.Mutations()[before:]; len(m) > 1 {
		t.Errorf("current method changed the system: %+v", m)
	}
	// A key typed or piped in (not marked as pasted) is shown and needs
	// a yes (FIX-04).
	before = len(f.Mutations())
	r = runSSH(t, f, "6", "4", "2", "1", testPub, "", "0", "0", "0")
	if !strings.Contains(r.stdout, "公钥: ed25519 SHA256:o/KaoigHu9ddvclgoQvWem5m+GbW7ahqewdkStS+qZw  laptop\n") || !strings.Contains(r.stdout, "添加这个公钥？ [y/N]") {
		t.Errorf("confirm key:\n%s", r.stdout)
	}
	if m := f.Mutations()[before:]; len(m) > 0 {
		t.Errorf("unconfirmed key changed the system: %+v", m)
	}
	// Paste a bad key and a private key, then a good one; it appears in
	// the list.
	r = runSSH(t, f, "6", "4", "2", "1", "not a key", "-----BEGIN OPENSSH PRIVATE KEY-----", testPub, "y", "0", "0", "0")
	for _, want := range []string{"不是有效的 OpenSSH 公钥", "这是私钥，已丢弃。", "已添加 1 个公钥。", " 公钥:\n   [1] ed25519 SHA256:o/KaoigHu9ddvclgoQvWem5m+GbW7ahqewdkStS+qZw  laptop", " 3. 删除公钥", "公钥: 1 个"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	// Generate a key pair: the private key is shown, not stored.
	r = runSSH(t, f, "6", "4", "1", "y", "", "0", "0", "0")
	if !strings.Contains(r.stdout, "-----BEGIN OPENSSH PRIVATE KEY-----") || !strings.Contains(r.stdout, "[2] ed25519") {
		t.Errorf("generate:\n%s", r.stdout)
	}
	for p, data := range f.Files {
		if strings.Contains(string(data), "PRIVATE KEY") {
			t.Errorf("private key stored in %s", p)
		}
	}
}

func TestSSHMenuSudoUser(t *testing.T) {
	f := sshSystem(effective22)
	f.Files["/root/.ssh/authorized_keys"] = []byte(testPub + "\n")
	// 5 accounts, 2 create: name; bad name first; generated password;
	// copy root keys: n.
	r := runSSH(t, f, "6", "5", "2", "Bad", "admin", "1", "n", "0", "0", "0")
	for _, want := range []string{"用户名格式不正确", "把 root 的 1 个公钥复制给 admin？ [y/N]", "用户 admin 已创建并加入 sudo 组", "admin 的密码（只显示这一次，请保存）："} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	var input string
	for _, c := range f.Calls() {
		if c.Command == "chpasswd" {
			input = string(c.Data)
		}
	}
	if !strings.HasPrefix(input, "admin:") || !strings.Contains(r.stdout, strings.TrimSpace(strings.TrimPrefix(input, "admin:"))) {
		t.Errorf("chpasswd input %q does not match the shown password", input)
	}
}

func TestSSHMenuPortChange(t *testing.T) {
	at := func(port string) string {
		return "port " + port + "\nlistenaddress 0.0.0.0:" + port + "\npubkeyauthentication yes\npasswordauthentication yes\nkbdinteractiveauthentication no\npermitrootlogin yes\n"
	}
	f := sshSystem(at("2222"))
	f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n   0: 00000000:08AE 00000000:0000 0A\n")
	// Once reloaded, sshd listens on 2200 (0x0898).
	f.OnCommand("systemctl reload "+ssh.Unit, func() {
		f.Respond(systemtest.Response{Result: system.Result{Stdout: at("2200")}}, ssh.SshdPath, "-T")
		f.Files["/proc/net/tcp"] = []byte("  sl  local_address rem_address   st\n   0: 00000000:0898 00000000:0000 0A\n")
	})
	f.Files["/etc/ufw/user.rules"] = []byte("### tuple ### allow tcp 2222 0.0.0.0/0 any 0.0.0.0/0 in\n")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "Status: active\n"}}, "ufw", "status")
	// 3: new port 2200; confirm; keep (y); delete old rules: y.
	r := runSSH(t, f, "6", "3", "2200", "y", "y", "y", "0", "0")
	for _, want := range []string{
		"修改前请确认云厂商的安全组或防火墙已放行 TCP 2200",
		"SSH 端口将从 2222 改为 2200，是否继续？ [y/N]",
		"请新开一个终端测试能否登录",
		"└─ ✓ SSH 端口已改为 2200。之后登录请使用：ssh -p 2200",
		"ufw 中还有放行旧端口 2222 的规则：2222/tcp。",
		"旧端口规则已删除。",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q", want)
		}
	}
	execs := strings.Join(f.Execs(), "\n")
	if !strings.Contains(execs, "ufw allow proto tcp to any port 2200 comment litenode:ssh") || !strings.Contains(execs, "ufw delete allow 2222/tcp") {
		t.Errorf("execs:\n%s", execs)
	}
	if st, _ := state.NewStore(f).Load(); st.SSH.Port != 2200 {
		t.Errorf("state = %+v", st.SSH)
	}
}

func TestSSHMenuSudoUserNameBack(t *testing.T) {
	f := sshSystem(effective22)
	// 5 accounts, 2 create: 0 goes back.
	r := runSSH(t, f, "6", "5", "2", "0", "0", "0", "0")
	if strings.Contains(r.stdout, "用户名格式不正确") || !strings.Contains(r.stdout, "0 返回）: ") || !strings.Contains(r.stdout, text.FlowCanceled) {
		t.Errorf("sudo name 0:\n%s", r.stdout)
	}
}

func TestSSHMenuRollbackNothing(t *testing.T) {
	// Told before any confirmation.
	r := runSSH(t, sshSystem(effective22), "6", "6", "0", "0")
	if !strings.Contains(r.stdout, "└─ 没有可以回滚的 SSH 修改。") || strings.Contains(r.stdout, text.ConfirmRollback) {
		t.Errorf("rollback:\n%s", r.stdout)
	}
}

func TestRollbackChanges(t *testing.T) {
	yes, no := true, false
	cur := state.SSH{Port: 2222, PubkeyAuth: &yes, PasswordAuth: &no, PermitRootLogin: ssh.RootNo}
	prev := state.SSH{PubkeyAuth: &yes, PasswordAuth: &no, PermitRootLogin: ssh.RootYes}
	got := strings.Join(rollbackChanges(cur, prev), "\n")
	want := "  端口: 2222 → 系统原有设置\n  root 登录: " + text.SSHRootNo + " → " + text.SSHRootYes
	if got != want {
		t.Errorf("rollbackChanges = %q, want %q", got, want)
	}
}

// accountsSystem has root and the sudo user alice (no password, no key).
func accountsSystem() *systemtest.Fake {
	f := sshSystem(effective22)
	f.Files[ssh.PasswdPath] = []byte("root:x:0:0:root:/root:/bin/bash\nalice:x:1000:1000::/home/alice:/bin/bash\n")
	f.Files[ssh.GroupPath] = []byte("root:x:0:\nsudo:x:27:alice\nalice:x:1000:\n")
	f.Files[ssh.ShadowPath] = []byte("root:$y$hash:19000:0:99999:7:::\nalice:!:19000::::::\n")
	f.Files["/root/.ssh/authorized_keys"] = []byte(testPub + "\n")
	f.Respond(systemtest.Response{Result: system.Result{ExitCode: 1}}, "pgrep", "-u", "alice")
	return f
}

func TestSSHAccountsPage(t *testing.T) {
	f := accountsSystem()
	r := runSSH(t, f, "6", "5", "0", "0", "0")
	for _, want := range []string{
		"sudo 用户: 1 个",
		" › 账户管理 ══",
		" 用户   sudo  公钥  密码\n root   -     1     有\n alice  是    0     无\n",
		" 1. 修改密码\n 2. 创建 sudo 用户\n 3. 删除 sudo 用户\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
}

// TestSSHChangePassword: choose alice, generated password, keep it. The
// password is shown before the confirmation and goes to chpasswd only
// through standard input.
func TestSSHChangePassword(t *testing.T) {
	f := accountsSystem()
	r := runSSH(t, f, "6", "5", "1", "2", "1", "y", "0", "0", "0")
	for _, want := range []string{"alice 的密码（只显示这一次，请保存）：", "请新开一个终端测试能否登录", "└─ ✓ alice 的密码已修改。"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	shown := r.stdout[strings.Index(r.stdout, "请保存）：")+len("请保存）："):]
	shown = shown[:strings.IndexByte(shown, '\n')]
	var input string
	for _, c := range f.Calls() {
		if strings.Contains(c.Command, shown) {
			t.Errorf("password in command line: %s", c.Command)
		}
		if c.Command == "chpasswd" {
			input = string(c.Data)
		}
	}
	if input != "alice:"+shown+"\n" {
		t.Errorf("chpasswd input %q, shown %q", input, shown)
	}
}

// keysOnly22 is effective22 with password login off.
const keysOnly22 = "port 22\nlistenaddress 0.0.0.0:22\npubkeyauthentication yes\npasswordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin yes\n"

// reportAfterReload makes the fake sshd report effective once reloaded.
func reportAfterReload(f *systemtest.Fake, effective string) {
	f.OnCommand("systemctl reload "+ssh.Unit, func() {
		f.Respond(systemtest.Response{Result: system.Result{Stdout: effective}}, ssh.SshdPath, "-T")
	})
}

// chpasswdInput is what the last chpasswd read ("" if none ran).
func chpasswdInput(f *systemtest.Fake) string {
	var input string
	for _, c := range f.Calls() {
		if c.Command == "chpasswd" {
			input = string(c.Data)
		}
	}
	return input
}

// TestSSHLoginMethodPassword: switching to password only asks whose
// password and how; the password is set before the timed confirmation
// (FIX-04, D-126).
func TestSSHLoginMethodPassword(t *testing.T) {
	f := accountsSystem()
	f.Respond(systemtest.Response{Result: system.Result{Stdout: keysOnly22}}, ssh.SshdPath, "-T")
	reportAfterReload(f, strings.Replace(keysOnly22, "pubkeyauthentication yes\npasswordauthentication no", "pubkeyauthentication no\npasswordauthentication yes", 1))
	// 1 login method, 2 password only; user alice; generated; keep (y).
	r := runSSH(t, f, "6", "1", "2", "2", "1", "y", "0", "0")
	for _, want := range []string{
		"选择用密码登录的用户", "1. root\n", "2. alice\n",
		"alice 的登录密码", "1. 自动生成强密码（推荐，只显示一次）\n", "2. 手动输入密码\n",
		"alice 的密码（只显示这一次，请保存）：", "请新开一个终端测试能否登录",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	shown := r.stdout[strings.Index(r.stdout, "请保存）：")+len("请保存）："):]
	shown = shown[:strings.IndexByte(shown, '\n')]
	if got := chpasswdInput(f); got != "alice:"+shown+"\n" {
		t.Errorf("chpasswd input %q, shown %q", got, shown)
	}
	if strings.Contains(r.stdout, "保留现有密码") {
		t.Error("keep offered for alice, who has no password")
	}
}

// TestSSHLoginMethodPasswordChoices: root keys only leaves alice alone,
// without asking whose; root alone with a password may keep it; going
// back changes nothing.
func TestSSHLoginMethodPasswordChoices(t *testing.T) {
	f := accountsSystem()
	rootKeys := strings.Replace(effective22, "permitrootlogin yes", "permitrootlogin prohibit-password", 1)
	f.Respond(systemtest.Response{Result: system.Result{Stdout: rootKeys}}, ssh.SshdPath, "-T")
	r := runSSH(t, f, "6", "1", "2", "0", "0", "0")
	if strings.Contains(r.stdout, "选择用密码登录的用户") || !strings.Contains(r.stdout, "alice 的登录密码") {
		t.Errorf("root keys only:\n%s", r.stdout)
	}
	if st, _ := state.NewStore(f).Load(); chpasswdInput(f) != "" || st.SSH.PasswordAuth != nil {
		t.Errorf("going back changed the system: %+v", st.SSH)
	}

	f = sshSystem(effective22)
	reportAfterReload(f, strings.Replace(effective22, "pubkeyauthentication yes", "pubkeyauthentication no", 1))
	// Password only; root's password menu: 3 keep; keep the change (y).
	r = runSSH(t, f, "6", "1", "2", "3", "y", "0", "0")
	if !strings.Contains(r.stdout, "root 的登录密码") || !strings.Contains(r.stdout, "3. 保留现有密码\n") || strings.Contains(r.stdout, "选择用密码登录的用户") {
		t.Errorf("root alone:\n%s", r.stdout)
	}
	if got := chpasswdInput(f); got != "" {
		t.Errorf("kept password, but chpasswd read %q", got)
	}
	if st, _ := state.NewStore(f).Load(); st.SSH.PubkeyAuth == nil || *st.SSH.PubkeyAuth {
		t.Errorf("state = %+v", st.SSH)
	}
}

func TestSSHDeleteSudoUser(t *testing.T) {
	f := accountsSystem()
	// 3 delete: alice; the confirmation names the home directory.
	r := runSSH(t, f, "6", "5", "3", "1", "y", "0", "0", "0")
	if !strings.Contains(r.stdout, "删除用户 alice：主目录 /home/alice") || !strings.Contains(r.stdout, "└─ ✓ 用户 alice 已删除。") {
		t.Errorf("delete:\n%s", r.stdout)
	}
	if !strings.Contains(strings.Join(f.Execs(), "\n"), "userdel --remove -- alice") {
		t.Errorf("execs: %v", f.Execs())
	}
	// Refused when nobody else could log in: root has no key or password.
	f = accountsSystem()
	f.Files[ssh.ShadowPath] = []byte("root:!:19000::::::\nalice:$y$a:19000::::::\n")
	delete(f.Files, "/root/.ssh/authorized_keys")
	// Refused before the confirmation.
	r = runSSH(t, f, "6", "5", "3", "1", "0", "0", "0")
	if !strings.Contains(r.stdout, "删除后 root 和其他 sudo 用户都无法登录") || strings.Contains(r.stdout, "删除用户 alice：") ||
		strings.Contains(strings.Join(f.Execs(), "\n"), "userdel") {
		t.Errorf("lockout:\n%s", r.stdout)
	}
	// Without sudo users the option is hidden.
	r = runSSH(t, sshSystem(effective22), "6", "5", "0", "0", "0")
	if strings.Contains(r.stdout, "删除 sudo 用户") {
		t.Errorf("delete shown without sudo users:\n%s", r.stdout)
	}
}
