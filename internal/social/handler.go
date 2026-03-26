package social

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
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
	followerID := middleware.GetUserID(r)
	followingID := r.PathValue("id")

	nowFollowing, err := ToggleFollow(r.Context(), h.db, followerID, followingID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Notify the followed user (only on follow, not unfollow).
	if nowFollowing {
		var followerName string
		h.db.QueryRow(r.Context(),
			`SELECT COALESCE(display_name, username) FROM users WHERE id = $1`,
			followerID,
		).Scan(&followerName)
		h.notifSvc.Notify(r.Context(), followingID,
			notification.TypeNewFollower,
			"New follower",
			fmt.Sprintf("%s is now following you.", followerName),
			"/users/"+followerID,
		)
	}

	// Return HTMX fragment: updated follow button
	w.Header().Set("Content-Type", "text/html")
	if nowFollowing {
		fmt.Fprintf(w,
			`<button hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Unfollow</button>`,
			followingID)
	} else {
		fmt.Fprintf(w,
			`<button hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Follow</button>`,
			followingID)
	}
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
	)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<span>Flagged</span>`)
		return
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
