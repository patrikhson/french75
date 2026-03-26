package admin

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/auth"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/mail"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
)

type Handler struct {
	db       *pgxpool.Pool
	mailer   *mail.Mailer
	baseURL  string
	notifSvc *notification.Service
}

func NewHandler(db *pgxpool.Pool, mailer *mail.Mailer, baseURL string) *Handler {
	return &Handler{
		db:       db,
		mailer:   mailer,
		baseURL:  baseURL,
		notifSvc: notification.NewService(db, mailer, baseURL),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /admin", requireAdmin(http.HandlerFunc(h.dashboard)))

	mux.Handle("GET /admin/registrations", requireAdmin(http.HandlerFunc(h.listRegistrations)))
	mux.Handle("POST /admin/registrations/{id}/approve", requireAdmin(http.HandlerFunc(h.approveRegistration)))
	mux.Handle("POST /admin/registrations/{id}/reject", requireAdmin(http.HandlerFunc(h.rejectRegistration)))

	mux.Handle("GET /admin/checkins/pending", requireAdmin(http.HandlerFunc(h.listPendingCheckins)))
	mux.Handle("POST /admin/checkins/{id}/approve", requireAdmin(http.HandlerFunc(h.approveCheckin)))
	mux.Handle("POST /admin/checkins/{id}/reject", requireAdmin(http.HandlerFunc(h.rejectCheckin)))

	mux.Handle("GET /admin/drinks/requests", requireAdmin(http.HandlerFunc(h.listDrinkRequests)))

	mux.Handle("GET /admin/spam", requireAdmin(http.HandlerFunc(h.listSpam)))
	mux.Handle("POST /admin/spam/{id}/clear", requireAdmin(http.HandlerFunc(h.clearSpam)))

	mux.Handle("GET /admin/users", requireAdmin(http.HandlerFunc(h.listUsers)))
	mux.Handle("POST /admin/users/{id}/ban", requireAdmin(http.HandlerFunc(h.banUser)))
	mux.Handle("POST /admin/users/{id}/unban", requireAdmin(http.HandlerFunc(h.unbanUser)))
	mux.Handle("POST /admin/users/{id}/role", requireAdmin(http.HandlerFunc(h.setRole)))
}

// ---------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	c := GetCounts(r.Context(), h.db)
	w.Header().Set("Content-Type", "text/html")
	body := layout.AdminPage("Admin Dashboard", fmt.Sprintf(`<ul>
  <li><a href="/admin/registrations">Registrations</a> — %d pending</li>
  <li><a href="/admin/checkins/pending">Pending check-ins</a> — %d pending</li>
  <li><a href="/admin/drinks/requests">Drink requests</a> — %d pending</li>
  <li><a href="/admin/spam">Spam / flagged</a> — %d unreviewed flags</li>
  <li><a href="/admin/users">Users</a></li>
</ul>`,
		c.PendingRegistrations, c.PendingCheckins,
		c.PendingDrinkRequests, c.UnreviewedFlags,
	)) + "</body></html>"
	fmt.Fprint(w, body)
}

// ---------------------------------------------------------------
// Registrations
// ---------------------------------------------------------------

