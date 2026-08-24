package web

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

func ParseTemplates() (*template.Template, error) {
	return template.New("root").Funcs(template.FuncMap{"money": formatCents}).ParseFS(templateFS, "templates/*.tmpl")
}

func formatCents(cents int64) string {
	if cents < 0 {
		return "-" + formatCents(-cents)
	}
	return fmt.Sprintf("%d,%02d €", cents/100, cents%100)
}
