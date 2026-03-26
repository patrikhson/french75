package drink

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DrinkStats is the drink plus aggregate check-in stats.
type DrinkStats struct {
	ID           string
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

// GetDrinkStats returns a drink with aggregate check-in stats.
func GetDrinkStats(ctx context.Context, db *pgxpool.Pool, id string) (*DrinkStats, error) {
	var s DrinkStats
	err := db.QueryRow(ctx, `
		SELECT d.id, d.name, COALESCE(d.description,''),
		       COUNT(c.id),
		       COALESCE(ROUND(AVG(c.score)::numeric,1), 0),
		       COALESCE(MIN(c.score), 0),
		       COALESCE(MAX(c.score), 0)
		FROM drinks d
		LEFT JOIN check_ins c ON c.drink_id=d.id AND c.status='public'
		WHERE d.id=$1
		GROUP BY d.id, d.name, d.description`, id,
	).Scan(&s.ID, &s.Name, &s.Description,
		&s.CheckinCount, &s.AvgScore, &s.MinScore, &s.MaxScore)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// VenuesForDrink returns all venues where this drink was checked in, best avg score first.
func VenuesForDrink(ctx context.Context, db *pgxpool.Pool, drinkID string) ([]VenueForDrink, error) {
	rows, err := db.Query(ctx, `
		SELECT location_name,
		       AVG(location_lat), AVG(location_lng),
		       COUNT(*),
		       ROUND(AVG(score)::numeric,1),
		       MAX(score)
		FROM check_ins
		WHERE drink_id=$1 AND status='public'
		GROUP BY location_name
		ORDER BY AVG(score) DESC, COUNT(*) DESC`, drinkID)
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

// TopUsersForDrink returns users with the most and best check-ins for this drink.
func TopUsersForDrink(ctx context.Context, db *pgxpool.Pool, drinkID string) ([]TopUserForDrink, error) {
	rows, err := db.Query(ctx, `
		SELECT u.id, COALESCE(u.display_name, u.username),
		       COUNT(*), ROUND(AVG(c.score)::numeric,1), MAX(c.score)
		FROM check_ins c
		JOIN users u ON u.id=c.user_id
		WHERE c.drink_id=$1 AND c.status='public'
		GROUP BY u.id, u.display_name, u.username
		ORDER BY AVG(c.score) DESC, COUNT(*) DESC
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
		`SELECT id, name, COALESCE(description,''), COALESCE(recipe,''), active, created_at
		 FROM drinks WHERE active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var drinks []Drink
	for rows.Next() {
		var d Drink
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Recipe, &d.Active, &d.CreatedAt); err != nil {
			return nil, err
		}
		drinks = append(drinks, d)
	}
	return drinks, nil
}

func GetByID(ctx context.Context, db *pgxpool.Pool, id string) (*Drink, error) {
	var d Drink
	err := db.QueryRow(ctx,
		`SELECT id, name, COALESCE(description,''), COALESCE(recipe,''), active, created_at
		 FROM drinks WHERE id = $1`,
		id,
	).Scan(&d.ID, &d.Name, &d.Description, &d.Recipe, &d.Active, &d.CreatedAt)
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

// ApproveRequest approves a drink request, creates the drink, and returns the requester's user ID.
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

	var drinkID string
	err = tx.QueryRow(ctx,
		`INSERT INTO drinks (name, description, added_by) VALUES ($1,$2,$3) RETURNING id`,
		name, description, adminID,
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
