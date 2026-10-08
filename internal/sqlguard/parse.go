package sqlguard

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

type colUse struct {
	table     string
	col       string
	aggregate bool
}

type frame struct {
	alias     map[string]string
	physical  []string
	uses      []colUse
	stars     []string
	hasFrom   bool
	grouped   bool
	hasAgg    bool
	hasDetail bool
}

type edit struct {
	pos  int
	end  int
	text string
}

type parser struct {
	toks      []token
	i         int
	allowed   map[string]struct{}
	sensitive map[string]map[string]struct{}
	ctes      map[string]struct{}
	tables    []string
	seen      map[string]struct{}
	edits     []edit
	maxRows   int
	active    *frame
	shared    *frame
	aggCount  int
	detail    bool
	err       error
}

var aggregateFuncs = map[string]struct{}{
	"sum": {}, "count": {}, "avg": {}, "min": {}, "max": {},
}

var bannedFuncs = map[string]struct{}{
	"load_extension": {}, "writefile": {}, "readfile": {}, "fts3_tokenizer": {}, "edit": {},
}

func (p *parser) fail(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf(format, args...)
	}
}

func (p *parser) peek() token {
	if p.i >= len(p.toks) {
		return token{}
	}
	return p.toks[p.i]
}

func (p *parser) next() token {
	t := p.peek()
	if p.i < len(p.toks) {
		p.i++
	}
	return t
}

func (p *parser) isKW(lit string, words ...string) bool {
	for _, w := range words {
		if strings.EqualFold(lit, w) {
			return true
		}
	}
	return false
}