func (h *Handler) listRegistrations(w http.ResponseWriter, r *http.Request) {
	reqs, err := ListPendingRegistrations(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.AdminPage("Pending Registrations", `<table border="1" cellpadding="6"><tr><th>Name</th><th>Email</th><th>Requested</th><th></th></tr>`))
	for _, rq := range reqs {
		fmt.Fprintf(w, `<tr>
  <td>%s</td><td>%s</td><td>%s</td>
  <td>
    <form method="POST" action="/admin/registrations/%s/approve" style="display:inline">
      <button>Approve</button>
    </form>
    <form method="POST" action="/admin/registrations/%s/reject" style="display:inline">
      <button>Reject</button>
    </form>
  </td>
</tr>`, rq.Name, rq.Email, rq.CreatedAt.Format("2 Jan 2006"), rq.ID, rq.ID)
	}
	if len(reqs) == 0 {
		fmt.Fprint(w, `<tr><td colspan="4">No pending registrations.</td></tr>`)
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (h *Handler) approveRegistration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID, err := auth.SendApprovalEmail(r.Context(), h.db, h.mailer, h.baseURL, id)
	if err != nil {
		http.Error(w, "Could not approve: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// In-app notification (email already sent by SendApprovalEmail).
	if userID != "" {
		h.notifSvc.Notify(r.Context(), userID,
			notification.TypeAccountApproved,
			"Account approved",
			"Your account has been approved. Welcome to French 75 Tracker!",
			"/auth/login",
		)
	}
	http.Redirect(w, r, "/admin/registrations", http.StatusSeeOther)
}

func (h *Handler) rejectRegistration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := RejectRegistration(r.Context(), h.db, id); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/registrations", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Pending check-ins
// ---------------------------------------------------------------

func (h *Handler) listPendingCheckins(w http.ResponseWriter, r *http.Request) {
	items, err := ListPendingCheckins(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.AdminPage("Pending Check-ins", `<table border="1" cellpadding="6"><tr><th>User</th><th>Role</th><th>Drink</th><th>Date</th><th>Location</th><th>EXIF date</th><th>Device GPS</th><th>Photo GPS</th><th>Review</th><th></th></tr>`))
	for _, ci := range items {
		exif := "—"
		if ci.ExifPassed != nil {
			if *ci.ExifPassed {
				exif = "✓"
			} else {
				exif = "✗"
			}
		}
		gps := "—"
		if ci.GPSPassed != nil {
			if *ci.GPSPassed {
				gps = "✓"
			} else {
				dist := ""
				if ci.GPSDistanceM != nil {
					dist = fmt.Sprintf(" (%dm)", *ci.GPSDistanceM)
				}
				gps = "✗" + dist
			}
		}
		photoGPS := "—"
		if ci.PhotoGPSPassed != nil {
			if *ci.PhotoGPSPassed {
				photoGPS = fmt.Sprintf("✓ (%dm)", *ci.PhotoGPSDistanceM)
			} else {
				photoGPS = fmt.Sprintf("✗ (%dm)", *ci.PhotoGPSDistanceM)
			}
		}
		fmt.Fprintf(w, `<tr>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td>
  <td>%s</td>
  <td>
    <form method="POST" action="/admin/checkins/%s/approve" style="display:inline">
      <button>Approve</button>
    </form>
    <form method="POST" action="/admin/checkins/%s/reject" style="display:inline">
      <button>Reject</button>
    </form>
    <a href="/checkins/%s">View</a>
  </td>
</tr>`,
			ci.UserName, ci.UserRole, ci.DrinkName, ci.DrinkDate.Format("2 Jan 2006"),
			ci.LocationName, exif, gps, photoGPS,
			ci.Review,
			ci.ID, ci.ID, ci.ID,
		)
	}
	if len(items) == 0 {
		fmt.Fprint(w, `<tr><td colspan="9">No pending check-ins.</td></tr>`)
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (h *Handler) approveCheckin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ownerID, err := ApproveCheckin(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	h.notifSvc.Notify(r.Context(), ownerID,
		notification.TypeCheckinApproved,
		"Check-in approved",
		"Your check-in has been approved and is now public.",
		"/checkins/"+id,
	)
	http.Redirect(w, r, "/admin/checkins/pending", http.StatusSeeOther)
}

func (h *Handler) rejectCheckin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ownerID, err := RejectCheckin(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	h.notifSvc.Notify(r.Context(), ownerID,
		notification.TypeCheckinRejected,
		"Check-in rejected",
		"Your check-in was not approved.",
		"/checkins/"+id,
	)
	http.Redirect(w, r, "/admin/checkins/pending", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Drink requests
// ---------------------------------------------------------------

func (h *Handler) listDrinkRequests(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT dr.id, COALESCE(u.display_name, u.username), dr.name,
		        COALESCE(dr.description,''), COALESCE(dr.reason,''),
		        dr.status::text, dr.created_at
		 FROM drink_requests dr
		 JOIN users u ON u.id = dr.requested_by
		 WHERE dr.status = 'pending'
		 ORDER BY dr.created_at`)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.AdminPage("Drink Requests", `<table border="1" cellpadding="6"><tr><th>User</th><th>Drink</th><th>Description</th><th>Reason</th><th>Date</th><th></th></tr>`))

	adminID := middleware.GetUserID(r)
	empty := true
	for rows.Next() {
		empty = false
		var id, user, name, desc, reason, status string
		var createdAt interface{}
		rows.Scan(&id, &user, &name, &desc, &reason, &status, &createdAt)
		fmt.Fprintf(w, `<tr>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%v</td>
  <td>
    <form method="POST" action="/admin/drinks/requests/%s/approve" style="display:inline">
      <input type="hidden" name="admin_id" value="%s"><button>Approve</button>
    </form>
    <form method="POST" action="/admin/drinks/requests/%s/reject" style="display:inline">
      <input type="hidden" name="admin_id" value="%s">
      <input type="text" name="note" placeholder="Reason (optional)">
      <button>Reject</button>
    </form>
  </td>
</tr>`, user, name, desc, reason, createdAt, id, adminID, id, adminID)
	}
	if empty {
		fmt.Fprint(w, `<tr><td colspan="6">No pending drink requests.</td></tr>`)
	}
	fmt.Fprint(w, `</table></body></html>`)
}

// ---------------------------------------------------------------
// Spam
// ---------------------------------------------------------------

func (h *Handler) listSpam(w http.ResponseWriter, r *http.Request) {
	items, err := ListFlaggedCheckins(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.AdminPage("Flagged / Spam", `<table border="1" cellpadding="6"><tr><th>User</th><th>Drink</th><th>Flags</th><th>Status</th><th>Review</th><th></th></tr>`))
	for _, f := range items {
		fmt.Fprintf(w, `<tr>
  <td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td>
  <td>
    <form method="POST" action="/admin/spam/%s/clear" style="display:inline">
      <button>Clear &amp; restore</button>
    </form>
    <a href="/checkins/%s">View</a>
  </td>
</tr>`, f.UserName, f.DrinkName, f.FlagCount, f.Status, f.Review, f.ID, f.ID)
	}
	if len(items) == 0 {
		fmt.Fprint(w, `<tr><td colspan="6">No flagged check-ins.</td></tr>`)
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (h *Handler) clearSpam(w http.ResponseWriter, r *http.Request) {
	if err := ClearSpam(r.Context(), h.db, r.PathValue("id")); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/spam", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Users
// ---------------------------------------------------------------

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := ListUsers(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.AdminPage("Users", `<table border="1" cellpadding="6"><tr><th>Username</th><th>Display name</th><th>Role</th><th>Check-ins</th><th>Banned</th><th>Joined</th><th></th></tr>`))
	for _, u := range users {
		banned := "No"
		if u.IsBanned {
			banned = "Yes"
		}
		banAction := fmt.Sprintf(`<form method="POST" action="/admin/users/%s/ban" style="display:inline"><button>Ban</button></form>`, u.ID)
		if u.IsBanned {
			banAction = fmt.Sprintf(`<form method="POST" action="/admin/users/%s/unban" style="display:inline"><button>Unban</button></form>`, u.ID)
		}
		fmt.Fprintf(w, `<tr>
  <td><a href="/users/%s">%s</a></td><td>%s</td><td>%s</td>
  <td>%d</td><td>%s</td><td>%s</td>
  <td>
    %s
    <form method="POST" action="/admin/users/%s/role" style="display:inline">
      <select name="role"><option value="passive">passive</option><option value="active">active</option><option value="admin">admin</option></select>
      <button>Set role</button>
    </form>
    <a href="/users/%s">Profile</a>
  </td>
</tr>`,
			u.ID, u.Username, u.DisplayName, u.Role,
			u.CheckinCount, banned, u.CreatedAt.Format("2 Jan 2006"),
			banAction, u.ID, u.ID,
		)
	}
	fmt.Fprint(w, `</table></body></html>`)
}

func (h *Handler) banUser(w http.ResponseWriter, r *http.Request) {
	SetUserBan(r.Context(), h.db, r.PathValue("id"), true)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) unbanUser(w http.ResponseWriter, r *http.Request) {
	SetUserBan(r.Context(), h.db, r.PathValue("id"), false)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) setRole(w http.ResponseWriter, r *http.Request) {
	role := r.FormValue("role")
	if role != "passive" && role != "active" && role != "admin" {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}
	SetUserRole(r.Context(), h.db, r.PathValue("id"), role)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

