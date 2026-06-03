package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"
)

type UserHandler struct {
	userStore  *store.UserStore
	tokenStore *store.TokenStore
	cfg        *config.Config
	renderer   Renderer
}

func NewUserHandler(us *store.UserStore, ts *store.TokenStore, cfg *config.Config, r Renderer) *UserHandler {
	return &UserHandler{userStore: us, tokenStore: ts, cfg: cfg, renderer: r}
}

func (h *UserHandler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	h.renderer.Render(w, "user_settings", map[string]interface{}{
		"User":    user,
		"Success": r.URL.Query().Get("success"),
		"Error":   r.URL.Query().Get("error"),
	})
}

func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	user.FullName = strings.TrimSpace(r.FormValue("full_name"))
	user.Email = strings.TrimSpace(r.FormValue("email"))

	if err := h.userStore.Update(r.Context(), user); err != nil {
		http.Redirect(w, r, "/settings?error=Failed+to+update+profile", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=Profile+updated", http.StatusSeeOther)
}

func (h *UserHandler) KeysPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	keys, _ := h.userStore.ListSSHKeys(r.Context(), user.ID)

	h.renderer.Render(w, "user_keys", map[string]interface{}{
		"User":    user,
		"Keys":    keys,
		"Error":   r.URL.Query().Get("error"),
		"Success": r.URL.Query().Get("success"),
	})
}

func (h *UserHandler) AddKey(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	name := strings.TrimSpace(r.FormValue("name"))
	pubKeyStr := strings.TrimSpace(r.FormValue("public_key"))

	if name == "" || pubKeyStr == "" {
		http.Redirect(w, r, "/settings/keys?error=Name+and+key+are+required", http.StatusSeeOther)
		return
	}

	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		http.Redirect(w, r, "/settings/keys?error=Invalid+SSH+public+key", http.StatusSeeOther)
		return
	}

	fp := store.ComputeSSHFingerprint(pubKey)

	key := &models.SSHKey{
		UserID:      user.ID,
		Name:        name,
		Fingerprint: fp,
		PublicKey:    pubKeyStr,
	}

	if err := h.userStore.AddSSHKey(r.Context(), key); err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Redirect(w, r, "/settings/keys?error=Key+already+exists", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/settings/keys?error=Failed+to+add+key", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings/keys?success=SSH+key+added", http.StatusSeeOther)
}

func (h *UserHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.userStore.DeleteSSHKey(r.Context(), id, user.ID)
	http.Redirect(w, r, "/settings/keys?success=Key+removed", http.StatusSeeOther)
}

func (h *UserHandler) TokensPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	tokens, _ := h.tokenStore.List(r.Context(), user.ID)

	h.renderer.Render(w, "user_tokens", map[string]interface{}{
		"User":     user,
		"Tokens":   tokens,
		"NewToken": r.URL.Query().Get("new_token"),
		"Error":    r.URL.Query().Get("error"),
		"Success":  r.URL.Query().Get("success"),
	})
}

func (h *UserHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	name := strings.TrimSpace(r.FormValue("name"))

	if name == "" {
		http.Redirect(w, r, "/settings/tokens?error=Name+is+required", http.StatusSeeOther)
		return
	}

	raw, hash, err := store.GenerateToken()
	if err != nil {
		http.Redirect(w, r, "/settings/tokens?error=Failed+to+generate+token", http.StatusSeeOther)
		return
	}

	token := &models.AccessToken{
		UserID:    user.ID,
		Name:      name,
		TokenHash: hash,
		Scopes:    "repo",
	}

	if err := h.tokenStore.Create(r.Context(), token); err != nil {
		http.Redirect(w, r, "/settings/tokens?error=Failed+to+create+token", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/settings/tokens?success=Token+created&new_token=%s", raw), http.StatusSeeOther)
}

func (h *UserHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.tokenStore.Delete(r.Context(), id, user.ID)
	http.Redirect(w, r, "/settings/tokens?success=Token+revoked", http.StatusSeeOther)
}
