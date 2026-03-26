package social

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/mail"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
)

type Handler struct {
	db       *pgxpool.Pool
	notifSvc *notification.Service
}

func NewHandler(db *pgxpool.Pool, mailer *mail.Mailer, baseURL string) *Handler {
	return &Handler{db: db, notifSvc: notification.NewService(db, mailer, baseURL)}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("POST /checkins/{id}/react", requireAuth(http.HandlerFunc(h.react)))
	mux.Handle("POST /checkins/{id}/flag", requireAuth(http.HandlerFunc(h.flag)))
	mux.Handle("POST /users/{id}/follow", requireAuth(http.HandlerFunc(h.follow)))
	mux.Handle("GET /follow-requests", requireAuth(http.HandlerFunc(h.listFollowRequests)))
	mux.Handle("POST /follow-requests/{id}/approve", requireAuth(http.HandlerFunc(h.approveFollowRequest)))
	mux.Handle("POST /follow-requests/{id}/reject", requireAuth(http.HandlerFunc(h.rejectFollowRequest)))
}

// ---------------------------------------------------------------
// Reactions
// ---------------------------------------------------------------

func (h *Handler) react(w http.ResponseWriter, r *http.Request) {
	checkinID := r.PathValue("id")
	userID := middleware.GetUserID(r)
	reactionType := r.URL.Query().Get("type")

	if reactionType != "like" && reactionType != "helpful" {
		http.Error(w, "type must be like or helpful", http.StatusBadRequest)
		return
	}

	count, active, err := toggleReaction(r.Context(), h.db, userID, checkinID, reactionType)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Notify the check-in owner when a new reaction is added (not on toggle-off).
	if active {
		var ownerID, reactorName string
		h.db.QueryRow(r.Context(),
			`SELECT c.user_id, COALESCE(u.display_name, u.username)
			 FROM check_ins c, users u
			 WHERE c.id = $1 AND u.id = $2`,
			checkinID, userID,
		).Scan(&ownerID, &reactorName)
		if ownerID != "" && ownerID != userID {
			label := "liked"
			if reactionType == "helpful" {
				label = "marked helpful"
			}
			h.notifSvc.Notify(r.Context(), ownerID,
				notification.TypeCheckinReaction,
				"Someone reacted to your check-in",
				fmt.Sprintf("%s %s your check-in.", reactorName, label),
				"/checkins/"+checkinID,
				checkinID,
			)
		}
	}

	// Return HTMX fragment: just the updated span that the button targets
	label := "👍"
	spanID := fmt.Sprintf("reaction-like-%s", checkinID)
	if reactionType == "helpful" {
		label = "💡"
		spanID = fmt.Sprintf("reaction-helpful-%s", checkinID)
	}

	style := ""
	if active {
		style = ` style="font-weight:bold"`
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span id="%s"%s>%s %d</span>`, spanID, style, label, count)
}

// ---------------------------------------------------------------
// Follows
// ---------------------------------------------------------------

func (h *Handler) follow(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.GetUserID(r)
	targetID := r.PathValue("id")

	requested, state, err := RequestOrCancelFollow(r.Context(), h.db, requesterID, targetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Notify when a new follow request is sent.
	if state == "requested" {
		// Look up the new request ID so we can link the notification to it.
		var requestID, requesterName string
		h.db.QueryRow(r.Context(),
			`SELECT fr.id, COALESCE(u.display_name, u.username)
			 FROM follow_requests fr JOIN users u ON u.id=fr.requester_id
			 WHERE fr.requester_id=$1 AND fr.target_id=$2`,
			requesterID, targetID,
		).Scan(&requestID, &requesterName)
		h.notifSvc.Notify(r.Context(), targetID,
			notification.TypeFollowRequestReceived,
			"Follow request",
			fmt.Sprintf("%s wants to follow you.", requesterName),
			"/follow-requests",
			requestID,
		)
	}

	// Notify when someone unfollows (no notification — that's normal social behaviour).
	_ = requested

	// Return HTMX fragment: updated button.
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, followButton(targetID, state))
}

// followButton returns the HTMX follow button HTML for the given state.
// state: "requested" | "cancelled" | "unfollowed" | "following" | "none"
func followButton(targetID, state string) string {
	switch state {
	case "requested":
		return fmt.Sprintf(
			`<button class="btn" hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Cancel follow request</button>`,
			targetID)
	default: // "cancelled", "unfollowed", "none"
		return fmt.Sprintf(
			`<button class="btn" hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Request to follow</button>`,
			targetID)
	}
}

// ---------------------------------------------------------------
// Follow requests
// ---------------------------------------------------------------

func (h *Handler) listFollowRequests(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)

	reqs, err := PendingRequestsForUser(r.Context(), h.db, userID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Follow Requests", role, unread, ""))
	fmt.Fprint(w, `<h2>Follow Requests</h2>`)

	if len(reqs) == 0 {
		fmt.Fprint(w, `<p>No pending follow requests.</p>`)
		fmt.Fprint(w, layout.PageEnd())
		return
	}

	for _, req := range reqs {
		fmt.Fprintf(w, `<div class="card">
  <strong><a href="/users/%s">%s</a></strong>
  <span class="card-meta"> wants to follow you · %s</span>
  <div class="card-actions" style="margin-top:8px">
    <form method="POST" action="/follow-requests/%s/approve" style="display:inline">
      <button type="submit" class="btn">Approve</button>
    </form>
    <form method="POST" action="/follow-requests/%s/reject" style="display:inline;margin-left:8px">
      <button type="submit" class="btn btn-ghost">Decline</button>
    </form>
  </div>
</div>`,
			req.RequesterID, req.RequesterName,
			req.CreatedAt.Format("2 Jan 2006"),
			req.ID,
			req.ID,
		)
	}

	fmt.Fprint(w, layout.PageEnd())
}

func (h *Handler) approveFollowRequest(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")
	targetID := middleware.GetUserID(r)

	requesterID, err := ApproveFollowRequest(r.Context(), h.db, requestID, targetID)
	if err != nil {
		http.Error(w, "Could not approve: "+err.Error(), http.StatusBadRequest)
		return
	}

	var targetName string
	h.db.QueryRow(r.Context(),
		`SELECT COALESCE(display_name, username) FROM users WHERE id = $1`, targetID,
	).Scan(&targetName)

	notification.AutoManageByEntity(r.Context(), h.db, notification.TypeFollowRequestReceived, requestID)
	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeFollowRequestApproved,
		"Follow request approved",
		fmt.Sprintf("%s approved your follow request.", targetName),
		"/users/"+targetID,
		requestID,
	)

	http.Redirect(w, r, "/follow-requests", http.StatusSeeOther)
}

func (h *Handler) rejectFollowRequest(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")
	targetID := middleware.GetUserID(r)

	requesterID, err := RejectFollowRequest(r.Context(), h.db, requestID, targetID)
	if err != nil {
		http.Error(w, "Could not decline: "+err.Error(), http.StatusBadRequest)
		return
	}

	var targetName string
	h.db.QueryRow(r.Context(),
		`SELECT COALESCE(display_name, username) FROM users WHERE id = $1`, targetID,
	).Scan(&targetName)

	notification.AutoManageByEntity(r.Context(), h.db, notification.TypeFollowRequestReceived, requestID)
	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeFollowRequestRejected,
		"Follow request not approved",
		fmt.Sprintf("%s did not approve your follow request.", targetName),
		"/users/"+targetID,
		requestID,
	)

	http.Redirect(w, r, "/follow-requests", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Spam flagging
// ---------------------------------------------------------------

func (h *Handler) flag(w http.ResponseWriter, r *http.Request) {
	checkinID := r.PathValue("id")
	userID := middleware.GetUserID(r)
	reason := r.FormValue("reason")

	err := flagCheckIn(r.Context(), h.db, userID, checkinID, reason)
	if err != nil {
		// Unique constraint = already flagged
		http.Error(w, "Already flagged or not found", http.StatusBadRequest)
		return
	}

	h.notifSvc.NotifyAdmins(r.Context(),
		notification.TypeAdminSpamFlag,
		"Check-in flagged",
		"A check-in has been flagged as spam.",
		"/admin/spam",
		checkinID,
	)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<span>Flagged</span>`)
		return
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
