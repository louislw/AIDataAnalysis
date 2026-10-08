package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var fileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Opener resolves business SQLite files under DATA_ROOT/sources and keeps a
// read-only pool per file.
type Opener struct {
	root  string
	mu    sync.Mutex
	pools map[string]*sql.DB
}

func New(dataRoot string) *Opener {
	return &Opener{root: dataRoot, pools: map[string]*sql.DB{}}
}

func (o *Opener) Resolve(fileName string) (string, error) {
	if !fileNameRE.MatchString(fileName) || strings.Contains(fileName, "..") {
		return "", fmt.Errorf("文件名无效")
	}
	sources := filepath.Join(o.root, "sources")
	full := filepath.Join(sources, fileName)
	info, err := os.Lstat(full)
	if err != nil {
		return "", fmt.Errorf("找不到业务库文件")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("拒绝符号链接")
	}
	realSources, err := filepath.EvalSymlinks(sources)
	if err != nil {
		return "", err
	}
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realSources, realFull)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("业务库路径越界")
	}
	return realFull, nil
}

func (o *Opener) Open(fileName string) (*sql.DB, string, error) {
	path, err := o.Resolve(fileName)
	if err != nil {
		return nil, "", err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if db, ok := o.pools[path]; ok {
		return db, path, nil
	}
	dsn := "file:" + path + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(4)
	o.pools[path] = db
	return db, path, nil
}

func (o *Opener) TableCount(ctx context.Context, fileName string) (int, error) {
	db, _, err := o.Open(fileName)
	if err != nil {
		return 0, err
	}
	var n int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&n)
	return n, err
}

type PhysicalTable struct {
	Name    string
	Columns []PhysicalColumn
}

type PhysicalColumn struct {
	Name string
	Type string
}

func (o *Opener) Tables(ctx context.Context, fileName string) ([]string, error) {
	db, _, err := o.Open(fileName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (o *Opener) Columns(ctx context.Context, fileName, table string) ([]PhysicalColumn, error) {
	if !safeIdent(table) {
		return nil, fmt.Errorf("表名无效")
	}
	db, _, err := o.Open(fileName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quote(table)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PhysicalColumn
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		out = append(out, PhysicalColumn{Name: name, Type: typ})
	}
	return out, rows.Err()
}

func (o *Opener) Preview(ctx context.Context, fileName, table string, sensitive map[string]bool, limit int) ([]string, [][]string, error) {
	if !safeIdent(table) {
		return nil, nil, fmt.Errorf("表名无效")
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	db, _, err := o.Open(fileName)
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT * FROM %s LIMIT %d`, quote(table), limit))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]string
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		line := make([]string, len(cols))
		for i, col := range cols {
			if sensitive[strings.ToLower(col)] {
				line[i] = "***"
				continue
			}
			switch v := raw[i].(type) {
			case nil:
				line[i] = ""
			case []byte:
				line[i] = string(v)
			default:
				line[i] = fmt.Sprint(v)
			}
		}
		out = append(out, line)
	}
	return cols, out, rows.Err()
}

func safeIdent(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		if r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		if r > 127 {
			continue
		}
		return false
	}
	return true
}

func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
