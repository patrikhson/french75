package drink

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// slugify converts a drink name to a URL-friendly slug.
func slugify(name string) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s := re.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}

// DrinkStats is the drink plus aggregate check-in stats.
type DrinkStats struct {
	ID           string
	Slug         string
	Name         string
	Description  string
	CheckinCount int
	AvgScore     float64
	MinScore     int
	MaxScore     int
}

// VenueForDrink is one venue's stats for a specific drink.
type VenueForDrink struct {
	Name         string
	Lat          float64
	Lng          float64
	CheckinCount int
	AvgScore     float64
	BestScore    int
}

// TopUserForDrink is a user's stats for a specific drink.
type TopUserForDrink struct {
	UserID    string
	UserName  string
	Count     int
	AvgScore  float64
	BestScore int
}

// CheckinRowForDrink is one check-in shown on the drink detail page.
type CheckinRowForDrink struct {
	ID           string
	UserID       string
	UserName     string
	Score        int
	DrinkDate    time.Time
	LocationName string
	Review       string
}

// GetDrinkStats returns a drink with aggregate check-in stats, looked up by slug.
func GetDrinkStats(ctx context.Context, db *pgxpool.Pool, slug string) (*DrinkStats, error) {
	var s DrinkStats
	err := db.QueryRow(ctx, `
		SELECT d.id, d.slug, d.name, COALESCE(d.description,''),
		       COUNT(c.id),
		       COALESCE(ROUND(AVG(c.score)::numeric,1), 0),
		       COALESCE(MIN(c.score), 0),
		       COALESCE(MAX(c.score), 0)
		FROM drinks d
		LEFT JOIN check_ins c ON c.drink_id=d.id AND c.status='public'
		WHERE d.slug=$1
		GROUP BY d.id, d.slug, d.name, d.description`, slug,
	).Scan(&s.ID, &s.Slug, &s.Name, &s.Description,
		&s.CheckinCount, &s.AvgScore, &s.MinScore, &s.MaxScore)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// allowedVenueSort maps URL param values to safe SQL column expressions.
var allowedVenueSort = map[string]string{
	"name":     "location_name",
	"checkins": "checkin_count",
	"avg":      "avg_score",
	"best":     "best_score",
}

// VenuesForDrink returns all venues where this drink was checked in.
func VenuesForDrink(ctx context.Context, db *pgxpool.Pool, drinkID, sort, order string) ([]VenueForDrink, error) {
	col, ok := allowedVenueSort[sort]
	if !ok {
		col = "avg_score"
	}
	dir := "DESC"
	if order == "asc" {
		dir = "ASC"
	}
	rows, err := db.Query(ctx, `
		SELECT location_name,
		       AVG(location_lat), AVG(location_lng),
		       COUNT(*)                        AS checkin_count,
		       ROUND(AVG(score)::numeric,1)    AS avg_score,
		       MAX(score)                      AS best_score
		FROM check_ins
		WHERE drink_id=$1 AND status='public'
		GROUP BY location_name
		ORDER BY `+col+` `+dir, drinkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VenueForDrink
	for rows.Next() {
		var v VenueForDrink
		rows.Scan(&v.Name, &v.Lat, &v.Lng, &v.CheckinCount, &v.AvgScore, &v.BestScore)
		out = append(out, v)
	}
	return out, nil
}

// allowedUserSort maps URL param values to safe SQL column expressions.
var allowedUserSort = map[string]string{
	"user":     "username",
	"checkins": "cnt",
	"avg":      "avg_score",
	"best":     "best_score",
}

// TopUsersForDrink returns users with the most/best check-ins for this drink.
func TopUsersForDrink(ctx context.Context, db *pgxpool.Pool, drinkID, sort, order string) ([]TopUserForDrink, error) {
	col, ok := allowedUserSort[sort]
	if !ok {
		col = "avg_score"
	}
	dir := "DESC"
	if order == "asc" {
		dir = "ASC"
	}
	rows, err := db.Query(ctx, `
		SELECT u.id, COALESCE(u.display_name, u.username) AS username,
		       COUNT(*)                     AS cnt,
		       ROUND(AVG(c.score)::numeric,1) AS avg_score,
		       MAX(c.score)                 AS best_score
		FROM check_ins c
		JOIN users u ON u.id=c.user_id
		WHERE c.drink_id=$1 AND c.status='public'
		GROUP BY u.id, u.display_name, u.username
		ORDER BY `+col+` `+dir+`
		LIMIT 10`, drinkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopUserForDrink
	for rows.Next() {
		var u TopUserForDrink
		rows.Scan(&u.UserID, &u.UserName, &u.Count, &u.AvgScore, &u.BestScore)
		out = append(out, u)
	}
	return out, nil
}

// RecentCheckinsForDrink returns the latest public check-ins for this drink.
func RecentCheckinsForDrink(ctx context.Context, db *pgxpool.Pool, drinkID string) ([]CheckinRowForDrink, error) {
	rows, err := db.Query(ctx, `
		SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
		       c.score, c.drink_date, c.location_name, LEFT(c.review,200)
		FROM check_ins c
		JOIN users u ON u.id=c.user_id
		WHERE c.drink_id=$1 AND c.status='public'
		ORDER BY c.submitted_at DESC
		LIMIT 30`, drinkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CheckinRowForDrink
	for rows.Next() {
		var ci CheckinRowForDrink
		rows.Scan(&ci.ID, &ci.UserID, &ci.UserName,
			&ci.Score, &ci.DrinkDate, &ci.LocationName, &ci.Review)
		out = append(out, ci)
	}
	return out, nil
}

type Drink struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Recipe      string
	Active      bool
	CreatedAt   time.Time
}

type Request struct {
	ID          string
	RequestedBy string
	Name        string
	Description string
	Reason      string
	Status      string
	CreatedAt   time.Time
}

func ListActive(ctx context.Context, db *pgxpool.Pool) ([]Drink, error) {
	rows, err := db.Query(ctx,
		`SELECT id, slug, name, COALESCE(description,''), COALESCE(recipe,''), active, created_at
		 FROM drinks WHERE active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var drinks []Drink
	for rows.Next() {
		var d Drink
		if err := rows.Scan(&d.ID, &d.Slug, &d.Name, &d.Description, &d.Recipe, &d.Active, &d.CreatedAt); err != nil {
			return nil, err
		}
		drinks = append(drinks, d)
	}
	return drinks, nil
}

func GetByID(ctx context.Context, db *pgxpool.Pool, id string) (*Drink, error) {
	var d Drink
	err := db.QueryRow(ctx,
		`SELECT id, slug, name, COALESCE(description,''), COALESCE(recipe,''), active, created_at
		 FROM drinks WHERE id = $1`,
		id,
	).Scan(&d.ID, &d.Slug, &d.Name, &d.Description, &d.Recipe, &d.Active, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func CreateRequest(ctx context.Context, db *pgxpool.Pool, userID, name, description, reason string) (requestID string, err error) {
	err = db.QueryRow(ctx,
		`INSERT INTO drink_requests (requested_by, name, description, reason)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, name, description, reason,
	).Scan(&requestID)
	return requestID, err
}

func ListPendingRequests(ctx context.Context, db *pgxpool.Pool) ([]Request, error) {
	rows, err := db.Query(ctx,
		`SELECT id, requested_by::text, name, COALESCE(description,''), COALESCE(reason,''), status::text, created_at
		 FROM drink_requests WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.RequestedBy, &r.Name, &r.Description, &r.Reason, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

// ApproveRequest approves a drink request, creates the drink with a slug, and returns the requester's user ID.
func ApproveRequest(ctx context.Context, db *pgxpool.Pool, requestID, adminID string) (requesterID string, err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var name, description string
	err = tx.QueryRow(ctx,
		`UPDATE drink_requests SET status='approved', reviewed_by=$2, reviewed_at=NOW()
		 WHERE id=$1 AND status='pending'
		 RETURNING name, COALESCE(description,''), requested_by`,
		requestID, adminID,
	).Scan(&name, &description, &requesterID)
	if err != nil {
		return "", err
	}

	baseSlug := slugify(name)
	if baseSlug == "" {
		baseSlug = "drink"
	}

	// Insert with slug; if slug already exists, append 8 chars of the new UUID.
	var drinkID string
	err = tx.QueryRow(ctx, `
		INSERT INTO drinks (name, description, slug, added_by)
		SELECT $1, $2,
		       CASE WHEN EXISTS (SELECT 1 FROM drinks WHERE slug = $3)
		            THEN $3 || '-' || LEFT(gen_random_uuid()::text, 8)
		            ELSE $3
		       END,
		       $4
		RETURNING id`,
		name, description, baseSlug, adminID,
	).Scan(&drinkID)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx,
		`UPDATE drink_requests SET drink_id=$1 WHERE id=$2`,
		drinkID, requestID,
	)
	if err != nil {
		return "", err
	}

	return requesterID, tx.Commit(ctx)
}

// RejectRequest rejects a drink request and returns the requester's user ID.
func RejectRequest(ctx context.Context, db *pgxpool.Pool, requestID, adminID, note string) (requesterID string, err error) {
	err = db.QueryRow(ctx,
		`UPDATE drink_requests SET status='rejected', reviewed_by=$2, reviewed_at=NOW(), review_note=$3
		 WHERE id=$1 AND status='pending'
		 RETURNING requested_by`,
		requestID, adminID, note,
	).Scan(&requesterID)
	return requesterID, err
}
