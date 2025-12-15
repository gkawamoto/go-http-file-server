package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gkawamoto/go-http-file-server/auth"
	"github.com/gkawamoto/go-http-file-server/files"
)

//go:embed static
var static embed.FS

func main() {
	port := flag.Int("port", envOrDefault("PORT", 8080), "Port to listen to")
	addr := flag.String("addr", envOrDefault("ADDR", "0.0.0.0"), "Address to listen to")

	directoryPath := flag.String("dir", envOrDefault("DIR", "."), "Directory to serve")

	username := flag.String("username", envOrDefault("USERNAME", "admin"), "Username for authentication")
	password := flag.String("password", envOrDefault("PASSWORD", "admin"), "Password for authentication")
	jwtSecret := flag.String("jwt-secret", envOrDefault("JWT_SECRET", ""), "JWT secret for authentication")

	jsonLogs := flag.Bool("json-logs", envOrDefault("JSON_LOGS", false), "Enable JSON formatted logs")

	flag.Parse()

	if *jsonLogs {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})))
	}

	mux := http.NewServeMux()

	// Create auth instance
	authHandler, err := auth.NewAuth(*username, *password, *jwtSecret)
	if err != nil {
		slog.Error("error creating auth handler", "error", err)
		os.Exit(1)
	}

	// Auth routes - all under /auth/
	mux.Handle("/auth/", http.StripPrefix("/auth", authHandler))

	// Protected routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/", http.StatusTemporaryRedirect)
	})
	mux.Handle("/files/", authHandler.RequireAuth(files.NewHandler(*directoryPath)))
	mux.Handle("/static/", http.FileServerFS(static))

	s := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", *addr, *port),
		Handler: mux,
	}

	slog.Info("starting server", "address", fmt.Sprintf("http://%s:%d", *addr, *port))

	if err := s.ListenAndServe(); err != nil {
		slog.Error("error starting server", "error", err)
	}
}

func envOrDefault[T any](key string, defaultValue T) T {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}

	switch any(defaultValue).(type) {
	case string:
		return any(value).(T)
	case int:
		result, err := strconv.Atoi(value)
		if err != nil {
			return defaultValue
		}
		return any(result).(T)
	case bool:
		switch strings.ToLower(value) {
		case "true", "t", "1", "yes", "y", "on":
			return any(true).(T)
		case "false", "f", "0", "no", "n", "off":
			return any(false).(T)
		default:
			return defaultValue
		}
	default:
		slog.Error("type not supported in envOrDefault", "type", fmt.Sprintf("%T", any(defaultValue)))
		return defaultValue
	}
}
