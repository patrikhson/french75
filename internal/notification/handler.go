package notification

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/mail"
	"github.com/patrikhson/french75/internal/middleware"
)

// Handler serves the notifications and preferences pages.
type Handler struct {
	svc *Service
	db  *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool, mailer *mail.Mailer, baseURL string) *Handler {
	return &Handler{
		svc: NewService(db, mailer, baseURL),
		db:  db,
	}
}

// Svc exposes the Service so main.go can pass it to other handlers.
func (h *Handler) Svc() *Service {
	return h.svc
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /notifications", requireAuth(http.HandlerFunc(h.listNotifications)))
	mux.Handle("POST /notifications/{id}/manage", requireAuth(http.HandlerFunc(h.markManaged)))
	mux.Handle("POST /notifications/manage-all", requireAuth(http.HandlerFunc(h.markAllManaged)))
	mux.Handle("GET /settings/notifications", requireAuth(http.HandlerFunc(h.showPreferences)))
	mux.Handle("POST /settings/notifications", requireAuth(http.HandlerFunc(h.savePreferences)))
	// HTMX polling endpoint — returns just the bell <a> element with fresh unread count.
	mux.Handle("GET /api/bell", requireAuth(http.HandlerFunc(h.bellFragment)))
}

// ---------------------------------------------------------------
// Notifications page
// ---------------------------------------------------------------

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)

	ns, err := ListForUser(r.Context(), h.db, userID)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	unread := UnreadCount(r.Context(), h.db, userID)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Notifications", role, unread, ""))
	fmt.Fprint(w, `<h2>Notifications</h2>`)

	if len(ns) > 0 {
		fmt.Fprint(w, `<form method="POST" action="/notifications/manage-all">
  <button type="submit">Mark all as managed</button>
</form><br>`)
	}

	if len(ns) == 0 {
		fmt.Fprint(w, `<p>No notifications yet.</p>`)
	}

	for _, n := range ns {
		managed := ""
		if n.Managed {
			managed = ` style="opacity:0.5"`
		}
		link := ""
		if n.Link != "" {
			link = fmt.Sprintf(` &nbsp; <a href="%s">View</a>`, n.Link)
		}
		manageBtn := ""
		if !n.Managed {
			manageBtn = fmt.Sprintf(
				`&nbsp; <form method="POST" action="/notifications/%s/manage" style="display:inline">
  <button type="submit">Mark managed</button>
</form>`, n.ID)
		}
		fmt.Fprintf(w, `<div%s style="border:1px solid #ccc;padding:8px;margin:6px 0;border-radius:4px">
  <strong>%s</strong><br>
  <small>%s</small><br>
  %s%s%s
</div>`,
			managed,
			n.Title,
			n.CreatedAt.Format("2 Jan 2006 15:04"),
			n.Body,
			link,
			manageBtn,
		)
	}

	fmt.Fprint(w, layout.PageEnd())
}

func (h *Handler) bellFragment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	unread := UnreadCount(r.Context(), h.db, userID)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.BellFragment(unread))
}

func (h *Handler) markManaged(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	id := r.PathValue("id")
	MarkManaged(r.Context(), h.db, id, userID)
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

func (h *Handler) markAllManaged(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	MarkAllManaged(r.Context(), h.db, userID)
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Preferences page
// ---------------------------------------------------------------

func (h *Handler) showPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	prefs := GetPreferences(r.Context(), h.db, userID)
	unread := UnreadCount(r.Context(), h.db, userID)

	// Fetch current digest_hour for admin users.
	var digestHour *int
	if role == "admin" {
		var dh *int
		h.db.QueryRow(r.Context(), `SELECT digest_hour FROM users WHERE id = $1`, userID).Scan(&dh)
		digestHour = dh
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Notification Preferences", role, unread, ""))
	fmt.Fprint(w, `<h2>Notification Preferences</h2>
<form method="POST" action="/settings/notifications">
<table border="1" cellpadding="6">
<tr>
  <th>Notification</th>
  <th>In-app</th>
  <th>Email</th>
</tr>`)

	for _, t := range AllTypes {
		// Hide admin-type preferences for non-admin users.
		if IsAdminType(t) && role != "admin" {
			continue
		}
		p := prefs[t]
		inAppChecked := ""
		if p.InAppEnabled {
			inAppChecked = " checked"
		}
		emailChecked := ""
		if p.EmailEnabled {
			emailChecked = " checked"
		}
		fmt.Fprintf(w, `<tr>
  <td>%s</td>
  <td><input type="checkbox" name="in_app_%s"%s></td>
  <td><input type="checkbox" name="email_%s"%s></td>
</tr>`, TypeLabel(t), t, inAppChecked, t, emailChecked)
	}

	fmt.Fprint(w, `</table><br>`)

	if role == "admin" {
		digestVal := ""
		if digestHour != nil {
			digestVal = strconv.Itoa(*digestHour)
		}
		fmt.Fprintf(w, `<h3>Daily digest email</h3>
<p>Receive a daily email digest of pending admin items. Leave blank to disable.</p>
<label>Hour (0–23 UTC): <input type="number" name="digest_hour" min="0" max="23" value="%s"></label><br><br>`,
			digestVal)
	}

	fmt.Fprint(w, `<button type="submit">Save preferences</button>
</form>
<hr>
<p><a href="/settings/passkeys">Manage passkeys →</a></p>`)
	fmt.Fprint(w, layout.PageEnd())
}

func (h *Handler) savePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	for _, t := range AllTypes {
		if IsAdminType(t) && role != "admin" {
			continue
		}
		emailEnabled := r.FormValue("email_"+t) == "on"
		inAppEnabled := r.FormValue("in_app_"+t) == "on"
		SavePreference(r.Context(), h.db, userID, t, emailEnabled, inAppEnabled)
	}

	if role == "admin" {
		raw := strings.TrimSpace(r.FormValue("digest_hour"))
		if raw == "" {
			h.db.Exec(r.Context(), `UPDATE users SET digest_hour = NULL WHERE id = $1`, userID)
		} else if hour, err := strconv.Atoi(raw); err == nil && hour >= 0 && hour <= 23 {
			h.db.Exec(r.Context(), `UPDATE users SET digest_hour = $1 WHERE id = $2`, hour, userID)
		}
	}

	http.Redirect(w, r, "/settings/notifications", http.StatusSeeOther)
}
