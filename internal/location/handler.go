package location

import (
	"io"
	"net/http"
	"net/url"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /location/search", requireAuth(http.HandlerFunc(h.search)))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "q parameter required", http.StatusBadRequest)
		return
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("format", "json")
	params.Set("limit", "5")
	params.Set("addressdetails", "1")

	req, err := http.NewRequestWithContext(r.Context(), "GET",
		"https://nominatim.openstreetmap.org/search?"+params.Encode(), nil)
	if err != nil {
		http.Error(w, "Location search unavailable", http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "French75Tracker/1.0 (https://french75.paftech.se)")
	req.Header.Set("Accept-Language", "en")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Location search unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}
