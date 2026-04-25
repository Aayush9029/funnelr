package tailscale

import "testing"

func TestFindURL(t *testing.T) {
	data := []byte(`{"Web":{"TCP":{"443":{"HTTPS":true,"Handlers":{"/":{"Proxy":"http://127.0.0.1:1234","URL":"https://mac.example.ts.net/"}}}}}}`)
	got := findURL(data)
	if got != "https://mac.example.ts.net/" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusHasFunnel(t *testing.T) {
	var st Status
	st.Self.Capabilities = []string{"https://tailscale.com/cap/file-sharing", "https://tailscale.com/cap/funnel-ports?ports=443,8443,10000"}
	if !st.HasFunnel() {
		t.Fatal("expected funnel capability")
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"1.96.5-t4ee448d3a", true},
		{"1.52.0", true},
		{"1.51.9", false},
		{"bad", false},
	}
	for _, tc := range cases {
		if got := versionAtLeast(tc.version, 1, 52); got != tc.want {
			t.Fatalf("versionAtLeast(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
