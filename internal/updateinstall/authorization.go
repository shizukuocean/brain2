package updateinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AuthorizationAuthorized       = "authorized"
	AuthorizationNotAuthenticated = "not_authenticated"
	AuthorizationDenied           = "denied"
	AuthorizationUnavailable      = "unavailable"
)

type AuthorizationResult struct {
	Status          string `json:"status"`
	Message         string `json:"message"`
	ErrorCode       string `json:"error_code,omitempty"`
	AuthorizationID string `json:"authorization_id,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

type AuthorizationClient struct {
	Endpoint     string
	SessionToken string
	HTTPClient   *http.Client
}

type authorizationRequest struct {
	ProductID string `json:"product_id"`
	Version   string `json:"version"`
}

type authorizationResponse struct {
	Authorized      bool   `json:"authorized"`
	Message         string `json:"message"`
	AuthorizationID string `json:"authorization_id"`
	ExpiresAt       string `json:"expires_at"`
}

func (c AuthorizationClient) Check(ctx context.Context, productID, version string) AuthorizationResult {
	if strings.TrimSpace(c.SessionToken) == "" {
		return AuthorizationResult{Status: AuthorizationNotAuthenticated, ErrorCode: "AUTH_REQUIRED", Message: "请先在 Launcher 登录，再核验本次安装资格"}
	}
	endpoint, err := url.Parse(strings.TrimSpace(c.Endpoint))
	if err != nil || !strings.EqualFold(endpoint.Scheme, "https") || endpoint.Host == "" {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_ENDPOINT_INVALID", Message: "安装授权服务尚未安全配置"}
	}
	payload, _ := json.Marshal(authorizationRequest{ProductID: strings.TrimSpace(productID), Version: strings.TrimSpace(version)})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_UNAVAILABLE", Message: "无法创建安装授权请求"}
	}
	request.Header.Set("Authorization", "Bearer "+c.SessionToken)
	request.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_UNAVAILABLE", Message: "安装授权服务暂时不可用"}
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return AuthorizationResult{Status: AuthorizationNotAuthenticated, ErrorCode: "AUTH_REQUIRED", Message: "登录已失效，请重新登录后再安装"}
	case http.StatusForbidden:
		return AuthorizationResult{Status: AuthorizationDenied, ErrorCode: "AUTH_DENIED", Message: "当前会员或购买状态不包含这个版本的安装权限"}
	case http.StatusOK:
	default:
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_UNAVAILABLE", Message: fmt.Sprintf("安装授权服务返回 HTTP %d", response.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_RESPONSE_INVALID", Message: "无法读取安装授权结果"}
	}
	var decoded authorizationResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_RESPONSE_INVALID", Message: "安装授权结果格式无效"}
	}
	message := strings.TrimSpace(decoded.Message)
	if !decoded.Authorized {
		if message == "" {
			message = "当前会员或购买状态不包含这个版本的安装权限"
		}
		return AuthorizationResult{Status: AuthorizationDenied, ErrorCode: "AUTH_DENIED", Message: message}
	}
	if strings.TrimSpace(decoded.AuthorizationID) == "" {
		return AuthorizationResult{Status: AuthorizationUnavailable, ErrorCode: "AUTH_RESPONSE_INVALID", Message: "授权服务未返回本次安装授权编号"}
	}
	if message == "" {
		message = "本次安装授权已核验"
	}
	return AuthorizationResult{Status: AuthorizationAuthorized, Message: message, AuthorizationID: decoded.AuthorizationID, ExpiresAt: decoded.ExpiresAt}
}
