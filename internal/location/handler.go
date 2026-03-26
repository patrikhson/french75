package location

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
)

type Handler struct {
	placesKey      string
	db             *pgxpool.Pool
	photoURLPrefix string
}

func NewHandler(placesKey string, db *pgxpool.Pool, photoURLPrefix string) *Handler {
	return &Handler{placesKey: placesKey, db: db, photoURLPrefix: photoURLPrefix}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /location/search", requireAuth(http.HandlerFunc(h.search)))
	mux.Handle("GET /locations", requireAuth(http.HandlerFunc(h.venues)))
}

// VenueLink returns the URL for a venue detail page.
func VenueLink(name string) string {
	return "/locations?name=" + url.QueryEscape(name)
}

// ---------------------------------------------------------------
// Places autocomplete
// ---------------------------------------------------------------

// result is the JSON shape our frontend expects (same as before).
type result struct {
	DisplayName string  `json:"display_name"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	OsmID       string  `json:"osm_id"`
	OsmType     string  `json:"osm_type"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "q parameter required", http.StatusBadRequest)
		return
	}

	if h.placesKey == "" {
		http.Error(w, "location search not configured", http.StatusServiceUnavailable)
		return
	}

	body, _ := json.Marshal(map[string]any{
		"input":        q,
		"languageCode": "sv",
	})

	req, err := http.NewRequestWithContext(r.Context(), "POST",
		"https://places.googleapis.com/v1/places:autocomplete", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "location search unavailable", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", h.placesKey)
	req.Header.Set("X-Goog-FieldMask", "suggestions.placePrediction.placeId,suggestions.placePrediction.text")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "location search unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var ac struct {
		Suggestions []struct {
			PlacePrediction struct {
				PlaceID string `json:"placeId"`
				Text    struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"placePrediction"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(raw, &ac); err != nil || len(ac.Suggestions) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	// For each suggestion, fetch the place details to get lat/lng.
	var results []result
	for _, s := range ac.Suggestions {
		placeID := s.PlacePrediction.PlaceID
		name := s.PlacePrediction.Text.Text

		detReq, err := http.NewRequestWithContext(r.Context(), "GET",
			"https://places.googleapis.com/v1/places/"+placeID, nil)
		if err != nil {
			continue
		}
		detReq.Header.Set("X-Goog-Api-Key", h.placesKey)
		detReq.Header.Set("X-Goog-FieldMask", "location,displayName")

		detResp, err := http.DefaultClient.Do(detReq)
		if err != nil {
			continue
		}
		var det struct {
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
		}
		detBody, _ := io.ReadAll(detResp.Body)
		detResp.Body.Close()
		if err := json.Unmarshal(detBody, &det); err != nil {
			continue
		}

		results = append(results, result{
			DisplayName: name,
			Lat:         fmt.Sprintf("%f", det.Location.Latitude),
			Lon:         fmt.Sprintf("%f", det.Location.Longitude),
			OsmID:       placeID,
			OsmType:     "google",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ---------------------------------------------------------------
// Venue list + detail (both served at GET /locations)
// ---------------------------------------------------------------

func (h *Handler) venues(w http.ResponseWriter, r *http.Request) {
	if name := r.URL.Query().Get("name"); name != "" {
		h.venueDetail(w, r, name)
		return
	}
	h.venueList(w, r)
}

func (h *Handler) venueList(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)

	venues, err := ListVenues(r.Context(), h.db)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Venues", role, unread, layout.LeafletCSS))
	fmt.Fprint(w, `<h2>Venues</h2>`)

	if len(venues) == 0 {
		fmt.Fprint(w, `<p>No venues yet.</p>`)
		fmt.Fprint(w, layout.PageEnd())
		return
	}

	// Map
	fmt.Fprint(w, `<div id="venueMap" class="map-container"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script>(function(){
  const map = L.map('venueMap');
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
    attribution:'© OpenStreetMap contributors',maxZoom:19
  }).addTo(map);
  const group = L.featureGroup();
`)
	for _, v := range venues {
		popup := fmt.Sprintf(`<a href=\"%s\">%s</a><br>%d check-in(s), avg %.1f`,
			VenueLink(v.Name), v.Name, v.CheckinCount, v.AvgScore)
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

	// Table
	fmt.Fprint(w, `<div class="table-wrap"><table>
<thead><tr>
  <th>Venue</th><th>Check-ins</th><th>Avg score</th><th>Drinks</th><th>Visitors</th>
</tr></thead><tbody>`)
	for _, v := range venues {
		fmt.Fprintf(w, `<tr>
  <td><a href="%s">%s</a></td>
  <td>%d</td><td>%.1f</td><td>%d</td><td>%d</td>
</tr>`, VenueLink(v.Name), v.Name, v.CheckinCount, v.AvgScore, v.UniqueDrinks, v.UniqueUsers)
	}
	fmt.Fprint(w, `</tbody></table></div>`)
	fmt.Fprint(w, layout.PageEnd())
}

func (h *Handler) venueDetail(w http.ResponseWriter, r *http.Request, name string) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	unread := notification.UnreadCount(r.Context(), h.db, userID)

	stats, err := GetVenueStats(r.Context(), h.db, name)
	if err != nil {
		http.Error(w, "Venue not found", http.StatusNotFound)
		return
	}

	drinks, _ := DrinksAtVenue(r.Context(), h.db, name)
	checkins, _ := CheckinsAtVenue(r.Context(), h.db, name)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart(name, role, unread, layout.LeafletCSS))
	fmt.Fprintf(w, `<p><a href="/locations">← All venues</a></p>
<h2>%s</h2>
<p class="card-meta">%d check-in(s) · avg score %.1f · %d drink(s) · %d visitor(s)</p>`,
		stats.Name, stats.CheckinCount, stats.AvgScore, stats.UniqueDrinks, stats.UniqueUsers)

	// Single-point map
	fmt.Fprintf(w, `<div id="venueMap" class="map-container"></div>
<script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
<script>(function(){
  const map = L.map('venueMap').setView([%f,%f],15);
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',{
    attribution:'© OpenStreetMap contributors',maxZoom:19
  }).addTo(map);
  L.marker([%f,%f]).addTo(map).bindPopup(%q).openPopup();
})();
</script>`, stats.Lat, stats.Lng, stats.Lat, stats.Lng, stats.Name)

	// Drinks at this venue
	if len(drinks) > 0 {
		fmt.Fprint(w, `<h3>Drinks</h3><div class="table-wrap"><table>
<thead><tr><th>Drink</th><th>Check-ins</th><th>Avg</th><th>Best</th><th>Worst</th></tr></thead><tbody>`)
		for _, d := range drinks {
			fmt.Fprintf(w, `<tr>
  <td><a href="/drinks/%s">%s</a></td>
  <td>%d</td><td>%.1f</td><td>%d</td><td>%d</td>
</tr>`, d.DrinkID, d.DrinkName, d.CheckinCount, d.AvgScore, d.MaxScore, d.MinScore)
		}
		fmt.Fprint(w, `</tbody></table></div>`)
	}

	// All check-ins at this venue
	if len(checkins) > 0 {
		fmt.Fprint(w, `<h3>Check-ins</h3><div class="table-wrap"><table>
<thead><tr><th>User</th><th>Drink</th><th>Score</th><th>Date</th><th>Review</th><th></th></tr></thead><tbody>`)
		for _, ci := range checkins {
			fmt.Fprintf(w, `<tr>
  <td><a href="/users/%s">%s</a></td>
  <td><a href="/drinks/%s">%s</a></td>
  <td>%d</td>
  <td>%s</td>
  <td>%s</td>
  <td><a href="/checkins/%s">View</a></td>
</tr>`,
				ci.UserID, ci.UserName,
				ci.DrinkID, ci.DrinkName,
				ci.Score,
				ci.DrinkDate.Format("2 Jan 2006"),
				ci.Review,
				ci.ID,
			)
		}
		fmt.Fprint(w, `</tbody></table></div>`)
	}

	fmt.Fprint(w, layout.PageEnd())
}
