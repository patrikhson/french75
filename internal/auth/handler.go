package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/mail"
)

type Handler struct {
	db         *pgxpool.Pool
	webAuthn   *webauthn.WebAuthn
	mailer     *mail.Mailer
	baseURL    string
	isProd     bool
	sessionKey string
}

func NewHandler(db *pgxpool.Pool, wa *webauthn.WebAuthn, mailer *mail.Mailer, baseURL, sessionKey string, isProd bool) *Handler {
	return &Handler{
		db:         db,
		webAuthn:   wa,
		mailer:     mailer,
		baseURL:    baseURL,
		isProd:     isProd,
		sessionKey: sessionKey,
	}
}

// RegisterRoutes wires up all auth routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Registration request flow
	mux.HandleFunc("GET /auth/request", h.showRequestForm)
	mux.HandleFunc("POST /auth/request", h.submitRequest)
	mux.HandleFunc("GET /auth/verify-email", h.verifyEmail)

	// Passkey registration (after admin approval)
	mux.HandleFunc("GET /auth/register", h.showRegisterPasskey)
	mux.HandleFunc("POST /auth/register/begin", h.beginRegisterPasskey)
	mux.HandleFunc("POST /auth/register/finish", h.finishRegisterPasskey)

	// Login
	mux.HandleFunc("GET /auth/login", h.showLogin)
	mux.HandleFunc("POST /auth/login/begin", h.beginLogin)
	mux.HandleFunc("POST /auth/login/finish", h.finishLogin)

	// Logout
	mux.HandleFunc("POST /auth/logout", h.logout)
}

// ---------------------------------------------------------------
// Step 1: Request access form
// ---------------------------------------------------------------

func (h *Handler) showRequestForm(w http.ResponseWriter, r *http.Request) {
	// TODO: render template auth/request.html
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, requestFormHTML)
}

func (h *Handler) submitRequest(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	email := r.FormValue("email")

	if name == "" || email == "" {
		http.Error(w, "Name and email are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if email already used
	var exists bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM registration_requests WHERE email = $1)`, email).Scan(&exists)
	if exists {
		// Show same success message to avoid email enumeration
		h.showRequestSuccess(w, r)
		return
	}

	token := uuid.NewString()
	_, err := h.db.Exec(ctx,
		`INSERT INTO registration_requests (token, name, email, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '24 hours')`,
		token, name, email,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	link := h.baseURL + "/auth/verify-email?token=" + token
	if err := h.mailer.SendVerification(email, name, link); err != nil {
		// Log but don't expose to user
		fmt.Printf("mail error: %v\n", err)
	}

	h.showRequestSuccess(w, r)
}

func (h *Handler) showRequestSuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h2>Check your email</h2>
<p>If that email address is valid, we've sent a verification link. Check your inbox.</p>
</body></html>`)
}

