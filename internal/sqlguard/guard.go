package sqlguard

import (
	"fmt"
	"sort"
	"strings"
)

// Result is a single SELECT that is safe to run against the persona's views.
type Result struct {
	SQL       string
	Tables    []string
	Detail    bool
	Aggregate bool
}

// Check parses one SELECT, rejects unauthorized tables and misuse of sensitive
// columns, and adds a LIMIT to detail queries.
func Check(sql string, allowed []string, sensitive map[string][]string, maxRows int) (Result, error) {
	if maxRows <= 0 {
		maxRows = 50
	}
	cleaned, err := stripComments(sql)
	if err != nil {
		return Result{}, err
	}
	toks, err := tokenize(cleaned)
	if err != nil {
		return Result{}, err
	}
	allow := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allow[fold(name)] = struct{}{}
	}
	sens := make(map[string]map[string]struct{}, len(sensitive))
	for table, cols := range sensitive {
		set := make(map[string]struct{}, len(cols))
		for _, col := range cols {
			set[fold(col)] = struct{}{}
		}
		sens[fold(table)] = set
	}
	p := &parser{
		toks:      toks,
		allowed:   allow,
		sensitive: sens,
		ctes:      map[string]struct{}{},
		seen:      map[string]struct{}{},
		maxRows:   maxRows,
	}
	p.parseQuery()
	if p.err != nil {
		return Result{}, p.err
	}
	if len(p.tables) == 0 {
		return Result{}, fmt.Errorf("查询必须读取授权表")
	}
	final := applyEdits(cleaned, p.edits)
	return Result{
		SQL:       strings.TrimSpace(final),
		Tables:    p.tables,
		Detail:    p.detail,
		Aggregate: !p.detail,
	}, nil
}

func applyEdits(sql string, edits []edit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].pos > edits[j].pos })
	for _, e := range edits {
		if e.pos < 0 || e.end > len(sql) || e.pos > e.end {
			continue
		}
		sql = sql[:e.pos] + e.text + sql[e.end:]
	}
	return sql
}
