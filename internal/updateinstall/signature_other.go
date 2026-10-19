//go:build !windows

package updateinstall

import (
	"context"
	"strings"

	"meow-launcher/internal/updatecontract"
)

type PlatformSignatureVerifier struct{}

func (PlatformSignatureVerifier) Verify(_ context.Context, _ string, signature updatecontract.Signature) error {
	if signature.Required {
		return newError("SIGNATURE_METHOD_UNSUPPORTED", "当前平台不支持所要求的 "+strings.TrimSpace(signature.Method)+" 签名验证", nil)
	}
	return nil
}
