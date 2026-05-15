package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
)

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Company  string `json:"company"`
}

type accountUser struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Company   string    `json:"company"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *App) handleRegister(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
	var req authRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Company = strings.TrimSpace(req.Company)
	if !strings.Contains(req.Email, "@") || len(req.Password) < 8 || req.Company == "" {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "email_password_company_required"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "password_hash_failed"})
		return
	}

	var user accountUser
	err = a.db.QueryRow(`
		INSERT INTO users (email, company, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, company, created_at
	`, req.Email, req.Company, string(hash)).Scan(&user.ID, &user.Email, &user.Company, &user.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeJSON(ctx, fasthttp.StatusConflict, map[string]any{"error": "email_already_registered"})
			return
		}
		log.Printf("register failed: %v", err)
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "register_failed"})
		return
	}
	a.setSessionCookie(ctx, user.ID)
	go sendWelcomeEmail(user.Email, user.Company)
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user})
}

func (a *App) handleLogin(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
	var req authRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var user accountUser
	var hash string
	err := a.db.QueryRow(`
		SELECT id, email, company, password_hash, created_at
		FROM users
		WHERE email = $1
	`, req.Email).Scan(&user.ID, &user.Email, &user.Company, &hash, &user.CreatedAt)
	if err == sql.ErrNoRows {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "invalid_login"})
		return
	}
	if err != nil {
		log.Printf("login failed: %v", err)
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "login_failed"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "invalid_login"})
		return
	}
	a.setSessionCookie(ctx, user.ID)
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user})
}

func (a *App) handleLogout(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey("sp_session")
	cookie.SetValue("")
	cookie.SetPath("/")
	cookie.SetExpire(time.Now().Add(-time.Hour))
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	ctx.Response.Header.SetCookie(cookie)
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMe(ctx *fasthttp.RequestCtx) {
	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
	userID, ok := a.userIDFromSession(ctx)
	if !ok {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "not_logged_in"})
		return
	}
	var user accountUser
	err := a.db.QueryRow(`
		SELECT id, email, company, created_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email, &user.Company, &user.CreatedAt)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "not_logged_in"})
		return
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user})
}

func (a *App) setSessionCookie(ctx *fasthttp.RequestCtx, userID int64) {
	expires := time.Now().Add(30 * 24 * time.Hour)
	payload := fmt.Sprintf("%d:%d:%s", userID, expires.Unix(), randomToken(16))
	sig := a.sign(payload)
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey("sp_session")
	cookie.SetValue(base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + sig)))
	cookie.SetPath("/")
	cookie.SetExpire(expires)
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	if strings.HasPrefix(a.baseURL, "https://") {
		cookie.SetSecure(true)
	}
	ctx.Response.Header.SetCookie(cookie)
}

func (a *App) userIDFromSession(ctx *fasthttp.RequestCtx) (int64, bool) {
	raw := string(ctx.Request.Header.Cookie("sp_session"))
	if raw == "" {
		return 0, false
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, false
	}
	parts := strings.Split(string(data), ":")
	if len(parts) != 4 {
		return 0, false
	}
	payload := strings.Join(parts[:3], ":")
	if subtle.ConstantTimeCompare([]byte(a.sign(payload)), []byte(parts[3])) != 1 {
		return 0, false
	}
	expires, err := parseInt64(parts[1])
	if err != nil || time.Now().Unix() > expires {
		return 0, false
	}
	userID, err := parseInt64(parts[0])
	return userID, err == nil
}

func (a *App) sign(payload string) string {
	mac := hmac.New(sha256.New, a.sessionSecret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
