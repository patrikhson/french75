package checkin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/middleware"
)

type Handler struct {
	db      *pgxpool.Pool
	photoURLPrefix string
}

func NewHandler(db *pgxpool.Pool, photoURLPrefix string) *Handler {
	return &Handler{db: db, photoURLPrefix: photoURLPrefix}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /checkins/new", requireAuth(http.HandlerFunc(h.showNew)))
	mux.Handle("POST /checkins", requireAuth(http.HandlerFunc(h.create)))
	mux.Handle("GET /checkins/{id}", requireAuth(http.HandlerFunc(h.show)))
}

func (h *Handler) showNew(w http.ResponseWriter, r *http.Request) {
	// Load active drinks for the selector
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name FROM drinks WHERE active=TRUE ORDER BY name`)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type drinkOpt struct{ ID, Name string }
	var drinks []drinkOpt
	for rows.Next() {
		var d drinkOpt
		rows.Scan(&d.ID, &d.Name)
		drinks = append(drinks, d)
	}

	today := time.Now().UTC().Format("2006-01-02")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>New Check-in — French 75 Tracker</title></head>
<body>
<h2>New Check-in</h2>
<form id="checkinForm" method="POST" action="/checkins">

  <label>Drink<br>
  <select name="drink_id" required>`)
	for _, d := range drinks {
		fmt.Fprintf(w, `<option value="%s">%s</option>`, d.ID, d.Name)
	}
	fmt.Fprintf(w, `</select></label><br><br>

  <label>Score (0–100)<br>
  <input type="range" name="score" min="0" max="100" value="75" oninput="this.nextElementSibling.value=this.value">
  <output>75</output></label><br><br>

  <label>Review<br>
  <textarea name="review" rows="4" required placeholder="How was it?"></textarea></label><br><br>

  <label>Date<br>
  <input type="date" name="drink_date" value="%s" required max="%s"></label><br><br>

  <label>Location<br>
  <input type="text" id="locationSearch" placeholder="Search for a bar or venue...">
  <div id="locationResults"></div>
  <input type="hidden" name="location_name" id="locationName" required>
  <input type="hidden" name="location_lat" id="locationLat" required>
  <input type="hidden" name="location_lng" id="locationLng" required>
  <input type="hidden" name="location_osm_id" id="locationOsmId">
  <input type="hidden" name="location_osm_type" id="locationOsmType">
  <div id="locationDisplay"></div>
  </label><br><br>

  <input type="hidden" name="submission_lat" id="submissionLat">
  <input type="hidden" name="submission_lng" id="submissionLng">
  <input type="hidden" name="submission_accuracy" id="submissionAccuracy">

  <label>Photo (required)<br>
  <input type="file" id="photoInput" accept="image/jpeg,image/png" multiple></label>
  <div id="photoPreview"></div>
  <div id="photoIds"></div><br>

  <button type="submit">Submit Check-in</button>
</form>

<script>
// GPS capture
if (navigator.geolocation) {
  navigator.geolocation.getCurrentPosition(pos => {
    document.getElementById('submissionLat').value = pos.coords.latitude;
    document.getElementById('submissionLng').value = pos.coords.longitude;
    document.getElementById('submissionAccuracy').value = pos.coords.accuracy;
  });
}

// Location search
let locationTimer;
document.getElementById('locationSearch').addEventListener('input', e => {
  clearTimeout(locationTimer);
  const q = e.target.value.trim();
  if (q.length < 3) { document.getElementById('locationResults').innerHTML=''; return; }
  locationTimer = setTimeout(() => searchLocation(q), 400);
});

async function searchLocation(q) {
  const resp = await fetch('/location/search?q=' + encodeURIComponent(q));
  if (!resp.ok) return;
  const results = await resp.json();
  const div = document.getElementById('locationResults');
  div.innerHTML = '';
  (results || []).forEach(r => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.textContent = r.display_name;
    btn.style.display = 'block';
    btn.addEventListener('click', () => {
      document.getElementById('locationName').value = r.display_name;
      document.getElementById('locationLat').value = r.lat;
      document.getElementById('locationLng').value = r.lon;
      document.getElementById('locationOsmId').value = r.osm_id || '';
      document.getElementById('locationOsmType').value = r.osm_type || '';
      document.getElementById('locationDisplay').textContent = '✓ ' + r.display_name;
      document.getElementById('locationSearch').value = '';
      div.innerHTML = '';
    });
    div.appendChild(btn);
  });
}

// Photo upload
document.getElementById('photoInput').addEventListener('change', async e => {
  for (const file of e.target.files) {
    await uploadPhoto(file);
  }
  e.target.value = '';
});

async function uploadPhoto(file) {
  const fd = new FormData();
  fd.append('photo', file);
  const resp = await fetch('/photos/upload', {method:'POST', body:fd});
  if (!resp.ok) { alert('Photo upload failed: ' + await resp.text()); return; }
  const data = await resp.json();

  // Add hidden input with photo ID
  const input = document.createElement('input');
  input.type = 'hidden';
  input.name = 'photo_ids';
  input.value = data.id;
  document.getElementById('photoIds').appendChild(input);

  // Show thumbnail preview
  const img = document.createElement('img');
  img.src = data.thumbnail_url;
  img.style.cssText = 'width:80px;height:80px;object-fit:cover;margin:4px;';
  document.getElementById('photoPreview').appendChild(img);
}
</script>
</body></html>`, today, today)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r)
	userRole := middleware.GetUserRole(r)

	score, err := strconv.Atoi(r.FormValue("score"))
	if err != nil {
		http.Error(w, "Invalid score", http.StatusBadRequest)
		return
	}

	photoIDs := r.Form["photo_ids"]

	p := CreateParams{
		UserID:    userID,
		UserRole:  userRole,
		DrinkID:   r.FormValue("drink_id"),
		Score:     score,
		Review:    strings.TrimSpace(r.FormValue("review")),
		DrinkDate: r.FormValue("drink_date"),
		LocationName: r.FormValue("location_name"),
		PhotoIDs:  photoIDs,
	}

	if lat, err := strconv.ParseFloat(r.FormValue("location_lat"), 64); err == nil {
		p.LocationLat = lat
	}
	if lng, err := strconv.ParseFloat(r.FormValue("location_lng"), 64); err == nil {
		p.LocationLng = lng
	}
	if osmID, err := strconv.ParseInt(r.FormValue("location_osm_id"), 10, 64); err == nil {
		p.LocationOsmID = &osmID
	}
	if t := r.FormValue("location_osm_type"); t != "" {
		p.LocationOsmType = &t
	}
	if lat, err := strconv.ParseFloat(r.FormValue("submission_lat"), 64); err == nil {
		p.SubmissionLat = &lat
	}
	if lng, err := strconv.ParseFloat(r.FormValue("submission_lng"), 64); err == nil {
		p.SubmissionLng = &lng
	}
	if acc, err := strconv.ParseFloat(r.FormValue("submission_accuracy"), 64); err == nil {
		p.SubmissionAccuracy = &acc
	}

	id, err := Create(r.Context(), h.db, p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/checkins/"+id, http.StatusSeeOther)
}

