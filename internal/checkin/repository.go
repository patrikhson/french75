package checkin

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CheckIn struct {
	ID           string
	UserID       string
	DrinkID      string
	DrinkName    string
	Score        int
	Review       string
	DrinkDate    time.Time
	Status       string
	LocationName string
	LocationLat  float64
	LocationLng  float64
	EditDeadline time.Time
	SubmittedAt  time.Time
	ExifPassed   *bool
	GPSPassed    *bool
	GPSDistanceM *int
}

type Photo struct {
	ID           string
	URL          string
	ThumbnailURL string
}

type CreateParams struct {
	UserID              string
	UserRole            string
	DrinkID             string
	Score               int
	Review              string
	DrinkDate           string // "2026-03-22"
	LocationName        string
	LocationLat         float64
	LocationLng         float64
	LocationOsmID       *int64
	LocationOsmType     *string
	SubmissionLat       *float64
	SubmissionLng       *float64
	SubmissionAccuracy  *float64
	PhotoIDs            []string
}

// Create inserts a new check-in within a transaction.
// Validates drink date, photo ownership, and links photos.
// Records EXIF and GPS validation results without blocking on failure.
func Create(ctx context.Context, db *pgxpool.Pool, p CreateParams) (string, error) {
	if len(p.PhotoIDs) == 0 {
		return "", fmt.Errorf("at least one photo is required")
	}

	drinkDate, err := time.Parse("2006-01-02", p.DrinkDate)
	if err != nil {
		return "", fmt.Errorf("invalid drink date")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)
	if drinkDate.Before(yesterday) || drinkDate.After(today) {
		return "", fmt.Errorf("drink date must be today or yesterday")
	}

	if p.Score < 0 || p.Score > 100 {
		return "", fmt.Errorf("score must be between 0 and 100")
	}
	if p.Review == "" {
		return "", fmt.Errorf("review is required")
	}
	if p.LocationName == "" {
		return "", fmt.Errorf("location is required")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Verify drink is active
	var drinkExists bool
	tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drinks WHERE id=$1 AND active=TRUE)`, p.DrinkID).Scan(&drinkExists)
	if !drinkExists {
		return "", fmt.Errorf("drink not found")
	}

	// Verify all photos belong to this user and are not yet linked to a check-in
	for _, photoID := range p.PhotoIDs {
		var ok bool
		tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM photos WHERE id=$1 AND user_id=$2 AND checkin_id IS NULL)`,
			photoID, p.UserID,
		).Scan(&ok)
		if !ok {
			return "", fmt.Errorf("invalid photo ID: %s", photoID)
		}
	}

	// Run anti-lying checks first so they can influence the final status.

	// EXIF check: compare first photo's EXIF timestamp to drink_date.
	// nil exifPassed means no EXIF data — not penalised.
	var exifTS *time.Time
	tx.QueryRow(ctx, `SELECT exif_timestamp FROM photos WHERE id=$1`, p.PhotoIDs[0]).Scan(&exifTS)

	var exifPassed *bool
	if exifTS != nil {
		diff := exifTS.UTC().Sub(drinkDate)
		if diff < 0 {
			diff = -diff
		}
		passed := diff < 24*time.Hour
		exifPassed = &passed
	}

	// GPS check: distance between device location and selected venue.
	// nil gpsPassed means no device GPS — not penalised.
	var gpsPassed *bool
	var gpsDistanceM *int
	if p.SubmissionLat != nil && p.SubmissionLng != nil {
		dist := haversineMeters(*p.SubmissionLat, *p.SubmissionLng, p.LocationLat, p.LocationLng)
		distInt := int(dist)
		gpsDistanceM = &distInt
		passed := dist <= 1000 // 1 km threshold
		gpsPassed = &passed
	}

	// Active/admin users go public unless a check with data present actually failed.
	checkFailed := (exifPassed != nil && !*exifPassed) || (gpsPassed != nil && !*gpsPassed)
	status := "pending"
	if (p.UserRole == "active" || p.UserRole == "admin") && !checkFailed {
		status = "public"
	}

	id := uuid.NewString()
	_, err = tx.Exec(ctx,
		`INSERT INTO check_ins (id, user_id, drink_id, score, review, drink_date, status,
		                        location_name, location_lat, location_lng,
		                        location_osm_id, location_osm_type,
		                        submission_lat, submission_lng, submission_accuracy,
		                        exif_timestamp, exif_check_passed, gps_check_passed, gps_distance_m,
		                        edit_deadline)
		 VALUES ($1,$2,$3,$4,$5,$6,$7::checkin_status,$8,$9,$10,$11,$12,$13,$14,$15,
		         (SELECT exif_timestamp FROM photos WHERE id=$16),$17,$18,$19,
		         NOW()+INTERVAL '24 hours')`,
		id, p.UserID, p.DrinkID, p.Score, p.Review, drinkDate, status,
		p.LocationName, p.LocationLat, p.LocationLng,
		p.LocationOsmID, p.LocationOsmType,
		p.SubmissionLat, p.SubmissionLng, p.SubmissionAccuracy,
		p.PhotoIDs[0], exifPassed, gpsPassed, gpsDistanceM,
	)
	if err != nil {
		return "", err
	}

	// Link photos to this check-in
	for i, photoID := range p.PhotoIDs {
		tx.Exec(ctx,
			`UPDATE photos SET checkin_id=$1, sort_order=$2 WHERE id=$3`,
			id, i, photoID,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// GetByID fetches a check-in with its drink name and photos.
func GetByID(ctx context.Context, db *pgxpool.Pool, id string) (*CheckIn, []Photo, error) {
	var ci CheckIn
	err := db.QueryRow(ctx,
		`SELECT c.id, c.user_id, c.drink_id, d.name, c.score, c.review,
		        c.drink_date, c.status::text, c.location_name,
		        c.location_lat, c.location_lng, c.edit_deadline, c.submitted_at,
		        c.exif_check_passed, c.gps_check_passed, c.gps_distance_m
		 FROM check_ins c JOIN drinks d ON d.id=c.drink_id
		 WHERE c.id=$1`,
		id,
	).Scan(
		&ci.ID, &ci.UserID, &ci.DrinkID, &ci.DrinkName, &ci.Score, &ci.Review,
		&ci.DrinkDate, &ci.Status, &ci.LocationName,
		&ci.LocationLat, &ci.LocationLng, &ci.EditDeadline, &ci.SubmittedAt,
		&ci.ExifPassed, &ci.GPSPassed, &ci.GPSDistanceM,
	)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.Query(ctx,
		`SELECT id, storage_path, thumbnail_path FROM photos
		 WHERE checkin_id=$1 ORDER BY sort_order`,
		id,
	)
	if err != nil {
		return &ci, nil, nil
	}
	defer rows.Close()

	var photos []Photo
	for rows.Next() {
		var p Photo
		rows.Scan(&p.ID, &p.URL, &p.ThumbnailURL)
		photos = append(photos, p)
	}
	return &ci, photos, nil
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
