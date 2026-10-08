package sqlguard

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var placeholderRE = regexp.MustCompile(`(?i)\{\{\s*user\.(dept|id|username|display_name)\s*\}\}`)

// ValidateRowFilter accepts only column comparisons, {{user.*}} placeholders,
// parentheses, and AND/OR. An empty filter is valid.
func ValidateRowFilter(filter string) error {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	_, err := compileFilter(filter, nil)
	return err
}

// CompileRowFilter turns a validated filter into a SQL boolean expression.
// fields maps placeholder names (dept, id, username, display_name) to values.
func CompileRowFilter(filter string, fields map[string]string) (string, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return "", nil
	}
	return compileFilter(filter, fields)
}

func compileFilter(filter string, fields map[string]string) (string, error) {
	quoted := placeholderRE.ReplaceAllStringFunc(filter, func(match string) string {
		sub := placeholderRE.FindStringSubmatch(match)
		return "'{{user." + strings.ToLower(sub[1]) + "}}'"
	})
	leftover := placeholderRE.ReplaceAllString(filter, "")
	if strings.ContainsAny(leftover, "{}") {
		return "", fmt.Errorf("行级条件的值必须是 {{user.字段}} 占位符")
	}
	toks, err := tokenize(quoted)
	if err != nil {
		return "", err
	}
	p := &filterParser{toks: toks, fields: fields, checkOnly: fields == nil}
	expr := p.parseOr()
	if p.err != nil {
		return "", p.err
	}
	if p.peek().lit != "" {
		return "", fmt.Errorf("行级条件含有不允许的内容（%s）", p.peek().lit)
	}
	return expr, nil
}

type filterParser struct {
	toks      []token
	i         int
	fields    map[string]string
	checkOnly bool
	err       error
}

func (p *filterParser) fail(msg string) string {
	if p.err == nil {
		p.err = fmt.Errorf("%s", msg)
	}
	return ""
}

func (p *filterParser) peek() token {
	if p.i >= len(p.toks) {
		return token{}
	}
	return p.toks[p.i]
}

func (p *filterParser) next() token {
	t := p.peek()
	if p.i < len(p.toks) {
		p.i++
	}
	return t
}

func (p *filterParser) parseOr() string {
	left := p.parseAnd()
	for p.err == nil && strings.EqualFold(p.peek().lit, "OR") {
		p.next()
		right := p.parseAnd()
		left = "(" + left + " OR " + right + ")"
	}
	return left
}

func (p *filterParser) parseAnd() string {
	left := p.parseCmp()
	for p.err == nil && strings.EqualFold(p.peek().lit, "AND") {
		p.next()
		right := p.parseCmp()
		left = "(" + left + " AND " + right + ")"
	}
	return left
}

func (p *filterParser) parseCmp() string {
	if p.peek().lit == "(" {
		p.next()
		inner := p.parseOr()
		if p.peek().lit != ")" {
			return p.fail("行级条件的括号没有结束")
		}
		p.next()
		return "(" + inner + ")"
	}
	col := p.next()
	if !isFilterIdent(col.lit) {
		return p.fail("行级条件只允许列名、占位符、比较和 AND/OR")
	}
	op := p.next().lit
	switch op {
	case "=", "!=", "<>", "<", ">", "<=", ">=":
	default:
		return p.fail("行级条件只允许列名、占位符、比较和 AND/OR")
	}
	val := p.next()
	if !strings.HasPrefix(val.lit, "'") || !strings.HasSuffix(val.lit, "'") {
		return p.fail("行级条件的值必须是 {{user.字段}} 占位符")
	}
	raw := val.lit[1 : len(val.lit)-1]
	if !strings.HasPrefix(raw, "{{") || !strings.HasSuffix(raw, "}}") {
		return p.fail("行级条件的值必须是 {{user.字段}} 占位符")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(raw, "{{"), "}}")
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(strings.ToLower(body), "user.") {
		return p.fail("行级条件只支持 {{user.dept}}、{{user.id}}、{{user.username}}、{{user.display_name}}")
	}
	field := strings.ToLower(strings.TrimPrefix(body, "user."))
	switch field {
	case "dept", "id", "username", "display_name":
	default:
		return p.fail("行级条件只支持 {{user.dept}}、{{user.id}}、{{user.username}}、{{user.display_name}}")
	}
	quotedCol := `"` + strings.ReplaceAll(col.lit, `"`, `""`) + `"`
	if p.checkOnly {
		return quotedCol + " " + op + " ''"
	}
	value, ok := p.fields[field]
	if !ok {
		p.fail("缺少占位符 user." + field)
		return ""
	}
	if field == "id" {
		if !isNumber(value) {
			p.fail("user.id 不是整数")
			return ""
		}
		return quotedCol + " " + op + " " + value
	}
	return quotedCol + " " + op + " '" + strings.ReplaceAll(value, "'", "''") + "'"
}

func isFilterIdent(lit string) bool {
	if lit == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(lit)
	if !isIdentStart(r) {
		return false
	}
	for _, rr := range lit[size:] {
		if !isIdentCont(rr) {
			return false
		}
	}
	switch strings.ToLower(lit) {
	case "and", "or", "not", "select", "from", "where":
		return false
	default:
		return true
	}
}
