package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/middleware"
)

const sessionCookie = "session"
const sessionDuration = 30 * 24 * time.Hour

type Session struct {
	ID        string
	UserID    string
	UserRole  string
}

func CreateSession(ctx context.Context, db *pgxpool.Pool, w http.ResponseWriter, r *http.Request, userID string, secure bool) error {
	id := uuid.NewString()
	_, err := db.Exec(ctx,
		`INSERT INTO sessions (id, user_id, ip_address, user_agent, expires_at)
		 VALUES ($1, $2, $3::inet, $4, NOW() + INTERVAL '30 days')`,
		id, userID, r.RemoteAddr, r.UserAgent(),
	)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	return nil
}

func GetSession(ctx context.Context, db *pgxpool.Pool, r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, nil // no session cookie
	}
	var s Session
	err = db.QueryRow(ctx,
		`UPDATE sessions SET last_seen_at = NOW()
		 FROM users
		 WHERE sessions.id = $1
		   AND sessions.expires_at > NOW()
		   AND sessions.user_id = users.id
		   AND users.is_banned = FALSE
		 RETURNING sessions.id, sessions.user_id, users.role::text`,
		cookie.Value,
	).Scan(&s.ID, &s.UserID, &s.UserRole)
	if err != nil {
		return nil, nil // expired or invalid
	}
	return &s, nil
}

func DeleteSession(ctx context.Context, db *pgxpool.Pool, w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		db.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// RequireAuth middleware — redirects to login if no valid session.
func RequireAuth(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, _ := GetSession(r.Context(), db, r)
			if s == nil {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			r = middleware.WithValue(r, middleware.UserIDKey, s.UserID)
			r = middleware.WithValue(r, middleware.UserRoleKey, s.UserRole)
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole middleware — returns 403 if role doesn't match.
func RequireRole(db *pgxpool.Pool, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireAuth(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if middleware.GetUserRole(r) != role {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// CleanupSessions deletes expired sessions. Run periodically.
func CleanupSessions(ctx context.Context, db *pgxpool.Pool) {
	db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
}
