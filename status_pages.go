package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

type statusPagePayload struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	WebsiteURL   string `json:"website_url"`
	CustomDomain string `json:"custom_domain"`
	PublicEmail  string `json:"public_email"`
}

func (a *App) handleStatusPages(ctx *fasthttp.RequestCtx) {
	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
	userID, ok := a.userIDFromSession(ctx)
	if !ok {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "not_logged_in"})
		return
	}
	switch string(ctx.Method()) {
	case fasthttp.MethodGet:
		a.listStatusPages(ctx, userID)
	case fasthttp.MethodPost:
		a.createStatusPage(ctx, userID)
	default:
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	}
}

func (a *App) listStatusPages(ctx *fasthttp.RequestCtx, userID int64) {
	rows, err := a.db.Query(`
		SELECT name, slug, description, website_url, custom_domain, public_email, directory_listed, published, created_at
		FROM status_pages WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "list_failed"})
		return
	}
	defer rows.Close()
	var pages []map[string]any
	for rows.Next() {
		var name, slug, desc, website, customDomain, email string
		var listed, published bool
		var created time.Time
		if err := rows.Scan(&name, &slug, &desc, &website, &customDomain, &email, &listed, &published, &created); err == nil {
			pages = append(pages, map[string]any{"name": name, "slug": slug, "description": desc, "website_url": website, "custom_domain": customDomain, "public_email": email, "directory_listed": listed, "published": published, "url": "/s/" + slug, "created_at": created})
		}
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"status_pages": pages})
}

func (a *App) createStatusPage(ctx *fasthttp.RequestCtx, userID int64) {
	var req statusPagePayload
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = sanitizeSlug(req.Slug)
	if req.Slug == "" {
		req.Slug = sanitizeSlug(req.Name)
	}
	if req.Name == "" || !slugPattern.MatchString(req.Slug) {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "name_and_valid_slug_required"})
		return
	}
	var id int64
	err := a.db.QueryRow(`
		INSERT INTO status_pages (user_id, name, slug, description, website_url, custom_domain, public_email)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, userID, req.Name, req.Slug, strings.TrimSpace(req.Description), strings.TrimSpace(req.WebsiteURL), strings.TrimSpace(req.CustomDomain), strings.TrimSpace(req.PublicEmail)).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			writeJSON(ctx, fasthttp.StatusConflict, map[string]any{"error": "slug_already_taken"})
			return
		}
		log.Printf("create status page: %v", err)
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "create_failed"})
		return
	}
	_, _ = a.db.Exec(`
		INSERT INTO status_page_domains (status_page_id, hostname, domain_type, route_slug, status)
		VALUES ($1, $2, 'path', $3, 'active')
	`, id, "/s/"+req.Slug, req.Slug)
	writeJSON(ctx, fasthttp.StatusCreated, map[string]any{"id": id, "slug": req.Slug, "url": "/s/" + req.Slug})
}