func (p *parser) acceptKW(words ...string) bool {
	if p.err != nil {
		return false
	}
	if p.isKW(p.peek().lit, words...) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expectKW(word string) bool {
	if p.acceptKW(word) {
		return true
	}
	p.fail("期望 %s", word)
	return false
}

func (p *parser) acceptLit(lit string) bool {
	if p.peek().lit == lit {
		p.next()
		return true
	}
	return false
}

func (p *parser) parseQuery() {
	if p.acceptKW("WITH") {
		p.acceptKW("RECURSIVE")
		for {
			name := p.expectIdent()
			if name == "" || p.err != nil {
				return
			}
			key := fold(name)
			if _, ok := p.allowed[key]; ok {
				p.fail("CTE 不能覆盖授权表 %s", name)
				return
			}
			p.ctes[key] = struct{}{}
			if !p.expectKW("AS") || p.peek().lit != "(" {
				p.fail("CTE 需要子查询")
				return
			}
			p.parseSubquery()
			if p.err != nil || !p.acceptLit(",") {
				break
			}
		}
	}
	if p.err != nil {
		return
	}
	p.detail = p.parseCompound(true)
	if p.err != nil {
		return
	}
	if p.peek().lit != "" {
		p.fail("无法解析的 SQL（%s）", p.peek().lit)
	}
}

func (p *parser) parseCompound(applyLimit bool) bool {
	need := p.parseSelect()
	if p.err != nil {
		return need
	}
	for p.acceptKW("UNION", "INTERSECT", "EXCEPT") {
		p.acceptKW("ALL")
		if p.parseSelect() {
			need = true
		}
		if p.err != nil {
			return need
		}
	}
	if p.acceptKW("ORDER") {
		if !p.expectKW("BY") {
			return need
		}
		prev := p.active
		if p.shared == nil {
			p.shared = &frame{alias: map[string]string{}}
		}
		p.active = p.shared
		for {
			p.parseExpr(false)
			p.acceptKW("ASC")
			p.acceptKW("DESC")
			if p.err != nil || !p.acceptLit(",") {
				break
			}
		}
		p.resolveFrame(p.shared)
		p.active = prev
	}
	if p.err != nil {
		return need
	}
	p.applyLimit(applyLimit && need)
	return need
}

func (p *parser) applyLimit(need bool) {
	if p.err != nil {
		return
	}
	if !p.acceptKW("LIMIT") {
		if need {
			at := p.peek().pos
			p.edits = append(p.edits, edit{pos: at, end: at, text: fmt.Sprintf(" LIMIT %d", p.maxRows)})
		}
		return
	}
	num := p.next()
	if !isNumber(num.lit) {
		p.fail("LIMIT 需要非负整数")
		return
	}
	n, err := strconv.Atoi(num.lit)
	if err != nil || n < 0 {
		p.fail("LIMIT 需要非负整数")
		return
	}
	limitPos, limitEnd := num.pos, num.pos+len(num.lit)
	if p.acceptLit(",") {
		count := p.next()
		if !isNumber(count.lit) {
			p.fail("LIMIT 需要非负整数")
			return
		}
		c, convErr := strconv.Atoi(count.lit)
		if convErr != nil || c < 0 {
			p.fail("LIMIT 需要非负整数")
			return
		}
		n = c
		limitPos, limitEnd = count.pos, count.pos+len(count.lit)
	}
	if p.acceptKW("OFFSET") {
		off := p.next()
		if !isNumber(off.lit) {
			p.fail("OFFSET 需要非负整数")
		}
	}
	if need && n > p.maxRows {
		p.edits = append(p.edits, edit{pos: limitPos, end: limitEnd, text: strconv.Itoa(p.maxRows)})
	}
}

func (p *parser) parseSelect() bool {
	if !p.expectKW("SELECT") {
		return true
	}
	p.acceptKW("DISTINCT")
	p.acceptKW("ALL")
	fr := &frame{alias: map[string]string{}}
	prev := p.active
	p.active = fr
	p.parseSelectList(fr)
	if p.err != nil {
		p.active = prev
		return true
	}
	if p.acceptKW("FROM") {
		fr.hasFrom = true
		p.parseFrom(fr)
	}
	if p.err != nil {
		p.active = prev
		return true
	}
	if p.acceptKW("WHERE") {
		p.parseExpr(false)
	}
	if p.acceptKW("GROUP") {
		if p.expectKW("BY") {
			fr.grouped = true
			p.parseExprList(false)
		}
	}
	if p.acceptKW("HAVING") {
		p.parseExpr(false)
	}
	if p.err != nil {
		p.active = prev
		return true
	}
	p.resolveFrame(fr)
	p.merge(fr)
	p.active = prev
	if fr.grouped || (fr.hasAgg && !fr.hasDetail) {
		return false
	}
	return fr.hasDetail || fr.hasFrom
}

func (p *parser) merge(fr *frame) {
	if p.shared == nil {
		p.shared = &frame{alias: map[string]string{}}
	}
	for k, v := range fr.alias {
		p.shared.alias[k] = v
	}
	p.shared.physical = append(p.shared.physical, fr.physical...)
}

func (p *parser) parseSelectList(fr *frame) {
	for {
		if p.peek().lit == "*" {
			p.next()
			fr.stars = append(fr.stars, "")
			fr.hasDetail = true
		} else if p.looksQualifiedStar() {
			table := p.next().lit
			p.next() // dot
			p.next() // star
			fr.stars = append(fr.stars, table)
			fr.hasDetail = true
		} else {
			beforeUse := len(fr.uses)
			beforeAgg := p.aggCount
			p.parseExpr(false)
			if p.aggCount > beforeAgg {
				fr.hasAgg = true
			}
			for _, use := range fr.uses[beforeUse:] {
				if !use.aggregate {
					fr.hasDetail = true
				}
			}
			if p.acceptKW("AS") {
				p.expectIdent()
			} else if isBareIdent(p.peek().lit) && !p.startsClause(p.peek().lit) {
				p.next()
			}
		}
		if p.err != nil || !p.acceptLit(",") {
			return
		}
	}
}

func (p *parser) looksQualifiedStar() bool {
	if p.i+2 >= len(p.toks) {
		return false
	}
	return isBareIdent(p.toks[p.i].lit) && p.toks[p.i+1].lit == "." && p.toks[p.i+2].lit == "*"
}

func (p *parser) startsClause(lit string) bool {
	return p.isKW(lit, "FROM", "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", "UNION", "INTERSECT", "EXCEPT", "WINDOW", "JOIN", "INNER", "LEFT", "CROSS", "ON")
}

func (p *parser) parseFrom(fr *frame) {
	p.parseTableRef(fr)
	for p.err == nil {
		comma := p.acceptLit(",")
		join := false
		if !comma {
			if p.acceptKW("CROSS", "INNER", "LEFT", "RIGHT", "FULL") {
				p.acceptKW("OUTER")
				if !p.expectKW("JOIN") {
					return
				}
				join = true
			} else if p.acceptKW("JOIN") {
				join = true
			}
		}
		if !comma && !join {
			return
		}
		p.parseTableRef(fr)
		if join && p.acceptKW("ON") {
			p.parseExpr(false)
		}
		if join && p.acceptKW("USING") {
			if p.peek().lit != "(" {
				p.fail("USING 语法错误")
				return
			}
			p.next()
			for {
				p.expectIdent()
				if !p.acceptLit(",") {
					break
				}
			}
			if p.peek().lit != ")" {
				p.fail("USING 没有结束")
				return
			}
			p.next()
		}
	}
}

func (p *parser) parseTableRef(fr *frame) {
	if p.peek().lit == "(" {
		p.parseSubquery()
		p.parseAlias(fr, "")
		return
	}
	name := p.expectIdent()
	if name == "" || p.err != nil {
		p.fail("表名无效")
		return
	}
	table := name
	schema := ""
	if p.acceptLit(".") {
		schema = table
		table = p.expectIdent()
		if table == "" {
			p.fail("表名无效")
			return
		}
	}
	if schema != "" && !strings.EqualFold(schema, "temp") && !strings.EqualFold(schema, "main") {
		p.fail("不能直接访问数据库 %s", schema)
		return
	}
	key := fold(table)
	physical := ""
	if _, ok := p.ctes[key]; !ok {
		if _, ok := p.allowed[key]; !ok {
			p.fail("表 %s 未授权", table)
			return
		}
		physical = key
		if _, ok := p.seen[key]; !ok {
			p.seen[key] = struct{}{}
			p.tables = append(p.tables, table)
		}
		fr.physical = append(fr.physical, key)
	}
	alias := p.parseAlias(fr, physical)
	if alias == "" {
		fr.alias[key] = physical
	}
}

func (p *parser) parseAlias(fr *frame, physical string) string {
	if p.acceptKW("AS") {
		id := p.expectIdent()
		if id != "" {
			fr.alias[fold(id)] = physical
		}
		return id
	}
	lit := p.peek().lit
	if isBareIdent(lit) && !p.startsClause(lit) && !p.isKW(lit, "WHERE", "GROUP", "ORDER", "LIMIT", "UNION", "INTERSECT", "EXCEPT") {
		p.next()
		fr.alias[fold(lit)] = physical
		return lit
	}
	return ""
}

func (p *parser) parseSubquery() {
	if p.peek().lit != "(" {
		p.fail("期望子查询")
		return
	}
	p.next()
	p.parseCompound(true)
	if p.peek().lit != ")" {
		p.fail("子查询没有结束")
		return
	}
	p.next()
}

func (p *parser) resolveFrame(fr *frame) {
	if fr == nil || p.err != nil {
		return
	}
	for _, starTable := range fr.stars {
		tables := fr.physical
		if starTable != "" {
			phys, ok := fr.alias[fold(starTable)]
			if !ok || phys == "" {
				p.fail("未知的表 %s", starTable)
				return
			}
			tables = []string{phys}
		}
		for _, table := range tables {
			if len(p.sensitive[table]) > 0 {
				p.fail("结果包含敏感列，不能使用 *")
				return
			}
		}
	}
	for _, use := range fr.uses {
		p.checkColumn(fr, use)
		if p.err != nil {
			return
		}
	}
}

func (p *parser) checkColumn(fr *frame, use colUse) {
	col := fold(use.col)
	var tables []string
	if use.table != "" {
		phys, ok := fr.alias[fold(use.table)]
		if !ok {
			if _, cte := p.ctes[fold(use.table)]; cte {
				return
			}
			p.fail("未知的表 %s", use.table)
			return
		}
		if phys == "" {
			return
		}
		tables = []string{phys}
	} else {
		tables = fr.physical
	}
	if use.aggregate {
		return
	}
	for _, table := range tables {
		if _, ok := p.sensitive[table][col]; ok {
			p.fail("敏感列 %s 只能作为 SUM、COUNT、AVG、MIN、MAX 的参数", use.col)
			return
		}
	}
}

func (p *parser) parseExprList(aggregate bool) {
	for {
		p.parseExpr(aggregate)
		if p.err != nil || !p.acceptLit(",") {
			return
		}
	}
}

func (p *parser) parseExpr(aggregate bool) {
	p.parseOr(aggregate)
}

func (p *parser) parseOr(aggregate bool) {
	p.parseAnd(aggregate)
	for p.acceptKW("OR") {
		p.parseAnd(aggregate)
	}
}

func (p *parser) parseAnd(aggregate bool) {
	p.parseNot(aggregate)
	for p.acceptKW("AND") {
		p.parseNot(aggregate)
	}
}

func (p *parser) parseNot(aggregate bool) {
	if p.acceptKW("NOT") {
		p.parseNot(aggregate)
		return
	}
	p.parseCmp(aggregate)
}

func (p *parser) parseCmp(aggregate bool) {
	if p.acceptKW("EXISTS") {
		p.parseSubquery()
		return
	}
	p.parseAdd(aggregate)
	if p.err != nil {
		return
	}
	if p.acceptKW("IS") {
		p.acceptKW("NOT")
		if p.acceptKW("NULL") {
			return
		}
		p.parseAdd(aggregate)
		return
	}
	not := p.acceptKW("NOT")
	switch {
	case p.acceptKW("IN"):
		p.parseIn(aggregate)
	case p.acceptKW("LIKE"):
		p.parseAdd(aggregate)
		if p.acceptKW("ESCAPE") {
			p.parseAdd(aggregate)
		}
	case p.acceptKW("BETWEEN"):
		p.parseAdd(aggregate)
		if p.expectKW("AND") {
			p.parseAdd(aggregate)
		}
	case not:
		p.fail("不支持的 NOT 表达式")
	case isCmpOp(p.peek().lit):
		p.next()
		p.parseAdd(aggregate)
	}
}

func (p *parser) parseIn(aggregate bool) {
	if p.peek().lit != "(" {
		p.fail("IN 语法错误")
		return
	}
	p.next()
	if p.isKW(p.peek().lit, "SELECT", "WITH") {
		p.parseCompound(true)
	} else {
		p.parseExprList(aggregate)
	}
	if p.peek().lit != ")" {
		p.fail("IN 没有结束")
		return
	}
	p.next()
}

func (p *parser) parseAdd(aggregate bool) {
	p.parseMul(aggregate)
	for p.peek().lit == "+" || p.peek().lit == "-" || p.peek().lit == "||" {
		p.next()
		p.parseMul(aggregate)
	}
}

func (p *parser) parseMul(aggregate bool) {
	p.parseUnary(aggregate)
	for p.peek().lit == "*" || p.peek().lit == "/" || p.peek().lit == "%" {
		p.next()
		p.parseUnary(aggregate)
	}
}

func (p *parser) parseUnary(aggregate bool) {
	if p.peek().lit == "+" || p.peek().lit == "-" || p.isKW(p.peek().lit, "NOT") {
		p.next()
		p.parseUnary(aggregate)
		return
	}
	p.parsePrimary(aggregate)
}

func (p *parser) parsePrimary(aggregate bool) {
	if p.err != nil {
		return
	}
	t := p.peek()
	switch {
	case t.lit == "(":
		p.next()
		if p.isKW(p.peek().lit, "SELECT", "WITH") {
			p.parseCompound(true)
		} else {
			p.parseExpr(aggregate)
		}
		if p.peek().lit != ")" {
			p.fail("括号没有结束")
			return
		}
		p.next()
	case t.lit == "*":
		p.fail("此处不能使用 *")
	case isNumber(t.lit) || strings.HasPrefix(t.lit, "'"):
		p.next()
	case p.isKW(t.lit, "NULL"):
		p.next()
	case p.isKW(t.lit, "CASE"):
		p.next()
		if !p.isKW(p.peek().lit, "WHEN") {
			p.parseExpr(aggregate)
		}
		for p.acceptKW("WHEN") {
			p.parseExpr(aggregate)
			if !p.expectKW("THEN") {
				return
			}
			p.parseExpr(aggregate)
		}
		if p.acceptKW("ELSE") {
			p.parseExpr(aggregate)
		}
		p.expectKW("END")
	case p.isKW(t.lit, "CAST"):
		p.next()
		if p.peek().lit != "(" {
			p.fail("CAST 语法错误")
			return
		}
		p.next()
		p.parseExpr(aggregate)
		if !p.expectKW("AS") {
			return
		}
		p.expectIdent()
		if p.peek().lit != ")" {
			p.fail("CAST 没有结束")
			return
		}
		p.next()
	case isBareIdent(t.lit):
		p.next()
		name := t.lit
		if _, ban := bannedFuncs[fold(name)]; ban {
			p.fail("函数 %s 不允许使用", name)
			return
		}
		if p.peek().lit == "(" {
			p.next()
			_, isAgg := aggregateFuncs[fold(name)]
			if isAgg {
				p.aggCount++
			}
			inner := aggregate || isAgg
			if p.peek().lit == "*" && isAgg {
				p.next()
			} else if p.peek().lit != ")" {
				p.acceptKW("DISTINCT")
				p.parseExprList(inner)
			}
			if p.peek().lit != ")" {
				p.fail("函数 %s 没有结束", name)
				return
			}
			p.next()
			return
		}
		if p.acceptLit(".") {
			if p.peek().lit == "*" {
				p.fail("此处不能使用 *")
				return
			}
			col := p.expectIdent()
			if col == "" {
				p.fail("列名无效")
				return
			}
			p.noteUse(name, col, aggregate)
			return
		}
		p.noteUse("", name, aggregate)
	default:
		if t.lit == "" {
			p.fail("SQL 意外结束")
			return
		}
		p.fail("无法解析的 SQL（%s）", t.lit)
	}
}

func (p *parser) noteUse(table, col string, aggregate bool) {
	if p.active == nil {
		return
	}
	p.active.uses = append(p.active.uses, colUse{table: table, col: col, aggregate: aggregate})
}

func (p *parser) expectIdent() string {
	t := p.next()
	if !isBareIdent(t.lit) {
		p.fail("期望标识符")
		return ""
	}
	return t.lit
}

func isNumber(lit string) bool {
	if lit == "" {
		return false
	}
	for _, r := range lit {
		if r == '.' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isCmpOp(lit string) bool {
	switch lit {
	case "=", "!=", "<>", "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func isBareIdent(lit string) bool {
	if lit == "" || strings.HasPrefix(lit, "'") || isNumber(lit) {
		return false
	}
	r, _ := utf8Decode(lit)
	return isIdentStart(r)
}

func utf8Decode(s string) (rune, int) {
	return utf8.DecodeRuneInString(s)
}
