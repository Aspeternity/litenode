package xray

import (
	"os"
	"testing"
)

func TestAssetName(t *testing.T) {
	for arch, want := range map[string]string{"amd64": "Xray-linux-64.zip", "arm64": "Xray-linux-arm64-v8a.zip"} {
		if got, err := AssetName(arch); err != nil || got != want {
			t.Errorf("AssetName(%s) = (%q, %v), want %q", arch, got, err, want)
		}
	}
	for _, arch := range []string{"386", "arm", "riscv64", ""} {
		if _, err := AssetName(arch); err == nil {
			t.Errorf("AssetName(%q) should fail", arch)
		}
	}
	if got := DownloadURL("v26.3.27", "Xray-linux-64.zip"); got !=
		"https://github.com/XTLS/Xray-core/releases/download/v26.3.27/Xray-linux-64.zip" {
		t.Errorf("DownloadURL = %q", got)
	}
}

// realDigest is Xray-linux-64.zip.dgst from the v26.3.27 release.
const realDigest = `MD5= ee4e2ff74948a9b464624b1cabc44409
SHA1= b55b06e74e89083b9cedfdecf0d68b579cd2af72
SHA2-256= 23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae
SHA2-512= e8bc40a0687cac184bbe4b5c1f047e69064ccedc489fb25e208889ae287bbf8736dff16b108d68fc00dc33edc8bb53502e47a9698a277f4f51b67b83d899e518
`

func TestParseDigest(t *testing.T) {
	got, err := ParseDigest([]byte(realDigest))
	if err != nil || got != "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae" {
		t.Errorf("got (%q, %v)", got, err)
	}
	for name, bad := range map[string]string{
		"missing":   "MD5= ee4e2ff74948a9b464624b1cabc44409\n",
		"short":     "SHA2-256= abcd\n",
		"not hex":   "SHA2-256= " + string(make([]byte, 64)) + "\n",
		"sha2-512":  "SHA2-512= e8bc40a0\n",
		"empty":     "",
		"no spaces": "SHA2-256=zz\n",
	} {
		if _, err := ParseDigest([]byte(bad)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{
		"26.3.27": "v26.3.27", "v26.3.27": "v26.3.27", " v1.8.4 ": "v1.8.4", "v26.10.17-beta.1": "v26.10.17-beta.1",
	} {
		if got, err := NormalizeVersion(in); err != nil || got != want {
			t.Errorf("NormalizeVersion(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "latest", "v26", "26.3", "v26.3.27/../x", "v26.3.27 x"} {
		if _, err := NormalizeVersion(in); err == nil {
			t.Errorf("NormalizeVersion(%q) should fail", in)
		}
	}
}

func TestParseLatestRelease(t *testing.T) {
	got, err := ParseLatestRelease([]byte(`{"tag_name": "v26.3.27", "prerelease": false}`))
	if err != nil || got != "v26.3.27" {
		t.Errorf("got (%q, %v)", got, err)
	}
	if _, err := ParseLatestRelease([]byte(`{"message": "API rate limit exceeded"}`)); err == nil {
		t.Error("missing tag should fail")
	}
}

func TestParseVersionOutput(t *testing.T) {
	out := "Xray 26.3.27 (Xray, Penetrates Everything.) 2eb5d3b (go1.26.1 linux/amd64)\nA unified platform for anti-censorship.\n"
	if got, err := ParseVersionOutput(out); err != nil || got != "v26.3.27" {
		t.Errorf("got (%q, %v)", got, err)
	}
	if _, err := ParseVersionOutput("V2Ray 5.1.0\n"); err == nil {
		t.Error("non-xray output should fail")
	}
}

func TestUnitGolden(t *testing.T) {
	want, err := os.ReadFile("testdata/litenode-xray.service.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := Unit(); got != string(want) {
		t.Errorf("unit mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
