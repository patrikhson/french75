package user

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
	"github.com/patrikhson/french75/internal/social"
)

type Handler struct {
	db             *pgxpool.Pool
	photoURLPrefix string
}

func NewHandler(db *pgxpool.Pool, photoURLPrefix string) *Handler {
	return &Handler{db: db, photoURLPrefix: photoURLPrefix}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /users/{id}", requireAuth(http.HandlerFunc(h.profile)))
	mux.Handle("GET /settings/profile", requireAuth(http.HandlerFunc(h.editProfile)))
	mux.Handle("POST /settings/profile", requireAuth(http.HandlerFunc(h.saveProfile)))
}

type profileData struct {
	ID           string
	Username     string
	DisplayName  string
	Bio          string
	CheckinCount int
	Role         string
}

func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	currentUserID := middleware.GetUserID(r)

	var p profileData
	err := h.db.QueryRow(r.Context(),
		`SELECT id, username, COALESCE(display_name, username), COALESCE(bio,''),
		        checkin_count, role::text
		 FROM users WHERE id=$1 AND is_banned=FALSE`,
		targetID,
	).Scan(&p.ID, &p.Username, &p.DisplayName, &p.Bio, &p.CheckinCount, &p.Role)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	followers, following := social.FollowCounts(r.Context(), h.db, targetID)
	isFollowing := currentUserID != targetID && social.IsFollowing(r.Context(), h.db, currentUserID, targetID)
	hasPending := currentUserID != targetID && social.HasPendingRequest(r.Context(), h.db, currentUserID, targetID)
	isOwnProfile := currentUserID == targetID

	// Recent public check-ins
	rows, err := h.db.Query(r.Context(),
		`SELECT c.id, d.name, c.score, LEFT(c.review, 200), c.drink_date,
		        c.location_name, COALESCE(ph.thumbnail_path,''), c.submitted_at,
		        c.location_lat, c.location_lng
		 FROM check_ins c
		 JOIN drinks d ON d.id = c.drink_id
		 LEFT JOIN LATERAL (
		     SELECT thumbnail_path FROM photos WHERE checkin_id=c.id ORDER BY sort_order LIMIT 1
		 ) ph ON true
		 WHERE c.user_id=$1 AND c.status='public'
		 ORDER BY c.submitted_at DESC LIMIT 20`,
		targetID,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type checkinRow struct {
		ID, DrinkName, Review, LocationName, Thumbnail string
		Score                                           int
		DrinkDate                                       time.Time
		SubmittedAt                                     time.Time
		Lat, Lng                                        float64
	}
	var checkins []checkinRow
	for rows.Next() {
		var c checkinRow
		rows.Scan(&c.ID, &c.DrinkName, &c.Score, &c.Review,
			&c.DrinkDate, &c.LocationName, &c.Thumbnail, &c.SubmittedAt,
			&c.Lat, &c.Lng)
		checkins = append(checkins, c)
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart(p.DisplayName, middleware.GetUserRole(r), notification.UnreadCount(r.Context(), h.db, currentUserID), layout.LeafletCSS))
	fmt.Fprintf(w, `<h2>%s</h2>
<p class="card-meta">@%s · %d check-ins · %d followers · %d following</p>`,
		p.DisplayName, p.Username,
		p.CheckinCount, followers, following,
	)

	if p.Bio != "" {
		fmt.Fprintf(w, `<p>%s</p>`, p.Bio)
	}

	if isOwnProfile {
		pendingCount := social.PendingRequestCount(r.Context(), h.db, currentUserID)
		fmt.Fprint(w, `<p><a href="/settings/profile">Edit profile</a>`)
		if pendingCount > 0 {
			fmt.Fprintf(w, ` &nbsp; <a href="/follow-requests">%d follow request(s) pending</a>`, pendingCount)
		}
		fmt.Fprint(w, `</p>`)
	} else {
		var btnState string
		switch {
		case isFollowing:
			btnState = "following"
		case hasPending:
			btnState = "requested"
		default:
			btnState = "none"
		}
		fmt.Fprint(w, followButtonHTML(targetID, btnState))
	}

	fmt.Fprint(w, `<hr><h3>Check-ins</h3>`)

	for _, c := range checkins {
		thumbHTML := ""
		if c.Thumbnail != "" {
			thumbHTML = fmt.Sprintf(
				`<img src="%s/%s" style="width:60px;height:60px;object-fit:cover;float:right;">`,
				h.photoURLPrefix, c.Thumbnail)
		}
		fmt.Fprintf(w, `<div class="card">
  %s
  <div class="card-title">%s — %s</div>
  <div class="card-meta">%s · %s · <small>%s</small></div>
  <p>%s</p>
  <a href="/checkins/%s">View</a>
</div>`,
			thumbHTML,
			c.DrinkName, layout.ScoreHTML(c.Score),
			c.LocationName, c.DrinkDate.Format("2 Jan 2006"),
			c.SubmittedAt.Format("2 Jan 2006 15:04"),
			c.Review,
			c.ID,
		)
	}

	if len(checkins) == 0 {
		fmt.Fprint(w, `<p>No public check-ins yet.</p>`)
	}

	if len(checkins) > 0 {
		fmt.Fprint(w, `<hr><h3>Map</h3><div id="profileMap" class="map-container"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script>(function(){
  const map = L.map('profileMap');
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
    attribution:'© OpenStreetMap contributors',maxZoom:19
  }).addTo(map);
  const group = L.featureGroup();
`)
		for _, c := range checkins {
			fmt.Fprintf(w, "  L.marker([%f,%f]).bindPopup('<a href=\"/checkins/%s\">%s</a>').addTo(group);\n",
				c.Lat, c.Lng, c.ID, c.DrinkName)
		}
		fmt.Fprint(w, `  group.addTo(map);
  if (group.getLayers().length === 1) {
    map.setView(group.getLayers()[0].getLatLng(), 14);
  } else {
    map.fitBounds(group.getBounds().pad(0.15));
  }
})();
</script>`)
	}

	fmt.Fprint(w, layout.PageEnd())
}

