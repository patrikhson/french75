package notification

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification is a single in-app notification record.
type Notification struct {
	ID        string
	UserID    string
	Type      string
	Title     string
	Body      string
	Link      string
	Managed   bool
	CreatedAt time.Time
}

// Preference holds per-type email/in-app settings for a user.
type Preference struct {
	EmailEnabled  bool
	InAppEnabled  bool
}

// AdminDigestUser is an admin whose digest_hour matches the current run.
type AdminDigestUser struct {
	ID          string
	Username    string
	DisplayName string
	DigestHour  int
}

// Create inserts a new notification. entityID links it to a specific resource
// (e.g. a check-in ID) so it can be auto-managed when that resource is acted on.
// Pass "" for entityID when no specific entity applies.
func Create(ctx context.Context, db *pgxpool.Pool, userID, ntype, title, body, link, entityID string) error {
	var eid interface{}
	if entityID != "" {
		eid = entityID
	}
	_, err := db.Exec(ctx,
		`INSERT INTO notifications (user_id, type, title, body, link, entity_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, ntype, title, body, link, eid)
	return err
}

// AutoManageByEntity marks all unmanaged notifications of the given type and
// entity_id as managed. Call this after taking action on the underlying resource
// (e.g. approving a check-in) so admins don't have to dismiss them manually.
func AutoManageByEntity(ctx context.Context, db *pgxpool.Pool, ntype, entityID string) {
	if entityID == "" {
		return
	}
	db.Exec(ctx,
		`UPDATE notifications SET is_managed=TRUE
		 WHERE type=$1 AND entity_id=$2 AND is_managed=FALSE`,
		ntype, entityID)
}

// ListForUser returns all notifications for a user, newest first.
func ListForUser(ctx context.Context, db *pgxpool.Pool, userID string) ([]Notification, error) {
	rows, err := db.Query(ctx,
		`SELECT id, user_id, type, title, body, COALESCE(link,''), is_managed, created_at
		 FROM notifications
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ns []Notification
	for rows.Next() {
		var n Notification
		rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Link, &n.Managed, &n.CreatedAt)
		ns = append(ns, n)
	}
	return ns, nil
}

// UnreadCount returns the count of unmanaged notifications for a user.
func UnreadCount(ctx context.Context, db *pgxpool.Pool, userID string) int {
	var count int
	db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_managed = FALSE`,
		userID).Scan(&count)
	return count
}

// MarkManaged marks one notification as managed (must belong to the given user).
func MarkManaged(ctx context.Context, db *pgxpool.Pool, id, userID string) error {
	_, err := db.Exec(ctx,
		`UPDATE notifications SET is_managed = TRUE WHERE id = $1 AND user_id = $2`,
		id, userID)
	return err
}

// MarkAllManaged marks all notifications for a user as managed.
func MarkAllManaged(ctx context.Context, db *pgxpool.Pool, userID string) error {
	_, err := db.Exec(ctx,
		`UPDATE notifications SET is_managed = TRUE WHERE user_id = $1`,
		userID)
	return err
}

// GetPreference returns the preference for a specific type, defaulting to both enabled.
func GetPreference(ctx context.Context, db *pgxpool.Pool, userID, ntype string) Preference {
	p := Preference{EmailEnabled: true, InAppEnabled: true}
	db.QueryRow(ctx,
		`SELECT email_enabled, in_app_enabled
		 FROM notification_preferences
		 WHERE user_id = $1 AND type = $2`,
		userID, ntype).Scan(&p.EmailEnabled, &p.InAppEnabled)
	return p
}

// GetPreferences returns preferences for all known types, defaulting missing rows to both enabled.
func GetPreferences(ctx context.Context, db *pgxpool.Pool, userID string) map[string]Preference {
	prefs := make(map[string]Preference, len(AllTypes))
	for _, t := range AllTypes {
		prefs[t] = Preference{EmailEnabled: true, InAppEnabled: true}
	}

	rows, err := db.Query(ctx,
		`SELECT type, email_enabled, in_app_enabled
		 FROM notification_preferences
		 WHERE user_id = $1`,
		userID)
	if err != nil {
		return prefs
	}
	defer rows.Close()

	for rows.Next() {
		var t string
		var p Preference
		rows.Scan(&t, &p.EmailEnabled, &p.InAppEnabled)
		prefs[t] = p
	}
	return prefs
}

// SavePreference upserts a single preference row.
func SavePreference(ctx context.Context, db *pgxpool.Pool, userID, ntype string, emailEnabled, inAppEnabled bool) error {
	_, err := db.Exec(ctx,
		`INSERT INTO notification_preferences (user_id, type, email_enabled, in_app_enabled)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, type) DO UPDATE
		   SET email_enabled = EXCLUDED.email_enabled,
		       in_app_enabled = EXCLUDED.in_app_enabled`,
		userID, ntype, emailEnabled, inAppEnabled)
	return err
}

// AdminsForDigest returns all admin users whose digest_hour matches the given hour.
func AdminsForDigest(ctx context.Context, db *pgxpool.Pool, hour int) ([]AdminDigestUser, error) {
	rows, err := db.Query(ctx,
		`SELECT id, username, COALESCE(display_name, username)
		 FROM users
		 WHERE role = 'admin' AND digest_hour = $1 AND is_banned = FALSE`,
		hour)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []AdminDigestUser
	for rows.Next() {
		var a AdminDigestUser
		a.DigestHour = hour
		rows.Scan(&a.ID, &a.Username, &a.DisplayName)
		admins = append(admins, a)
	}
	return admins, nil
}

// AllAdminIDs returns the user IDs of all non-banned admins.
func AllAdminIDs(ctx context.Context, db *pgxpool.Pool) ([]string, error) {
	rows, err := db.Query(ctx,
		`SELECT id FROM users WHERE role = 'admin' AND is_banned = FALSE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}
