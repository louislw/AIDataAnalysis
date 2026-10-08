package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestExecuteRowFilterSensitiveAndWrites(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "hr.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE employee (
		id INTEGER PRIMARY KEY,
		name TEXT,
		dept_id TEXT,
		salary INTEGER
	);
	INSERT INTO employee (name, dept_id, salary) VALUES
		('Ada', 'D3', 10),
		('Bea', 'D9', 99);`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	req := Request{
		FilePath: path,
		Tables: []Table{{
			Name:    "employee",
			Columns: []string{"id", "name", "dept_id", "salary"},
		}},
		Sensitive: map[string][]string{"employee": {"salary"}},
		SQL:       "SELECT name FROM employee ORDER BY name",
		MaxRows:   50,
	}
	got, err := Execute(ctx, req, `"dept_id" = 'D3'`)
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 || got.Rows[0][0] != "Ada" {
		t.Fatalf("rows = %#v", got.Rows)
	}
	if !strings.Contains(got.Text, "这是前 50 行明细，不是全量") {
		t.Fatalf("tool text = %s", got.Text)
	}

	if _, err := Execute(ctx, Request{
		FilePath:  path,
		Tables:    req.Tables,
		Sensitive: req.Sensitive,
		SQL:       "SELECT salary FROM employee",
		MaxRows:   50,
	}, `"dept_id" = 'D3'`); err == nil {
		t.Fatal("expected sensitive column to be rejected")
	}

	sum, err := Execute(ctx, Request{
		FilePath:  path,
		Tables:    req.Tables,
		Sensitive: req.Sensitive,
		SQL:       "SELECT SUM(salary) FROM employee",
		MaxRows:   50,
	}, `"dept_id" = 'D3'`)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Rows[0][0] != "10" {
		t.Fatalf("sum = %#v", sum.Rows)
	}

	if _, err := Execute(ctx, Request{
		FilePath:  path,
		Tables:    req.Tables,
		Sensitive: req.Sensitive,
		SQL:       "INSERT INTO employee (name, dept_id, salary) VALUES ('X','D3',1)",
		MaxRows:   50,
	}, ""); err == nil {
		t.Fatal("expected insert to be rejected")
	}
	if _, err := Execute(ctx, Request{
		FilePath:  path,
		Tables:    req.Tables,
		Sensitive: req.Sensitive,
		SQL:       "SELECT name FROM src.employee",
		MaxRows:   50,
	}, ""); err == nil {
		t.Fatal("expected base table read to be rejected")
	}
}
