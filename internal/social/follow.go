package social

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FollowRequest is a pending request from one user to follow another.
type FollowRequest struct {
	ID            string
	RequesterID   string
	RequesterName string
	CreatedAt     time.Time
}

// RequestOrCancelFollow handles the follow button on a profile page.
// - If already following → unfollows, returns false, "unfollowed"
// - If request pending → cancels request, returns false, "cancelled"
// - Otherwise → creates a follow request, returns true, "requested"
func RequestOrCancelFollow(ctx context.Context, db *pgxpool.Pool, requesterID, targetID string) (requested bool, state string, err error) {
	if requesterID == targetID {
		return false, "", fmt.Errorf("cannot follow yourself")
	}

	// Already following?
	var isFollowing bool
	db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id=$1 AND following_id=$2)`,
		requesterID, targetID,
	).Scan(&isFollowing)
	if isFollowing {
		db.Exec(ctx, `DELETE FROM follows WHERE follower_id=$1 AND following_id=$2`,
			requesterID, targetID)
		return false, "unfollowed", nil
	}

	// Pending request?
	var pendingID string
	db.QueryRow(ctx,
		`SELECT id FROM follow_requests WHERE requester_id=$1 AND target_id=$2`,
		requesterID, targetID,
	).Scan(&pendingID)
	if pendingID != "" {
		db.Exec(ctx, `DELETE FROM follow_requests WHERE id=$1`, pendingID)
		return false, "cancelled", nil
	}

	// Create request.
	_, err = db.Exec(ctx,
		`INSERT INTO follow_requests (requester_id, target_id) VALUES ($1,$2)
		 ON CONFLICT (requester_id, target_id) DO NOTHING`,
		requesterID, targetID,
	)
	if err != nil {
		return false, "", err
	}
	return true, "requested", nil
}

// ApproveFollowRequest approves a pending request directed at targetID.
// Inserts a row into follows and deletes the request.
// Returns the requesterID so the caller can send a notification.
func ApproveFollowRequest(ctx context.Context, db *pgxpool.Pool, requestID, targetID string) (requesterID string, err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`DELETE FROM follow_requests WHERE id=$1 AND target_id=$2 RETURNING requester_id`,
		requestID, targetID,
	).Scan(&requesterID)
	if err != nil {
		return "", fmt.Errorf("request not found")
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO follows (follower_id, following_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		requesterID, targetID,
	)
	if err != nil {
		return "", err
	}

	return requesterID, tx.Commit(ctx)
}

// RejectFollowRequest deletes a pending request directed at targetID.
// Returns the requesterID so the caller can send a notification.
func RejectFollowRequest(ctx context.Context, db *pgxpool.Pool, requestID, targetID string) (requesterID string, err error) {
	err = db.QueryRow(ctx,
		`DELETE FROM follow_requests WHERE id=$1 AND target_id=$2 RETURNING requester_id`,
		requestID, targetID,
	).Scan(&requesterID)
	if err != nil {
		return "", fmt.Errorf("request not found")
	}
	return requesterID, nil
}

// PendingRequestsForUser returns pending follow requests targeting userID.
func PendingRequestsForUser(ctx context.Context, db *pgxpool.Pool, userID string) ([]FollowRequest, error) {
	rows, err := db.Query(ctx,
		`SELECT fr.id, fr.requester_id, COALESCE(u.display_name, u.username), fr.created_at
		 FROM follow_requests fr
		 JOIN users u ON u.id = fr.requester_id
		 WHERE fr.target_id = $1
		 ORDER BY fr.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []FollowRequest
	for rows.Next() {
		var r FollowRequest
		rows.Scan(&r.ID, &r.RequesterID, &r.RequesterName, &r.CreatedAt)
		reqs = append(reqs, r)
	}
	return reqs, nil
}

// PendingRequestCount returns how many pending follow requests are waiting for userID.
func PendingRequestCount(ctx context.Context, db *pgxpool.Pool, userID string) int {
	var n int
	db.QueryRow(ctx, `SELECT COUNT(*) FROM follow_requests WHERE target_id=$1`, userID).Scan(&n)
	return n
}

// HasPendingRequest returns true if requesterID has an outstanding request to targetID.
func HasPendingRequest(ctx context.Context, db *pgxpool.Pool, requesterID, targetID string) bool {
	var exists bool
	db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follow_requests WHERE requester_id=$1 AND target_id=$2)`,
		requesterID, targetID,
	).Scan(&exists)
	return exists
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
