package web

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPagination_pageURLPreservesFilters(t *testing.T) {
	r := httptest.NewRequest("GET", "/services?q=foo&status=active&page=2", nil)
	p := paginationFromRequest(r, 100, "q", "status")
	if p.Query["q"] != "foo" || p.Query["status"] != "active" {
		t.Fatalf("query: %+v", p.Query)
	}
	u, err := url.Parse(p.PageURL(3))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("page") != "3" {
		t.Fatalf("page=%s", u.Query().Get("page"))
	}
	if u.Query().Get("q") != "foo" {
		t.Fatalf("q=%s", u.Query().Get("q"))
	}
}

func TestPagination_histPageParam(t *testing.T) {
	p := Pagination{Page: 2, PerPage: 25, Total: 50, Base: "/payments/pending", PageParam: "hist_page"}
	u, _ := url.Parse(p.NextURL())
	if u.Query().Get("hist_page") != "3" {
		t.Fatalf("hist_page=%s", u.Query().Get("hist_page"))
	}
}

func TestPagination_offset(t *testing.T) {
	p := Pagination{Page: 3, PerPage: 25, Total: 100}
	if p.Offset() != 50 {
		t.Fatalf("offset=%d", p.Offset())
	}
	if p.From() != 51 || p.To() != 75 {
		t.Fatalf("from=%d to=%d", p.From(), p.To())
	}
}
