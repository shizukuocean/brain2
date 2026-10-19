//go:build !windows

package updateinstall

import (
	"net/http"
	"net/url"
)

func platformProxy(request *http.Request) (*url.URL, error) {
	return http.ProxyFromEnvironment(request)
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