// ---------------------------------------------------------------
// Step 2: Email verification → redirect to passkey registration
// ---------------------------------------------------------------

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Invalid link", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Generate a passkey token so the user can register their passkey immediately.
	passkeyToken := uuid.NewString()
	res, err := h.db.Exec(ctx,
		`UPDATE registration_requests
		 SET email_verified = TRUE, email_verified_at = NOW(),
		     passkey_token = $2, passkey_token_expires_at = NOW() + INTERVAL '24 hours'
		 WHERE token = $1 AND expires_at > NOW() AND email_verified = FALSE`,
		token, passkeyToken,
	)
	if err != nil || res.RowsAffected() == 0 {
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/auth/register?token="+passkeyToken, http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Step 3: Admin approves → welcome email with passkey registration link
// (admin handler calls this service method)
// ---------------------------------------------------------------

func (h *Handler) SendApprovalEmail(ctx context.Context, requestID string) error {
	return SendApprovalEmail(ctx, h.db, h.mailer, h.baseURL, requestID)
}

// SendApprovalEmail is a package-level function so the admin handler can call it
// without a circular import on the auth.Handler.
// It creates the user account, moves the pending credential, and sends an approval email.
func SendApprovalEmail(ctx context.Context, db *pgxpool.Pool, mailer *mail.Mailer, baseURL, requestID string) error {
	var name, email string
	var credJSON []byte
	err := db.QueryRow(ctx,
		`SELECT name, email, pending_credential FROM registration_requests
		 WHERE id = $1 AND status = 'pending' AND pending_credential IS NOT NULL`,
		requestID,
	).Scan(&name, &email, &credJSON)
	if err != nil {
		return fmt.Errorf("registration not found or passkey not yet registered: %w", err)
	}

	var cred storedCredential
	if err := json.Unmarshal(credJSON, &cred); err != nil {
		return fmt.Errorf("corrupt credential data: %w", err)
	}

	// Create the user account.
	userID := uuid.NewString()
	_, err = db.Exec(ctx,
		`INSERT INTO users (id, username, display_name, role) VALUES ($1, $2, $3, 'passive')`,
		userID, email, name,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// Move the credential to the user.
	_, err = db.Exec(ctx,
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, aaguid, sign_count, name)
		 VALUES ($1, $2, $3, $4, $5, 'Primary passkey')`,
		userID, cred.ID, cred.PublicKey, cred.AAGUID, cred.SignCount,
	)
	if err != nil {
		return fmt.Errorf("store credential: %w", err)
	}

	// Mark request completed.
	db.Exec(ctx,
		`UPDATE registration_requests SET status = 'completed', user_id = $1 WHERE id = $2`,
		userID, requestID,
	)

	return mailer.SendApproved(email, name, baseURL+"/auth/login")
}

// ---------------------------------------------------------------
// Step 4: Passkey registration
// ---------------------------------------------------------------

func (h *Handler) showRegisterPasskey(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Invalid link", http.StatusBadRequest)
		return
	}

	var name string
	err := h.db.QueryRow(r.Context(),
		`SELECT name FROM registration_requests
		 WHERE passkey_token = $1 AND passkey_token_expires_at > NOW()
		   AND status = 'pending' AND email_verified = TRUE`,
		token,
	).Scan(&name)
	if err != nil {
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, registerPasskeyHTML, name, token)
}

func (h *Handler) beginRegisterPasskey(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	ctx := r.Context()

	var reqID, name, email string
	err := h.db.QueryRow(ctx,
		`SELECT id, name, email FROM registration_requests
		 WHERE passkey_token = $1 AND passkey_token_expires_at > NOW()
		   AND status = 'pending' AND email_verified = TRUE`,
		token,
	).Scan(&reqID, &name, &email)
	if err != nil {
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
		return
	}

	// Create a temporary user for WebAuthn ceremony
	waUser := &waUser{id: reqID, name: name, displayName: name, email: email}

	options, sessionData, err := h.webAuthn.BeginRegistration(waUser)
	if err != nil {
		http.Error(w, "WebAuthn error", http.StatusInternalServerError)
		return
	}

	// Store session data in DB temporarily
	sessionJSON, _ := json.Marshal(sessionData)
	h.db.Exec(ctx,
		`UPDATE registration_requests SET webauthn_session = $1 WHERE id = $2`,
		sessionJSON, reqID,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (h *Handler) finishRegisterPasskey(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	ctx := r.Context()

	var reqID, name, email string
	var sessionJSON []byte
	err := h.db.QueryRow(ctx,
		`SELECT id, name, email, webauthn_session FROM registration_requests
		 WHERE passkey_token = $1 AND passkey_token_expires_at > NOW()
		   AND status = 'pending' AND email_verified = TRUE`,
		token,
	).Scan(&reqID, &name, &email, &sessionJSON)
	if err != nil {
		http.Error(w, "Invalid or expired link", http.StatusBadRequest)
		return
	}

	var sessionData webauthn.SessionData
	if err := json.Unmarshal(sessionJSON, &sessionData); err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	waUser := &waUser{id: reqID, name: name, displayName: name, email: email}
	credential, err := h.webAuthn.FinishRegistration(waUser, sessionData, r)
	if err != nil {
		http.Error(w, "Passkey registration failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Store credential in the registration request — user account is created when admin approves.
	pubKey, _ := json.Marshal(credential.PublicKey)
	stored := storedCredential{
		ID:        credential.ID,
		PublicKey: pubKey,
		AAGUID:    credential.Authenticator.AAGUID,
		SignCount: credential.Authenticator.SignCount,
	}
	storedJSON, _ := json.Marshal(stored)
	h.db.Exec(ctx,
		`UPDATE registration_requests
		 SET pending_credential = $1, passkey_registered_at = NOW()
		 WHERE id = $2`,
		storedJSON, reqID,
	)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h2>Passkey registered!</h2>
<p>Your request is now in the queue. You'll receive an email once an admin approves your account.</p>
</body></html>`)
}

// storedCredential is the subset of webauthn.Credential persisted in registration_requests.
type storedCredential struct {
	ID        []byte `json:"id"`
	PublicKey []byte `json:"public_key"`
	AAGUID    []byte `json:"aaguid"`
	SignCount  uint32 `json:"sign_count"`
}

// ---------------------------------------------------------------
// Login
// ---------------------------------------------------------------

func (h *Handler) showLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, loginHTML)
}

