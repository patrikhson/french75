package drink

import (
	"fmt"
	"net/http"
	"net/url"

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
	mux.Handle("GET /drinks/{id}", requireAuth(http.HandlerFunc(h.drinkDetail)))
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
	fmt.Fprint(w, layout.PageStart("Drinks", role, unread, ""))
	fmt.Fprint(w, `<h2>Drinks</h2><ul>`)
	for _, d := range drinks {
		fmt.Fprintf(w, `<li><strong><a href="/drinks/%s">%s</a></strong>`, d.ID, d.Name)
		if d.Description != "" {
			fmt.Fprintf(w, ` — %s`, d.Description)
		}
		fmt.Fprint(w, `</li>`)
	}
	fmt.Fprint(w, `</ul>
<p><a href="/locations">Browse all venues →</a></p>
<hr>
<h3>Request a drink</h3>
<form class="form" method="POST" action="/drinks/request">
  <label>Drink name<input type="text" name="name" required></label>
  <label>Description<textarea name="description"></textarea></label>
  <label>Why should we add it?<textarea name="reason"></textarea></label>
  <button type="submit">Submit request</button>
</form>`)
	fmt.Fprint(w, layout.PageEnd())
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

	requestID, err := CreateRequest(r.Context(), h.db, userID, name, description, reason)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	h.notifSvc.NotifyAdmins(r.Context(),
		notification.TypeAdminNewDrinkRequest,
		"New drink request",
		fmt.Sprintf("%q has been requested.", name),
		"/admin/drinks/requests",
		requestID,
	)

	http.Redirect(w, r, "/drinks?requested=1", http.StatusSeeOther)
}

func (h *Handler) approveRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	adminID := middleware.GetUserID(r)

	requesterID, err := ApproveRequest(r.Context(), h.db, id, adminID)
	if err != nil {
		http.Error(w, "Could not approve: "+err.Error(), http.StatusInternalServerError)
		return
	}

	notification.AutoManageByEntity(r.Context(), h.db, notification.TypeAdminNewDrinkRequest, id)
	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeDrinkRequestApproved,
		"Drink request approved",
		"Your drink request has been approved and added to the list.",
		"/drinks",
		id,
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
	notification.AutoManageByEntity(r.Context(), h.db, notification.TypeAdminNewDrinkRequest, id)
	h.notifSvc.Notify(r.Context(), requesterID,
		notification.TypeDrinkRequestRejected,
		"Drink request rejected",
		msg,
		"/drinks",
		id,
	)

	if r.Header.Get("HX-Request") != "" {
		fmt.Fprint(w, "Rejected")
		return
	}
	http.Redirect(w, r, "/admin/drinks/requests", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Drink detail page
// ---------------------------------------------------------------

func (h *Handler) drinkDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)

	stats, err := GetDrinkStats(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "Drink not found", http.StatusNotFound)
		return
	}

	venues, _ := VenuesForDrink(r.Context(), h.db, id)
	topUsers, _ := TopUsersForDrink(r.Context(), h.db, id)
	checkins, _ := RecentCheckinsForDrink(r.Context(), h.db, id)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart(stats.Name, role, unread, layout.LeafletCSS))
	fmt.Fprintf(w, `<p><a href="/drinks">← Drinks</a></p>
<h2>%s</h2>`, stats.Name)
	if stats.Description != "" {
		fmt.Fprintf(w, `<p>%s</p>`, stats.Description)
	}
	if stats.CheckinCount > 0 {
		fmt.Fprintf(w,
			`<p class="card-meta">%d check-in(s) · avg score %.1f · best %d · worst %d</p>`,
			stats.CheckinCount, stats.AvgScore, stats.MaxScore, stats.MinScore)
	} else {
		fmt.Fprint(w, `<p class="card-meta">No check-ins yet.</p>`)
	}

	// Venues ranked by avg score + map
	if len(venues) > 0 {
		fmt.Fprint(w, `<h3>Venues</h3>
<div class="table-wrap"><table>
<thead><tr><th>Venue</th><th>Check-ins</th><th>Avg score</th><th>Best</th></tr></thead><tbody>`)
		for _, v := range venues {
			fmt.Fprintf(w, `<tr>
  <td><a href="/locations?name=%s">%s</a></td>
  <td>%d</td><td>%.1f</td><td>%d</td>
</tr>`, url.QueryEscape(v.Name), v.Name, v.CheckinCount, v.AvgScore, v.BestScore)
		}
		fmt.Fprint(w, `</tbody></table></div>`)

		// Map of venues for this drink
		fmt.Fprint(w, `<div id="drinkMap" class="map-container"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script>(function(){
  const map = L.map('drinkMap');
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
    attribution:'© OpenStreetMap contributors',maxZoom:19
  }).addTo(map);
  const group = L.featureGroup();
`)
		for _, v := range venues {
			popup := fmt.Sprintf(`<a href=\"/locations?name=%s\">%s</a><br>avg %.1f, best %d`,
				url.QueryEscape(v.Name), v.Name, v.AvgScore, v.BestScore)
			fmt.Fprintf(w, "  L.marker([%f,%f]).bindPopup(%q).addTo(group);\n",
				v.Lat, v.Lng, popup)
		}
		fmt.Fprint(w, `  group.addTo(map);
  if (group.getLayers().length === 1) {
    map.setView(group.getLayers()[0].getLatLng(), 14);
  } else {
    map.fitBounds(group.getBounds().pad(0.1));
  }
})();
</script>`)
	}

	// Top scorers
	if len(topUsers) > 0 {
		fmt.Fprint(w, `<h3>Top scorers</h3>
<div class="table-wrap"><table>
<thead><tr><th>User</th><th>Check-ins</th><th>Avg score</th><th>Best</th></tr></thead><tbody>`)
		for _, u := range topUsers {
			fmt.Fprintf(w, `<tr>
  <td><a href="/users/%s">%s</a></td>
  <td>%d</td><td>%.1f</td><td>%d</td>
</tr>`, u.UserID, u.UserName, u.Count, u.AvgScore, u.BestScore)
		}
		fmt.Fprint(w, `</tbody></table></div>`)
	}

	// Recent check-ins
	if len(checkins) > 0 {
		fmt.Fprint(w, `<h3>Recent check-ins</h3>`)
		for _, ci := range checkins {
			fmt.Fprintf(w, `<div class="card">
  <div class="card-title">%d/100 — <a href="/users/%s">%s</a></div>
  <div class="card-meta"><a href="/locations?name=%s">%s</a> · %s</div>
  <p>%s</p>
  <a href="/checkins/%s">View</a>
</div>`,
				ci.Score, ci.UserID, ci.UserName,
				url.QueryEscape(ci.LocationName), ci.LocationName,
				ci.DrinkDate.Format("2 Jan 2006"),
				ci.Review,
				ci.ID,
			)
		}
	}

	fmt.Fprint(w, layout.PageEnd())
}
