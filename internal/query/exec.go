package query

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/louislw/AIDataAnalysis/internal/sqlguard"
	_ "modernc.org/sqlite"
)

const queryTimeout = 30 * time.Second
const cellLimit = 80
const aggregateCap = 5000

// Table is one authorized business table and its physical columns.
type Table struct {
	Name    string
	Columns []string
}

// Request runs one user SQL statement against a read-only business file.
type Request struct {
	FilePath  string
	Tables    []Table
	Sensitive map[string][]string
	RowFilter string
	SQL       string
	MaxRows   int
}

// Result is the evidence returned to the agent and stored on the message.
type Result struct {
	SQL       string
	Tables    []string
	Columns   []string
	Rows      [][]string
	RowCount  int
	Truncated bool
	Detail    bool
	Markdown  string
	Text      string
}

// Execute checks SQL, installs temp views, and runs the statement.
// rowFilterSQL is a compiled boolean expression, or empty.
func Execute(ctx context.Context, req Request, rowFilterSQL string) (Result, error) {
	allowed := make([]string, 0, len(req.Tables))
	for _, table := range req.Tables {
		allowed = append(allowed, table.Name)
	}
	checked, err := sqlguard.Check(req.SQL, allowed, req.Sensitive, req.MaxRows)
	if err != nil {
		return Result{}, err
	}
	return run(ctx, req, checked, rowFilterSQL)
}

func run(ctx context.Context, req Request, checked sqlguard.Result, rowFilterSQL ...string) (Result, error) {
	filter := ""
	if len(rowFilterSQL) > 0 {
		filter = rowFilterSQL[0]
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		clearAuthorizer(conn)
		conn.Close()
	}()

	uri := fileReadOnlyURI(req.FilePath)
	if _, err := conn.ExecContext(ctx, "ATTACH '"+escapeLiteral(uri)+"' AS "+attachedDB); err != nil {
		return Result{}, fmt.Errorf("打开业务库失败: %w", err)
	}
	byName := map[string]Table{}
	viewNames := make([]string, 0, len(req.Tables))
	for _, table := range req.Tables {
		byName[strings.ToLower(table.Name)] = table
		viewNames = append(viewNames, table.Name)
		if err := createView(ctx, conn, table, filter); err != nil {
			return Result{}, err
		}
	}
	if err := installAuthorizer(conn, viewNames); err != nil {
		return Result{}, err
	}

	rows, err := conn.QueryContext(ctx, checked.SQL)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	capRows := aggregateCap
	if checked.Detail {
		capRows = req.MaxRows
		if capRows <= 0 {
			capRows = 50
		}
	}
	var out [][]string
	truncated := false
	for rows.Next() {
		if len(out) >= capRows {
			truncated = true
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		line := make([]string, len(cols))
		for i, v := range raw {
			line[i] = formatCell(v)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	if checked.Detail && len(out) >= capRows {
		truncated = true
	}
	md := markdownTable(cols, out)
	text := formatToolText(checked.Detail, len(out), truncated, capRows, md)
	_ = byName
	return Result{
		SQL:       checked.SQL,
		Tables:    checked.Tables,
		Columns:   cols,
		Rows:      out,
		RowCount:  len(out),
		Truncated: truncated,
		Detail:    checked.Detail,
		Markdown:  md,
		Text:      text,
	}, nil
}

func createView(ctx context.Context, conn *sql.Conn, table Table, filter string) error {
	if len(table.Columns) == 0 {
		return fmt.Errorf("表 %s 没有列", table.Name)
	}
	cols := make([]string, len(table.Columns))
	for i, col := range table.Columns {
		cols[i] = quoteIdent(col)
	}
	stmt := "CREATE TEMP VIEW " + quoteIdent(table.Name) + " AS SELECT " + strings.Join(cols, ", ") +
		" FROM " + attachedDB + "." + quoteIdent(table.Name)
	if strings.TrimSpace(filter) != "" {
		stmt += " WHERE " + filter
	}
	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("建立表 %s 的查询视图失败: %w", table.Name, err)
	}
	return nil
}

func formatToolText(detail bool, count int, truncated bool, limit int, md string) string {
	trunc := "否"
	if truncated {
		trunc = "是"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "行数: %d\n截断: %s\n", count, trunc)
	if detail {
		fmt.Fprintf(&b, "这是前 %d 行明细，不是全量\n", limit)
	}
	b.WriteString(md)
	return b.String()
}

func markdownTable(cols []string, rows [][]string) string {
	if len(cols) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("|")
	for _, col := range cols {
		b.WriteString(" ")
		b.WriteString(escapeCell(col))
		b.WriteString(" |")
	}
	b.WriteString("\n|")
	for range cols {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows {
		b.WriteString("|")
		for _, cell := range row {
			b.WriteString(" ")
			b.WriteString(escapeCell(cell))
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if utf8.RuneCountInString(s) > cellLimit {
		runes := []rune(s)
		s = string(runes[:cellLimit]) + "…"
	}
	return s
}

func formatCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func escapeLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func fileReadOnlyURI(path string) string {
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	return u.String()
}
