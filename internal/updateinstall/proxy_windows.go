package updateinstall

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func platformProxy(request *http.Request) (*url.URL, error) {
	if proxy, err := http.ProxyFromEnvironment(request); proxy != nil || err != nil {
		return proxy, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer key.Close()
	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil, nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil, nil
	}
	return windowsProxyURL(server, request.URL.Scheme)
}

func platformProxyForURL(source string) string {
	request, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return ""
	}
	proxy, err := platformProxy(request)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.String()
}

func windowsProxyURL(server, scheme string) (*url.URL, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil, nil
	}
	selected := server
	if strings.Contains(server, "=") {
		selected = ""
		fallback := ""
		for _, entry := range strings.Split(server, ";") {
			key, value, found := strings.Cut(strings.TrimSpace(entry), "=")
			if !found {
				continue
			}
			key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
			if key == strings.ToLower(scheme) {
				selected = value
				break
			}
			if key == "http" {
				fallback = value
			}
		}
		if selected == "" {
			selected = fallback
		}
	}
	if selected == "" {
		return nil, nil
	}
	if !strings.Contains(selected, "://") {
		selected = "http://" + selected
	}
	return url.Parse(selected)
}
