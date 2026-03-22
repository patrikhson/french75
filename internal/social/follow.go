package social

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ToggleFollow follows or unfollows followingID on behalf of followerID.
// Returns true if the user is now following, false if unfollowed.
func ToggleFollow(ctx context.Context, db *pgxpool.Pool, followerID, followingID string) (bool, error) {
	if followerID == followingID {
		return false, fmt.Errorf("cannot follow yourself")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id=$1 AND following_id=$2)`,
		followerID, followingID,
	).Scan(&exists)

	if exists {
		tx.Exec(ctx, `DELETE FROM follows WHERE follower_id=$1 AND following_id=$2`,
			followerID, followingID)
	} else {
		tx.Exec(ctx, `INSERT INTO follows (follower_id, following_id) VALUES ($1,$2)`,
			followerID, followingID)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return !exists, nil
}

// FollowCounts returns follower and following counts for a user.
func FollowCounts(ctx context.Context, db *pgxpool.Pool, userID string) (followers, following int) {
	db.QueryRow(ctx, `SELECT COUNT(*) FROM follows WHERE following_id=$1`, userID).Scan(&followers)
	db.QueryRow(ctx, `SELECT COUNT(*) FROM follows WHERE follower_id=$1`, userID).Scan(&following)
	return
}

// IsFollowing returns true if followerID follows followingID.
func IsFollowing(ctx context.Context, db *pgxpool.Pool, followerID, followingID string) bool {
	var exists bool
	db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id=$1 AND following_id=$2)`,
		followerID, followingID,
	).Scan(&exists)
	return exists
}
