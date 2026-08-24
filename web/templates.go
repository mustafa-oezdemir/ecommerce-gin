package web

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
	"unicode/utf8"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

func ParseTemplates() (*template.Template, error) {
	return template.New("root").Funcs(template.FuncMap{
		"money":    formatCents,
		"mulCents": mulCents,
		"initials": initials,
	}).ParseFS(templateFS, "templates/*.tmpl")
}

func formatCents(cents int64) string {
	if cents < 0 {
		return "-" + formatCents(-cents)
	}
	return fmt.Sprintf("%d,%02d €", cents/100, cents%100)
}

func mulCents(priceCents int64, quantity int) int64 {
	return priceCents * int64(quantity)
}

func initials(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(trimmed)
	if first == utf8.RuneError {
		return ""
	}
	return strings.ToUpper(string(first))
}
