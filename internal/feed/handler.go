package feed

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db             *pgxpool.Pool
	photoURLPrefix string
}

func NewHandler(db *pgxpool.Pool, photoURLPrefix string) *Handler {
	return &Handler{db: db, photoURLPrefix: photoURLPrefix}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /", requireAuth(http.HandlerFunc(h.index)))
	mux.Handle("GET /feed", requireAuth(http.HandlerFunc(h.index)))
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" — let the mux fall through for other paths
	if r.URL.Path != "/" && r.URL.Path != "/feed" {
		http.NotFound(w, r)
		return
	}

	before := time.Now().Add(time.Second) // slightly in the future to include "now"
	if c := r.URL.Query().Get("before"); c != "" {
		if t, err := time.Parse(time.RFC3339, c); err == nil {
			before = t
		}
	}

	items, err := List(r.Context(), h.db, before)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	isHTMX := r.Header.Get("HX-Request") != ""

	if !isHTMX {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
</head>
<body>
<header>
  <h1>French 75 Tracker</h1>
  <nav>
    <a href="/checkins/new">+ Check-in</a> |
    <a href="/drinks">Drinks</a> |
    <a href="/auth/logout" hx-post="/auth/logout" hx-push-url="true">Log out</a>
  </nav>
</header>
<main id="feed">`)
	}

	for _, it := range items {
		h.renderCard(w, it)
	}

	if len(items) == pageSize {
		last := items[len(items)-1].SubmittedAt
		fmt.Fprintf(w,
			`<div hx-get="/feed?before=%s" hx-trigger="revealed" hx-swap="outerHTML" hx-target="this">
			   <a href="/feed?before=%s">Load more</a>
			 </div>`,
			last.UTC().Format(time.RFC3339),
			last.UTC().Format(time.RFC3339),
		)
	}

	if !isHTMX {
		fmt.Fprint(w, `</main></body></html>`)
	}
}

func (h *Handler) renderCard(w http.ResponseWriter, it Item) {
	thumbHTML := ""
	if it.Thumbnail != "" {
		thumbHTML = fmt.Sprintf(`<img src="%s/%s" alt="" style="width:80px;height:80px;object-fit:cover;float:right;">`,
			h.photoURLPrefix, it.Thumbnail)
	}

	fmt.Fprintf(w, `<article id="ci-%s" style="border:1px solid #ccc;padding:12px;margin:8px 0;border-radius:4px;">
  %s
  <div><strong>%s</strong> — %d/100</div>
  <div>%s · %s · %s</div>
  <p>%s</p>
  <div>
    <button hx-post="/checkins/%s/react?type=like" hx-target="#reaction-like-%s" hx-swap="outerHTML">
      <span id="reaction-like-%s">👍 %d</span>
    </button>
    <button hx-post="/checkins/%s/react?type=helpful" hx-target="#reaction-helpful-%s" hx-swap="outerHTML">
      <span id="reaction-helpful-%s">💡 %d</span>
    </button>
    <a href="/checkins/%s">View</a>
  </div>
</article>`,
		it.ID,
		thumbHTML,
		it.DrinkName, it.Score,
		it.UserName, it.LocationName, it.DrinkDate.Format("2 Jan 2006"),
		it.Review,
		it.ID, it.ID, it.ID, it.LikeCount,
		it.ID, it.ID, it.ID, it.HelpfulCount,
		it.ID,
	)
}
