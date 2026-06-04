package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// Cookie / auth-handler tunables. Centralized here so future work that
// needs to tweak the cookie (e.g. expose the name to tests) only has
// one site to change.
const (
	sessionCookieName = "opc_session"
	// sessionCookieMaxAge mirrors the auth package's sessionTTL (7 days).
	// We hand the lifetime to the cookie so the browser garbage-collects
	// stale cookies even before the server row expires.
	sessionCookieMaxAge = 7 * 24 * 60 * 60

	// registerEnabled gates POST /api/auth/register. Phase 2 ships with
	// registration closed by default — the operator is expected to seed
	// the first admin user out-of-band (see Phase 2 README) and any
	// later self-service flow is a deliberate flip of this knob. We read
	// it from REGISTRATION_ENABLED so the value is configurable per
	// environment without rebuilding.
	registerEnabledEnv = "REGISTRATION_ENABLED"
)

// authCookiePath pins the cookie to "/api" so the static assets under
// "/" never carry the session token. This is the right blast radius:
// the cookie is only sent to the JSON API, not to the Next.js frontend
// shell (which never needs it; the API reverse-proxies through /api and
// the browser auto-attaches the cookie to those requests).
const authCookiePath = "/api"

// envBool turns a "1/true/yes/on" string into true. Anything else
// (including empty) is false. Used for the registration toggle so a
// missing env var does not silently enable self-service sign-up.
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// authLogger is the structured logger every auth handler routes its
// warnings/errors through. The constructor falls back to
// slog.Default() so callers do not need to plumb one through.
func authLogger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

// authCookieIsSecure reports whether the session cookie should set
// the Secure flag. We default to true — production runs behind HTTPS
// via the nginx reverse proxy and Secure cookies are the safe choice.
// OPC_INSECURE_COOKIES=1 opts out for local HTTP development so the
// loopback browser can still send the cookie.
func authCookieIsSecure() bool {
	return !envBool("OPC_INSECURE_COOKIES")
}

// setSessionCookie writes the session token to the response as an
// HttpOnly cookie. We keep the cookie path scoped to /api so the
// frontend HTML never sees the token, and the lifetime matches the
// server-side expiry.
func setSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		sessionCookieName,
		token,
		sessionCookieMaxAge,
		authCookiePath,
		"", // domain: host-only
		authCookieIsSecure(),
		true, // httpOnly
	)
}

// clearSessionCookie blanks the cookie. We mirror setSessionCookie's
// path/domain/Secure choices so the browser actually evicts the row.
func clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		sessionCookieName,
		"",
		-1,
		authCookiePath,
		"",
		authCookieIsSecure(),
		true,
	)
}

// sessionTokenFromCookie reads the session token from the
// request cookie. Returns "" if the cookie is missing — the caller
// (RequireAuth / Me) maps that to 401.
func sessionTokenFromCookie(c *gin.Context) string {
	v, err := c.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return v
}

// AuthHandler exposes the /api/auth/* REST surface. It is stateless
// beyond its *gorm.DB handle, which makes it trivial to construct
// once in main.go and register on both the public and the
// auth-required router groups.
type AuthHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewAuthHandler wires an AuthHandler backed by the given *gorm.DB.
// A nil logger falls back to slog.Default().
func NewAuthHandler(db *gorm.DB, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{db: db, logger: authLogger(logger)}
}

// RegisterRoutes attaches the public /api/auth/* routes:
//
//   - POST /api/auth/login     — exchange email+password for a session
//   - POST /api/auth/logout    — invalidate the current session
//   - POST /api/auth/register  — create a new user (gated by env var)
//   - GET  /api/auth/me        — return the current user from the cookie
//
// The route group is fixed at /api/auth so the auth surface is
// obvious in the URL space; the auth-required routes (e.g. /api/topics)
// live in the same /api prefix but in a separate group protected by
// RequireAuth.
func (h *AuthHandler) RegisterRoutes(r gin.IRouter) {
	authGroup := r.Group("/api/auth")
	authGroup.POST("/login", h.Login)
	authGroup.POST("/logout", h.Logout)
	authGroup.POST("/register", h.Register)
	authGroup.GET("/me", h.Me)
}

// loginRequest is the JSON body for POST /api/auth/login. The
// password field is never logged.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// registerRequest is the JSON body for POST /api/auth/register.
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// userResponse is the public projection of a User. We deliberately
// mirror the model (minus the hash) so /api/auth/me can return the
// same shape Register/Login return on success.
type userResponse struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u models.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
	}
}

