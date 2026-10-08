package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatRequiresConfigAndParsesToolCall(t *testing.T) {
	if _, err := (HTTPClient{}).Chat(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "模型未配置") {
		t.Fatalf("err = %v", err)
	}
	var sawAuth, sawModel bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer secret"
		body, _ := io.ReadAll(r.Body)
		sawModel = strings.Contains(string(body), `"model":"demo"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"executeSql","arguments":"{\"sql\":\"SELECT 1\"}"}}]}}]}`))
	}))
	defer srv.Close()
	msg, err := (HTTPClient{
		BaseURL: srv.URL + "/v1",
		APIKey:  "secret",
		Model:   "demo",
		Timeout: time.Second,
	}).Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, []Tool{{Type: "function"}})
	if err != nil {
		t.Fatal(err)
	}
	if !sawAuth || !sawModel || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "executeSql" {
		t.Fatalf("msg=%+v auth=%v model=%v", msg, sawAuth, sawModel)
	}
}
