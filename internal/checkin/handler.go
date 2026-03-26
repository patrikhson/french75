package checkin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/mail"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
)

type Handler struct {
	db             *pgxpool.Pool
	photoURLPrefix string
	notifSvc       *notification.Service
}

func NewHandler(db *pgxpool.Pool, photoURLPrefix string, mailer *mail.Mailer, baseURL string) *Handler {
	return &Handler{
		db:             db,
		photoURLPrefix: photoURLPrefix,
		notifSvc:       notification.NewService(db, mailer, baseURL),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /checkins/new", requireAuth(http.HandlerFunc(h.showNew)))
	mux.Handle("POST /checkins", requireAuth(http.HandlerFunc(h.create)))
	mux.Handle("GET /checkins/{id}", requireAuth(http.HandlerFunc(h.show)))
	mux.Handle("GET /checkins/{id}/edit", requireAuth(http.HandlerFunc(h.showEdit)))
	mux.Handle("POST /checkins/{id}/edit", requireAuth(http.HandlerFunc(h.edit)))
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

	role := middleware.GetUserRole(r)
	userID := middleware.GetUserID(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)
	today := time.Now().UTC().Format("2006-01-02")

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>New Check-in — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">
</head>
<body>`)
	fmt.Fprint(w, layout.Nav(role, unread))
	fmt.Fprint(w, `<main>
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
  <div id="locationMap" style="height:200px;margin-top:8px;display:none;border-radius:4px;"></div>
  </label><br>
  <details style="margin-bottom:8px;">
    <summary style="cursor:pointer;color:#555;font-size:0.9em;">Can't find it? Enter location manually</summary>
    <div style="margin-top:8px;padding:8px;background:#f9f9f9;border-radius:4px;">
      <label>Venue name<br>
      <input type="text" id="manualName" placeholder="e.g. Operabaren"></label><br><br>
      <button type="button" id="useGpsBtn">Use my current GPS position</button>
      <span id="gpsStatus" style="margin-left:8px;font-size:0.85em;color:#555;"></span>
    </div>
  </details>

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
  if (!results || results.length === 0) {
    div.textContent = 'No results — try the manual entry below.';
    return;
  }
  results.forEach(r => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.textContent = r.display_name;
    btn.style.display = 'block';
    btn.addEventListener('click', () => {
      setLocation(r.display_name, parseFloat(r.lat), parseFloat(r.lon), r.osm_id||'', r.osm_type||'');
      document.getElementById('locationSearch').value = '';
      div.innerHTML = '';
    });
    div.appendChild(btn);
  });
}

function setLocation(name, lat, lng, osmId, osmType) {
  document.getElementById('locationName').value = name;
  document.getElementById('locationLat').value = lat;
  document.getElementById('locationLng').value = lng;
  document.getElementById('locationOsmId').value = osmId;
  document.getElementById('locationOsmType').value = osmType;
  document.getElementById('locationDisplay').textContent = '✓ ' + name;
  showLocationMap(lat, lng, name);
}

// Manual GPS entry
let _gpsPosition = null;
if (navigator.geolocation) {
  navigator.geolocation.getCurrentPosition(pos => {
    _gpsPosition = pos;
    document.getElementById('submissionLat').value = pos.coords.latitude;
    document.getElementById('submissionLng').value = pos.coords.longitude;
    document.getElementById('submissionAccuracy').value = pos.coords.accuracy;
  });
}

document.getElementById('useGpsBtn').addEventListener('click', () => {
  const name = document.getElementById('manualName').value.trim();
  if (!name) { alert('Please enter a venue name first.'); return; }
  const status = document.getElementById('gpsStatus');
  if (_gpsPosition) {
    setLocation(name, _gpsPosition.coords.latitude, _gpsPosition.coords.longitude, '', '');
    status.textContent = '✓ Location set';
  } else if (navigator.geolocation) {
    status.textContent = 'Getting GPS...';
    navigator.geolocation.getCurrentPosition(pos => {
      _gpsPosition = pos;
      document.getElementById('submissionLat').value = pos.coords.latitude;
      document.getElementById('submissionLng').value = pos.coords.longitude;
      document.getElementById('submissionAccuracy').value = pos.coords.accuracy;
      setLocation(name, pos.coords.latitude, pos.coords.longitude, '', '');
      status.textContent = '✓ Location set';
    }, () => { status.textContent = 'GPS unavailable.'; });
  } else {
    status.textContent = 'GPS not available on this device.';
  }
});

