package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv    string
	Port      string
	IsProd    bool
	InviteOnly bool

	DatabaseURL string

	SessionSecret string
	CSRFKey       string

	GoogleClientID     string
	GoogleClientSecret string

	WebAuthnRPID          string
	WebAuthnRPOrigin      string
	WebAuthnRPDisplayName string

	StoragePath      string
	StorageURLPrefix string
	AppBaseURL       string

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	GooglePlacesKey string
}

func Load() (*Config, error) {
	c := &Config{
		AppEnv:                getEnv("APP_ENV", "development"),
		Port:                  getEnv("PORT", "8080"),
		InviteOnly:            getEnv("INVITE_ONLY", "true") == "true",
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		SessionSecret:         getEnv("SESSION_SECRET", ""),
		CSRFKey:               getEnv("CSRF_KEY", ""),
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		WebAuthnRPID:          getEnv("WEBAUTHN_RPID", "localhost"),
		WebAuthnRPOrigin:      getEnv("WEBAUTHN_RPORIGIN", "http://localhost:8080"),
		WebAuthnRPDisplayName: getEnv("WEBAUTHN_RPDISPLAYNAME", "French 75 Tracker"),
		StoragePath:           getEnv("STORAGE_PATH", "./photos"),
		StorageURLPrefix:      getEnv("STORAGE_URL_PREFIX", "http://localhost:8080/photos"),
		AppBaseURL:            getEnv("APP_BASE_URL", "http://localhost:8080"),
		SMTPHost:              getEnv("SMTP_HOST", ""),
		SMTPPort:              getEnv("SMTP_PORT", "587"),
		SMTPUser:              getEnv("SMTP_USER", ""),
		SMTPPass:              getEnv("SMTP_PASS", ""),
		SMTPFrom:              getEnv("SMTP_FROM", ""),
		GooglePlacesKey:       getEnv("GOOGLE_PLACES_KEY", ""),
	}

	c.IsProd = c.AppEnv == "production"

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.SessionSecret == "" || c.SessionSecret == "changeme" {
		if c.IsProd {
			return nil, fmt.Errorf("SESSION_SECRET must be set in production")
		}
		c.SessionSecret = "dev-session-secret-not-for-production"
	}
	if c.CSRFKey == "" || c.CSRFKey == "changeme" {
		if c.IsProd {
			return nil, fmt.Errorf("CSRF_KEY must be set in production")
		}
		c.CSRFKey = "dev-csrf-key-32-bytes-padded-here"
	}

	return c, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
