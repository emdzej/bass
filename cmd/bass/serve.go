package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/emdzej/bass/internal/apps"
	"github.com/emdzej/bass/internal/auth"
	"github.com/emdzej/bass/internal/changes"
	"github.com/emdzej/bass/internal/cors"
	"github.com/emdzej/bass/internal/devices"
	"github.com/emdzej/bass/internal/discovery"
	"github.com/emdzej/bass/internal/pairing"
	"github.com/emdzej/bass/internal/storage"
	syncpkg "github.com/emdzej/bass/internal/sync"
)

type serveConfig struct {
	Addr            string
	PublicBaseURL   string
	PublicWSBaseURL string
	DBPath          string
	OIDCIssuer      string
	OIDCAudience    string
	OIDCClientID    string
	TokenTTL        time.Duration
	RefreshTTL      time.Duration
	StateTTL        time.Duration
	MaxValueBytes   int
	MaxBatchItems   int
	AllowedOrigins  []string
	TLSCert         string
	TLSKey          string
	NoAuth          bool
}

func runServe(args []string) int {
	cfg := serveConfig{
		Addr:            envOr("BASS_ADDR", ":8080"),
		PublicBaseURL:   envOr("BASS_PUBLIC_BASE_URL", "http://localhost:8080"),
		PublicWSBaseURL: os.Getenv("BASS_PUBLIC_WS_BASE_URL"),
		DBPath:          envOr("BASS_DB_PATH", "bass.db"),
		OIDCIssuer:      os.Getenv("BASS_OIDC_ISSUER"),
		OIDCAudience:    os.Getenv("BASS_OIDC_AUDIENCE"),
		OIDCClientID:    os.Getenv("BASS_OIDC_CLIENT_ID"),
		TokenTTL:        envDuration("BASS_TOKEN_TTL", 24*time.Hour),
		RefreshTTL:      envDuration("BASS_REFRESH_TTL", 30*24*time.Hour),
		StateTTL:        envDuration("BASS_PAIR_STATE_TTL", 5*time.Minute),
		MaxValueBytes:   envInt("BASS_MAX_VALUE_BYTES", 65536),
		MaxBatchItems:   envInt("BASS_MAX_BATCH_ITEMS", 1024),
		TLSCert:         os.Getenv("BASS_TLS_CERT"),
		TLSKey:          os.Getenv("BASS_TLS_KEY"),
		NoAuth:          envBool("BASS_NO_AUTH", false),
	}
	if v := os.Getenv("BASS_ALLOWED_ORIGINS"); v != "" {
		cfg.AllowedOrigins = splitCSV(v)
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.PublicBaseURL, "public-base-url", cfg.PublicBaseURL, "public HTTP base URL for discovery + pair callback")
	fs.StringVar(&cfg.PublicWSBaseURL, "public-ws-base-url", cfg.PublicWSBaseURL, "public WS base URL (derived from public-base-url if empty)")
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "SQLite database path")
	fs.StringVar(&cfg.OIDCIssuer, "oidc-issuer", cfg.OIDCIssuer, "OIDC issuer URL")
	fs.StringVar(&cfg.OIDCAudience, "oidc-audience", cfg.OIDCAudience, "expected JWT audience")
	fs.StringVar(&cfg.OIDCClientID, "oidc-client-id", cfg.OIDCClientID, "OAuth public client id for pairing (PKCE; no secret required)")
	fs.DurationVar(&cfg.TokenTTL, "token-ttl", cfg.TokenTTL, "sync token lifetime")
	fs.DurationVar(&cfg.RefreshTTL, "refresh-ttl", cfg.RefreshTTL, "refresh token lifetime")
	fs.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "PEM cert path (set with --tls-key for native TLS)")
	fs.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "PEM key path")
	fs.BoolVar(&cfg.NoAuth, "no-auth", cfg.NoAuth, "DEV ONLY: disable OIDC verification, mint tokens for synthetic user")
	originsFlag := fs.String("allowed-origins", strings.Join(cfg.AllowedOrigins, ","), "CSV of CORS origin glob patterns (host:port)")
	_ = fs.Parse(args)
	if *originsFlag != "" {
		cfg.AllowedOrigins = splitCSV(*originsFlag)
	}
	if cfg.PublicWSBaseURL == "" {
		cfg.PublicWSBaseURL = httpToWS(cfg.PublicBaseURL)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if !cfg.NoAuth && cfg.OIDCIssuer == "" {
		logger.Error("BASS_OIDC_ISSUER (or --oidc-issuer) is required; pass --no-auth for local dev only")
		return 2
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		logger.Error("--tls-cert and --tls-key must both be set, or both omitted")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		logger.Error("open db", "err", err)
		return 1
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		logger.Error("migrate", "err", err)
		return 1
	}

	var verifier *auth.Verifier
	if cfg.NoAuth {
		logger.Warn("AUTH DISABLED — admin API is open and pairing mints anonymous devices. Do not use in production.")
	} else {
		v, err := auth.NewVerifier(ctx, cfg.OIDCIssuer, cfg.OIDCAudience)
		if err != nil {
			logger.Error("oidc init", "err", err)
			return 1
		}
		verifier = v
	}

	appsStore := &apps.Store{DB: db}
	devStore := &devices.Store{DB: db, TokenTTL: cfg.TokenTTL, RefreshTTL: cfg.RefreshTTL}
	syncStore := &syncpkg.Store{DB: db}
	pairCache := pairing.NewCache(cfg.StateTTL)
	hub := changes.NewHub()

	pairAPI := &pairing.API{
		Apps:        appsStore,
		Devices:     devStore,
		Verifier:    verifier,
		Cache:       pairCache,
		ClientID:    cfg.OIDCClientID,
		CallbackURL: cfg.PublicBaseURL + "/v1/pair/callback",
	}
	syncAPI := &syncpkg.API{
		Store:         syncStore,
		Apps:          appsStore,
		Devices:       devStore,
		Publisher:     hub,
		MaxValueBytes: cfg.MaxValueBytes,
		MaxBatchItems: cfg.MaxBatchItems,
	}
	changesHandler := &changes.Handler{
		Hub:                   hub,
		Devices:               devStore,
		Logger:                logger,
		AllowedOriginPatterns: cfg.AllowedOrigins,
	}
	appsAPI := &apps.API{Store: appsStore, Verifier: verifier}
	devAPI := &devices.API{Store: devStore, Verifier: verifier}

	go pairAPI.RunSweeper(ctx, 60)

	mux := http.NewServeMux()
	appsAPI.Register(mux)
	devAPI.Register(mux)
	pairAPI.Register(mux)
	syncAPI.Register(mux)
	changesHandler.Register(mux)
	mux.Handle("GET /.well-known/bass-config", discovery.Handler(
		cfg.PublicBaseURL,
		cfg.PublicWSBaseURL,
		verifier,
		discovery.Limits{MaxValueBytes: cfg.MaxValueBytes, MaxBatchItems: cfg.MaxBatchItems},
	))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	corsMW := &cors.Middleware{AllowedOriginPatterns: cfg.AllowedOrigins}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           corsMW.Wrap(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsEnabled := cfg.TLSCert != ""
	go func() {
		logger.Info("listening",
			"addr", cfg.Addr,
			"tls", tlsEnabled,
			"issuer", cfg.OIDCIssuer,
			"public_base_url", cfg.PublicBaseURL,
		)
		var err error
		if tlsEnabled {
			err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server crashed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	return 0
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func httpToWS(httpURL string) string {
	if strings.HasPrefix(httpURL, "https://") {
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	}
	if strings.HasPrefix(httpURL, "http://") {
		return "ws://" + strings.TrimPrefix(httpURL, "http://")
	}
	return httpURL
}
