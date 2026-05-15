package main

import (
	"strings"

	"github.com/valyala/fasthttp"
)

type directoryCustomer struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
	Uptime      string `json:"uptime"`
}

var builtInCustomers = []directoryCustomer{
	{Name: "app.nz", Domain: "app.nz", URL: "https://app.nz", Description: "New Zealand app projects and product directory.", Tags: "apps nz software products", Uptime: "100%"},
	{Name: "gobed", Domain: "github.com/lee101/gobed", URL: "https://github.com/lee101/gobed", Description: "Fast embedding and search engine work by lee101.", Tags: "search embeddings ai engine lee101 gobed", Uptime: "100%"},
	{Name: "devmate.co.nz", Domain: "devmate.co.nz", URL: "https://devmate.co.nz", Description: "Developer tooling and web services.", Tags: "developer tools hosting", Uptime: "100%"},
	{Name: "kiwihost.nz", Domain: "kiwihost.nz", URL: "https://kiwihost.nz", Description: "Hosting and infrastructure for NZ businesses.", Tags: "hosting infrastructure nz", Uptime: "100%"},
	{Name: "buildtogether.co", Domain: "buildtogether.co", URL: "https://buildtogether.co", Description: "Product studio for teams building online.", Tags: "product studio collaboration", Uptime: "100%"},
	{Name: "velocity.nz", Domain: "velocity.nz", URL: "https://velocity.nz", Description: "Performance-minded web services.", Tags: "performance uptime web", Uptime: "100%"},
	{Name: "nzcheckouts.com", Domain: "nzcheckouts.com", URL: "https://nzcheckouts.com", Description: "Checkout and payment experiences for local businesses.", Tags: "payments ecommerce checkout", Uptime: "100%"},
}

func (a *App) handleCustomerDirectoryAPI(ctx *fasthttp.RequestCtx) {
	q := strings.ToLower(strings.TrimSpace(string(ctx.QueryArgs().Peek("q"))))
	results := make([]directoryCustomer, 0, len(builtInCustomers))
	for _, customer := range builtInCustomers {
		haystack := strings.ToLower(customer.Name + " " + customer.Domain + " " + customer.Description + " " + customer.Tags)
		if q == "" || strings.Contains(haystack, q) {
			results = append(results, customer)
		}
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"customers": results, "query": q})
}
