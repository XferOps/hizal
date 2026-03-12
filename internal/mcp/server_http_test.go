package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeHTTPInitializeNegotiatesProtocolVersion(t *testing.T) {
	srv := &Server{}

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("MCP-Protocol-Version"); got != "2025-03-26" {
		t.Fatalf("expected negotiated protocol header, got %q", got)
	}

	var resp struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Result.ProtocolVersion != "2025-03-26" {
		t.Fatalf("expected initialize result protocol version %q, got %q", "2025-03-26", resp.Result.ProtocolVersion)
	}
}

func TestServeHTTPAcceptsInitializedNotification(t *testing.T) {
	srv := &Server{}

	body := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty response body, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Fatalf("expected no content type for notification response, got %q", got)
	}
}

func TestServeHTTPRejectsUnsupportedTransportMethods(t *testing.T) {
	srv := &Server{}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp", nil)
			rec := httptest.NewRecorder()

			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected status 405, got %d", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != "GET, POST, DELETE" {
				t.Fatalf("expected Allow header, got %q", got)
			}
		})
	}
}