// Location mini-map
let _map = null, _marker = null;
function showLocationMap(lat, lng, name) {
  const el = document.getElementById('locationMap');
  el.style.display = 'block';
  if (!_map) {
    _map = L.map('locationMap').setView([lat, lng], 15);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '© OpenStreetMap contributors', maxZoom: 19
    }).addTo(_map);
    _marker = L.marker([lat, lng]).addTo(_map).bindPopup(name).openPopup();
  } else {
    _map.setView([lat, lng], 15);
    _marker.setLatLng([lat, lng]).bindPopup(name).openPopup();
  }
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
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
</main></body></html>`, today, today)
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

	// Notify admins if the check-in landed in pending status.
	var status string
	h.db.QueryRow(r.Context(), `SELECT status::text FROM check_ins WHERE id=$1`, id).Scan(&status)
	if status == "pending" {
		h.notifSvc.NotifyAdmins(r.Context(),
			notification.TypeAdminNewCheckin,
			"New check-in pending",
			"A new check-in has been submitted and requires review.",
			"/admin/checkins/pending",
		)
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
<title>%s Check-in — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">
</head>
<body>%s<main>
<p><a href="/">← Feed</a></p>
<h2>%s</h2>
<p><strong>Score:</strong> %d/100</p>
<p><strong>Date:</strong> %s</p>
<p><strong>Location:</strong> %s</p>
<p><strong>Status:</strong> %s</p>
<blockquote>%s</blockquote>
`,
		ci.DrinkName, layout.Nav(userRole, notification.UnreadCount(r.Context(), h.db, userID)), ci.DrinkName,
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
		fmt.Fprintf(w, `<p><small>You can edit this check-in until %s. <a href="/checkins/%s/edit">Edit</a></small></p>`,
			ci.EditDeadline.Format("15:04 on 2 Jan"), ci.ID)
	}

	fmt.Fprintf(w,
		`<div id="map" style="height:250px;margin:16px 0;border-radius:4px;"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script>
(function(){
  const map = L.map('map').setView([%f, %f], 15);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '© OpenStreetMap contributors', maxZoom: 19
  }).addTo(map);
  L.marker([%f, %f]).addTo(map).bindPopup(%q).openPopup();
})();
</script>`,
		ci.LocationLat, ci.LocationLng,
		ci.LocationLat, ci.LocationLng,
		ci.LocationName,
	)
	fmt.Fprint(w, `</main></body></html>`)
}

func (h *Handler) showEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)
	userRole := middleware.GetUserRole(r)

	ci, _, err := GetByID(r.Context(), h.db, id)
	if err != nil || ci.UserID != userID || !time.Now().Before(ci.EditDeadline) {
		http.Error(w, "Not found or edit window closed", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Edit Check-in — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
</head>
<body>%s<main>
<h2>Edit Check-in</h2>
<form method="POST" action="/checkins/%s/edit">
  <label>Score (0–100)<br>
  <input type="range" name="score" min="0" max="100" value="%d" oninput="this.nextElementSibling.value=this.value">
  <output>%d</output></label><br><br>

  <label>Review<br>
  <textarea name="review" rows="4" required>%s</textarea></label><br><br>

  <button type="submit">Save changes</button>
  <a href="/checkins/%s">Cancel</a>
</form>
</main></body></html>`,
		layout.Nav(userRole, notification.UnreadCount(r.Context(), h.db, userID)),
		ci.ID, ci.Score, ci.Score, ci.Review, ci.ID)
}

func (h *Handler) edit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := middleware.GetUserID(r)

	ci, _, err := GetByID(r.Context(), h.db, id)
	if err != nil || ci.UserID != userID || !time.Now().Before(ci.EditDeadline) {
		http.Error(w, "Not found or edit window closed", http.StatusNotFound)
		return
	}

	score, err := strconv.Atoi(r.FormValue("score"))
	if err != nil || score < 0 || score > 100 {
		http.Error(w, "Invalid score", http.StatusBadRequest)
		return
	}
	review := strings.TrimSpace(r.FormValue("review"))
	if review == "" {
		http.Error(w, "Review is required", http.StatusBadRequest)
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE check_ins SET score=$1, review=$2 WHERE id=$3 AND user_id=$4`,
		score, review, id, userID,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/checkins/"+id, http.StatusSeeOther)
}
