package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type StripeService struct {
	secretKey string
	client    *http.Client
}

type CheckoutSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func NewStripeService(secretKey string) *StripeService {
	return &StripeService{secretKey: secretKey, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *StripeService) CreateSubscriptionCheckout(email, priceID, successURL, cancelURL string, metadata map[string]string) (*CheckoutSession, error) {
	vals := url.Values{
		"mode":                                 {"subscription"},
		"customer_email":                       {email},
		"success_url":                          {successURL},
		"cancel_url":                           {cancelURL},
		"line_items[0][price]":                 {priceID},
		"line_items[0][quantity]":              {"1"},
		"subscription_data[metadata][product]": {"statuspage.app.nz"},
	}
	for k, v := range metadata {
		vals.Set(fmt.Sprintf("metadata[%s]", k), v)
		vals.Set(fmt.Sprintf("subscription_data[metadata][%s]", k), v)
	}

	var session CheckoutSession
	if err := s.post("/v1/checkout/sessions", vals, &session); err != nil {
		return nil, err
	}
	if session.URL == "" {
		return nil, errors.New("stripe did not return a checkout url")
	}
	return &session, nil
}

func (s *StripeService) post(apiPath string, vals url.Values, out any) error {
	if s.secretKey == "" {
		return errors.New("stripe secret key is empty")
	}
	req, err := http.NewRequest("POST", "https://api.stripe.com"+apiPath, strings.NewReader(vals.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Stripe-Version", "2024-06-20")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stripe %d: %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}

type WebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

func ConstructWebhookEvent(payload []byte, sigHeader, webhookSecret string) (*WebhookEvent, error) {
	if err := verifyWebhookSignature(payload, sigHeader, webhookSecret, 5*time.Minute); err != nil {
		return nil, err
	}
	var evt WebhookEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}
	return &evt, nil
}

func verifyWebhookSignature(payload []byte, sigHeader, secret string, tolerance time.Duration) error {
	timestamp, signatures, err := parseStripeSignature(sigHeader)
	if err != nil {
		return err
	}
	if time.Since(timestamp) > tolerance {
		return errors.New("stripe signature timestamp is too old")
	}

	signedPayload := []byte(fmt.Sprintf("%d.%s", timestamp.Unix(), payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signedPayload)
	expected := mac.Sum(nil)

	for _, sigHex := range signatures {
		got, err := hex.DecodeString(sigHex)
		if err == nil && subtle.ConstantTimeCompare(got, expected) == 1 {
			return nil
		}
	}
	return errors.New("stripe signature verification failed")
}

func parseStripeSignature(header string) (time.Time, []string, error) {
	var timestamp time.Time
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			n, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return time.Time{}, nil, fmt.Errorf("invalid stripe timestamp: %w", err)
			}
			timestamp = time.Unix(n, 0)
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}
	if timestamp.IsZero() {
		return time.Time{}, nil, errors.New("missing stripe timestamp")
	}
	if len(signatures) == 0 {
		return time.Time{}, nil, errors.New("missing stripe v1 signature")
	}
	return timestamp, signatures, nil
}
