package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var allowedPerPage = []int{10, 25, 50, 100}

// Pagination holds list paging state for templates.
type Pagination struct {
	Page      int
	PerPage   int
	Total     int
	Base      string
	Query     map[string]string
	PageParam string // query key for page number, default "page"
}

func (p Pagination) pageKey() string {
	if p.PageParam != "" {
		return p.PageParam
	}
	return "page"
}

func (p Pagination) TotalPages() int {
	if p.PerPage <= 0 {
		return 1
	}
	n := p.Total / p.PerPage
	if p.Total%p.PerPage != 0 {
		n++
	}
	if n < 1 {
		return 1
	}
	return n
}

func (p Pagination) Offset() int {
	if p.Page < 1 {
		return 0
	}
	return (p.Page - 1) * p.PerPage
}

func (p Pagination) From() int {
	if p.Total == 0 {
		return 0
	}
	return p.Offset() + 1
}

func (p Pagination) To() int {
	end := p.Offset() + p.PerPage
	if end > p.Total {
		end = p.Total
	}
	return end
}

func (p Pagination) HasPrev() bool { return p.Page > 1 }
func (p Pagination) HasNext() bool { return p.Page < p.TotalPages() }

func (p Pagination) PrevURL() string { return p.pageURL(p.Page - 1) }
func (p Pagination) NextURL() string { return p.pageURL(p.Page + 1) }

func (p Pagination) pageURL(page int) string {
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	for k, v := range p.Query {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Set(p.pageKey(), strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(p.PerPage))
	if p.Base == "" {
		return "?" + q.Encode()
	}
	return p.Base + "?" + q.Encode()
}

func (p Pagination) PageURL(page int) string {
	return p.pageURL(page)
}

// PageNumbers returns page numbers to show (with -1 for ellipsis).
func (p Pagination) PageNumbers() []int {
	total := p.TotalPages()
	if total <= 7 {
		out := make([]int, total)
		for i := range out {
			out[i] = i + 1
		}
		return out
	}
	cur := p.Page
	if cur < 1 {
		cur = 1
	}
	pages := []int{1}
	if cur > 3 {
		pages = append(pages, -1)
	}
	start := cur - 1
	if start < 2 {
		start = 2
	}
	end := cur + 1
	if end > total-1 {
		end = total - 1
	}
	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}
	if end < total-1 {
		pages = append(pages, -1)
	}
	pages = append(pages, total)
	return pages
}

func paginationFromRequest(r *http.Request, total int, preserve ...string) Pagination {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage <= 0 {
		perPage = 25
	}
	if !validPerPage(perPage) {
		perPage = 25
	}
	q := map[string]string{}
	for _, k := range preserve {
		if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
			q[k] = v
		}
	}
	return Pagination{
		Page: page, PerPage: perPage, Total: total,
		Base: r.URL.Path, Query: q,
	}
}

func validPerPage(n int) bool {
	for _, v := range allowedPerPage {
		if v == n {
			return true
		}
	}
	return false
}

func paginationShowing(lang string, p Pagination) string {
	if p.Total == 0 {
		return ""
	}
	// Use fmt for numbers; template can call this via func
	_ = lang
	return fmt.Sprintf("%d–%d / %d", p.From(), p.To(), p.Total)
}

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || v < 1 {
		return def
	}
	return v
}
