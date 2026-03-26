package drink

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
	db      *pgxpool.Pool
	notifSvc *notification.Service
}

func NewHandler(db *pgxpool.Pool, mailer *mail.Mailer, baseURL string) *Handler {
	return &Handler{db: db, notifSvc: notification.NewService(db, mailer, baseURL)}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /drinks", requireAuth(http.HandlerFunc(h.listDrinks)))
	mux.Handle("POST /drinks/request", requireAuth(http.HandlerFunc(h.submitRequest)))

	mux.Handle("POST /admin/drinks/requests/{id}/approve", requireAdmin(http.HandlerFunc(h.approveRequest)))
	mux.Handle("POST /admin/drinks/requests/{id}/reject", requireAdmin(http.HandlerFunc(h.rejectRequest)))
}

func (h *Handler) listDrinks(w http.ResponseWriter, r *http.Request) {
	drinks, err := ListActive(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	role := middleware.GetUserRole(r)
	userID := middleware.GetUserID(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Drinks — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
</head><body>%s<main>
<h2>Drinks</h2>
<ul>`, layout.Nav(role, unread))
	for _, d := range drinks {
		fmt.Fprintf(w, `<li><strong>%s</strong>`, d.Name)
		if d.Description != "" {
			fmt.Fprintf(w, ` — %s`, d.Description)
		}
		fmt.Fprint(w, `</li>`)
	}
	fmt.Fprint(w, `</ul>
<hr>
<h3>Request a drink</h3>
<form method="POST" action="/drinks/request">
  <label>Drink name<br><input type="text" name="name" required></label><br><br>
  <label>Description<br><textarea name="description"></textarea></label><br><br>
  <label>Why should we add it?<br><textarea name="reason"></textarea></label><br><br>
  <button type="submit">Submit request</button>
</form>
</main></body></html>`)
}

func (h *Handler) submitRequest(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	name := r.FormValue("name")
	description := r.FormValue("description")
	reason := r.FormValue("reason")

	if name == "" {
		http.Error(w, "Drink name is required", http.StatusBadRequest)
		return
	}

	if err := CreateRequest(r.Context(), h.db, userID, name, description, reason); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	h.notifSvc.NotifyAdmins(r.Context(),
		notification.TypeAdminNewDrinkRequest,
		"New drink request",
		fmt.Sprintf("%q has been requested.", name),
		"/admin/drinks/requests",
	)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h2>Request submitted</h2>
<p>Thanks! An admin will review your request.</p>
<p><a href="/drinks">Back to drinks</a></p>
</body></html>`)
}

func (h *Handler) approveRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	adminID := middleware.GetUserID(r)

	requesterID, err := ApproveRequest(r.Context(), h.db, id, adminID)
	if err != nil {
		http.Error(w, "Could not approve: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeDrinkRequestApproved,
		"Drink request approved",
		"Your drink request has been approved and added to the list.",
		"/drinks",
	)

	// HTMX or plain redirect
	if r.Header.Get("HX-Request") != "" {
		fmt.Fprint(w, "Approved")
		return
	}
	http.Redirect(w, r, "/admin/drinks/requests", http.StatusSeeOther)
}

func (h *Handler) rejectRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	adminID := middleware.GetUserID(r)
	note := r.FormValue("note")

	requesterID, err := RejectRequest(r.Context(), h.db, id, adminID, note)
	if err != nil {
		http.Error(w, "Could not reject: "+err.Error(), http.StatusInternalServerError)
		return
	}

	msg := "Your drink request was not approved."
	if note != "" {
		msg += " Note: " + note
	}
	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeDrinkRequestRejected,
		"Drink request rejected",
		msg,
		"/drinks",
	)

	if r.Header.Get("HX-Request") != "" {
		fmt.Fprint(w, "Rejected")
		return
	}
	http.Redirect(w, r, "/admin/drinks/requests", http.StatusSeeOther)
}
