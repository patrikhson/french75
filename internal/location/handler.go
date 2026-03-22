package location

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Handler struct {
	placesKey string
}

func NewHandler(placesKey string) *Handler { return &Handler{placesKey: placesKey} }

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /location/search", requireAuth(http.HandlerFunc(h.search)))
}

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
