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

func TestParseLatestLocation(t *testing.T) {
	for loc, want := range map[string]string{
		"https://github.com/XTLS/Xray-core/releases/tag/v26.3.27":     "v26.3.27",
		"https://github.com/Aspeternity/litenode/releases/tag/v0.3.4": "v0.3.4",
		"/XTLS/Xray-core/releases/tag/v26.10.17":                      "v26.10.17",
	} {
		if got, err := ParseLatestLocation(loc); err != nil || got != want {
			t.Errorf("ParseLatestLocation(%q) = (%q, %v), want %q", loc, got, err, want)
		}
	}
	// A repository without releases redirects to the releases list.
	for _, loc := range []string{"https://github.com/XTLS/Xray-core/releases", "https://github.com/login", "https://github.com/x/y/releases/tag/latest-build"} {
		if got, err := ParseLatestLocation(loc); err == nil {
			t.Errorf("ParseLatestLocation(%q) = %q, want an error", loc, got)
		}
	}
}

func TestParseFeed(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xml:lang="en-US">
  <link type="text/html" rel="alternate" href="https://github.com/XTLS/Xray-core/releases"/>
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/v26.10.17"/>
    <title>Xray-core v26.10.17</title>
  </entry>
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/nightly"/>
  </entry>
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/v26.3.27"/>
  </entry>
</feed>`
	rs, err := ParseFeed([]byte(feed))
	if err != nil || len(rs) != 2 || rs[0].Tag != "v26.10.17" || rs[1].Tag != "v26.3.27" {
		t.Errorf("ParseFeed = (%v, %v)", rs, err)
	}
	if _, err := ParseFeed([]byte("<html>rate limited")); err == nil {
		t.Error("garbage should fail")
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

func TestCompareVersions(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"v26.9.8", "v26.9.8", 0}, {"v26.10.17", "v26.9.8", 1}, {"v26.3.27", "v26.9.8", -1},
		{"v26.9.30", "v26.9.8", 1}, {"v27.1.1", "v26.12.31", 1}, {"v26.9.8-beta.1", "v26.9.8", 0},
	} {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	for v, want := range map[string]bool{"v26.9.8": true, "v26.10.17": true, "v26.7.28": false, "v26.3.27": false, "": false} {
		if SupportsMLKEMTargets(v) != want {
			t.Errorf("SupportsMLKEMTargets(%q) = %v", v, !want)
		}
	}
}

func TestPickDefault(t *testing.T) {
	releases := []Release{
		{Tag: "v26.10.17", Prerelease: true}, {Tag: "v26.10.10", Prerelease: true},
		{Tag: "v26.7.28", Prerelease: true}, {Tag: "v26.3.27"}, {Tag: "v26.11.1", Draft: true},
	}
	// Today: stable is too old, so the newest qualifying pre-release.
	if got := PickDefault("v26.3.27", releases); got != "v26.10.17" {
		t.Errorf("got %s, want v26.10.17", got)
	}
	// Once a qualifying stable exists, stable wins.
	if got := PickDefault("v26.12.1", releases); got != "v26.12.1" {
		t.Errorf("got %s, want stable v26.12.1", got)
	}
	// Nothing qualifies: stable.
	if got := PickDefault("v26.3.27", []Release{{Tag: "v26.7.28", Prerelease: true}}); got != "v26.3.27" {
		t.Errorf("got %s, want v26.3.27", got)
	}
}

func TestDefaultVersionUsesReleaseList(t *testing.T) {
	f := fakeFetcher{
		LatestPageURL(Repo): []byte("https://github.com/XTLS/Xray-core/releases/tag/v26.3.27"),
		FeedURL(Repo):       []byte(`<feed><entry><link href="https://github.com/XTLS/Xray-core/releases/tag/v26.10.20"/></entry></feed>`),
		ReleasesURL:         []byte(`[{"tag_name": "v26.10.17", "prerelease": true}, {"tag_name": "v26.3.27"}]`),
	}
	in := &Installer{Fetcher: f}
	if v, err := in.DefaultVersion(t.Context()); err != nil || v != "v26.10.20" {
		t.Errorf("feed: DefaultVersion = (%q, %v)", v, err)
	}
	delete(f, FeedURL(Repo)) // feed unavailable: the API list
	if v, err := in.DefaultVersion(t.Context()); err != nil || v != "v26.10.17" {
		t.Errorf("api: DefaultVersion = (%q, %v)", v, err)
	}
	delete(f, ReleasesURL) // list unavailable: fall back to stable
	if v, err := in.DefaultVersion(t.Context()); err != nil || v != "v26.3.27" {
		t.Errorf("fallback = (%q, %v)", v, err)
	}
	// A stable release new enough needs no list.
	f = fakeFetcher{LatestPageURL(Repo): []byte("https://github.com/XTLS/Xray-core/releases/tag/v26.11.1")}
	if v, err := (&Installer{Fetcher: f}).DefaultVersion(t.Context()); err != nil || v != "v26.11.1" {
		t.Errorf("new stable = (%q, %v)", v, err)
	}
	// The latest stable unknown: an error.
	if _, err := (&Installer{Fetcher: fakeFetcher{}}).DefaultVersion(t.Context()); err == nil {
		t.Error("unknown latest: want an error")
	}
}
