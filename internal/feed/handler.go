package feed

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
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
	mux.Handle("GET /feed/following", requireAuth(http.HandlerFunc(h.following)))
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
		fmt.Fprint(w, layout.PageStart("Feed", middleware.GetUserRole(r), notification.UnreadCount(r.Context(), h.db, middleware.GetUserID(r)), ""))
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
		fmt.Fprint(w, layout.PageEnd())
	}
}

func (h *Handler) following(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	before := time.Now().Add(time.Second)
	if c := r.URL.Query().Get("before"); c != "" {
		if t, err := time.Parse(time.RFC3339, c); err == nil {
			before = t
		}
	}

	items, err := ListFollowing(r.Context(), h.db, userID, before)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	isHTMX := r.Header.Get("HX-Request") != ""

	if !isHTMX {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, layout.PageStart("Following", middleware.GetUserRole(r), notification.UnreadCount(r.Context(), h.db, userID), ""))
	}

	if len(items) == 0 && !isHTMX {
		fmt.Fprint(w, `<p>Nothing here yet. Follow some people to see their check-ins.</p>`)
	}

	for _, it := range items {
		h.renderCard(w, it)
	}

	if len(items) == pageSize {
		last := items[len(items)-1].SubmittedAt
		fmt.Fprintf(w,
			`<div hx-get="/feed/following?before=%s" hx-trigger="revealed" hx-swap="outerHTML" hx-target="this">
			   <a href="/feed/following?before=%s">Load more</a>
			 </div>`,
			last.UTC().Format(time.RFC3339),
			last.UTC().Format(time.RFC3339),
		)
	}

	if !isHTMX {
		fmt.Fprint(w, layout.PageEnd())
	}
}

func (h *Handler) renderCard(w http.ResponseWriter, it Item) {
	thumbHTML := ""
	if it.Thumbnail != "" {
		thumbHTML = fmt.Sprintf(`<img src="%s/%s" alt="" class="card-thumb">`,
			h.photoURLPrefix, it.Thumbnail)
	}

	fmt.Fprintf(w, `<article id="ci-%s" class="card">
  %s
  <div class="card-title"><a href="/drinks/%s">%s</a> — %d/100</div>
  <div class="card-meta"><a href="/users/%s">%s</a> · <a href="/locations?name=%s">%s</a> · %s</div>
  <div class="card-body">%s</div>
  <div class="card-actions">
    <button class="btn-sm" hx-post="/checkins/%s/react?type=like" hx-target="#reaction-like-%s" hx-swap="outerHTML">
      <span id="reaction-like-%s">👍 %d</span>
    </button>
    <button class="btn-sm" hx-post="/checkins/%s/react?type=helpful" hx-target="#reaction-helpful-%s" hx-swap="outerHTML">
      <span id="reaction-helpful-%s">💡 %d</span>
    </button>
    <a href="/checkins/%s">View</a>
  </div>
</article>`,
		it.ID,
		thumbHTML,
		it.DrinkID, it.DrinkName, it.Score,
		it.UserID, it.UserName,
		url.QueryEscape(it.LocationName), it.LocationName,
		it.DrinkDate.Format("2 Jan 2006"),
		it.Review,
		it.ID, it.ID, it.ID, it.LikeCount,
		it.ID, it.ID, it.ID, it.HelpfulCount,
		it.ID,
	)
}
