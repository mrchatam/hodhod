package web

import (
	"embed"
	"fmt"
	"html/template"
	"strconv"
	"time"
)

//go:embed templates/layout.html templates/partials/*.html templates/pages/*.html templates/login.html
var templateFS embed.FS

var pageNames = []string{
	"dashboard", "payments", "services", "service_create", "onboarding",
	"agents", "agent_edit", "panels", "panel_edit", "bots", "bot_settings",
	"agent_bots", "agent_plans", "agent_bot_settings",
}

func parseTemplates() (map[string]*template.Template, *template.Template, error) {
	funcs := template.FuncMap{
		"toman": formatToman,
		"gib":   formatGiB,
		"date":  formatDate,
		"time":  formatTime,
		"deref": derefInt64,
	}
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		t, err := template.New("layout").Funcs(funcs).ParseFS(
			templateFS,
			"templates/layout.html",
			"templates/partials/*.html",
			"templates/pages/"+name+".html",
		)
		if err != nil {
			return nil, nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		pages[name] = t
	}
	loginT, err := template.ParseFS(templateFS, "templates/login.html")
	if err != nil {
		return nil, nil, err
	}
	return pages, loginT, nil
}

func formatToman(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func formatGiB(bytes int64) string {
	if bytes <= 0 {
		return "0 GB"
	}
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.2f GB", gb)
}

func formatDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
