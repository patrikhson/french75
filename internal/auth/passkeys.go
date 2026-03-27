package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/patrikhson/french75/internal/layout"
	"github.com/patrikhson/french75/internal/middleware"
	"github.com/patrikhson/french75/internal/notification"
)

// ---------------------------------------------------------------
// Passkey settings page
// ---------------------------------------------------------------

type passkeyRow struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func (h *Handler) showPasskeys(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	role := middleware.GetUserRole(r)
	ctx := r.Context()

	rows, err := h.db.Query(ctx,
		`SELECT id::text, COALESCE(name,''), created_at, last_used_at
		 FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var creds []passkeyRow
	for rows.Next() {
		var p passkeyRow
		rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt)
		creds = append(creds, p)
	}

	unread := notification.UnreadCount(ctx, h.db, userID)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, layout.PageStart("Passkeys", role, unread, ""))
	fmt.Fprint(w, `<h2>Passkeys</h2>`)
	fmt.Fprint(w, `<p><a href="/settings/notifications">← Notification preferences</a></p>`)

	if len(creds) == 0 {
		fmt.Fprint(w, `<p>No passkeys found.</p>`)
	} else {
		fmt.Fprint(w, `<table><thead><tr><th>Name</th><th>Added</th><th>Last used</th><th></th></tr></thead><tbody>`)
		for _, c := range creds {
			lastUsed := "never"
			if c.LastUsedAt != nil {
				lastUsed = c.LastUsedAt.Format("2 Jan 2006")
			}
			deleteBtn := ""
			if len(creds) > 1 {
				deleteBtn = fmt.Sprintf(
					`<form method="POST" action="/settings/passkeys/%s/delete" style="display:inline" onsubmit="return confirm('Delete this passkey?')">
					   <button type="submit" class="btn-sm btn-danger">Delete</button>
					 </form>`,
					c.ID,
				)
			}
			fmt.Fprintf(w,
				`<tr>
				   <td>
				     <form method="POST" action="/settings/passkeys/%s/rename" style="display:flex;gap:0.4rem;align-items:center">
				       <input type="text" name="name" value="%s" required style="width:14rem">
				       <button type="submit" class="btn-sm">Rename</button>
				     </form>
				   </td>
				   <td>%s</td>
				   <td>%s</td>
				   <td>%s</td>
				 </tr>`,
				c.ID, htmlEscapeAttr(c.Name),
				c.CreatedAt.Format("2 Jan 2006"),
				lastUsed,
				deleteBtn,
			)
		}
		fmt.Fprint(w, `</tbody></table>`)
	}

	fmt.Fprint(w, `
<h3>Add a new passkey</h3>
<p>Register a passkey on another device — phone, laptop, or security key.</p>
<div class="form">
  <label>Passkey name (e.g. "iPhone 15" or "YubiKey")
    <input type="text" id="newPasskeyName" placeholder="My device" style="width:16rem">
  </label><br>
  <button id="addPasskeyBtn" class="btn-sm" style="margin-top:0.5rem">Add passkey</button>
  <p id="addPasskeyMsg" style="color:var(--color-error,red)"></p>
</div>
<script>
document.getElementById('addPasskeyBtn').addEventListener('click', async () => {
  const name = document.getElementById('newPasskeyName').value.trim();
  const msg = document.getElementById('addPasskeyMsg');
  msg.textContent = '';
  if (!name) { msg.textContent = 'Please enter a name first.'; return; }

  const beginResp = await fetch('/settings/passkeys/add/begin', {method: 'POST'});
  if (!beginResp.ok) { msg.textContent = await beginResp.text(); return; }

  const options = await beginResp.json();
  options.publicKey.challenge = base64ToBuffer(options.publicKey.challenge);
  options.publicKey.user.id = base64ToBuffer(options.publicKey.user.id);
  if (options.publicKey.excludeCredentials) {
    options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(c => ({
      ...c, id: base64ToBuffer(c.id)
    }));
  }

  let credential;
  try {
    credential = await navigator.credentials.create(options);
  } catch (e) {
    msg.textContent = 'Passkey creation cancelled or failed: ' + e.message;
    return;
  }

  const finishResp = await fetch('/settings/passkeys/add/finish?name=' + encodeURIComponent(name), {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      id: credential.id,
      rawId: bufferToBase64(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64(credential.response.attestationObject),
        clientDataJSON: bufferToBase64(credential.response.clientDataJSON),
      },
    }),
  });
  if (finishResp.ok) {
    window.location.reload();
  } else {
    msg.textContent = await finishResp.text();
  }
});

