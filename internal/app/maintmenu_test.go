package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/maint"
	"github.com/Aspeternity/litenode/internal/state"
	"github.com/Aspeternity/litenode/internal/system"
	"github.com/Aspeternity/litenode/internal/system/systemtest"
)

func TestMaintExportImport(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	// Export to a chosen path.
	r := runNodes(t, f, "7\n1\n/root/x.json\n0\n0\n")
	for _, want := range []string{" 1. 导出配置", " 2. 导入配置", "已导出到 /root/x.json：1 个节点、1 个客户端、0 条转发规则、0 条防火墙规则。", "权限已设为 600"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("export: missing %q:\n%s", want, r.stdout)
		}
	}
	if f.Modes["/root/x.json"] != 0o600 {
		t.Errorf("export mode %o", f.Modes["/root/x.json"])
	}
	// Delete the node, then import it back.
	store := state.NewStore(f)
	st, _ := store.Load()
	st.Nodes = nil
	if err := store.Save(st); err != nil {
		t.Fatal(err)
	}
	r = runNodes(t, f, "7\n2\n/root/x.json\ny\n0\n0\n")
	for _, want := range []string{"包含 1 个节点、1 个客户端", "将替换当前的 0 个节点", "配置已导入。"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("import: missing %q:\n%s", want, r.stdout)
		}
	}
	if st, _ := state.NewStore(f).Load(); len(st.Nodes) != 1 {
		t.Errorf("nodes after import: %d", len(st.Nodes))
	}
	// Errors: missing file, not an export.
	f.Files["/root/state.json"] = []byte(`{"schema_version":1}`)
	r = runNodes(t, f, "7\n2\n/root/none.json\n2\n/root/state.json\n2\n\n0\n0\n")
	for _, want := range []string{"文件不存在。", "这不是 LiteNode 导出的配置文件。"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("errors: missing %q:\n%s", want, r.stdout)
		}
	}
}

type releaseFetcher map[string][]byte

func (f releaseFetcher) Fetch(_ context.Context, url string, _ int64) ([]byte, error) {
	if d, ok := f[url]; ok {
		return d, nil
	}
	return nil, errors.New("404")
}

func TestMaintUpdate(t *testing.T) {
	bin := []byte("new build")
	sum := sha256.Sum256(bin)
	fetch := releaseFetcher{
		maint.LatestURL: []byte(`{"tag_name":"v0.1.0"}`),
		maint.AssetURL("v0.1.0", "litenode-linux-"+runtime.GOARCH): bin,
		maint.AssetURL("v0.1.0", "sha256sums.txt"):                 []byte(hex.EncodeToString(sum[:]) + "  litenode-linux-" + runtime.GOARCH + "\n"),
	}
	f := supported()
	f.Files[maint.BinPath] = []byte("old build")
	f.Respond(systemtest.Response{Result: system.Result{Stdout: "LiteNode v0.1.0 (linux/amd64)\n"}}, maint.BinPath+".new", "version")
	env := Env{Runner: f, Euid: 0, LockPath: filepath.Join(t.TempDir(), "lock"), XrayFetcher: fetch, Version: "v0.0.2"}
	// The update quits LiteNode: no more input is read.
	r := runEnv(t, env, "7\n3\ny\n")
	for _, want := range []string{"当前版本 v0.0.2，最新版本 v0.1.0。确定更新？", "已更新到 v0.1.0，旧版本保留为 /usr/local/bin/litenode.bak", "请重新运行 lnode"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.stdout)
		}
	}
	if string(f.Files[maint.BinPath]) != "new build" || string(f.Files[maint.BakPath]) != "old build" {
		t.Error("program not replaced")
	}
	// Already the latest.
	env.Version = "v0.1.0"
	r = runEnv(t, env, "7\n3\n0\n0\n")
	if !strings.Contains(r.stdout, "已是最新版本（v0.1.0）。") {
		t.Errorf("latest:\n%s", r.stdout)
	}
}

func TestMaintUninstallMenu(t *testing.T) {
	f := nodeSystem()
	runNodes(t, f, strings.Join([]string{"1", "1", "1", "", "3", "www.example.com", "", "0", "0"}, "\n")+"\n")
	f.Files[maint.BinPath] = []byte("litenode")
	// Declining either confirmation changes nothing.
	r := runNodes(t, f, "7\n5\ny\nn\n4\nn\n0\n0\n")
	if !strings.Contains(r.stdout, " 4. 卸载脚本") || !strings.Contains(r.stdout, " 5. 全部卸载") || !strings.Contains(r.stdout, "此操作不可恢复") {
		t.Errorf("menu:\n%s", r.stdout)
	}
	if f.Files[maint.BinPath] == nil || f.Files[state.DefaultPath] == nil {
		t.Fatal("declined uninstall removed files")
	}
	r = runNodes(t, f, "7\n5\ny\ny\n")
	for _, want := range []string{"LiteNode 已全部卸载。", "重新安装："} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q:\n%s", want, r.stdout)
		}
	}
	if f.Files[maint.BinPath] != nil || f.Files[state.DefaultPath] != nil {
		t.Error("files left after full uninstall")
	}
}
