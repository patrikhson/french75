package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/joho/godotenv"
	"github.com/patrikhson/french75/internal/admin"
	"github.com/patrikhson/french75/internal/auth"
	"github.com/patrikhson/french75/internal/checkin"
	"github.com/patrikhson/french75/internal/config"
	"github.com/patrikhson/french75/internal/db"
	"github.com/patrikhson/french75/internal/drink"
	"github.com/patrikhson/french75/internal/feed"
	"github.com/patrikhson/french75/internal/location"
	"github.com/patrikhson/french75/internal/mail"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
	"github.com/patrikhson/french75/internal/photo"
	"github.com/patrikhson/french75/internal/social"
	"github.com/patrikhson/french75/internal/user"
)

func main() {
	// Load .env in development (ignored if file doesn't exist in production)
	_ = godotenv.Load()

	// Bootstrap: approve a registration request and send the welcome email.
	// Usage: french75 -bootstrap-approve paf@paftech.se
	// Only works when no admin users exist yet.
	if len(os.Args) == 3 && os.Args[1] == "-bootstrap-approve" {
		email := os.Args[2]
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		ctx := context.Background()
		pool, err := db.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("db: %v", err)
		}
		mailer := mail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
		var reqID string
		err = pool.QueryRow(ctx,
			`SELECT id FROM registration_requests
			 WHERE email=$1 AND status='pending' AND email_verified=true AND pending_credential IS NOT NULL`,
			email,
		).Scan(&reqID)
		if err != nil {
			log.Fatalf("no passkey-registered pending request for %s: %v", email, err)
		}
		if _, err := auth.SendApprovalEmail(ctx, pool, mailer, cfg.AppBaseURL, reqID); err != nil {
			log.Fatalf("send approval email: %v", err)
		}
		log.Printf("approved %s and sent welcome email", email)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("storage dir: %v", err)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	log.Println("database connected and migrations applied")

	// WebAuthn
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: cfg.WebAuthnRPDisplayName,
		RPID:          cfg.WebAuthnRPID,
		RPOrigins:     []string{cfg.WebAuthnRPOrigin},
	})
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}

	mailer := mail.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)

	authHandler := auth.NewHandler(pool, wa, mailer, cfg.AppBaseURL, cfg.SessionSecret, cfg.IsProd)

	// Session cleanup goroutine
	go func() {
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()
		for range t.C {
			auth.CleanupSessions(context.Background(), pool)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	authHandler.RegisterRoutes(mux)

	drinkHandler := drink.NewHandler(pool, mailer, cfg.AppBaseURL)
	drinkHandler.RegisterRoutes(mux,
		auth.RequireAuth(pool),
		auth.RequireRole(pool, "admin"),
	)

	storage := photo.NewLocalStorage(cfg.StoragePath, cfg.StorageURLPrefix)
	photoHandler := photo.NewHandler(pool, storage)
	photoHandler.RegisterRoutes(mux, auth.RequireAuth(pool))

	location.NewHandler(cfg.GooglePlacesKey).RegisterRoutes(mux, auth.RequireAuth(pool))

	checkinHandler := checkin.NewHandler(pool, cfg.StorageURLPrefix, mailer, cfg.AppBaseURL)
	checkinHandler.RegisterRoutes(mux, auth.RequireAuth(pool))

	feed.NewHandler(pool, cfg.StorageURLPrefix).RegisterRoutes(mux, auth.RequireAuth(pool))
	social.NewHandler(pool, mailer, cfg.AppBaseURL).RegisterRoutes(mux, auth.RequireAuth(pool))
	user.NewHandler(pool, cfg.StorageURLPrefix).RegisterRoutes(mux, auth.RequireAuth(pool))

	admin.NewHandler(pool, mailer, cfg.AppBaseURL).RegisterRoutes(mux, auth.RequireRole(pool, "admin"))

	notifHandler := notification.NewHandler(pool, mailer, cfg.AppBaseURL)
	notifHandler.RegisterRoutes(mux, auth.RequireAuth(pool))

	// Digest goroutine: runs every hour, sends daily digest emails to admins.
	notifSvc := notifHandler.Svc()
	go func() {
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()
		for range t.C {
			ctx := context.Background()
			hour := time.Now().UTC().Hour()
			admins, err := notification.AdminsForDigest(ctx, pool, hour)
			if err != nil {
				log.Printf("digest: list admins: %v", err)
				continue
			}
			counts := notification.FetchDigestCounts(ctx, pool)
			for _, a := range admins {
				notifSvc.SendDigest(ctx, a, counts)
			}
		}
	}()

	// Serve uploaded photos as static files
	mux.Handle("GET /photos/", http.StripPrefix("/photos/",
		http.FileServer(http.Dir(cfg.StoragePath))))

	handler := middleware.Logging(middleware.SecurityHeaders(mux))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s (env=%s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("stopped")
}
