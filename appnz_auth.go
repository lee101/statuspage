package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/valyala/fasthttp"
	"golang.org/x/crypto/argon2"
)

const appNZSharedSessionCookie = "appnz_session"

type appNZAuth struct {
	db           *sql.DB
	loginURL     string
	cookieDomain string
	secureCookie bool
}

type appNZUser struct {
	ID    string
	Email string
}

func newAppNZAuthFromEnv() *appNZAuth {
	path := strings.TrimSpace(os.Getenv("APPNZ_DATABASE_PATH"))
	if path == "" {
		return nil
	}
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Printf("app.nz auth disabled: %v", err)
		return nil
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		log.Printf("app.nz auth disabled: %v", err)
		return nil
	}
	auth := &appNZAuth{
		db:           db,
		loginURL:     strings.TrimRight(getenv("APPNZ_LOGIN_URL", "https://app.nz/login"), "/"),
		cookieDomain: getenv("APPNZ_COOKIE_DOMAIN", ".app.nz"),
		secureCookie: getenv("APPNZ_SECURE_COOKIES", "true") != "false",
	}
	if err := auth.runMigrations(); err != nil {
		db.Close()
		log.Printf("app.nz auth disabled: %v", err)
		return nil
	}
	return auth
}

func (a *appNZAuth) runMigrations() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			salt TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			disabled_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sso_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			token_hash TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			used_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user ON password_reset_tokens(user_id)`,
	}
	for _, stmt := range stmts {
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (a *appNZAuth) userFromRequest(ctx *fasthttp.RequestCtx) (*appNZUser, bool) {
	if a == nil || a.db == nil {
		return nil, false
	}
	token := string(ctx.Request.Header.Cookie(appNZSharedSessionCookie))
	if token == "" {
		return nil, false
	}
	return a.userFromToken(token)
}

func (a *appNZAuth) userFromToken(token string) (*appNZUser, bool) {
	var userID string
	var expiresAt time.Time
	err := a.db.QueryRow("SELECT user_id, expires_at FROM sso_sessions WHERE id = ?", appNZHashToken(token)).Scan(&userID, &expiresAt)
	if err != nil || time.Now().After(expiresAt) {
		return nil, false
	}
	var user appNZUser
	err = a.db.QueryRow("SELECT id, email FROM users WHERE id = ? AND disabled_at IS NULL", userID).Scan(&user.ID, &user.Email)
	return &user, err == nil
}

func (a *appNZAuth) register(email, password string) (*appNZUser, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !strings.Contains(email, "@") || len(password) < 6 {
		return nil, "", fmt.Errorf("valid email and password of at least 6 characters required")
	}
	userID := appNZRandomString(24)
	salt := appNZRandomString(32)
	_, err := a.db.Exec(
		"INSERT INTO users (id, email, password_hash, salt, created_at) VALUES (?, ?, ?, ?, ?)",
		userID, email, appNZHashPassword(password, salt), salt, time.Now().UTC(),
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, "", errAccountExists
		}
		return nil, "", err
	}
	token, err := a.createSession(userID)
	if err != nil {
		return nil, "", err
	}
	return &appNZUser{ID: userID, Email: email}, token, nil
}

func (a *appNZAuth) login(email, password string) (*appNZUser, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var userID, passwordHash, salt string
	err := a.db.QueryRow("SELECT id, password_hash, salt FROM users WHERE email = ? AND disabled_at IS NULL", email).Scan(&userID, &passwordHash, &salt)
	if err == sql.ErrNoRows || subtle.ConstantTimeCompare([]byte(appNZHashPassword(password, salt)), []byte(passwordHash)) != 1 {
		return nil, "", errInvalidLogin
	}
	if err != nil {
		return nil, "", err
	}
	token, err := a.createSession(userID)
	if err != nil {
		return nil, "", err
	}
	return &appNZUser{ID: userID, Email: email}, token, nil
}

func (a *appNZAuth) createSession(userID string) (string, error) {
	token := appNZRandomString(40)
	_, err := a.db.Exec(
		"INSERT INTO sso_sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		appNZHashToken(token), userID, time.Now().UTC(), time.Now().UTC().Add(90*24*time.Hour),
	)
	return token, err
}

func (a *appNZAuth) clearSession(token string) {
	if token != "" {
		_, _ = a.db.Exec("DELETE FROM sso_sessions WHERE id = ?", appNZHashToken(token))
	}
}

func (a *appNZAuth) createResetToken(email string) (string, string, bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var userID string
	if err := a.db.QueryRow("SELECT id FROM users WHERE email = ? AND disabled_at IS NULL", email).Scan(&userID); err == sql.ErrNoRows {
		return "", email, false, nil
	} else if err != nil {
		return "", email, false, err
	}
	token := appNZRandomString(40)
	_, err := a.db.Exec(
		"INSERT INTO password_reset_tokens (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)",
		appNZHashToken(token), userID, time.Now().UTC().Add(time.Hour), time.Now().UTC(),
	)
	return token, email, true, err
}

func (a *appNZAuth) resetPassword(email, token, password string) (*appNZUser, string, error) {
	if len(password) < 6 {
		return nil, "", fmt.Errorf("password must be at least 6 characters")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	var userID string
	var expiresAt time.Time
	var usedAt sql.NullTime
	err := a.db.QueryRow(`
		SELECT t.user_id, t.expires_at, t.used_at
		FROM password_reset_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ? AND u.email = ? AND u.disabled_at IS NULL
	`, appNZHashToken(token), email).Scan(&userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows || usedAt.Valid || time.Now().After(expiresAt) {
		return nil, "", fmt.Errorf("invalid or expired reset link")
	}
	if err != nil {
		return nil, "", err
	}
	salt := appNZRandomString(32)
	if _, err = a.db.Exec("UPDATE users SET password_hash = ?, salt = ? WHERE id = ?", appNZHashPassword(password, salt), salt, userID); err != nil {
		return nil, "", err
	}
	if _, err = a.db.Exec("UPDATE password_reset_tokens SET used_at = ? WHERE token_hash = ?", time.Now().UTC(), appNZHashToken(token)); err != nil {
		return nil, "", err
	}
	sessionToken, err := a.createSession(userID)
	if err != nil {
		return nil, "", err
	}
	return a.userByID(userID), sessionToken, nil
}

func (a *appNZAuth) userByID(userID string) *appNZUser {
	var user appNZUser
	if err := a.db.QueryRow("SELECT id, email FROM users WHERE id = ? AND disabled_at IS NULL", userID).Scan(&user.ID, &user.Email); err != nil {
		return nil
	}
	return &user
}

func (a *appNZAuth) setSessionCookie(ctx *fasthttp.RequestCtx, token string) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(appNZSharedSessionCookie)
	cookie.SetValue(token)
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSecure(a.secureCookie)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	cookie.SetMaxAge(90 * 24 * 60 * 60)
	if a.cookieDomain != "" {
		cookie.SetDomain(a.cookieDomain)
	}
	ctx.Response.Header.SetCookie(cookie)
}

func (a *appNZAuth) clearSessionCookie(ctx *fasthttp.RequestCtx) {
	cookie := fasthttp.AcquireCookie()
	defer fasthttp.ReleaseCookie(cookie)
	cookie.SetKey(appNZSharedSessionCookie)
	cookie.SetValue("")
	cookie.SetPath("/")
	cookie.SetHTTPOnly(true)
	cookie.SetSecure(a.secureCookie)
	cookie.SetSameSite(fasthttp.CookieSameSiteLaxMode)
	cookie.SetMaxAge(-1)
	if a.cookieDomain != "" {
		cookie.SetDomain(a.cookieDomain)
	}
	ctx.Response.Header.SetCookie(cookie)
}

func (a *appNZAuth) loginRedirect(next string) string {
	if a == nil {
		return ""
	}
	u := a.loginURL
	if next == "" {
		return u
	}
	return u + "?next=" + url.QueryEscape(next)
}

func appNZHashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func appNZHashPassword(password, salt string) string {
	saltBytes, _ := base64.RawURLEncoding.DecodeString(salt)
	hash := argon2.IDKey([]byte(password), saltBytes, 1, 64*1024, 4, 32)
	return base64.RawURLEncoding.EncodeToString(hash)
}

func appNZRandomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func parseJSONBody(ctx *fasthttp.RequestCtx, dst any) error {
	if err := json.Unmarshal(ctx.PostBody(), dst); err != nil {
		return err
	}
	return nil
}
