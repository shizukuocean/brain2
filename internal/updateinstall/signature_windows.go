//go:build windows

package updateinstall

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"meow-launcher/internal/updatecontract"
)

type PlatformSignatureVerifier struct{}

func (PlatformSignatureVerifier) Verify(ctx context.Context, path string, signature updatecontract.Signature) error {
	if !signature.Required {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(signature.Method), "authenticode") {
		return newError("SIGNATURE_METHOD_UNSUPPORTED", "Windows 安装包必须使用 Authenticode 签名", nil)
	}
	const script = `& { param([string]$FilePath) $s = Get-AuthenticodeSignature -LiteralPath $FilePath; [pscustomobject]@{Status=$s.Status.ToString();Subject=if($s.SignerCertificate){$s.SignerCertificate.Subject}else{''}} | ConvertTo-Json -Compress }`
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script, path)
	keepUpdateCommandInBackground(command)
	output, err := command.Output()
	if err != nil {
		return newError("SIGNATURE_CHECK_FAILED", "无法验证安装包的 Windows 签名", err)
	}
	var result struct {
		Status  string `json:"Status"`
		Subject string `json:"Subject"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return newError("SIGNATURE_CHECK_FAILED", "Windows 签名验证结果无效", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result.Status), "Valid") {
		return newError("SIGNATURE_INVALID", "安装包没有有效的 Windows Authenticode 签名", nil)
	}
	if !publisherMatches(result.Subject, signature.Publisher) {
		return newError("SIGNATURE_PUBLISHER_MISMATCH", "安装包签名发布者与发布清单不一致", nil)
	}
	return nil
}

func publisherMatches(subject, expected string) bool {
	subject = strings.TrimSpace(subject)
	expected = strings.TrimSpace(expected)
	if subject == "" || expected == "" {
		return false
	}
	if strings.EqualFold(subject, expected) {
		return true
	}
	for _, part := range strings.Split(subject, ",") {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, expected) || (strings.HasPrefix(strings.ToUpper(part), "CN=") && strings.EqualFold(strings.TrimSpace(part[3:]), expected)) {
			return true
		}
	}
	return false
}
