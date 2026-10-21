package repoformat

import (
	"strings"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	for _, ok := range []string{"https://cdn.acme.com", "https://cdn.acme.com:8443", "https://10.0.0.1"} {
		if err := CheckOrigin(ok); err != nil {
			t.Errorf("CheckOrigin(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"", "cdn.acme.com", "http://cdn.acme.com", "https://cdn.acme.com/", "https://cdn.acme.com/lib",
		"https://cdn.acme.com?x", "https://cdn.acme.com#x", "https://cdn.acme.com:443", "https://*.acme.com",
		"*", "https:", "https://CDN.acme.com", "https://a.com; connect-src *", "https://a.com https://b.com",
		"https://u@a.com", "https://bücher.de", "data:",
	} {
		if err := CheckOrigin(bad); err == nil {
			t.Errorf("CheckOrigin(%q) = nil, want an error", bad)
		}
	}
}

func TestCheckApp(t *testing.T) {
	const app = `<!doctype html>
<html><head>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/a@1/a.css">
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/b@1/b.css" integrity="sha384-x" crossorigin="anonymous">
<link rel="icon" href="favicon.ico">
<script src="https://cdn.jsdelivr.net/npm/echarts@6/echarts.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/ok@1/ok.js" integrity="sha384-y" crossorigin="anonymous"></script>
<script src="./chart.js"></script>
<style>
  .a { background: url("bg.png") }
  .b { background: url(data:image/png;base64,AA) }
</style>
</head><body>
<img src="/logo.png" srcset="a.png 1x, https://x.com/b.png 2x">
<div style="background-image: url('../shared/c.png')"></div>
<a href="#top">top</a> <img src="data:image/png;base64,AA"> <img src="https://x.com/i.png">
<script>const s = "url(nope.png)"; </script>
</body></html>`
	var got []string
	for _, i := range CheckApp([]byte(app)) {
		got = append(got, i.String())
	}
	want := []string{
		`line 3: the stylesheet https://cdn.jsdelivr.net/npm/a@1/a.css has no integrity hash`,
		`line 5: "favicon.ico" is a file beside the app`,
		`line 6: the script https://cdn.jsdelivr.net/npm/echarts@6/echarts.min.js has no integrity hash`,
		`line 8: "./chart.js" is a file beside the app`,
		`line 10: "bg.png" is a file beside the app`,
		`line 14: "/logo.png" is a file beside the app`,
		`line 14: "a.png" is a file beside the app`,
		`line 15: "../shared/c.png" is a file beside the app`,
	}
	if len(got) != len(want) {
		t.Fatalf("CheckApp: got %d issues, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("issue %d = %q, want it to start %q", i, got[i], want[i])
		}
	}
}

func TestCheckAppClean(t *testing.T) {
	if issues := CheckApp([]byte(`<title>T</title><p>hi`)); len(issues) != 0 {
		t.Errorf("CheckApp = %v, want none", issues)
	}
}