// Login — POST /api/auth/login
//
// Body: {email, password}
// 200:  {user}  (and Set-Cookie: opc_session=...)
// 400:  invalid body
// 401:  unknown email or wrong password
// 500:  unexpected backend failure
//
// We collapse "unknown email" and "wrong password" into a single 401
// with the same body so the endpoint cannot be used to enumerate
// valid email addresses.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Same parser-error hiding the other handlers use — see topics.go.
		h.logger.Warn("invalid login body", "err", err.Error(), "request_id", c.GetString("request_id"))
		msg := "invalid request body"
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		if !errors.As(err, &syntaxErr) && !errors.As(err, &unmarshalErr) && !errors.Is(err, io.EOF) {
			msg = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
		return
	}

	var u models.User
	if err := h.db.WithContext(c.Request.Context()).Where("email = ?", req.Email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Run a dummy bcrypt to keep the response time roughly
			// constant against an enumeration attack. We discard the
			// error — the call only exists to consume wall-clock time.
			_, _ = auth.HashPassword(req.Password)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		h.logger.Error("login user lookup failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}
	if err := auth.VerifyPassword(u.PasswordHash, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	s, err := auth.CreateSession(h.db, u.ID)
	if err != nil {
		h.logger.Error("create session failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}
	setSessionCookie(c, s.Token)
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(u)})
}

// Logout — POST /api/auth/logout
//
// Always returns 204. Deleting a missing/stale session is a no-op in
// auth.DeleteSession, so the endpoint is idempotent.
func (h *AuthHandler) Logout(c *gin.Context) {
	token := sessionTokenFromCookie(c)
	if err := auth.DeleteSession(h.db, token); err != nil {
		// We still return 204: the cookie is being cleared, which is
		// the only thing the client can act on. A 500 here would just
		// confuse the UX ("did logout work?") with no recovery action.
		h.logger.Warn("delete session failed", "err", err.Error(), "request_id", c.GetString("request_id"))
	}
	clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

// Register — POST /api/auth/register
//
// Body: {email, password, name}
// 201:  {user}  (and Set-Cookie: opc_session=...)
// 400:  invalid body
// 403:  registration disabled (default)
// 409:  email already exists
// 500:  backend failure
//
// Registration is OFF by default (REGISTRATION_ENABLED must be set
// to 1/true/yes/on). Phase 2's first user is seeded out-of-band per
// the migration runbook — see docs/PHASE2-AUTH.md.
func (h *AuthHandler) Register(c *gin.Context) {
	if !envBool(registerEnabledEnv) {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration is disabled"})
		return
	}
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid register body", "err", err.Error(), "request_id", c.GetString("request_id"))
		msg := "invalid request body"
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		if !errors.As(err, &syntaxErr) && !errors.As(err, &unmarshalErr) && !errors.Is(err, io.EOF) {
			msg = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || req.Password == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email, password, and name are required"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		// bcrypt's only documented failure here is the 72-byte limit.
		if errors.Is(err, auth.ErrPasswordTooLong) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password is too long"})
			return
		}
		h.logger.Error("hash password failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}

	u := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Name:         req.Name,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&u).Error; err != nil {
		// Unique-email violation: gorm surfaces this as a wrapped error
		// whose text contains "UNIQUE constraint failed: users.email".
		// We match on the table+column rather than a sentinel type so
		// we do not have to import the SQLite driver here.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "users.email") {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		h.logger.Error("create user failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}

	s, err := auth.CreateSession(h.db, u.ID)
	if err != nil {
		h.logger.Error("create session after register failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}
	setSessionCookie(c, s.Token)
	c.JSON(http.StatusCreated, gin.H{"user": toUserResponse(u)})
}

// Me — GET /api/auth/me
//
// 200:  {user}
// 401:  no cookie, unknown token, or expired session
//
// This endpoint is the cheapest way for the web UI to recover the
// current user on page load (after a refresh) and to detect a
// logged-out state without redirecting.
func (h *AuthHandler) Me(c *gin.Context) {
	token := sessionTokenFromCookie(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	u, err := auth.ValidateSession(h.db, token)
	if err != nil {
		// Both ErrSessionNotFound and ErrSessionExpired map to 401
		// from the client's perspective — the recovery action is the
		// same (re-login).
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(u)})
}
