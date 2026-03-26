package layout

import "fmt"

// Nav renders the authenticated top navigation bar.
// unread is the count of unmanaged personal notifications.
func Nav(role string, unread int) string {
	adminLink := ""
	if role == "admin" {
		adminLink = ` <a href="/admin">Admin</a> |`
	}

	bell := "🔔"
	if unread == 0 {
		bell = "🔕"
	}

	return fmt.Sprintf(`<header>
  <h1><a href="/" style="text-decoration:none;color:inherit;">French 75 Tracker</a></h1>
  <nav>
    <a href="/checkins/new">+ Check-in</a> |
    <a href="/feed/following">Following</a> |
    <a href="/drinks">Drinks</a> |%s
    <a href="/notifications" title="Notifications">%s</a> |
    <a href="/settings/notifications">Prefs</a> |
    <a href="/auth/logout" hx-post="/auth/logout" hx-push-url="true">Log out</a>
  </nav>
</header>`, adminLink, bell)
}

// PageStart renders the opening HTML with the authenticated nav.
func PageStart(title, role string, unread int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s — French 75 Tracker</title>
<script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
</head>
<body>
%s
<main>`, title, Nav(role, unread))
}

// PageEnd renders the closing HTML.
func PageEnd() string {
	return `</main>
</body></html>`
}

// PublicPageStart renders the opening HTML for unauthenticated pages (no nav).
func PublicPageStart(title string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s — French 75 Tracker</title>
</head>
<body>
<h2>%s</h2>`, title, title)
}

// AdminPage renders the opening HTML for admin pages with nav back to the site.
func AdminPage(title, content string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s — Admin</title></head>
<body>
<nav style="margin-bottom:8px">
  <a href="/admin">Admin dashboard</a> |
  <a href="/">← Back to site</a>
</nav>
<h2>%s</h2>
%s`, title, title, content)
}
