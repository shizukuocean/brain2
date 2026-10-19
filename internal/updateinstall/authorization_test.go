package updateinstall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizationClientSeparatesEntitlementFromPublicRelease(t *testing.T) {
	var got authorizationRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer session-secret" {
			t.Fatalf("authorization header missing")
		}
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"authorized":true,"message":"ok","authorization_id":"grant-123","expires_at":"2026-08-11T12:00:00Z"}`))
	}))
	defer server.Close()

	result := (AuthorizationClient{Endpoint: server.URL, SessionToken: "session-secret", HTTPClient: server.Client()}).Check(context.Background(), "dev-helper", "1.2.3")
	if result.Status != AuthorizationAuthorized || result.AuthorizationID != "grant-123" || got.ProductID != "dev-helper" || got.Version != "1.2.3" {
		t.Fatalf("result=%+v request=%+v", result, got)
	}
}

func TestAuthorizationClientRequiresFreshSession(t *testing.T) {
	result := (AuthorizationClient{Endpoint: "https://auth.example.invalid"}).Check(context.Background(), "dev-helper", "1.2.3")
	if result.Status != AuthorizationNotAuthenticated || result.ErrorCode != "AUTH_REQUIRED" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAuthorizationClientRejectsInsecureEndpoint(t *testing.T) {
	result := (AuthorizationClient{Endpoint: "http://auth.example.invalid", SessionToken: "session"}).Check(context.Background(), "dev-helper", "1.2.3")
	if result.Status != AuthorizationUnavailable || result.ErrorCode != "AUTH_ENDPOINT_INVALID" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAuthorizationClientExplainsForbiddenPurchase(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	result := (AuthorizationClient{Endpoint: server.URL, SessionToken: "session", HTTPClient: server.Client()}).Check(context.Background(), "brain2-manager", "1.0.0")
	if result.Status != AuthorizationDenied || result.ErrorCode != "AUTH_DENIED" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAuthorizationClientRejectsSuccessWithoutOneTimeAuthorizationID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"authorized":true}`))
	}))
	defer server.Close()
	result := (AuthorizationClient{Endpoint: server.URL, SessionToken: "session", HTTPClient: server.Client()}).Check(context.Background(), "dev-helper", "1.2.3")
	if result.Status != AuthorizationUnavailable || result.ErrorCode != "AUTH_RESPONSE_INVALID" {
		t.Fatalf("result=%+v", result)
	}
}
