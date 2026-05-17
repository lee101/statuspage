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
	{Name: "Text-Generator.io", Domain: "text-generator.io", URL: "https://text-generator.io", Description: "Unified text, vision, and speech API for technical AI products.", Tags: "ai text vision speech api developer tools", Uptime: "100%"},
	{Name: "Netwrck", Domain: "netwrck.com", URL: "https://netwrck.com", Description: "AI character chat and multimodal creation hub with voice, sharing, chats, and AI art.", Tags: "ai characters chat multimodal creation art voice", Uptime: "100%"},
	{Name: "SimplexGen", Domain: "simplexgen.com", URL: "https://simplexgen.com", Description: "Image-to-3D generation and mesh tooling for creators and developers.", Tags: "image to 3d mesh tools ai creator developer", Uptime: "100%"},
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
