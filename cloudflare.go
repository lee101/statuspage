package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type dnsProvisioner interface {
	UpsertCNAME(ctx context.Context, name, target string, proxied bool) (string, error)
}

type cloudflareClient struct {
	apiToken   string
	apiKey     string
	apiEmail   string
	zoneID     string
	baseURL    string
	httpClient *http.Client
}

func newDNSProvisionerFromEnv() dnsProvisioner {
	zoneID := strings.TrimSpace(getenv("CLOUDFLARE_ZONE_ID", ""))
	if zoneID == "" {
		return nil
	}
	token := strings.TrimSpace(getenv("CLOUDFLARE_API_TOKEN", ""))
	if token != "" {
		return &cloudflareClient{apiToken: token, zoneID: zoneID, baseURL: "https://api.cloudflare.com/client/v4", httpClient: &http.Client{Timeout: 12 * time.Second}}
	}
	key := strings.TrimSpace(getenv("CLOUDFLARE_API_KEY", ""))
	email := strings.TrimSpace(getenv("CLOUDFLARE_API_EMAIL", ""))
	if key != "" && email != "" {
		return &cloudflareClient{apiKey: key, apiEmail: email, zoneID: zoneID, baseURL: "https://api.cloudflare.com/client/v4", httpClient: &http.Client{Timeout: 12 * time.Second}}
	}
	return nil
}

func (c *cloudflareClient) UpsertCNAME(ctx context.Context, name, target string, proxied bool) (string, error) {
	recordID, _, err := c.lookupRecord(ctx, name)
	if err != nil {
		return "", err
	}
	payload := map[string]any{"type": "CNAME", "name": name, "content": target, "proxied": proxied, "ttl": 300}
	if recordID == "" {
		return c.createRecord(ctx, payload)
	}
	return recordID, c.updateRecord(ctx, recordID, payload)
}

func (c *cloudflareClient) lookupRecord(ctx context.Context, name string) (string, string, error) {
	values := url.Values{"type": {"CNAME"}, "name": {name}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/zones/%s/dns_records?%s", c.baseURL, c.zoneID, values.Encode()), nil)
	if err != nil {
		return "", "", err
	}
	c.decorate(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out cloudflareListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if !out.Success {
		return "", "", fmt.Errorf("cloudflare lookup failed: %v", out.Errors)
	}
	if len(out.Result) == 0 {
		return "", "", nil
	}
	return out.Result[0].ID, out.Result[0].Content, nil
}

func (c *cloudflareClient) createRecord(ctx context.Context, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL, c.zoneID), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.decorate(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out cloudflareRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !out.Success {
		return "", fmt.Errorf("cloudflare create failed: %v", out.Errors)
	}
	if out.Result.ID == "" {
		return "", fmt.Errorf("cloudflare create returned no result")
	}
	return out.Result.ID, nil
}

func (c *cloudflareClient) updateRecord(ctx context.Context, recordID string, payload map[string]any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, c.zoneID, recordID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.decorate(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out cloudflareRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("cloudflare update failed: %v", out.Errors)
	}
	return nil
}

func (c *cloudflareClient) decorate(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" && c.apiEmail != "" {
		req.Header.Set("X-Auth-Key", c.apiKey)
		req.Header.Set("X-Auth-Email", c.apiEmail)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
}

type cloudflareListResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"result"`
}

type cloudflareRecordResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"result"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
