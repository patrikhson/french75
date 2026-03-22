package user

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/middleware"
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
}

type profile struct {
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

	var p profile
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
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">
</head>
<body>%s<main>
<h2>%s</h2>
<p>@%s · %d check-ins · %d followers · %d following</p>`,
		p.DisplayName, layout.Nav(middleware.GetUserRole(r)), p.DisplayName, p.Username,
		p.CheckinCount, followers, following,
	)

	if p.Bio != "" {
		fmt.Fprintf(w, `<p>%s</p>`, p.Bio)
	}

	if !isOwnProfile {
		label := "Follow"
		if isFollowing {
			label = "Unfollow"
		}
		fmt.Fprintf(w,
			`<button hx-post="/users/%s/follow" hx-target="this" hx-swap="outerHTML">%s</button>`,
			targetID, label)
	}

	fmt.Fprint(w, `<hr><h3>Check-ins</h3>`)

	for _, c := range checkins {
		thumbHTML := ""
		if c.Thumbnail != "" {
			thumbHTML = fmt.Sprintf(
				`<img src="%s/%s" style="width:60px;height:60px;object-fit:cover;float:right;">`,
				h.photoURLPrefix, c.Thumbnail)
		}
		fmt.Fprintf(w, `<div style="border:1px solid #ccc;padding:8px;margin:6px 0;">
  %s
  <strong>%s</strong> — %d/100<br>
  %s · %s<br>
  <small>%s</small>
  <p>%s</p>
  <a href="/checkins/%s">View</a>
</div>`,
			thumbHTML,
			c.DrinkName, c.Score,
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
		fmt.Fprint(w, `<hr><h3>Map</h3><div id="profileMap" style="height:300px;border-radius:4px;"></div>
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

	fmt.Fprint(w, `</main></body></html>`)
}
