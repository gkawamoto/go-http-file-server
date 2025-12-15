package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"

	_ "embed"
)

//go:embed index.html
var loginPage string

const (
	cookieName     = "auth_session"
	cookieDuration = 24 * time.Hour
)

// Claims represents the JWT claims
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Auth handles authentication and provides middleware
type Auth struct {
	username  string
	password  string
	jwtSecret []byte

	mux  *http.ServeMux
	tplt *template.Template
}

// NewAuth creates a new Auth instance with configured routes
func NewAuth(username, password, jwtSecret string) (*Auth, error) {
	if jwtSecret == "" {
		// Default secret - should be changed in production
		jwtSecret = "your-secret-key-change-this-in-production"
	}

	tplt, err := template.New("").Parse(loginPage)
	if err != nil {
		return nil, fmt.Errorf("error parsing login page template: %w", err)
	}

	auth := &Auth{
		username:  username,
		password:  password,
		jwtSecret: []byte(jwtSecret),

		tplt: tplt,
	}

	mux := http.NewServeMux()
	// Login routes - both /auth/ and /auth/login for convenience
	mux.HandleFunc("/", auth.handleAuthRequest)
	mux.HandleFunc("/login", auth.handleAuthRequest)
	mux.HandleFunc("/logout", auth.handleLogout)

	auth.mux = mux
	return auth, nil
}

// ServeHTTP implements http.Handler, making Auth usable as a handler
func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *Auth) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.showLoginPage(w, r)
	case http.MethodPost:
		a.handleLogin(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *Auth) showLoginPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	// Replace the error placeholder with empty string for normal display
	if err := a.tplt.Execute(w, map[string]string{}); err != nil {
		http.Error(w, "failed to render login page", http.StatusInternalServerError)
		return
	}
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Simple authentication (in production, use proper password hashing)
	if username != a.username || password != a.password {
		// Show login page with error
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		if err := a.tplt.Execute(w, map[string]string{"error": "Invalid username or password"}); err != nil {
			http.Error(w, "failed to render login page", http.StatusInternalServerError)
		}
		return
	}

	// Generate JWT token
	token, err := a.generateJWTToken(username)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	// Set authentication cookie with JWT token
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Expires:  time.Now().Add(cookieDuration),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})

	// Redirect to the original destination or files page
	redirectURL := r.FormValue("redirect")
	if redirectURL == "" {
		redirectURL = "/files/"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleLogout handles user logout (internal handler)
func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear the authentication cookie
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/auth/", http.StatusFound)
}

// RequireAuth middleware that checks for authentication
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || !a.isValidJWTToken(cookie.Value) {
			// Redirect to login with the current URL as redirect parameter
			loginURL := fmt.Sprintf("/auth/?redirect=%s", url.QueryEscape(r.URL.String()))
			http.Redirect(w, r, loginURL, http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// generateJWTToken creates a new JWT token for the user
func (a *Auth) generateJWTToken(username string) (string, error) {
	// Create the claims
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cookieDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "file-server",
			Subject:   username,
			ID:        generateRandomID(),
		},
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign and return the token
	tokenString, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// isValidJWTToken validates the JWT token
func (a *Auth) isValidJWTToken(tokenString string) bool {
	if tokenString == "" {
		return false
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// Make sure the token method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return false
	}

	// Check if token is valid and not expired
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		// Additional validation can be added here (e.g., check username)
		return claims.Username != ""
	}

	return false
}

// generateRandomID generates a random ID for JWT
func generateRandomID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
