package query

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAuthorizerAllowsViewAndBlocksBaseAndWrites(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "hr.sqlite")
	src, err := sql.Open("sqlite", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(`CREATE TABLE employee (id INTEGER PRIMARY KEY, name TEXT, dept_id TEXT, salary INTEGER);
		INSERT INTO employee (name, dept_id, salary) VALUES ('Ada', 'D3', 10), ('Bea', 'D9', 20)`); err != nil {
		t.Fatal(err)
	}
	src.Close()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	attach := "ATTACH '" + escapeSQL(fileURI(srcPath)) + "' AS src"
	if _, err := conn.ExecContext(ctx, attach); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TEMP VIEW employee AS SELECT id, name, dept_id, salary FROM src.employee WHERE dept_id = 'D3'`); err != nil {
		t.Fatal(err)
	}
	if err := installAuthorizer(conn, []string{"employee"}); err != nil {
		t.Fatal(err)
	}
	defer clearAuthorizer(conn)

	var name string
	if err := conn.QueryRowContext(ctx, `SELECT name FROM employee`).Scan(&name); err != nil {
		t.Fatalf("view query: %v", err)
	}
	if name != "Ada" {
		t.Fatalf("name = %s", name)
	}

	if _, err := conn.ExecContext(ctx, `SELECT name FROM src.employee`); err == nil {
		t.Fatal("expected direct base-table read to be denied")
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO employee (name, dept_id, salary) VALUES ('X','D3',1)`); err == nil {
		t.Fatal("expected insert to be denied")
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA database_list`); err == nil {
		t.Fatal("expected pragma to be denied")
	}
}

func escapeSQL(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

func fileURI(path string) string {
	// Tests and the executor both use a SQLite URI so mode=ro can be set later.
	return "file:" + path + "?mode=ro"
}
