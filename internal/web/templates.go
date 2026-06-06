package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"time"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
)

//go:embed templates/layout.html templates/partials/*.html templates/pages/*.html templates/login.html static/app.css
var templateFS embed.FS

var pageNames = []string{
	"dashboard", "payments", "services", "service_create", "onboarding",
	"agents", "agent_edit", "agent_panels", "agent_customers", "agent_panel_customers", "customer_create", "panels", "panel_edit", "panel_users", "panel_backups", "bots", "bot_settings",
	"agent_bots", "agent_plans", "agent_bot_settings",
}

func parseTemplates() (map[string]*template.Template, *template.Template, error) {
	funcs := template.FuncMap{
		"T":           i18n.Admin,
		"toman":       formatToman,
		"gib":         formatGiB,
		"date":        formatDate,
		"time":        formatTime,
		"deref":       derefInt64,
		"derefStr":    derefString,
		"agentDomain": db.AgentDomain,
		"dict":        templateDict,
		"urlPath":     urlPathEscape,
		"inboundLabel": formatInboundLabels,
		"usagePct":    usagePercent,
		"add":         func(a, b int) int { return a + b },
		"sub":         func(a, b int) int { return a - b },
		"seq":         seqPages,
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
	loginT, err := template.New("login").Funcs(funcs).ParseFS(templateFS, "templates/login.html")
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

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func templateDict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: odd argument count")
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key must be string")
		}
		m[key] = values[i+1]
	}
	return m, nil
}

func urlPathEscape(s string) string {
	return url.PathEscape(s)
}

func usagePercent(used, limit int64) int {
	if limit <= 0 {
		return 0
	}
	p := int(float64(used) / float64(limit) * 100)
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

func seqPages(n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
