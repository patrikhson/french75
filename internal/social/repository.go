package social

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// toggleReaction inserts or deletes a reaction and keeps the denormalized count in sync.
// Returns the new count and whether the reaction is now active.
func toggleReaction(ctx context.Context, db *pgxpool.Pool, userID, checkinID, reactionType string) (int, bool, error) {
	if reactionType != "like" && reactionType != "helpful" {
		return 0, false, fmt.Errorf("invalid reaction type")
	}

	col := "like_count"
	if reactionType == "helpful" {
		col = "helpful_count"
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reactions WHERE user_id=$1 AND checkin_id=$2 AND type=$3::reaction_type)`,
		userID, checkinID, reactionType,
	).Scan(&exists)

	if exists {
		tx.Exec(ctx,
			`DELETE FROM reactions WHERE user_id=$1 AND checkin_id=$2 AND type=$3::reaction_type`,
			userID, checkinID, reactionType,
		)
		tx.Exec(ctx,
			`UPDATE check_ins SET `+col+` = GREATEST(`+col+`-1, 0) WHERE id=$1`,
			checkinID,
		)
	} else {
		tx.Exec(ctx,
			`INSERT INTO reactions (user_id, checkin_id, type) VALUES ($1,$2,$3::reaction_type)`,
			userID, checkinID, reactionType,
		)
		tx.Exec(ctx,
			`UPDATE check_ins SET `+col+` = `+col+`+1 WHERE id=$1`,
			checkinID,
		)
	}

	var newCount int
	tx.QueryRow(ctx, `SELECT `+col+` FROM check_ins WHERE id=$1`, checkinID).Scan(&newCount)

	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return newCount, !exists, nil
}

// flagCheckIn inserts a spam flag. Returns an error if already flagged (unique constraint).
// Auto-sets status to 'spam' when flag_count reaches 3.
func flagCheckIn(ctx context.Context, db *pgxpool.Pool, userID, checkinID, reason string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO spam_flags (checkin_id, flagged_by, reason) VALUES ($1,$2,$3)`,
		checkinID, userID, reason,
	)
	if err != nil {
		return err // duplicate flag or invalid checkin_id
	}

	var flagCount int
	tx.QueryRow(ctx,
		`UPDATE check_ins SET flag_count = flag_count+1 WHERE id=$1 RETURNING flag_count`,
		checkinID,
	).Scan(&flagCount)

	if flagCount >= 3 {
		tx.Exec(ctx,
			`UPDATE check_ins SET status='spam'::checkin_status WHERE id=$1 AND status='public'`,
			checkinID,
		)
	}

	return tx.Commit(ctx)
}