function base64ToBuffer(b64) {
  const s = atob(b64.replace(/-/g,'+').replace(/_/g,'/'));
  return Uint8Array.from(s, c => c.charCodeAt(0)).buffer;
}
function bufferToBase64(buf) {
  return btoa(String.fromCharCode(...new Uint8Array(buf)))
    .replace(/\+/g,'-').replace(/\//g,'_').replace(/=/g,'');
}
</script>
`)
	fmt.Fprint(w, layout.PageEnd())
}

// ---------------------------------------------------------------
// Add passkey: begin / finish
// ---------------------------------------------------------------

func (h *Handler) beginAddPasskey(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	ctx := r.Context()

	var displayName, email string
	h.db.QueryRow(ctx,
		`SELECT COALESCE(display_name,''), username FROM users WHERE id = $1`,
		userID,
	).Scan(&displayName, &email)

	waUser, err := h.loadWAUser(ctx, userID, displayName, email)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	options, sessionData, err := h.webAuthn.BeginRegistration(waUser)
	if err != nil {
		http.Error(w, "WebAuthn error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionJSON, _ := json.Marshal(sessionData)
	h.db.Exec(ctx,
		`INSERT INTO webauthn_add_sessions (user_id, session_data, expires_at)
		 VALUES ($1, $2, NOW() + INTERVAL '5 minutes')
		 ON CONFLICT (user_id) DO UPDATE SET session_data = $2, expires_at = NOW() + INTERVAL '5 minutes'`,
		userID, sessionJSON,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (h *Handler) finishAddPasskey(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Passkey"
	}
	ctx := r.Context()

	var sessionJSON []byte
	err := h.db.QueryRow(ctx,
		`DELETE FROM webauthn_add_sessions WHERE user_id = $1 AND expires_at > NOW()
		 RETURNING session_data`,
		userID,
	).Scan(&sessionJSON)
	if err != nil {
		http.Error(w, "Session expired, please try again", http.StatusBadRequest)
		return
	}

	var sd webauthn.SessionData
	if err := json.Unmarshal(sessionJSON, &sd); err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	var displayName, email string
	h.db.QueryRow(ctx,
		`SELECT COALESCE(display_name,''), username FROM users WHERE id = $1`,
		userID,
	).Scan(&displayName, &email)

	waUser, _ := h.loadWAUser(ctx, userID, displayName, email)
	credential, err := h.webAuthn.FinishRegistration(waUser, sd, r)
	if err != nil {
		http.Error(w, "Passkey registration failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	pubKey, _ := json.Marshal(credential.PublicKey)
	_, err = h.db.Exec(ctx,
		`INSERT INTO webauthn_credentials
		 (user_id, credential_id, public_key, aaguid, sign_count, backup_eligible, backup_state, name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID, credential.ID, pubKey,
		credential.Authenticator.AAGUID, credential.Authenticator.SignCount,
		credential.Flags.BackupEligible, credential.Flags.BackupState,
		name,
	)
	if err != nil {
		http.Error(w, "Failed to save passkey: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------
// Rename passkey
// ---------------------------------------------------------------

func (h *Handler) renamePasskey(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	credID := r.PathValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	h.db.Exec(r.Context(),
		`UPDATE webauthn_credentials SET name = $1 WHERE id = $2 AND user_id = $3`,
		name, credID, userID,
	)
	http.Redirect(w, r, "/settings/passkeys", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Delete passkey
// ---------------------------------------------------------------

func (h *Handler) deletePasskey(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	credID := r.PathValue("id")
	ctx := r.Context()

	var count int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = $1`,
		userID,
	).Scan(&count)

	if count <= 1 {
		http.Error(w, "Cannot delete your only passkey", http.StatusBadRequest)
		return
	}

	h.db.Exec(ctx,
		`DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`,
		credID, userID,
	)
	http.Redirect(w, r, "/settings/passkeys", http.StatusSeeOther)
}

// htmlEscapeAttr escapes a string for use in an HTML attribute value.
func htmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
