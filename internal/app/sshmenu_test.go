package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/ssh"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
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
		"═════════════════ SSH 管理 ═════════════════",
		" 端口: 22   登录方式: 密钥 + 密码 ⚠\n root 登录: 允许   公钥: 0 个\n",
		" 1. 密钥管理\n 2. 登录方式\n 3. root 登录\n 4. 修改 SSH 端口\n 5. 创建 sudo 用户\n 6. 回滚到上次修改前\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	// Key management: no keys, so "delete" is hidden.
	r = runSSH(t, f, "6", "1", "0", "0", "0")
	if !strings.Contains(r.stdout, " 公钥: （无）\n──────────────────────────────────────────────\n 1. 生成密钥对\n 2. 添加公钥\n──") || strings.Contains(r.stdout, "删除公钥") {
		t.Errorf("key menu:\n%s", r.stdout)
	}
}

func TestSSHMenuInterlockAndKeys(t *testing.T) {
	f := sshSystem(effective22)
	// Key only without a key: refused with an explanation.
	r := runSSH(t, f, "6", "2", "1", "0", "0")
	if !strings.Contains(r.stdout, "当前: 密钥 + 密码") || !strings.Contains(r.stdout, "1. 仅密钥") || !strings.Contains(r.stdout, "2. 仅密码") || !strings.Contains(r.stdout, "3. 密钥 + 密码") ||
		!strings.Contains(r.stdout, "请先添加公钥（并确认可以用密钥登录），再关闭密码登录。") {
		t.Errorf("interlock:\n%s", r.stdout)
	}
	// Choosing the current method does nothing.
	before := len(f.Mutations())
	runSSH(t, f, "6", "2", "3", "0", "0")
	if m := f.Mutations()[before:]; len(m) > 1 {
		t.Errorf("current method changed the system: %+v", m)
	}
	// Paste a bad key, then a good one; it appears in the list.
	r = runSSH(t, f, "6", "1", "2", "1", "not a key", testPub, "0", "0", "0")
	for _, want := range []string{"不是有效的 OpenSSH 公钥", "已添加 1 个公钥。", " 公钥:\n   [1] ed25519 SHA256:o/KaoigHu9ddvclgoQvWem5m+GbW7ahqewdkStS+qZw  laptop", " 3. 删除公钥", "公钥: 1 个"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q\n%s", want, r.stdout)
		}
	}
	// Generate a key pair: the private key is shown, not stored.
	r = runSSH(t, f, "6", "1", "1", "y", "", "0", "0", "0")
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
	// 5: name; bad name first; generated password; copy root keys: n.
	r := runSSH(t, f, "6", "5", "Bad", "admin", "1", "n", "0", "0")
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
	// 4: new port 2200; confirm; keep (y); delete old rules: y.
	r := runSSH(t, f, "6", "4", "2200", "y", "y", "y", "0", "0")
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

func TestSSHMenuRollbackNothing(t *testing.T) {
	r := runSSH(t, sshSystem(effective22), "6", "6", "y", "0", "0")
	if !strings.Contains(r.stdout, "没有可以回滚的 SSH 修改。") {
		t.Errorf("rollback:\n%s", r.stdout)
	}
}
