package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/louislw/AIDataAnalysis/internal/llm"
	"github.com/louislw/AIDataAnalysis/internal/query"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

// Turn is one earlier round reduced to a question, the SQL, and a one-line summary.
type Turn struct {
	Question string
	SQL      string
	Summary  string
}

// Scope is the persona's query boundary for one request.
type Scope struct {
	User         store.User
	Persona      store.Persona
	FilePath     string
	Tables       []query.Table
	Sensitive    map[string][]string
	ColumnNotes  map[string]map[string]string
	TableNotes   map[string]string
	RowFilterSQL string
	MaxRows      int
	MaxLoop      int
}

// Answer is what gets stored on the assistant message.
type Answer struct {
	Kind      string
	Content   string
	SQL       string
	Tables    []string
	RowCount  *int
	Truncated bool
	Reasoning string
}

// Run executes the tool loop. A missing successful query never becomes an answer.
func Run(ctx context.Context, client llm.Client, scope Scope, question string, history []Turn) (Answer, error) {
	if scope.MaxLoop <= 0 {
		scope.MaxLoop = 6
	}
	messages := []llm.Message{{Role: "system", Content: systemPrompt(scope)}}
	for _, turn := range history {
		messages = append(messages,
			llm.Message{Role: "user", Content: turn.Question},
			llm.Message{Role: "assistant", Content: historySummary(turn)},
		)
	}
	messages = append(messages, llm.Message{Role: "user", Content: question})

	var (
		successSQL  []string
		successTabs []string
		rowCount    int
		truncated   bool
		sawSuccess  bool
		reasoning   []string
	)
	tools := toolDefs()
	for i := 0; i < scope.MaxLoop; i++ {
		msg, err := client.Chat(ctx, messages, tools)
		if err != nil {
			return Answer{Kind: "error", Content: err.Error(), SQL: ""}, nil
		}
		if len(msg.ToolCalls) == 0 {
			content := strings.TrimSpace(msg.Content)
			if content == "" {
				content = "模型没有给出内容"
			}
			return finish(content, sawSuccess, successSQL, successTabs, rowCount, truncated, reasoning), nil
		}
		messages = append(messages, msg)
		for _, call := range msg.ToolCalls {
			outcome := runTool(ctx, scope, call)
			reasoning = append(reasoning, outcome.note)
			if outcome.result != nil {
				sawSuccess = true
				successSQL = append(successSQL, outcome.result.SQL)
				successTabs = append(successTabs, outcome.result.Tables...)
				rowCount = outcome.result.RowCount
				if outcome.result.Truncated {
					truncated = true
				}
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    outcome.text,
			})
		}
	}
	return Answer{
		Kind:      "error",
		Content:   "已达到最大推理轮次，未能形成有依据的回答",
		SQL:       "",
		Reasoning: strings.Join(reasoning, "\n"),
	}, nil
}

func finish(content string, sawSuccess bool, sqls, tables []string, rowCount int, truncated bool, reasoning []string) Answer {
	ans := Answer{
		Content:   content,
		Reasoning: strings.Join(reasoning, "\n"),
		Tables:    unique(tables),
	}
	if sawSuccess {
		ans.Kind = "answer"
		ans.SQL = strings.Join(sqls, "\n")
		n := rowCount
		ans.RowCount = &n
		ans.Truncated = truncated
		return ans
	}
	ans.SQL = ""
	if isClarification(content) {
		ans.Kind = "clarify"
		return ans
	}
	ans.Kind = "error"
	if !strings.Contains(content, "没有") && !strings.Contains(content, "失败") {
		ans.Content = "没有成功的查询，不能给出分析结论。模型回复：" + content
	}
	return ans
}

func isClarification(content string) bool {
	if strings.Contains(content, "\n|") || strings.Contains(content, "| ---") {
		return false
	}
	return strings.Contains(content, "？") || strings.Contains(content, "?")
}

type toolOutcome struct {
	text   string
	note   string
	result *query.Result
}

