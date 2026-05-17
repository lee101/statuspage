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
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/bcrypt"
)

var (
	errAccountExists = errors.New("account already exists")
	errInvalidLogin  = errors.New("invalid email or password")
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
	var req authRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Company = strings.TrimSpace(req.Company)
	if req.Company == "" {
		req.Company = companyFromEmail(req.Email)
	}
	if !strings.Contains(req.Email, "@") || len(req.Password) < 6 {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "email_password_required"})
		return
	}

	if a.appAuth != nil {
		appUser, token, err := a.appAuth.register(req.Email, req.Password)
		if err != nil {
			if errors.Is(err, errAccountExists) {
				writeJSON(ctx, fasthttp.StatusConflict, map[string]any{"error": "email_already_registered"})
				return
			}
			log.Printf("app.nz register failed: %v", err)
			writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "register_failed"})
			return
		}
		user, err := a.ensureLocalUser(appUser.Email, req.Company)
		if err != nil {
			log.Printf("local user after app.nz register failed: %v", err)
			writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "register_failed"})
			return
		}
		a.setSessionCookie(ctx, user.ID)
		a.appAuth.setSessionCookie(ctx, token)
		go sendWelcomeEmailOnce(a.db, user.Email, user.Company)
		writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user, "shared": true})
		return
	}

	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
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
	go sendWelcomeEmailOnce(a.db, user.Email, user.Company)
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user})
}

func (a *App) handleLogin(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	var req authRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if a.appAuth != nil {
		appUser, token, err := a.appAuth.login(req.Email, req.Password)
		if err != nil {
			if errors.Is(err, errInvalidLogin) {
				writeJSON(ctx, fasthttp.StatusUnauthorized, map[string]any{"error": "invalid_login"})
				return
			}
			log.Printf("app.nz login failed: %v", err)
			writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "login_failed"})
			return
		}
		user, err := a.ensureLocalUser(appUser.Email, companyFromEmail(appUser.Email))
		if err != nil {
			log.Printf("local user after app.nz login failed: %v", err)
			writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "login_failed"})
			return
		}
		a.setSessionCookie(ctx, user.ID)
		a.appAuth.setSessionCookie(ctx, token)
		writeJSON(ctx, fasthttp.StatusOK, map[string]any{"user": user, "shared": true})
		return
	}

	if a.db == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "database_not_configured"})
		return
	}
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
	if a.appAuth != nil {
		token := string(ctx.Request.Header.Cookie(appNZSharedSessionCookie))
		a.appAuth.clearSession(token)
		a.appAuth.clearSessionCookie(ctx)
	}
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

func (a *App) handleForgotPassword(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.appAuth == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "shared_login_not_configured"})
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	token, email, found, err := a.appAuth.createResetToken(req.Email)
	if err != nil {
		log.Printf("password reset token failed: %v", err)
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "reset_failed"})
		return
	}
	if found {
		resetURL := a.baseURL + "/account?reset_token=" + token + "&email=" + email
		body := fmt.Sprintf(`<p>Use this link to reset your app.nz password for statuspage.app.nz:</p><p><a href="%s">%s</a></p><p>This link expires in one hour.</p>`, htmlEscape(resetURL), htmlEscape(resetURL))
		if err := sendEmail(email, "Reset your app.nz password", body); err != nil {
			log.Printf("password reset email failed: %v", err)
		}
	}
	writeJSON(ctx, fasthttp.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleResetPassword(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		writeJSON(ctx, fasthttp.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
		return
	}
	if a.appAuth == nil {
		writeJSON(ctx, fasthttp.StatusServiceUnavailable, map[string]any{"error": "shared_login_not_configured"})
		return
	}
	var req struct {
		Email    string `json:"email"`
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	appUser, token, err := a.appAuth.resetPassword(req.Email, req.Token, req.Password)
	if err != nil {
		writeJSON(ctx, fasthttp.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	user, err := a.ensureLocalUser(appUser.Email, companyFromEmail(appUser.Email))
	if err != nil {
		writeJSON(ctx, fasthttp.StatusInternalServerError, map[string]any{"error": "reset_failed"})
		return
	}
	a.setSessionCookie(ctx, user.ID)
	a.appAuth.setSessionCookie(ctx, token)
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
	if raw != "" {
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err == nil {
			parts := strings.Split(string(data), ":")
			if len(parts) == 4 {
				payload := strings.Join(parts[:3], ":")
				if subtle.ConstantTimeCompare([]byte(a.sign(payload)), []byte(parts[3])) == 1 {
					expires, err := parseInt64(parts[1])
					if err == nil && time.Now().Unix() <= expires {
						userID, err := parseInt64(parts[0])
						if err == nil {
							return userID, true
						}
					}
				}
			}
		}
	}
	if a.appAuth != nil {
		if appUser, ok := a.appAuth.userFromRequest(ctx); ok {
			user, err := a.ensureLocalUser(appUser.Email, companyFromEmail(appUser.Email))
			if err == nil {
				a.setSessionCookie(ctx, user.ID)
				return user.ID, true
			}
			log.Printf("local user from app.nz session failed: %v", err)
		}
	}
	return 0, false
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

func (a *App) ensureLocalUser(email, company string) (accountUser, error) {
	var user accountUser
	if a.db == nil {
		return user, fmt.Errorf("database_not_configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	company = strings.TrimSpace(company)
	if company == "" {
		company = companyFromEmail(email)
	}
	err := a.db.QueryRow(`
		SELECT id, email, company, created_at
		FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.Company, &user.CreatedAt)
	if err == nil {
		if user.Company == "" && company != "" {
			_, _ = a.db.Exec(`UPDATE users SET company = $1, updated_at = now() WHERE id = $2`, company, user.ID)
			user.Company = company
		}
		return user, nil
	}
	if err != sql.ErrNoRows {
		return user, err
	}
	placeholderHash := "appnz:" + randomToken(12)
	err = a.db.QueryRow(`
		INSERT INTO users (email, company, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, company, created_at
	`, email, company, placeholderHash).Scan(&user.ID, &user.Email, &user.Company, &user.CreatedAt)
	return user, err
}

func companyFromEmail(email string) string {
	local, _, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok || local == "" {
		return "app.nz user"
	}
	local = strings.ReplaceAll(local, ".", " ")
	local = strings.ReplaceAll(local, "-", " ")
	local = strings.ReplaceAll(local, "_", " ")
	return strings.TrimSpace(local)
}

func (a *App) appNZLoginURL() string {
	if a.appAuth == nil {
		return ""
	}
	return a.appAuth.loginRedirect(a.baseURL + "/account")
}
