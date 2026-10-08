package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/louislw/AIDataAnalysis/internal/config"
	"github.com/louislw/AIDataAnalysis/internal/datasource"
	"github.com/louislw/AIDataAnalysis/internal/llm"
	"github.com/louislw/AIDataAnalysis/internal/store"
	_ "modernc.org/sqlite"
)

type scriptLLM struct {
	steps []llm.Message
	calls int
}

func (s *scriptLLM) Chat(context.Context, []llm.Message, []llm.Tool) (llm.Message, error) {
	if s.calls >= len(s.steps) {
		return llm.Message{}, errString("模型没有更多回复")
	}
	msg := s.steps[s.calls]
	s.calls++
	return msg, nil
}

type errString string

func (e errString) Error() string { return string(e) }

func TestHealthDoesNotCallModel(t *testing.T) {
	h, _ := newTestApp(t, &scriptLLM{steps: nil})
	res := getJSON(t, h, http.MethodGet, "/api/health", "", nil)
	if res["status"] != "ok" || res["llmTimeoutSeconds"].(float64) != 2 {
		t.Fatalf("health = %#v", res)
	}
}

func TestRegisterDatasourcePersonaAndSeeSQL(t *testing.T) {
	model := &scriptLLM{steps: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID: "c1", Type: "function",
				Function: llm.FunctionCall{Name: "executeSql", Arguments: `{"sql":"SELECT name FROM employee"}`},
			}},
		},
		{Role: "assistant", Content: "本部门有 Ada。"},
	}}
	h, _ := newTestApp(t, model)
	admin := login(t, h, "admin", "admin-pass")

	created := postJSON(t, h, "/api/datasources", admin, map[string]any{"name": "人事库", "fileName": "hr.sqlite"})
	dsID := int64(created["id"].(float64))
	tested := postJSON(t, h, "/api/datasources/test", admin, map[string]any{"fileName": "hr.sqlite"})
	if tested["tableCount"].(float64) != 1 {
		t.Fatalf("tableCount = %#v", tested)
	}
	putJSON(t, h, "/api/datasources/1/tables/employee/columns/salary/meta", admin, map[string]any{
		"comment": "工资", "description": "", "sensitive": true,
	})
	cols := getJSON(t, h, http.MethodGet, "/api/datasources/1/tables/employee/columns", admin, nil)
	foundSensitive := false
	for _, item := range cols["columns"].([]any) {
		col := item.(map[string]any)
		if col["name"] == "salary" && col["sensitive"] == true {
			foundSensitive = true
		}
	}
	if !foundSensitive {
		t.Fatalf("columns = %#v", cols)
	}
	preview := getJSON(t, h, http.MethodGet, "/api/datasources/1/tables/employee/preview", admin, nil)
	row0 := preview["rows"].([]any)[0].([]any)
	headers := preview["columns"].([]any)
	for i, name := range headers {
		if name == "salary" && row0[i] != "***" {
			t.Fatalf("salary preview = %#v", row0)
		}
	}

	persona := postJSON(t, h, "/api/personas", admin, map[string]any{
		"name": "部门分析", "description": "", "systemPrompt": "只回答本部门",
		"prohibitions": "不要猜测", "rowFilter": "dept_id = {{user.dept}}",
		"datasourceId": dsID, "status": 1,
	})
	pid := int64(persona["id"].(float64))
	putJSON(t, h, "/api/personas/1/tables", admin, map[string]any{"tables": []string{"employee"}})
	putJSON(t, h, "/api/personas/1/roles", admin, map[string]any{"roles": []string{"USER"}})
	putJSON(t, h, "/api/personas/1/metrics", admin, map[string]any{
		"metrics": []map[string]any{{"name": "在职人数", "definition": "员工行数", "sortOrder": 0}},
	})
	putJSON(t, h, "/api/personas/1/examples", admin, map[string]any{
		"examples": []map[string]any{{"question": "有哪些人", "sql": "SELECT name FROM employee", "sortOrder": 0}},
	})
	catalogRes := postJSON(t, h, "/api/personas/1/catalog/rebuild", admin, map[string]any{})
	text := catalogRes["catalogText"].(string)
	if strings.Contains(text, "salary") || !strings.Contains(text, "name") {
		t.Fatalf("catalog leaked or missed columns: %s", text)
	}

	putJSON(t, h, "/api/auth/users/1", admin, map[string]any{
		"role": "ADMIN", "displayName": "管理员", "dept": "D3", "enabled": true,
	})
	sess := postJSON(t, h, "/api/personas/1/sessions", admin, map[string]any{})
	answer := postJSON(t, h, "/api/sessions/"+sess["id"].(string)+"/query", admin, map[string]any{
		"question": "有哪些人？",
	})
	if answer["kind"] != "answer" || !strings.Contains(answer["sql"].(string), "employee") || !strings.Contains(answer["content"].(string), "Ada") {
		t.Fatalf("query = %#v", answer)
	}
	if answer["rowCount"].(float64) != 1 {
		t.Fatalf("rowCount = %#v", answer)
	}
	loaded := getJSON(t, h, http.MethodGet, "/api/sessions/"+sess["id"].(string), admin, nil)
	msgs := loaded["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %#v", loaded)
	}

	userTok := login(t, h, "user", "user-pass1")
	putJSON(t, h, "/api/auth/users/2", admin, map[string]any{
		"role": "USER", "displayName": "用户", "dept": "D3", "enabled": true,
	})
	mine := getJSON(t, h, http.MethodGet, "/api/personas/mine", userTok, nil)
	if len(mine["items"].([]any)) != 1 {
		t.Fatalf("mine = %#v", mine)
	}
	denied := do(t, h, http.MethodGet, "/api/datasources", userTok, nil)
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("user datasources status %d", denied.StatusCode)
	}
	denied.Body.Close()

	empty := putJSON(t, h, "/api/personas/1/tables", admin, map[string]any{"tables": []string{}})
	if n := len(empty["tables"].([]any)); n != 0 {
		t.Fatalf("tables = %#v", empty)
	}
	if pid != 1 {
		t.Fatalf("persona id %d", pid)
	}
}

