package updateinstall

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

type codedError struct {
	code    string
	message string
	cause   error
}

func (e codedError) Error() string { return e.message }
func (e codedError) Unwrap() error { return e.cause }

func newError(code, message string, cause error) error {
	return codedError{code: code, message: message, cause: cause}
}

func CodeOf(err error) string {
	var target codedError
	if errors.As(err, &target) {
		return target.code
	}
	return "INSTALL_FAILED"
}

func networkErrorDetail(err error) string {
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError.Err != nil {
		err = urlError.Err
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "连接超时，请检查系统代理或网络连接"
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return "网络连接失败，请检查系统代理或网络连接"
	}
	if len(detail) > 180 {
		detail = detail[:180] + "…"
	}
	return detail
}
