package updateinstall

import "testing"

func TestWindowsProxyURLSupportsCommonInternetSettingsFormats(t *testing.T) {
	tests := []struct {
		server string
		scheme string
		want   string
	}{
		{server: "127.0.0.1:7890", scheme: "https", want: "http://127.0.0.1:7890"},
		{server: "http=127.0.0.1:7890;https=127.0.0.1:7891", scheme: "https", want: "http://127.0.0.1:7891"},
		{server: "http=127.0.0.1:7890", scheme: "https", want: "http://127.0.0.1:7890"},
	}
	for _, test := range tests {
		got, err := windowsProxyURL(test.server, test.scheme)
		if err != nil {
			t.Fatalf("server=%q: %v", test.server, err)
		}
		if got == nil || got.String() != test.want {
			t.Fatalf("server=%q scheme=%q got=%v want=%q", test.server, test.scheme, got, test.want)
		}
	}
}
