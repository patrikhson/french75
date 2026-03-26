package admin

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Counts struct {
	PendingRegistrations int
	PendingCheckins      int
	PendingDrinkRequests int
	UnreviewedFlags      int
}

func GetCounts(ctx context.Context, db *pgxpool.Pool) Counts {
	var c Counts
	db.QueryRow(ctx, `SELECT COUNT(*) FROM registration_requests WHERE status='pending' AND email_verified=TRUE AND pending_credential IS NOT NULL`).Scan(&c.PendingRegistrations)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM check_ins WHERE status='pending'`).Scan(&c.PendingCheckins)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM drink_requests WHERE status='pending'`).Scan(&c.PendingDrinkRequests)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM spam_flags WHERE reviewed_at IS NULL`).Scan(&c.UnreviewedFlags)
	return c
}

// Registration requests

type RegistrationRequest struct {
	ID              string
	Name            string
	Email           string
	EmailVerified   bool
	Status          string
	CreatedAt       time.Time
}

func ListPendingRegistrations(ctx context.Context, db *pgxpool.Pool) ([]RegistrationRequest, error) {
	rows, err := db.Query(ctx,
		`SELECT id, name, email, email_verified, status::text, created_at
		 FROM registration_requests
		 WHERE status='pending' AND email_verified=TRUE AND pending_credential IS NOT NULL
		 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []RegistrationRequest
	for rows.Next() {
		var r RegistrationRequest
		rows.Scan(&r.ID, &r.Name, &r.Email, &r.EmailVerified, &r.Status, &r.CreatedAt)
		reqs = append(reqs, r)
	}
	return reqs, nil
}

func RejectRegistration(ctx context.Context, db *pgxpool.Pool, requestID string) error {
	_, err := db.Exec(ctx,
		`UPDATE registration_requests SET status='rejected' WHERE id=$1 AND status='pending'`,
		requestID)
	return err
}

// Pending check-ins

type PendingCheckin struct {
	ID                string
	UserName          string
	UserRole          string
	DrinkName         string
	Score             int
	Review            string
	DrinkDate         time.Time
	LocationLat       float64
	LocationLng       float64
	LocationName      string
	ExifPassed        *bool
	GPSPassed         *bool
	GPSDistanceM      *int
	PhotoGPSPassed    *bool
	PhotoGPSDistanceM *int
	SubmittedAt       time.Time
}