func (h *Handler) show(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ci, photos, err := GetByID(r.Context(), h.db, id)
	if err != nil {
		http.Error(w, "Check-in not found", http.StatusNotFound)
		return
	}

	// Only the owner can see pending check-ins; others see public ones
	userID := middleware.GetUserID(r)
	userRole := middleware.GetUserRole(r)
	if ci.Status != "public" && ci.UserID != userID && userRole != "admin" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s Check-in — French 75 Tracker</title></head>
<body>
<h2>%s</h2>
<p><strong>Score:</strong> %d/100</p>
<p><strong>Date:</strong> %s</p>
<p><strong>Location:</strong> %s</p>
<p><strong>Status:</strong> %s</p>
<blockquote>%s</blockquote>
`,
		ci.DrinkName, ci.DrinkName,
		ci.Score,
		ci.DrinkDate.Format("2 January 2006"),
		ci.LocationName,
		ci.Status,
		ci.Review,
	)

	for _, p := range photos {
		fmt.Fprintf(w, `<img src="%s/%s" style="max-width:100%%;display:block;margin:8px 0;">`,
			h.photoURLPrefix, p.ThumbnailURL)
	}

	canEdit := ci.UserID == userID && time.Now().Before(ci.EditDeadline)
	if canEdit {
		fmt.Fprintf(w, `<p><small>You can edit this check-in until %s.</small></p>`,
			ci.EditDeadline.Format("15:04 on 2 Jan"))
	}

	fmt.Fprint(w, `<p><a href="/checkins/new">New check-in</a></p></body></html>`)
}