func runTool(ctx context.Context, scope Scope, call llm.ToolCall) toolOutcome {
	rawArgs := strings.TrimSpace(call.Function.Arguments)
	if rawArgs == "" {
		rawArgs = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolOutcome{text: "工具参数不是合法 JSON", note: "工具参数无效: " + call.Function.Name}
	}
	switch call.Function.Name {
	case "listTables":
		names := make([]string, 0, len(scope.Tables))
		for _, table := range scope.Tables {
			names = append(names, table.Name)
		}
		if len(names) == 0 {
			return toolOutcome{text: "未授权任何表，不能查询。", note: "listTables"}
		}
		return toolOutcome{text: strings.Join(names, "\n"), note: "listTables"}
	case "getTableSchema":
		name, _ := args["table"].(string)
		for _, table := range scope.Tables {
			if !strings.EqualFold(table.Name, name) {
				continue
			}
			sens := map[string]struct{}{}
			for _, col := range scope.Sensitive[strings.ToLower(table.Name)] {
				sens[strings.ToLower(col)] = struct{}{}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "表 %s\n", table.Name)
			if note := scope.TableNotes[table.Name]; note != "" {
				b.WriteString(note)
				b.WriteString("\n")
			}
			for _, col := range table.Columns {
				if _, hide := sens[strings.ToLower(col)]; hide {
					continue
				}
				b.WriteString("- ")
				b.WriteString(col)
				if notes := scope.ColumnNotes[table.Name]; notes != nil && notes[col] != "" {
					b.WriteString("：")
					b.WriteString(notes[col])
				}
				b.WriteString("\n")
			}
			return toolOutcome{text: b.String(), note: "getTableSchema " + table.Name}
		}
		return toolOutcome{text: "表未授权", note: "getTableSchema 未授权"}
	case "executeSql":
		sqlText, _ := args["sql"].(string)
		result, err := query.Execute(ctx, query.Request{
			FilePath:  scope.FilePath,
			Tables:    scope.Tables,
			Sensitive: scope.Sensitive,
			SQL:       sqlText,
			MaxRows:   scope.MaxRows,
		}, scope.RowFilterSQL)
		if err != nil {
			return toolOutcome{text: "SQL 执行失败: " + err.Error(), note: "executeSql 失败: " + one(sqlText)}
		}
		note := fmt.Sprintf("executeSql 成功 rows=%d", result.RowCount)
		return toolOutcome{text: result.Text, note: note, result: &result}
	default:
		return toolOutcome{text: "未知工具", note: "未知工具 " + call.Function.Name}
	}
}

func systemPrompt(scope Scope) string {
	var b strings.Builder
	b.WriteString("你是数据分析助手。只能根据工具查询到的结果用中文回答。\n")
	b.WriteString("口径有歧义时，只提一个问题，不要给出数字结论，问题以问号结尾。\n")
	b.WriteString("需要数字、名单、排名时必须调用 executeSql。明细查询会被限制行数；合计、占比、排名必须写聚合 SQL。\n")
	b.WriteString("没有成功查询时，禁止编造分析。SQL 报错时根据报错改写后再查。\n")
	if strings.TrimSpace(scope.Persona.SystemPrompt) != "" {
		b.WriteString("\n人物设定：\n")
		b.WriteString(strings.TrimSpace(scope.Persona.SystemPrompt))
		b.WriteString("\n")
	}
	if strings.TrimSpace(scope.Persona.CatalogText) != "" {
		b.WriteString("\n")
		b.WriteString(scope.Persona.CatalogText)
	}
	return b.String()
}

func historySummary(turn Turn) string {
	var b strings.Builder
	if turn.SQL != "" {
		b.WriteString("SQL: ")
		b.WriteString(one(turn.SQL))
		b.WriteString("\n")
	}
	b.WriteString("摘要: ")
	b.WriteString(one(turn.Summary))
	return b.String()
}

func one(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 180 {
		return string([]rune(s)[:180]) + "…"
	}
	return s
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func toolDefs() []llm.Tool {
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	return []llm.Tool{
		{Type: "function", Function: llm.Function{
			Name:        "executeSql",
			Description: "对当前人物授权的表执行一条 SELECT。表名使用目录中的名字。",
			Parameters:  obj(map[string]any{"sql": map[string]any{"type": "string"}}, "sql"),
		}},
		{Type: "function", Function: llm.Function{
			Name:        "listTables",
			Description: "列出当前可以查询的表。",
			Parameters:  obj(map[string]any{}),
		}},
		{Type: "function", Function: llm.Function{
			Name:        "getTableSchema",
			Description: "查看一张授权表的字段。敏感字段不会出现。",
			Parameters:  obj(map[string]any{"table": map[string]any{"type": "string"}}, "table"),
		}},
	}
}