func ListPendingCheckins(ctx context.Context, db *pgxpool.Pool) ([]PendingCheckin, error) {
	rows, err := db.Query(ctx,
		`SELECT c.id, COALESCE(u.display_name, u.username), u.role::text,
		        d.name, c.score, LEFT(c.review, 200),
		        c.drink_date, c.location_lat, c.location_lng, c.location_name,
		        c.exif_check_passed, c.gps_check_passed, c.gps_distance_m,
		        ph.exif_gps_lat, ph.exif_gps_lng,
		        c.submitted_at
		 FROM check_ins c
		 JOIN users u ON u.id = c.user_id
		 JOIN drinks d ON d.id = c.drink_id
		 LEFT JOIN LATERAL (
		     SELECT exif_gps_lat, exif_gps_lng
		     FROM photos WHERE checkin_id=c.id ORDER BY sort_order LIMIT 1
		 ) ph ON true
		 WHERE c.status = 'pending'
		 ORDER BY c.submitted_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PendingCheckin
	for rows.Next() {
		var p PendingCheckin
		var photoLat, photoLng *float64
		rows.Scan(
			&p.ID, &p.UserName, &p.UserRole,
			&p.DrinkName, &p.Score, &p.Review,
			&p.DrinkDate, &p.LocationLat, &p.LocationLng, &p.LocationName,
			&p.ExifPassed, &p.GPSPassed, &p.GPSDistanceM,
			&photoLat, &photoLng,
			&p.SubmittedAt,
		)
		if photoLat != nil && photoLng != nil {
			dist := int(haversineMeters(*photoLat, *photoLng, p.LocationLat, p.LocationLng))
			p.PhotoGPSDistanceM = &dist
			passed := dist <= 1000
			p.PhotoGPSPassed = &passed
		}
		items = append(items, p)
	}
	return items, nil
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000
	const toRad = math.Pi / 180
	φ1, φ2 := lat1*toRad, lat2*toRad
	Δφ := (lat2 - lat1) * toRad
	Δλ := (lng2 - lng1) * toRad
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ApproveCheckin sets a pending check-in to public. Returns the check-in owner's user ID.
func ApproveCheckin(ctx context.Context, db *pgxpool.Pool, checkinID string) (ownerID string, err error) {
	err = db.QueryRow(ctx,
		`UPDATE check_ins SET status='public'::checkin_status
		 WHERE id=$1 AND status='pending'
		 RETURNING user_id`,
		checkinID).Scan(&ownerID)
	return ownerID, err
}

// RejectCheckin sets a pending check-in to spam. Returns the check-in owner's user ID.
func RejectCheckin(ctx context.Context, db *pgxpool.Pool, checkinID string) (ownerID string, err error) {
	err = db.QueryRow(ctx,
		`UPDATE check_ins SET status='spam'::checkin_status
		 WHERE id=$1 AND status='pending'
		 RETURNING user_id`,
		checkinID).Scan(&ownerID)
	return ownerID, err
}

// Spam flags

type FlaggedCheckin struct {
	ID           string
	UserName     string
	DrinkName    string
	Score        int
	Review       string
	LocationName string
	Status       string
	FlagCount    int
	SubmittedAt  time.Time
}

func ListFlaggedCheckins(ctx context.Context, db *pgxpool.Pool) ([]FlaggedCheckin, error) {
	rows, err := db.Query(ctx,
		`SELECT c.id, COALESCE(u.display_name, u.username), d.name,
		        c.score, LEFT(c.review, 200), c.location_name,
		        c.status::text, c.flag_count, c.submitted_at
		 FROM check_ins c
		 JOIN users u ON u.id = c.user_id
		 JOIN drinks d ON d.id = c.drink_id
		 WHERE c.flag_count > 0
		 ORDER BY c.flag_count DESC, c.submitted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FlaggedCheckin
	for rows.Next() {
		var f FlaggedCheckin
		rows.Scan(&f.ID, &f.UserName, &f.DrinkName,
			&f.Score, &f.Review, &f.LocationName,
			&f.Status, &f.FlagCount, &f.SubmittedAt)
		items = append(items, f)
	}
	return items, nil
}

func ClearSpam(ctx context.Context, db *pgxpool.Pool, checkinID string) error {
	_, err := db.Exec(ctx,
		`UPDATE check_ins SET status='public'::checkin_status, flag_count=0 WHERE id=$1`,
		checkinID)
	return err
}

// Users

type User struct {
	ID           string
	Username     string
	DisplayName  string
	Role         string
	CheckinCount int
	IsBanned     bool
	CreatedAt    time.Time
}

func ListUsers(ctx context.Context, db *pgxpool.Pool) ([]User, error) {
	rows, err := db.Query(ctx,
		`SELECT id, username, COALESCE(display_name, username),
		        role::text, checkin_count, is_banned, created_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.DisplayName,
			&u.Role, &u.CheckinCount, &u.IsBanned, &u.CreatedAt)
		users = append(users, u)
	}
	return users, nil
}

func SetUserBan(ctx context.Context, db *pgxpool.Pool, userID string, banned bool) error {
	_, err := db.Exec(ctx, `UPDATE users SET is_banned=$2 WHERE id=$1`, userID, banned)
	return err
}

func SetUserRole(ctx context.Context, db *pgxpool.Pool, userID, role string) error {
	_, err := db.Exec(ctx,
		`UPDATE users SET role=$2::user_role WHERE id=$1`, userID, role)
	return err
}
