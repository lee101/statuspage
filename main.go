package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"mime"
	"os"
	"path"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/valyala/fasthttp"
)

//go:embed public/*
var publicFS embed.FS

type App struct {
	baseURL       string
	db            *sql.DB
	sessionSecret []byte
	dns           dnsProvisioner
	stripe        *StripeService
	stripePriceID string
	webhookSecret string
}

type signupRequest struct {
	Email   string `json:"email"`
	Company string `json:"company"`
	Domain  string `json:"domain"`
}

func main() {
	loadDotEnv(".env")

	db, err := openDB()
	if err != nil {
		log.Printf("database disabled: %v", err)
	} else if err := runMigrations(db); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}

	app := &App{
		baseURL:       getenv("APP_URL", "http://localhost:"+getenv("PORT", "8080")),
		db:            db,
		sessionSecret: []byte(getenv("SESSION_SECRET", "dev-session-secret-change-me")),
		dns:           newDNSProvisionerFromEnv(),
		stripe:        NewStripeService(os.Getenv("STRIPE_SECRET_KEY")),
		stripePriceID: os.Getenv("STRIPE_PRICE_ID"),
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	port := getenv("PORT", "8080")
	server := &fasthttp.Server{
		Handler:            app.handle,
		Name:               "statuspage.app.nz",
		ReadTimeout:        15 * time.Second,
		WriteTimeout:       15 * time.Second,
		MaxRequestBodySize: 1 << 20,
	}

	log.Printf("statuspage.app.nz listening on :%s", port)
	if err := server.ListenAndServe(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func (a *App) handle(ctx *fasthttp.RequestCtx) {
	p := string(ctx.Path())
	switch {
	case p == "/":
		a.serveIndex(ctx)
	case p == "/blog" || p == "/blog/uptime-builds-trust":
		a.serveFile(ctx, "public/blog/uptime-builds-trust.html")
	case p == "/customers":
		a.serveFile(ctx, "public/customers.html")
	case p == "/api":
		a.serveFile(ctx, "public/api.html")
	case strings.HasPrefix(p, "/s/"):
		a.handlePublicStatusPage(ctx, strings.TrimPrefix(p, "/s/"))
	case p == "/health":
		writeJSON(ctx, fasthttp.StatusOK, map[string]any{"ok": true})
	case p == "/api/register":
		a.handleRegister(ctx)
	case p == "/api/login":
		a.handleLogin(ctx)
	case p == "/api/logout":
		a.handleLogout(ctx)
	case p == "/api/me":
		a.handleMe(ctx)
	case p == "/api/v1/status-pages":
		a.handleStatusPages(ctx)
	case strings.HasPrefix(p, "/api/v1/status-pages/") && strings.HasSuffix(p, "/domains"):
		a.handleStatusPageDomains(ctx, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/status-pages/"), "/domains"))
	case p == "/api/customers" || p == "/api/v1/customers":
		a.handleCustomerDirectoryAPI(ctx)
	case p == "/checkout/create":
		a.handleCreateCheckout(ctx)
	case p == "/stripe/webhook":
		a.handleStripeWebhook(ctx)
	case strings.HasPrefix(p, "/assets/"):
		a.serveFile(ctx, "public"+p)
	case strings.HasPrefix(p, "/tests/"):
		a.serveFile(ctx, "public"+p)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("text/plain; charset=utf-8")
		ctx.SetBodyString("not found")
	}
}

func (a *App) serveIndex(ctx *fasthttp.RequestCtx) {
	data, err := publicFS.ReadFile("public/index.html")
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBodyString("not found")
		return
	}
	body := string(data)
	if string(ctx.QueryArgs().Peek("test")) == "true" {
		body = injectJasmineHarness(body)
	}
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyString(body)
}

func (a *App) serveFile(ctx *fasthttp.RequestCtx, name string) {
	data, err := publicFS.ReadFile(name)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBodyString("not found")
		return
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	ctx.SetContentType(ct)
	if strings.HasSuffix(name, ".html") {
		ctx.SetContentType("text/html; charset=utf-8")
	}
	ctx.SetBody(data)
}

func injectJasmineHarness(page string) string {
	harness := `
<!-- Jasmine E2E Harness -->
<link rel="stylesheet" href="/assets/jasmine/jasmine.css">
<style>
  .jasmine_html-reporter { position: fixed; inset: 64px 8px 8px; z-index: 2147483000; overflow: auto; background: rgba(255,255,255,.98); border-radius: 8px; box-shadow: 0 12px 40px rgba(0,0,0,.28); }
  #jasmine-topbar { position: fixed; top: 8px; left: 8px; right: 8px; z-index: 2147483001; display: flex; gap: 12px; align-items: center; padding: 10px 12px; border-radius: 8px; background: #081133; color: #fff; font: 14px/1.4 system-ui, sans-serif; }
  #jasmine-topbar a { color: #9fe8ae; }
  #jasmine-topbar .status { margin-left: auto; color: rgba(255,255,255,.75); }
</style>
<div id="jasmine-topbar">
  <strong>statuspage.app.nz Jasmine E2E</strong>
  <a href="/">Exit tests</a>
  <span class="status" id="jasmine-status">booting</span>
</div>
<script src="/assets/jasmine/jasmine.js"></script>
<script src="/assets/jasmine/jasmine-html.js"></script>
<script src="/assets/jasmine/boot0.js"></script>
<script src="/tests/site.spec.js"></script>
<script>
  (function(){
    var env = jasmine.getEnv();
    var results = { specs: [], summary: { passed: 0, failed: 0, pending: 0 }, overallStatus: "running" };
    var statusEl = document.getElementById("jasmine-status");
    env.addReporter({
      specDone: function(r) {
        results.specs.push({ fullName: r.fullName, status: r.status, failedExpectations: r.failedExpectations || [] });
        if (r.status === "passed") results.summary.passed += 1;
        else if (r.status === "failed") results.summary.failed += 1;
        else results.summary.pending += 1;
      },
      jasmineDone: function(r) {
        results.overallStatus = r.overallStatus || (results.summary.failed ? "failed" : "passed");
        window.__JASMINE_RESULTS__ = results;
        localStorage.setItem("statuspageJasmineResults", JSON.stringify(results));
        document.documentElement.setAttribute("data-jasmine-status", results.overallStatus);
        var marker = document.createElement("meta");
        marker.id = "jasmine-result";
        marker.setAttribute("data-status", results.overallStatus);
        marker.setAttribute("data-failed", String(results.summary.failed));
        marker.setAttribute("data-passed", String(results.summary.passed));
        document.head.appendChild(marker);
        if (statusEl) statusEl.textContent = "done: " + results.overallStatus;
      }
    });
  })();
</script>
<script src="/assets/jasmine/boot1.js"></script>
<!-- End Jasmine E2E Harness -->
`
	idx := strings.LastIndex(strings.ToLower(page), "</body>")
	if idx < 0 {
		return page + harness
	}
	return page[:idx] + harness + page[idx:]
}

func (a *App) handleCreateCheckout(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.stripe.secretKey == "" || a.stripePriceID == "" {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{
			"error":   "stripe_not_configured",
			"message": "Set STRIPE_SECRET_KEY and STRIPE_PRICE_ID to enable checkout.",
		})
		return
	}

	var req signupRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Company = strings.TrimSpace(req.Company)
	req.Domain = strings.TrimSpace(req.Domain)
	if !strings.Contains(req.Email, "@") || req.Company == "" {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "email_and_company_required"})
		return
	}

	successURL := a.baseURL + "/?checkout=success&session_id={CHECKOUT_SESSION_ID}"
	cancelURL := a.baseURL + "/?checkout=cancelled"
	session, err := a.stripe.CreateSubscriptionCheckout(req.Email, a.stripePriceID, successURL, cancelURL, map[string]string{
		"company": req.Company,
		"domain":  req.Domain,
		"plan":    "statuspage_monthly_19",
	})
	if err != nil {
		log.Printf("stripe checkout: %v", err)
		writeJSON(ctx, fasthttp.StatusBadGateway, map[string]any{"error": "stripe_error"})
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"url": session.URL, "id": session.ID})
}

func (a *App) handleStripeWebhook(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.webhookSecret == "" {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "webhook_not_configured"})
		return
	}
	payload := append([]byte(nil), ctx.PostBody()...)
	sig := string(ctx.Request.Header.Peek("Stripe-Signature"))
	evt, err := ConstructWebhookEvent(payload, sig, a.webhookSecret)
	if err != nil {
		log.Printf("stripe webhook verification failed: %v", err)
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_signature"})
		return
	}

	switch evt.Type {
	case "checkout.session.completed", "customer.subscription.created", "customer.subscription.updated":
		log.Printf("stripe event accepted: %s %s", evt.Type, evt.ID)
	case "customer.subscription.deleted":
		log.Printf("stripe subscription cancelled: %s", evt.ID)
	default:
		log.Printf("stripe event ignored: %s %s", evt.Type, evt.ID)
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"received": true})
}

func writeJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(`{"error":"json_error"}`)
		return
	}
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json; charset=utf-8")
	ctx.SetBody(data)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
