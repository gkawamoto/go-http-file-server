package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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

// getJWTSecret returns the JWT secret from environment or default
func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Default secret - should be changed in production
		secret = "your-secret-key-change-this-in-production"
	}
	return []byte(secret)
}

// Claims represents the JWT claims
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// NewHandler creates a new auth handler
func NewHandler(username, password string) http.Handler {
	return &handler{
		username: username,
		password: password,
	}
}

type handler struct {
	username string
	password string
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.showLoginPage(w, r)
	case http.MethodPost:
		h.handleLogin(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *handler) showLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	// Replace the error placeholder with empty string for normal display
	cleanPage := strings.ReplaceAll(loginPage, "{{error}}", "")
	w.Write([]byte(cleanPage))
}

func (h *handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Simple authentication (in production, use proper password hashing)
	if username == h.username && password == h.password {
		// Generate JWT token
		token, err := generateJWTToken(username)
		if err != nil {
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}

		// Set authentication cookie with JWT token
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    token,
			Expires:  time.Now().Add(cookieDuration),
			HttpOnly: true,
			Secure:   false, // Set to true in production with HTTPS
			SameSite: http.SameSiteLaxMode,
		})

		// Redirect to the original destination or files page
		redirectURL := r.FormValue("redirect")
		if redirectURL == "" {
			redirectURL = "/files/"
		}
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
	} else {
		// Show login page with error
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		errorPage := strings.ReplaceAll(loginPage, "{{error}}", "<div class=\"error\">Invalid username or password</div>")
		w.Write([]byte(errorPage))
	}
}

// HandleLogout handles user logout
func HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear the authentication cookie
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/auth", http.StatusTemporaryRedirect)
}

// RequireAuth middleware that checks for authentication
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || !isValidJWTToken(cookie.Value) {
			// Redirect to login with the current URL as redirect parameter
			loginURL := fmt.Sprintf("/auth?redirect=%s", url.QueryEscape(r.URL.String()))
			http.Redirect(w, r, loginURL, http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// generateJWTToken creates a new JWT token for the user
func generateJWTToken(username string) (string, error) {
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
	tokenString, err := token.SignedString(getJWTSecret())
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// isValidJWTToken validates the JWT token
func isValidJWTToken(tokenString string) bool {
	if tokenString == "" {
		return false
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Make sure the token method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getJWTSecret(), nil
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

// GetUserFromToken extracts user information from JWT token in request
func GetUserFromToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", fmt.Errorf("no auth cookie found")
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getJWTSecret(), nil
	})

	if err != nil {
		return "", fmt.Errorf("invalid token: %v", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims.Username, nil
	}

	return "", fmt.Errorf("invalid token claims")
}
