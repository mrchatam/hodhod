package web

import "testing"

func TestParseTemplates_allPages(t *testing.T) {
	pages, loginT, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if loginT == nil {
		t.Fatal("login template missing")
	}
	for _, name := range pageNames {
		if pages[name] == nil {
			t.Fatalf("page %q not parsed", name)
		}
	}
}
