package web

import (
	"bytes"
	"testing"
)

func TestLoginTemplateExecute(t *testing.T) {
	_, loginT, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"Error": false, "Lang": "fa", "IsRTL": true}
	var buf bytes.Buffer
	if err := loginT.ExecuteTemplate(&buf, "login.html", data); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	if buf.Len() < 200 {
		t.Fatalf("login HTML too small: %d bytes", buf.Len())
	}
}
