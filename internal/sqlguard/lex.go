package sqlguard

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type token struct {
	lit string
	pos int
}

// stripComments removes -- and /* */ comments while keeping string and
// identifier literals intact. A single trailing semicolon is removed.
func stripComments(sql string) (string, error) {
	var b strings.Builder
	b.Grow(len(sql))
	inSingle, inDouble, inBack, inBracket := false, false, false, false
	semi := -1
	stmts := 0
	for i := 0; i < len(sql); {
		c := sql[i]
		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte('\'')
					i += 2
					continue
				}
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					b.WriteByte('"')
					i += 2
					continue
				}
				inDouble = false
			}
			i++
			continue
		}
		if inBack {
			b.WriteByte(c)
			if c == '`' {
				inBack = false
			}
			i++
			continue
		}
		if inBracket {
			b.WriteByte(c)
			if c == ']' {
				inBracket = false
			}
			i++
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("注释没有结束")
			}
			i += 2 + end + 2
			b.WriteByte(' ')
			continue
		}
		if c == '\'' {
			inSingle = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == '"' {
			inDouble = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == '`' {
			inBack = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == '[' {
			inBracket = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == ';' {
			stmts++
			semi = b.Len()
			b.WriteByte(c)
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	if inSingle || inDouble || inBack || inBracket {
		return "", fmt.Errorf("引号没有结束")
	}
	out := strings.TrimSpace(b.String())
	if stmts > 1 {
		return "", fmt.Errorf("只允许一条 SQL")
	}
	if stmts == 1 {
		trim := strings.TrimSpace(out)
		if !strings.HasSuffix(trim, ";") {
			return "", fmt.Errorf("只允许一条 SQL")
		}
		// The semicolon must be trailing, not hidden before trailing space only.
		if semi >= 0 && strings.TrimSpace(out[semi+1:]) != "" {
			return "", fmt.Errorf("只允许一条 SQL")
		}
		out = strings.TrimSpace(strings.TrimSuffix(trim, ";"))
	}
	if out == "" {
		return "", fmt.Errorf("SQL 为空")
	}
	return out, nil
}

func tokenize(sql string) ([]token, error) {
	var toks []token
	for i := 0; i < len(sql); {
		r, w := utf8.DecodeRuneInString(sql[i:])
		if r == utf8.RuneError && w == 1 {
			return nil, fmt.Errorf("SQL 含有无效字符")
		}
		if unicode.IsSpace(r) {
			i += w
			continue
		}
		switch r {
		case '\'':
			lit, n, err := readString(sql[i:])
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{lit: "'" + lit + "'", pos: i})
			i += n
		case '"', '`', '[':
			lit, n, err := readIdent(sql[i:])
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{lit: lit, pos: i})
			i += n
		default:
			if isIdentStart(r) {
				j := i + w
				for j < len(sql) {
					rr, ww := utf8.DecodeRuneInString(sql[j:])
					if !isIdentCont(rr) {
						break
					}
					j += ww
				}
				toks = append(toks, token{lit: sql[i:j], pos: i})
				i = j
				continue
			}
			if unicode.IsDigit(r) || (r == '.' && i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9') {
				j := i
				dot := false
				for j < len(sql) {
					if sql[j] == '.' && !dot {
						dot = true
						j++
						continue
					}
					if sql[j] < '0' || sql[j] > '9' {
						break
					}
					j++
				}
				toks = append(toks, token{lit: sql[i:j], pos: i})
				i = j
				continue
			}
			two := ""
			if i+1 < len(sql) {
				two = sql[i : i+2]
			}
			switch two {
			case "||", ">=", "<=", "<>", "!=":
				toks = append(toks, token{lit: two, pos: i})
				i += 2
				continue
			}
			toks = append(toks, token{lit: string(r), pos: i})
			i += w
		}
	}
	toks = append(toks, token{lit: "", pos: len(sql)})
	return toks, nil
}

func readString(s string) (string, int, error) {
	if len(s) < 2 || s[0] != '\'' {
		return "", 0, fmt.Errorf("字符串语法错误")
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
	}
	return "", 0, fmt.Errorf("字符串没有结束")
}

func readIdent(s string) (string, int, error) {
	if s == "" {
		return "", 0, fmt.Errorf("标识符语法错误")
	}
	var end byte
	switch s[0] {
	case '"':
		end = '"'
	case '`':
		end = '`'
	case '[':
		end = ']'
	default:
		return "", 0, fmt.Errorf("标识符语法错误")
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == end {
			if end != ']' && i+1 < len(s) && s[i+1] == end {
				b.WriteByte(end)
				i++
				continue
			}
			if b.Len() == 0 {
				return "", 0, fmt.Errorf("空标识符")
			}
			return b.String(), i + 1, nil
		}
		b.WriteByte(s[i])
	}
	return "", 0, fmt.Errorf("标识符没有结束")
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentCont(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func fold(s string) string { return strings.ToLower(s) }
