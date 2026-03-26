package notification

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/mail"
)

// Service handles creating and dispatching notifications.
type Service struct {
	db      *pgxpool.Pool
	mailer  *mail.Mailer
	baseURL string
}

func NewService(db *pgxpool.Pool, mailer *mail.Mailer, baseURL string) *Service {
	return &Service{db: db, mailer: mailer, baseURL: baseURL}
}

// Notify creates an in-app notification and optionally sends an email,
// according to the user's preferences.
func (s *Service) Notify(ctx context.Context, userID, ntype, title, body, link string) {
	pref := GetPreference(ctx, s.db, userID, ntype)

	if pref.InAppEnabled {
		if err := Create(ctx, s.db, userID, ntype, title, body, link); err != nil {
			log.Printf("notification: create in-app for user %s type %s: %v", userID, ntype, err)
		}
	}

	if pref.EmailEnabled {
		// Look up the user's email (stored in username).
		var email, displayName string
		err := s.db.QueryRow(ctx,
			`SELECT username, COALESCE(display_name, username) FROM users WHERE id = $1`,
			userID,
		).Scan(&email, &displayName)
		if err != nil {
			log.Printf("notification: email lookup for user %s: %v", userID, err)
			return
		}
		emailBody := fmt.Sprintf("Hi %s,\n\n%s\n\n%s\n\n— French 75 Tracker",
			displayName, body, s.baseURL+link)
		if err := s.mailer.Send(email, title+" — French 75 Tracker", emailBody); err != nil {
			log.Printf("notification: send email to %s: %v", email, err)
		}
	}
}

// NotifyAdmins sends a notification to every non-banned admin.
func (s *Service) NotifyAdmins(ctx context.Context, ntype, title, body, link string) {
	adminIDs, err := AllAdminIDs(ctx, s.db)
	if err != nil {
		log.Printf("notification: list admins: %v", err)
		return
	}
	for _, id := range adminIDs {
		s.Notify(ctx, id, ntype, title, body, link)
	}
}

// SendDigest emails a daily pending-items digest to a single admin.
// It only sends if there is at least one pending item.
func (s *Service) SendDigest(ctx context.Context, admin AdminDigestUser, counts DigestCounts) {
	if counts.Total() == 0 {
		return
	}

	body := fmt.Sprintf(`Hi %s,

Here is your daily digest of pending items on French 75 Tracker:

  Registrations:   %d pending  — %s/admin/registrations
  Check-ins:       %d pending  — %s/admin/checkins/pending
  Drink requests:  %d pending  — %s/admin/drinks/requests
  Spam flags:      %d pending  — %s/admin/spam

— French 75 Tracker`,
		admin.DisplayName,
		counts.Registrations, s.baseURL,
		counts.Checkins, s.baseURL,
		counts.DrinkRequests, s.baseURL,
		counts.SpamFlags, s.baseURL,
	)

	if err := s.mailer.Send(admin.Username, "Daily digest — French 75 Tracker", body); err != nil {
		log.Printf("notification: digest email to %s: %v", admin.Username, err)
	}
}

// DigestCounts holds pending-item counts for the admin digest email.
type DigestCounts struct {
	Registrations int
	Checkins      int
	DrinkRequests int
	SpamFlags     int
}

func (d DigestCounts) Total() int {
	return d.Registrations + d.Checkins + d.DrinkRequests + d.SpamFlags
}

// FetchDigestCounts queries pending counts from the database.
func FetchDigestCounts(ctx context.Context, db *pgxpool.Pool) DigestCounts {
	var c DigestCounts
	db.QueryRow(ctx, `SELECT COUNT(*) FROM registration_requests WHERE status='pending' AND email_verified=TRUE AND pending_credential IS NOT NULL`).Scan(&c.Registrations)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM check_ins WHERE status='pending'`).Scan(&c.Checkins)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM drink_requests WHERE status='pending'`).Scan(&c.DrinkRequests)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM spam_flags WHERE reviewed_at IS NULL`).Scan(&c.SpamFlags)
	return c
}
