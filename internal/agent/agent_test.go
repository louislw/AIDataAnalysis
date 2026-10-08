package agent

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/louislw/AIDataAnalysis/internal/llm"
	"github.com/louislw/AIDataAnalysis/internal/query"
	"github.com/louislw/AIDataAnalysis/internal/store"
	_ "modernc.org/sqlite"
)

type scriptLLM struct {
	steps []llm.Message
	seen  [][]llm.Message
}

func (s *scriptLLM) Chat(_ context.Context, messages []llm.Message, _ []llm.Tool) (llm.Message, error) {
	copied := append([]llm.Message(nil), messages...)
	s.seen = append(s.seen, copied)
	if len(s.seen) > len(s.steps) {
		return llm.Message{}, fmt.Errorf("模型没有更多回复")
	}
	return s.steps[len(s.seen)-1], nil
}

type failLLM struct{ err error }

func (f failLLM) Chat(context.Context, []llm.Message, []llm.Tool) (llm.Message, error) {
	return llm.Message{}, f.err
}

func TestRunAnswerUsesQueryEvidence(t *testing.T) {
	scope := hrScope(t)
	client := &scriptLLM{steps: []llm.Message{
		toolCall("c1", "executeSql", `{"sql":"SELECT name FROM employee"}`),
		{Role: "assistant", Content: "本部门有 Ada。"},
	}}
	ans, err := Run(context.Background(), client, scope, "有哪些人？", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ans.Kind != "answer" || ans.RowCount == nil || *ans.RowCount != 1 || ans.Truncated {
		t.Fatalf("answer = %+v", ans)
	}
	if !strings.Contains(ans.SQL, "employee") || !strings.Contains(ans.Content, "Ada") {
		t.Fatalf("answer = %+v", ans)
	}
	if len(ans.Tables) != 1 || ans.Tables[0] != "employee" {
		t.Fatalf("tables = %#v", ans.Tables)
	}
}

func TestRunClarifyAndUngroundedError(t *testing.T) {
	scope := Scope{MaxLoop: 3, Persona: store.Persona{Name: "人事"}}
	clarify, err := Run(context.Background(), &scriptLLM{steps: []llm.Message{
		{Role: "assistant", Content: "你想看哪个部门？"},
	}}, scope, "人数呢", nil)
	if err != nil || clarify.Kind != "clarify" || clarify.SQL != "" {
		t.Fatalf("clarify = %+v err=%v", clarify, err)
	}

	ungrounded, err := Run(context.Background(), &scriptLLM{steps: []llm.Message{
		{Role: "assistant", Content: "一共 42 人。"},
	}}, scope, "有多少人", nil)
	if err != nil || ungrounded.Kind != "error" || ungrounded.SQL != "" {
		t.Fatalf("ungrounded = %+v err=%v", ungrounded, err)
	}

	failed, err := Run(context.Background(), failLLM{err: fmt.Errorf("模型未配置")}, scope, "有多少人", nil)
	if err != nil || failed.Kind != "error" || !strings.Contains(failed.Content, "模型未配置") {
		t.Fatalf("failed = %+v err=%v", failed, err)
	}
}

func TestRunHidesSensitiveColumnAndStopsAtMaxLoop(t *testing.T) {
	scope := hrScope(t)
	client := &scriptLLM{steps: []llm.Message{
		toolCall("c1", "getTableSchema", `{"table":"employee"}`),
		{Role: "assistant", Content: "字段已列出。"},
	}}
	ans, err := Run(context.Background(), client, scope, "有哪些字段？", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ans.Kind != "error" {
		t.Fatalf("schema lookup alone is not an answer: %+v", ans)
	}
	toolText := client.seen[1][len(client.seen[1])-1].Content
	if strings.Contains(strings.ToLower(toolText), "salary") {
		t.Fatalf("schema leaked salary: %s", toolText)
	}
	if !strings.Contains(toolText, "name") {
		t.Fatalf("schema missing name: %s", toolText)
	}

	loop := &scriptLLM{steps: []llm.Message{
		toolCall("a", "listTables", `{}`),
		toolCall("b", "listTables", `{}`),
	}}
	scope.MaxLoop = 2
	exhausted, err := Run(context.Background(), loop, scope, "列出表", nil)
	if err != nil || exhausted.Kind != "error" || exhausted.SQL != "" {
		t.Fatalf("exhausted = %+v err=%v", exhausted, err)
	}
}

func TestRunRejectsSensitiveSelect(t *testing.T) {
	scope := hrScope(t)
	client := &scriptLLM{steps: []llm.Message{
		toolCall("c1", "executeSql", `{"sql":"SELECT salary FROM employee"}`),
		{Role: "assistant", Content: "工资是 10。"},
	}}
	ans, err := Run(context.Background(), client, scope, "工资是多少", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ans.Kind != "error" || ans.SQL != "" {
		t.Fatalf("sensitive select became an answer: %+v", ans)
	}
	if !strings.Contains(client.seen[1][len(client.seen[1])-1].Content, "SQL 执行失败") {
		t.Fatalf("tool text = %s", client.seen[1][len(client.seen[1])-1].Content)
	}
}

func toolCall(id, name, args string) llm.Message {
	return llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			ID: id, Type: "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		}},
	}
}

func hrScope(t *testing.T) Scope {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hr.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE employee (id INTEGER PRIMARY KEY, name TEXT, dept_id TEXT, salary INTEGER);
		INSERT INTO employee (name, dept_id, salary) VALUES ('Ada', 'D3', 10), ('Bea', 'D9', 99);`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	return Scope{
		Persona:      store.Persona{Name: "人事", SystemPrompt: "只回答本部门", Status: 1},
		FilePath:     path,
		Tables:       []query.Table{{Name: "employee", Columns: []string{"id", "name", "dept_id", "salary"}}},
		Sensitive:    map[string][]string{"employee": {"salary"}},
		RowFilterSQL: `"dept_id" = 'D3'`,
		MaxRows:      50,
		MaxLoop:      6,
	}
}
