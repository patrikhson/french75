package location

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VenueRow summarises a venue across all check-ins.
type VenueRow struct {
	Name         string
	Lat          float64
	Lng          float64
	CheckinCount int
	AvgScore     float64
	UniqueUsers  int
	UniqueDrinks int
}

// DrinkAtVenue is the per-drink breakdown for a single venue.
type DrinkAtVenue struct {
	DrinkID      string
	DrinkSlug    string
	DrinkName    string
	CheckinCount int
	AvgScore     float64
	MinScore     int
	MaxScore     int
}

// CheckinAtVenue is one check-in shown on the venue detail page.
type CheckinAtVenue struct {
	ID        string
	DrinkID   string
	DrinkSlug string
	DrinkName string
	UserID    string
	UserName  string
	Score     int
	DrinkDate time.Time
	Review    string
}

var allowedVenueListSort = map[string]string{
	"name":     "location_name",
	"checkins": "checkin_count",
	"avg":      "avg_score",
	"drinks":   "unique_drinks",
	"visitors": "unique_users",
}

// ListVenues returns all distinct venues ordered by the given column.
func ListVenues(ctx context.Context, db *pgxpool.Pool, sort, order string) ([]VenueRow, error) {
	col, ok := allowedVenueListSort[sort]
	if !ok {
		col = "checkin_count"
	}
	dir := "DESC"
	if order == "asc" {
		dir = "ASC"
	}
	rows, err := db.Query(ctx, `
		SELECT location_name,
		       AVG(location_lat),
		       AVG(location_lng),
		       COUNT(*)                        AS checkin_count,
		       ROUND(AVG(score)::numeric, 1)   AS avg_score,
		       COUNT(DISTINCT user_id)         AS unique_users,
		       COUNT(DISTINCT drink_id)        AS unique_drinks
		FROM check_ins
		WHERE status = 'public'
		GROUP BY location_name
		ORDER BY `+col+` `+dir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venues []VenueRow
	for rows.Next() {
		var v VenueRow
		rows.Scan(&v.Name, &v.Lat, &v.Lng,
			&v.CheckinCount, &v.AvgScore, &v.UniqueUsers, &v.UniqueDrinks)
		venues = append(venues, v)
	}
	return venues, nil
}

// GetVenueStats returns aggregate stats for a single venue by name.
func GetVenueStats(ctx context.Context, db *pgxpool.Pool, name string) (VenueRow, error) {
	var v VenueRow
	err := db.QueryRow(ctx, `
		SELECT location_name,
		       AVG(location_lat),
		       AVG(location_lng),
		       COUNT(*)                        AS checkin_count,
		       ROUND(AVG(score)::numeric, 1),
		       COUNT(DISTINCT user_id),
		       COUNT(DISTINCT drink_id)
		FROM check_ins
		WHERE location_name = $1 AND status = 'public'
		GROUP BY location_name`, name,
	).Scan(&v.Name, &v.Lat, &v.Lng,
		&v.CheckinCount, &v.AvgScore, &v.UniqueUsers, &v.UniqueDrinks)
	return v, err
}

// DrinksAtVenue returns per-drink stats at the given venue, sorted by avg score desc.
func DrinksAtVenue(ctx context.Context, db *pgxpool.Pool, name string) ([]DrinkAtVenue, error) {
	rows, err := db.Query(ctx, `
		SELECT d.id, d.slug, d.name,
		       COUNT(*)                        AS cnt,
		       ROUND(AVG(c.score)::numeric, 1),
		       MIN(c.score), MAX(c.score)
		FROM check_ins c
		JOIN drinks d ON d.id = c.drink_id
		WHERE c.location_name = $1 AND c.status = 'public'
		GROUP BY d.id, d.slug, d.name
		ORDER BY AVG(c.score) DESC`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DrinkAtVenue
	for rows.Next() {
		var d DrinkAtVenue
		rows.Scan(&d.DrinkID, &d.DrinkSlug, &d.DrinkName, &d.CheckinCount,
			&d.AvgScore, &d.MinScore, &d.MaxScore)
		out = append(out, d)
	}
	return out, nil
}

// CheckinsAtVenue returns all public check-ins at the given venue, best score first.
func CheckinsAtVenue(ctx context.Context, db *pgxpool.Pool, name string) ([]CheckinAtVenue, error) {
	rows, err := db.Query(ctx, `
		SELECT c.id, d.id, d.slug, d.name,
		       c.user_id, COALESCE(u.display_name, u.username),
		       c.score, c.drink_date, LEFT(c.review, 200)
		FROM check_ins c
		JOIN drinks d ON d.id = c.drink_id
		JOIN users  u ON u.id = c.user_id
		WHERE c.location_name = $1 AND c.status = 'public'
		ORDER BY c.score DESC, c.submitted_at DESC`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CheckinAtVenue
	for rows.Next() {
		var ci CheckinAtVenue
		rows.Scan(&ci.ID, &ci.DrinkID, &ci.DrinkSlug, &ci.DrinkName,
			&ci.UserID, &ci.UserName,
			&ci.Score, &ci.DrinkDate, &ci.Review)
		out = append(out, ci)
	}
	return out, nil
}
