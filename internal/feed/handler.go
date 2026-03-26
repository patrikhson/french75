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

const lastViewCookie = "feed_last_view"

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

// parseFeedParams reads sort + pagination cursors from the request.
// Returns: sort string, beforeDate, beforeTime.
func parseFeedParams(r *http.Request) (string, time.Time, time.Time) {
	sort := r.URL.Query().Get("sort")
	if sort != SortDrinkDate {
		sort = SortPosted
	}

	// beforeTime: used as primary cursor for "posted" sort, secondary for "drink_date".
	beforeTime := time.Now().Add(time.Second)
	if s := r.URL.Query().Get("bt"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			beforeTime = t
		}
	}

	// beforeDate: primary cursor for drink_date sort (defaults to tomorrow).
	beforeDate := time.Now().AddDate(0, 0, 1)
	if s := r.URL.Query().Get("bd"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			beforeDate = t
		}
	}

	return sort, beforeDate, beforeTime
}

// cursorParams builds the query-string fragment for the "load more" link.
func cursorParams(sort string, last Item) string {
	v := url.Values{}
	v.Set("sort", sort)
	v.Set("bd", last.DrinkDate.Format("2006-01-02"))
	v.Set("bt", last.SubmittedAt.UTC().Format(time.RFC3339))
	return v.Encode()
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/feed" {
		http.NotFound(w, r)
		return
	}

	sort, beforeDate, beforeTime := parseFeedParams(r)

	items, err := List(r.Context(), h.db, sort, beforeDate, beforeTime)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	isHTMX := r.Header.Get("HX-Request") != ""

	// Read last-view cookie before updating it.
	var lastView time.Time
	if c, err := r.Cookie(lastViewCookie); err == nil {
		if t, err := time.Parse(time.RFC3339, c.Value); err == nil {
			lastView = t
		}
	}
	if !isHTMX {
		http.SetCookie(w, &http.Cookie{
			Name:     lastViewCookie,
			Value:    time.Now().UTC().Format(time.RFC3339),
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, layout.PageStart("Feed", middleware.GetUserRole(r), notification.UnreadCount(r.Context(), h.db, middleware.GetUserID(r)), ""))
		fmt.Fprint(w, sortToggle(sort, "/feed"))
	}

	for _, it := range items {
		h.renderCard(w, it, lastView, sort)
	}

	if len(items) == pageSize {
		last := items[len(items)-1]
		qs := cursorParams(sort, last)
		fmt.Fprintf(w,
			`<div hx-get="/feed?%s" hx-trigger="revealed" hx-swap="outerHTML" hx-target="this">
			   <a href="/feed?%s">Load more</a>
			 </div>`,
			qs, qs,
		)
	}

	if !isHTMX {
		fmt.Fprint(w, layout.PageEnd())
	}
}

func (h *Handler) following(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	sort, beforeDate, beforeTime := parseFeedParams(r)

	items, err := ListFollowing(r.Context(), h.db, userID, sort, beforeDate, beforeTime)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	isHTMX := r.Header.Get("HX-Request") != ""

	var lastView time.Time
	if c, err := r.Cookie(lastViewCookie); err == nil {
		if t, err := time.Parse(time.RFC3339, c.Value); err == nil {
			lastView = t
		}
	}
	if !isHTMX {
		http.SetCookie(w, &http.Cookie{
			Name:     lastViewCookie,
			Value:    time.Now().UTC().Format(time.RFC3339),
			Path:     "/",
			MaxAge:   365 * 24 * 3600,
			SameSite: http.SameSiteLaxMode,
		})
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, layout.PageStart("Following", middleware.GetUserRole(r), notification.UnreadCount(r.Context(), h.db, userID), ""))
		fmt.Fprint(w, sortToggle(sort, "/feed/following"))
	}

	if len(items) == 0 && !isHTMX {
		fmt.Fprint(w, `<p>Nothing here yet. Follow some people to see their check-ins.</p>`)
	}

	for _, it := range items {
		h.renderCard(w, it, lastView, sort)
	}

	if len(items) == pageSize {
		last := items[len(items)-1]
		qs := cursorParams(sort, last)
		fmt.Fprintf(w,
			`<div hx-get="/feed/following?%s" hx-trigger="revealed" hx-swap="outerHTML" hx-target="this">
			   <a href="/feed/following?%s">Load more</a>
			 </div>`,
			qs, qs,
		)
	}

	if !isHTMX {
		fmt.Fprint(w, layout.PageEnd())
	}
}

// sortToggle renders the drink-date / posted-date toggle bar.
func sortToggle(current, base string) string {
	ddClass, poClass := "btn-sm", "btn-sm"
	if current == SortDrinkDate {
		ddClass = "btn-sm btn-active"
	} else {
		poClass = "btn-sm btn-active"
	}
	return fmt.Sprintf(
		`<div class="feed-sort">
  <a href="%s?sort=drink_date" class="%s">By drinking date</a>
  <a href="%s?sort=posted" class="%s">By posted date</a>
</div>`,
		base, ddClass, base, poClass,
	)
}

func (h *Handler) renderCard(w http.ResponseWriter, it Item, lastView time.Time, sort string) {
	thumbHTML := ""
	if it.Thumbnail != "" {
		thumbHTML = fmt.Sprintf(`<img src="%s/%s" alt="" class="card-thumb">`,
			h.photoURLPrefix, it.Thumbnail)
	}

	cardClass := "card"
	if !lastView.IsZero() && it.SubmittedAt.After(lastView) {
		cardClass = "card card--new"
	}

	var dateStr string
	if sort == SortDrinkDate {
		dateStr = it.DrinkDate.Format("2 Jan 2006")
	} else {
		dateStr = it.SubmittedAt.Format("2 Jan 2006")
	}

	fmt.Fprintf(w, `<article id="ci-%s" class="%s">
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
		it.ID, cardClass,
		thumbHTML,
		it.DrinkID, it.DrinkName, it.Score,
		it.UserID, it.UserName,
		url.QueryEscape(it.LocationName), it.LocationName,
		dateStr,
		it.Review,
		it.ID, it.ID, it.ID, it.LikeCount,
		it.ID, it.ID, it.ID, it.HelpfulCount,
		it.ID,
	)
}