func (h *Handler) beginLogin(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	ctx := r.Context()

	// Find user by email (username field stores email initially)
	var userID, name string
	err := h.db.QueryRow(ctx,
		`SELECT id, display_name FROM users WHERE username = $1 AND is_banned = FALSE`,
		email,
	).Scan(&userID, &name)
	if err != nil {
		// Don't reveal whether email exists
		http.Error(w, "No passkey found for that email", http.StatusBadRequest)
		return
	}

	waUser, err := h.loadWAUser(ctx, userID, name, email)
	if err != nil || len(waUser.credentials) == 0 {
		http.Error(w, "No passkey found for that email", http.StatusBadRequest)
		return
	}

	options, sessionData, err := h.webAuthn.BeginLogin(waUser)
	if err != nil {
		http.Error(w, "WebAuthn error", http.StatusInternalServerError)
		return
	}

	sessionJSON, _ := json.Marshal(sessionData)
	h.db.Exec(ctx,
		`INSERT INTO webauthn_login_sessions (id, user_id, session_data, expires_at)
		 VALUES ($1, $2, $3, NOW() + INTERVAL '5 minutes')
		 ON CONFLICT (user_id) DO UPDATE SET session_data = $3, expires_at = NOW() + INTERVAL '5 minutes'`,
		uuid.NewString(), userID, sessionJSON,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (h *Handler) finishLogin(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	ctx := r.Context()

	var userID, name string
	h.db.QueryRow(ctx,
		`SELECT id, display_name FROM users WHERE username = $1 AND is_banned = FALSE`,
		email,
	).Scan(&userID, &name)

	var sessionJSON []byte
	err := h.db.QueryRow(ctx,
		`DELETE FROM webauthn_login_sessions WHERE user_id = $1 AND expires_at > NOW()
		 RETURNING session_data`,
		userID,
	).Scan(&sessionJSON)
	if err != nil {
		http.Error(w, "Login session expired", http.StatusBadRequest)
		return
	}

	var sessionData webauthn.SessionData
	json.Unmarshal(sessionJSON, &sessionData)

	waUser, _ := h.loadWAUser(ctx, userID, name, email)
	credential, err := h.webAuthn.FinishLogin(waUser, sessionData, r)
	if err != nil {
		http.Error(w, "Passkey verification failed", http.StatusUnauthorized)
		return
	}

	// Update sign count
	h.db.Exec(ctx,
		`UPDATE webauthn_credentials SET sign_count = $1, last_used_at = NOW()
		 WHERE user_id = $2 AND credential_id = $3`,
		credential.Authenticator.SignCount, userID, credential.ID,
	)

	CreateSession(ctx, h.db, w, r, userID, h.isProd)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"redirect": "/"})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	DeleteSession(r.Context(), h.db, w, r)
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

// ---------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------

func (h *Handler) loadWAUser(ctx context.Context, userID, name, email string) (*waUser, error) {
	rows, err := h.db.Query(ctx,
		`SELECT credential_id, public_key, sign_count FROM webauthn_credentials WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	u := &waUser{id: userID, name: email, displayName: name, email: email}
	for rows.Next() {
		var credID, pubKeyJSON []byte
		var signCount uint32
		rows.Scan(&credID, &pubKeyJSON, &signCount)

		var pubKey []byte
		json.Unmarshal(pubKeyJSON, &pubKey)

		u.credentials = append(u.credentials, webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: signCount,
			},
		})
	}
	return u, nil
}

// waUser implements webauthn.User
type waUser struct {
	id          string
	name        string
	displayName string
	email       string
	credentials []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return []byte(u.id) }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// ---------------------------------------------------------------
// Inline HTML (temporary — will move to templates)
// ---------------------------------------------------------------

const requestFormHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Request Access — French 75 Tracker</title></head>
<body>
<h2>Request Access</h2>
<form method="POST" action="/auth/request">
  <label>Your name<br><input type="text" name="name" required></label><br><br>
  <label>Email address<br><input type="email" name="email" required></label><br><br>
  <button type="submit">Request Access</button>
</form>
</body></html>`

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Log in — French 75 Tracker</title></head>
<body>
<h2>Log in</h2>
<form id="loginForm">
  <label>Email address<br><input type="email" id="email" name="email" required></label><br><br>
  <button type="submit">Log in with passkey</button>
</form>
<script>
document.getElementById('loginForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const email = document.getElementById('email').value;

  const beginResp = await fetch('/auth/login/begin', {
    method: 'POST',
    body: new URLSearchParams({email}),
  });
  if (!beginResp.ok) { alert(await beginResp.text()); return; }

  const options = await beginResp.json();

  // Convert base64url to ArrayBuffer
  options.publicKey.challenge = base64ToBuffer(options.publicKey.challenge);
  if (options.publicKey.allowCredentials) {
    options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(c => ({
      ...c, id: base64ToBuffer(c.id)
    }));
  }

  const assertion = await navigator.credentials.get(options);
  const finishResp = await fetch('/auth/login/finish?email=' + encodeURIComponent(email), {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      id: assertion.id,
      rawId: bufferToBase64(assertion.rawId),
      type: assertion.type,
      response: {
        authenticatorData: bufferToBase64(assertion.response.authenticatorData),
        clientDataJSON: bufferToBase64(assertion.response.clientDataJSON),
        signature: bufferToBase64(assertion.response.signature),
        userHandle: assertion.response.userHandle ? bufferToBase64(assertion.response.userHandle) : null,
      },
    }),
  });
  if (!finishResp.ok) { alert(await finishResp.text()); return; }
  const {redirect} = await finishResp.json();
  window.location.href = redirect;
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
</body></html>`

var registerPasskeyHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Set up passkey — French 75 Tracker</title></head>
<body>
<h2>Welcome, %s!</h2>
<p>Set up your passkey to access French 75 Tracker. You can use Face ID, Touch ID, or a security key.</p>
<button id="registerBtn">Set up passkey</button>
<script>
const token = %q;
document.getElementById('registerBtn').addEventListener('click', async () => {
  const beginResp = await fetch('/auth/register/begin?token=' + token, {method: 'POST'});
  if (!beginResp.ok) { alert(await beginResp.text()); return; }

  const options = await beginResp.json();
  options.publicKey.challenge = base64ToBuffer(options.publicKey.challenge);
  options.publicKey.user.id = base64ToBuffer(options.publicKey.user.id);

  const credential = await navigator.credentials.create(options);
  const finishResp = await fetch('/auth/register/finish?token=' + token, {
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
    window.location.href = '/';
  } else {
    alert(await finishResp.text());
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
</body></html>`

// Ensure time is used
var _ = time.Now