// followButtonHTML returns the initial follow button for the profile page.
// state: "following" | "requested" | "none"
func followButtonHTML(targetID, state string) string {
	switch state {
	case "following":
		return fmt.Sprintf(
			`<button class="btn" hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Unfollow</button>`,
			targetID)
	case "requested":
		return fmt.Sprintf(
			`<button class="btn" hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Cancel follow request</button>`,
			targetID)
	default:
		return fmt.Sprintf(
			`<button class="btn" hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">Request to follow</button>`,
			targetID)
	}
}

// ---------------------------------------------------------------
// Profile editing
// ---------------------------------------------------------------

func (h *Handler) editProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)

	var displayName, bio string
	h.db.QueryRow(r.Context(),
		`SELECT COALESCE(display_name,''), COALESCE(bio,'') FROM users WHERE id=$1`,
		userID,
	).Scan(&displayName, &bio)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Edit Profile", role, unread, ""))
	fmt.Fprintf(w, `<h2>Edit Profile</h2>
<form class="form" method="POST" action="/settings/profile">
  <label>Display name
    <input type="text" name="display_name" value="%s" maxlength="80">
  </label>
  <label>Bio
    <textarea name="bio" rows="4" maxlength="500">%s</textarea>
  </label>
  <div style="display:flex;gap:10px;align-items:center">
    <button type="submit" class="btn">Save</button>
    <a href="/users/%s">Cancel</a>
  </div>
</form>`, displayName, bio, userID)
	fmt.Fprint(w, layout.PageEnd())
}

func (h *Handler) saveProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	bio := strings.TrimSpace(r.FormValue("bio"))

	if len(displayName) > 80 {
		displayName = displayName[:80]
	}
	if len(bio) > 500 {
		bio = bio[:500]
	}

	var dnVal interface{} = displayName
	if displayName == "" {
		dnVal = nil
	}
	var bioVal interface{} = bio
	if bio == "" {
		bioVal = nil
	}

	_, err := h.db.Exec(r.Context(),
		`UPDATE users SET display_name=$1, bio=$2 WHERE id=$3`,
		dnVal, bioVal, userID,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/users/"+userID, http.StatusSeeOther)
}
