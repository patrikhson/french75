// Package layout provides shared HTML page scaffolding for all pages.
//
// Every page should use one of the PageStart / AdminPageStart / PublicPageStart
// functions to open the page, and the corresponding PageEnd to close it.
// This guarantees a consistent header, footer, CSS, and viewport meta across
// the entire site.
package layout

import "fmt"

// LeafletCSS is the <link> tag for Leaflet maps. Pass it as extraHead to any
// PageStart function for pages that embed a map.
const LeafletCSS = `<link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css">`

// ---------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------

func head(title, extraHead string) string {
	return fmt.Sprintf(`<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>%s — French 75 Tracker</title>
  <link rel="stylesheet" href="/static/css/site.css">
  <script src="https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"></script>
  %s
</head>`, title, extraHead)
}

func footer() string {
	return `<footer class="site-footer">
  <p>French 75 Tracker</p>
</footer>`
}

// ---------------------------------------------------------------
// Authenticated pages
// ---------------------------------------------------------------

// BellFragment returns the notification bell element as an HTMX-swappable fragment.
// It polls /api/bell every 30 s so the count stays live across browser tabs.
func BellFragment(unread int) string {
	bellClass := "nav-bell"
	if unread > 0 {
		bellClass = "nav-bell nav-bell--unread"
	}
	icon := "🔕"
	if unread > 0 {
		icon = "🔔"
	}
	return fmt.Sprintf(
		`<a id="nav-bell" href="/notifications" title="Notifications" class="%s" `+
			`hx-get="/api/bell" hx-trigger="every 30s" hx-swap="outerHTML">%s</a>`,
		bellClass, icon)
}

// Nav returns the site header HTML for authenticated users.
// role is the current user's role ("passive", "active", "admin").
// unread is the count of unmanaged personal notifications.
func Nav(role string, unread int) string {
	adminLink := ""
	if role == "admin" {
		adminLink = `<span class="nav-sep">·</span><a href="/admin">Admin</a>`
	}

	return fmt.Sprintf(`<header class="site-header">
  <div class="site-header__inner">
    <a href="/" class="site-logo">French 75 Tracker</a>
    <nav class="site-nav">
      <a href="/checkins/new">+ Check-in</a>
      <span class="nav-sep">·</span>
      <a href="/">Feed</a>
      <span class="nav-sep">·</span>
      <a href="/feed/following">Following</a>
      <span class="nav-sep">·</span>
      <a href="/drinks">Drinks</a>
      %s
      <span class="nav-sep">·</span>
      %s
      <span class="nav-sep">·</span>
      <a href="/settings/notifications">Prefs</a>
      <span class="nav-sep">·</span>
      <a href="/auth/logout" hx-post="/auth/logout" hx-push-url="true">Log out</a>
    </nav>
  </div>
</header>`, adminLink, BellFragment(unread))
}

// PageStart returns the opening HTML for an authenticated page.
// extraHead is injected into <head>; use layout.LeafletCSS for map pages.
func PageStart(title, role string, unread int, extraHead string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
%s
<body>
%s
<main class="site-main">`, head(title, extraHead), Nav(role, unread))
}

// PageEnd returns the closing HTML for pages opened with PageStart.
func PageEnd() string {
	return footer() + "\n</body></html>"
}

// ---------------------------------------------------------------
// Admin pages
// ---------------------------------------------------------------

// AdminPageStart returns the opening HTML for an admin page.
// Admin pages get a wider layout and a breadcrumb bar with a back-to-site link.
func AdminPageStart(title string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
%s
<body>
<header class="site-header wide">
  <div class="site-header__inner">
    <a href="/admin" class="site-logo">French 75 Tracker — Admin</a>
  </div>
</header>
<div class="admin-bar">
  <div class="admin-bar__inner">
    <a href="/admin">Dashboard</a>
    <a href="/admin/registrations">Registrations</a>
    <a href="/admin/checkins/pending">Check-ins</a>
    <a href="/admin/drinks/requests">Drink requests</a>
    <a href="/admin/spam">Spam</a>
    <a href="/admin/users">Users</a>
    <a href="/">← Back to site</a>
  </div>
</div>
<main class="site-main wide">
<h2>%s</h2>
`, head(title+" — Admin", ""), title)
}

// AdminPageEnd returns the closing HTML for admin pages.
func AdminPageEnd() string {
	return footer() + "\n</body></html>"
}

// ---------------------------------------------------------------
// Public (unauthenticated) pages
// ---------------------------------------------------------------

// PublicPageStart returns the opening HTML for unauthenticated pages.
// extraHead is optional additional <head> content.
func PublicPageStart(title, extraHead string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
%s
<body>
<header class="site-header">
  <div class="site-header__inner">
    <a href="/auth/login" class="site-logo">French 75 Tracker</a>
  </div>
</header>
<main class="site-main">
<h2>%s</h2>
`, head(title, extraHead), title)
}

// PublicPageEnd returns the closing HTML for public pages.
func PublicPageEnd() string {
	return footer() + "\n</body></html>"
}