func (a *App) handleStatusPageDomains(ctx *fasthttp.RequestCtx, slug string) {
	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
	userID, ok := a.userIDFromSession(ctx)
	if !ok {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "not_logged_in"})
		return
	}
	slug = sanitizeSlug(slug)
	var pageID int64
	if err := a.db.QueryRow(`SELECT id FROM status_pages WHERE slug = $1 AND user_id = $2`, slug, userID).Scan(&pageID); err != nil {
		writeJSON(ctx, fasthttp.StatusNotFound, map[string]any{"error": "status_page_not_found"})
		return
	}
	if string(ctx.Method()) == fasthttp.MethodGet {
		a.listDomains(ctx, pageID)
		return
	}
	if string(ctx.Method()) != fasthttp.MethodPost {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	var req struct {
		Subdomain string `json:"subdomain"`
		Hostname  string `json:"hostname"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	domainType := strings.TrimSpace(req.Type)
	if domainType == "" {
		domainType = "statuspage_subdomain"
	}
	hostname := strings.ToLower(strings.TrimSpace(req.Hostname))
	if domainType == "statuspage_subdomain" {
		label := sanitizeSlug(req.Subdomain)
		if label == "" {
			label = slug
		}
		hostname = label + ".statuspage.app.nz"
	}
	if hostname == "" || strings.Contains(hostname, "/") {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "valid_hostname_required"})
		return
	}
	status, recordID, lastErr := a.provisionDomain(hostname, domainType)
	_, err := a.db.Exec(`
		INSERT INTO status_page_domains (status_page_id, hostname, domain_type, route_slug, status, dns_record_id, last_error)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''))
		ON CONFLICT (hostname) DO UPDATE SET status = EXCLUDED.status, dns_record_id = EXCLUDED.dns_record_id, last_error = EXCLUDED.last_error, updated_at = now()
	`, pageID, hostname, domainType, slug, status, recordID, lastErr)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "domain_save_failed"})
		return
	}
	writeJSON(ctx, fasthttp.StatusCreated, map[string]any{"hostname": hostname, "type": domainType, "status": status, "dns_record_id": recordID, "last_error": lastErr, "path_url": "/s/" + slug})
}

func (a *App) listDomains(ctx *fasthttp.RequestCtx, pageID int64) {
	rows, err := a.db.Query(`SELECT hostname, domain_type, status, COALESCE(dns_record_id,''), COALESCE(last_error,'') FROM status_page_domains WHERE status_page_id = $1 ORDER BY created_at DESC`, pageID)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "list_failed"})
		return
	}
	defer rows.Close()
	var domains []map[string]string
	for rows.Next() {
		var host, typ, status, record, lastErr string
		if err := rows.Scan(&host, &typ, &status, &record, &lastErr); err == nil {
			domains = append(domains, map[string]string{"hostname": host, "type": typ, "status": status, "dns_record_id": record, "last_error": lastErr})
		}
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"domains": domains})
}

func (a *App) provisionDomain(hostname, domainType string) (string, string, string) {
	if domainType != "statuspage_subdomain" {
		return "pending", "", "Add a CNAME to statuspage.app.nz, then contact us to activate SSL."
	}
	if a.dns == nil {
		return "pending", "", "cloudflare not configured"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	recordID, err := a.dns.UpsertCNAME(ctx, hostname, "statuspage.app.nz", true)
	if err != nil {
		return "error", "", err.Error()
	}
	return "active", recordID, ""
}

func (a *App) handlePublicStatusPage(ctx *fasthttp.RequestCtx, slug string) {
	slug = sanitizeSlug(strings.Split(slug, "/")[0])
	if slug == "" {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		return
	}
	page, ok := a.lookupPublicPage(slug)
	if !ok {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("text/html; charset=utf-8")
		ctx.SetBodyString("<h1>Status page not found</h1>")
		return
	}
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyString(renderStatusPage(page))
}

func (a *App) serveStatusPageHost(ctx *fasthttp.RequestCtx, requestPath string) bool {
	host := strings.ToLower(string(ctx.Host()))
	host = strings.Split(host, ":")[0]
	if host == "" || host == "statuspage.app.nz" || !strings.HasSuffix(host, ".statuspage.app.nz") {
		return false
	}
	if requestPath != "/" {
		return false
	}
	page, ok := a.lookupPublicPageByHost(host)
	if !ok {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("text/html; charset=utf-8")
		ctx.SetBodyString("<h1>Status page not found</h1>")
		return true
	}
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyString(renderStatusPage(page))
	return true
}

func (a *App) lookupPublicPage(slug string) (map[string]string, bool) {
	if a.db == nil {
		if slug == "demo" {
			return map[string]string{"name": "Demo Status Page", "slug": "demo", "description": "Example hosted status page.", "website_url": "https://statuspage.app.nz"}, true
		}
		return nil, false
	}
	var name, desc, website string
	err := a.db.QueryRow(`SELECT name, description, website_url FROM status_pages WHERE slug = $1 AND published = true`, slug).Scan(&name, &desc, &website)
	if err != nil {
		return nil, false
	}
	return map[string]string{"name": name, "slug": slug, "description": desc, "website_url": website}, true
}

func (a *App) lookupPublicPageByHost(hostname string) (map[string]string, bool) {
	if a.db == nil {
		return nil, false
	}
	var name, slug, desc, website string
	err := a.db.QueryRow(`
		SELECT sp.name, sp.slug, sp.description, sp.website_url
		FROM status_page_domains d
		JOIN status_pages sp ON sp.id = d.status_page_id
		WHERE d.hostname = $1 AND d.status = 'active' AND sp.published = true
	`, hostname).Scan(&name, &slug, &desc, &website)
	if err != nil {
		return nil, false
	}
	return map[string]string{"name": name, "slug": slug, "description": desc, "website_url": website}, true
}

func renderStatusPage(page map[string]string) string {
	name := html.EscapeString(page["name"])
	desc := html.EscapeString(page["description"])
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s status</title><link rel="stylesheet" href="/assets/site.css"></head><body class="public-status"><main class="public-status-shell"><div class="public-status-card"><h1>%s</h1><p><span class="dot"></span>All systems operational</p><p>%s</p><div class="public-status-grid"><span>Website <strong>100%%</strong></span><span>API <strong>100%%</strong></span><span>Email <strong>100%%</strong></span></div><small>Powered by statuspage.app.nz</small></div></main></body></html>`, name, name, desc)
}

func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			continue
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), "-")
}

func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}