func TestQueryClarifyAndSensitiveStayEmptySQL(t *testing.T) {
	model := &scriptLLM{steps: []llm.Message{
		{Role: "assistant", Content: "你想按部门还是按人看？"},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID: "c1", Type: "function",
				Function: llm.FunctionCall{Name: "executeSql", Arguments: `{"sql":"SELECT salary FROM employee"}`},
			}},
		},
		{Role: "assistant", Content: "工资是 10。"},
	}}
	h, _ := newTestApp(t, model)
	admin := login(t, h, "admin", "admin-pass")
	postJSON(t, h, "/api/datasources", admin, map[string]any{"name": "人事库", "fileName": "hr.sqlite"})
	postJSON(t, h, "/api/personas", admin, map[string]any{
		"name": "部门分析", "datasourceId": 1, "status": 1,
		"rowFilter": "dept_id = {{user.dept}}",
	})
	putJSON(t, h, "/api/personas/1/tables", admin, map[string]any{"tables": []string{"employee"}})
	putJSON(t, h, "/api/datasources/1/tables/employee/columns/salary/meta", admin, map[string]any{"sensitive": true})
	putJSON(t, h, "/api/auth/users/1", admin, map[string]any{"role": "ADMIN", "displayName": "管理员", "dept": "D3", "enabled": true})
	sess := postJSON(t, h, "/api/personas/1/sessions", admin, map[string]any{})
	clarify := postJSON(t, h, "/api/sessions/"+sess["id"].(string)+"/query", admin, map[string]any{"question": "看一下"})
	if clarify["kind"] != "clarify" || clarify["sql"] != "" {
		t.Fatalf("clarify = %#v", clarify)
	}
	blocked := postJSON(t, h, "/api/sessions/"+sess["id"].(string)+"/query", admin, map[string]any{"question": "工资"})
	if blocked["kind"] != "error" || blocked["sql"] != "" {
		t.Fatalf("blocked = %#v", blocked)
	}
}

func newTestApp(t *testing.T, client llm.Client) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "sources", "hr.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE employee (id INTEGER PRIMARY KEY, name TEXT, dept_id TEXT, salary INTEGER);
		INSERT INTO employee (name, dept_id, salary) VALUES ('Ada', 'D3', 10), ('Bea', 'D9', 99);`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Bootstrap("admin-pass", "user-pass1"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		DataRoot: dir, JWTSecret: "test-secret",
		LLMTimeout: 2 * time.Second, AgentMaxLoop: 6, AgentMaxRows: 50, AgentHistoryTurns: 6,
	}
	return New(cfg, st, datasource.New(dir), client), st
}

func login(t *testing.T, h http.Handler, username, password string) string {
	t.Helper()
	body := postJSON(t, h, "/api/auth/login", "", map[string]any{"username": username, "password": password})
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("login %s = %#v", username, body)
	}
	return token
}

func postJSON(t *testing.T, h http.Handler, path, token string, payload any) map[string]any {
	t.Helper()
	return finish(t, do(t, h, http.MethodPost, path, token, payload))
}

func putJSON(t *testing.T, h http.Handler, path, token string, payload any) map[string]any {
	t.Helper()
	return finish(t, do(t, h, http.MethodPut, path, token, payload))
}

func getJSON(t *testing.T, h http.Handler, method, path, token string, payload any) map[string]any {
	t.Helper()
	return finish(t, do(t, h, method, path, token, payload))
}

func finish(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		t.Fatalf("%s %s", res.Status, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		var arr []any
		if err2 := json.Unmarshal(raw, &arr); err2 == nil {
			return map[string]any{"items": arr}
		}
		t.Fatalf("json %s: %v", raw, err)
	}
	return obj
}

func do(t *testing.T, h http.Handler, method, path, token string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}
