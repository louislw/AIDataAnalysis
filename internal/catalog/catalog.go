package catalog

import (
	"strings"

	"github.com/louislw/AIDataAnalysis/internal/store"
)

// Column is a non-sensitive field shown to the model.
type Column struct {
	Name        string
	Type        string
	Comment     string
	Description string
}

// Table is an authorized table included in the catalog.
type Table struct {
	Name        string
	Comment     string
	Description string
	Columns     []Column
}

// Input is everything the catalog is allowed to mention.
type Input struct {
	SystemPrompt string
	Prohibitions string
	Tables       []Table
	Metrics      []store.Metric
	Examples     []store.Example
}

// Build renders the directory sent with the system prompt. Sensitive columns
// are omitted by the caller and must not appear here.
func Build(in Input) string {
	var b strings.Builder
	b.WriteString("数据目录\n")
	if len(in.Tables) == 0 {
		b.WriteString("未授权任何表，不能查询。\n")
	}
	for _, table := range in.Tables {
		b.WriteString("\n表 ")
		b.WriteString(table.Name)
		b.WriteString("\n")
		if table.Comment != "" {
			b.WriteString("注释：")
			b.WriteString(oneLine(table.Comment))
			b.WriteString("\n")
		}
		if table.Description != "" {
			b.WriteString("说明：")
			b.WriteString(oneLine(table.Description))
			b.WriteString("\n")
		}
		b.WriteString("字段：\n")
		for _, col := range table.Columns {
			b.WriteString("- ")
			b.WriteString(col.Name)
			if col.Type != "" {
				b.WriteString(" ")
				b.WriteString(col.Type)
			}
			note := col.Description
			if note == "" {
				note = col.Comment
			}
			if note != "" {
				b.WriteString("：")
				b.WriteString(oneLine(note))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n未列出的字段不可直接查询。合计、占比、排名必须写聚合 SQL。\n")
	if len(in.Metrics) > 0 {
		b.WriteString("\n口径：\n")
		for _, m := range in.Metrics {
			b.WriteString("- ")
			b.WriteString(oneLine(m.Name))
			b.WriteString("：")
			b.WriteString(oneLine(m.Definition))
			b.WriteString("\n")
		}
	}
	if len(in.Examples) > 0 {
		b.WriteString("\n示例：\n")
		for _, ex := range in.Examples {
			b.WriteString("问：")
			b.WriteString(oneLine(ex.Question))
			b.WriteString("\nSQL：")
			b.WriteString(oneLine(ex.SQL))
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(in.Prohibitions) != "" {
		b.WriteString("\n禁止事项：\n")
		b.WriteString(strings.TrimSpace(in.Prohibitions))
		b.WriteString("\n")
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
