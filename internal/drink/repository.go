package drink

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func CreateRequest(ctx context.Context, db *pgxpool.Pool, userID, name, description, reason string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO drink_requests (requested_by, name, description, reason)
		 VALUES ($1, $2, $3, $4)`,
		userID, name, description, reason,
	)
	return err
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

func ApproveRequest(ctx context.Context, db *pgxpool.Pool, requestID, adminID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var name, description string
	err = tx.QueryRow(ctx,
		`UPDATE drink_requests SET status='approved', reviewed_by=$2, reviewed_at=NOW()
		 WHERE id=$1 AND status='pending'
		 RETURNING name, COALESCE(description,'')`,
		requestID, adminID,
	).Scan(&name, &description)
	if err != nil {
		return err
	}

	var drinkID string
	err = tx.QueryRow(ctx,
		`INSERT INTO drinks (name, description, added_by) VALUES ($1,$2,$3) RETURNING id`,
		name, description, adminID,
	).Scan(&drinkID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE drink_requests SET drink_id=$1 WHERE id=$2`,
		drinkID, requestID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func RejectRequest(ctx context.Context, db *pgxpool.Pool, requestID, adminID, note string) error {
	_, err := db.Exec(ctx,
		`UPDATE drink_requests SET status='rejected', reviewed_by=$2, reviewed_at=NOW(), review_note=$3
		 WHERE id=$1 AND status='pending'`,
		requestID, adminID, note,
	)
	return err
}
