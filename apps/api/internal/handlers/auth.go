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

	// registerEnabled gates POST /api/auth/register. Registration is
	// OPEN by default so a fresh dev/MVP deployment can create its
	// first user through the signup form without manual seeding. To
	// close registration, set REGISTRATION_ENABLED to 0/false/no/off
	// (an explicit falsy value); unset means enabled. We keep the
	// environment-variable indirection so a production operator can
	// lock the endpoint down without rebuilding.
	registerEnabledEnv = "REGISTRATION_ENABLED"
)

// authCookiePath pins the cookie to "/api" so the static assets under
// "/" never carry the session token. This is the right blast radius:
// the cookie is only sent to the JSON API, not to the Next.js frontend
// shell (which never needs it; the API reverse-proxies through /api and
// the browser auto-attaches the cookie to those requests).
const authCookiePath = "/api"

// envBool turns a "1/true/yes/on" string into true. Anything else
// (including empty) is false. Used for opt-in feature flags that
// should default to disabled when the operator does not set the
// variable.
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// envBoolDefaultTrue is the inverse of envBool: it returns false only
// when the variable is *explicitly* set to a falsy value
// (0/false/no/off). Unset, empty, or any other value is treated as
// true. Used for the registration toggle so a fresh deployment does
// not need to set an env var to expose the signup form.
func envBoolDefaultTrue(name string) bool {
	val, ok := os.LookupEnv(name)
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
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

// bindErrorClass classifies a body-binding error into a stable short
// label so we can log the *kind* of failure (syntax vs type mismatch
// vs EOF vs other) without echoing the raw err.Error() string, which
// for a json.UnmarshalTypeError can include the offending JSON
// fragment from the request body. The auth endpoints take an email
// and a password; both are sensitive enough that we should not log
// arbitrary fragments of either, even on a 400 path.
func bindErrorClass(err error) string {
	var syntaxErr *json.SyntaxError
	var unmarshalErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxErr):
		return "json_syntax"
	case errors.As(err, &unmarshalErr):
		return "json_type_mismatch"
	case errors.Is(err, io.EOF):
		return "empty_body"
	default:
		return "validation"
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
		// We log only the *class* of the binding error (json_syntax /
		// json_type_mismatch / empty_body / validation) rather than
		// err.Error(), because the raw bcrypt/json error string can
		// include the raw value the client sent in the email or
		// password fields (for a type mismatch, json.UnmarshalTypeError
		// formats the offending JSON token). Even a 400-only leak of
		// the typed-wrong password into the log is worth avoiding.
		h.logger.Warn("invalid login body", "class", bindErrorClass(err), "request_id", c.GetString("request_id"))
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
// 403:  registration disabled (REGISTRATION_ENABLED=0|false|no|off)
// 409:  email already exists
// 500:  backend failure
//
// Registration is ON by default. To lock the endpoint down, set
// REGISTRATION_ENABLED to a falsy value (0/false/no/off); unset or
// empty means enabled. The same env var is still honored for the
// opt-out path, just inverted. See docs/PHASE2-AUTH.md.
func (h *AuthHandler) Register(c *gin.Context) {
	if !envBoolDefaultTrue(registerEnabledEnv) {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration is disabled"})
		return
	}
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Same scrubbing rationale as Login: log the class, not the raw
		// err.Error(), so the password/email tokens never reach the log
		// even on the 400 path.
		h.logger.Warn("invalid register body", "class", bindErrorClass(err), "request_id", c.GetString("request_id"))
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
	// Pre-insert dedup check: doing this here keeps the 409 path
	// deterministic across drivers (SQLite, Postgres, MySQL) rather
	// than coupling to a driver-specific error text. The unique index
	// on users.email is still the source of truth — if two requests
	// race past this check, the Create below will fail and we map the
	// gorm.ErrDuplicatedKey return into the same 409. Either path lands
	// in the same response, but the common case stays cheap and
	// portable.
	var existing models.User
	dedupErr := h.db.WithContext(c.Request.Context()).
		Select("id").
		Where("email = ?", req.Email).
		First(&existing).Error
	switch {
	case dedupErr == nil:
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	case errors.Is(dedupErr, gorm.ErrRecordNotFound):
		// happy path — proceed to Create
	default:
		h.logger.Error("register dedup lookup failed", "err", dedupErr.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&u).Error; err != nil {
		// Race-safety net: another request may have inserted the same
		// email between our dedup lookup and this Create. gorm.Config
		// has TranslateError enabled, so the driver-specific unique
		// constraint violation is surfaced as the portable
		// gorm.ErrDuplicatedKey sentinel — we no longer match on the
		// SQLite-only "UNIQUE constraint failed" English text.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
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
