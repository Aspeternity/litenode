package app

import (
	"strings"
	"testing"

	"github.com/Aspeternity/litenode/internal/state"
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
