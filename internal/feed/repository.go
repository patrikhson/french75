package feed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const pageSize = 20

type Item struct {
	ID           string
	UserID       string
	UserName     string
	DrinkName    string
	Score        int
	Review       string
	DrinkDate    time.Time
	LocationName string
	Thumbnail    string // storage path, may be empty
	LikeCount    int
	HelpfulCount int
	SubmittedAt  time.Time
}

// List returns up to pageSize public check-ins submitted before `before`, newest first.
func List(ctx context.Context, db *pgxpool.Pool, before time.Time) ([]Item, error) {
	rows, err := db.Query(ctx,
		`SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
		        d.name, c.score, LEFT(c.review, 280),
		        c.drink_date, c.location_name,
		        COALESCE(p.thumbnail_path, ''),
		        c.like_count, c.helpful_count, c.submitted_at
		 FROM check_ins c
		 JOIN users u ON u.id = c.user_id
		 JOIN drinks d ON d.id = c.drink_id
		 LEFT JOIN LATERAL (
		     SELECT thumbnail_path FROM photos
		     WHERE checkin_id = c.id ORDER BY sort_order LIMIT 1
		 ) p ON true
		 WHERE c.status = 'public' AND c.submitted_at < $1
		 ORDER BY c.submitted_at DESC
		 LIMIT $2`,
		before, pageSize,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.ID, &it.UserID, &it.UserName,
			&it.DrinkName, &it.Score, &it.Review,
			&it.DrinkDate, &it.LocationName,
			&it.Thumbnail,
			&it.LikeCount, &it.HelpfulCount, &it.SubmittedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}
