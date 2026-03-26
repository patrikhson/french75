package feed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const pageSize = 20

const (
	SortDrinkDate = "drink_date"
	SortPosted    = "posted"
)

type Item struct {
	ID           string
	UserID       string
	UserName     string
	DrinkID      string
	DrinkSlug    string
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

// List returns up to pageSize public check-ins, newest first.
//
// sort selects the ordering: SortDrinkDate (drink_date DESC, submitted_at DESC)
// or SortPosted (submitted_at DESC).
// beforeDate and beforeTime are the pagination cursors for the last seen item.
func List(ctx context.Context, db *pgxpool.Pool, sort string, beforeDate time.Time, beforeTime time.Time) ([]Item, error) {
	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error

	if sort == SortPosted {
		r, e := db.Query(ctx,
			`SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
			        d.id, d.slug, d.name, c.score, LEFT(c.review, 280),
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
			beforeTime, pageSize,
		)
		rows, err = r, e
	} else {
		// Default: sort by drink_date DESC, submitted_at DESC.
		// Compound cursor: items where drink_date < beforeDate,
		// or same drink_date but submitted_at < beforeTime.
		r, e := db.Query(ctx,
			`SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
			        d.id, d.slug, d.name, c.score, LEFT(c.review, 280),
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
			 WHERE c.status = 'public'
			   AND (c.drink_date < $1::date
			        OR (c.drink_date = $1::date AND c.submitted_at < $2))
			 ORDER BY c.drink_date DESC, c.submitted_at DESC
			 LIMIT $3`,
			beforeDate, beforeTime, pageSize,
		)
		rows, err = r, e
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}

// ListFollowing returns check-ins from users that currentUserID follows.
func ListFollowing(ctx context.Context, db *pgxpool.Pool, currentUserID string, sort string, beforeDate time.Time, beforeTime time.Time) ([]Item, error) {
	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error

	if sort == SortPosted {
		r, e := db.Query(ctx,
			`SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
			        d.id, d.slug, d.name, c.score, LEFT(c.review, 280),
			        c.drink_date, c.location_name,
			        COALESCE(p.thumbnail_path, ''),
			        c.like_count, c.helpful_count, c.submitted_at
			 FROM check_ins c
			 JOIN follows f ON f.following_id = c.user_id AND f.follower_id = $2
			 JOIN users u ON u.id = c.user_id
			 JOIN drinks d ON d.id = c.drink_id
			 LEFT JOIN LATERAL (
			     SELECT thumbnail_path FROM photos
			     WHERE checkin_id = c.id ORDER BY sort_order LIMIT 1
			 ) p ON true
			 WHERE c.status = 'public' AND c.submitted_at < $1
			 ORDER BY c.submitted_at DESC
			 LIMIT $3`,
			beforeTime, currentUserID, pageSize,
		)
		rows, err = r, e
	} else {
		r, e := db.Query(ctx,
			`SELECT c.id, c.user_id, COALESCE(u.display_name, u.username),
			        d.id, d.slug, d.name, c.score, LEFT(c.review, 280),
			        c.drink_date, c.location_name,
			        COALESCE(p.thumbnail_path, ''),
			        c.like_count, c.helpful_count, c.submitted_at
			 FROM check_ins c
			 JOIN follows f ON f.following_id = c.user_id AND f.follower_id = $3
			 JOIN users u ON u.id = c.user_id
			 JOIN drinks d ON d.id = c.drink_id
			 LEFT JOIN LATERAL (
			     SELECT thumbnail_path FROM photos
			     WHERE checkin_id = c.id ORDER BY sort_order LIMIT 1
			 ) p ON true
			 WHERE c.status = 'public'
			   AND (c.drink_date < $1::date
			        OR (c.drink_date = $1::date AND c.submitted_at < $2))
			 ORDER BY c.drink_date DESC, c.submitted_at DESC
			 LIMIT $4`,
			beforeDate, beforeTime, currentUserID, pageSize,
		)
		rows, err = r, e
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}

type scanner interface {
	Next() bool
	Scan(...any) error
	Close()
}

func scanItems(rows scanner) ([]Item, error) {
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.ID, &it.UserID, &it.UserName,
			&it.DrinkID, &it.DrinkSlug, &it.DrinkName, &it.Score, &it.Review,
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
