package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/middleware"
	"github.com/codehive/codehive/internal/webhook"
	"github.com/go-chi/chi/v5"
)

type WebhookHandler struct {
	webhookStore *store.WebhookStore
	repoStore    *store.RepoStore
	dispatcher   *webhook.Dispatcher
	renderer     Renderer
}

func NewWebhookHandler(ws *store.WebhookStore, rs *store.RepoStore, d *webhook.Dispatcher, rend Renderer) *WebhookHandler {
	return &WebhookHandler{
		webhookStore: ws,
		repoStore:    rs,
		dispatcher:   d,
		renderer:     rend,
	}
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hooks, _ := h.webhookStore.ListByRepo(r.Context(), repo.ID)

	h.renderer.Render(w, "webhook_list", map[string]interface{}{
		"Repo":     repo,
		"Webhooks": hooks,
		"User":     user,
		"Owner":    owner,
		"RepoName": repoName,
	})
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	url := strings.TrimSpace(r.FormValue("url"))
	secret := r.FormValue("secret")
	events := r.Form["events"]

	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	wh := &models.Webhook{
		RepoID:   repo.ID,
		URL:      url,
		Secret:   secret,
		Events:   events,
		IsActive: true,
	}

	h.webhookStore.Create(r.Context(), wh)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings/webhooks", owner, repoName), http.StatusFound)
}

func (h *WebhookHandler) View(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	hook, err := h.webhookStore.GetByID(r.Context(), hookID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	deliveries, _ := h.webhookStore.ListDeliveries(r.Context(), hook.ID, 1, 20)

	h.renderer.Render(w, "webhook_view", map[string]interface{}{
		"Repo":       repo,
		"Webhook":    hook,
		"Deliveries": deliveries,
		"User":       user,
		"Owner":      owner,
		"RepoName":   repoName,
	})
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	hook, err := h.webhookStore.GetByID(r.Context(), hookID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	hook.URL = strings.TrimSpace(r.FormValue("url"))
	if secret := r.FormValue("secret"); secret != "" {
		hook.Secret = secret
	}
	r.ParseForm()
	hook.Events = r.Form["events"]
	hook.IsActive = r.FormValue("is_active") == "on"

	h.webhookStore.Update(r.Context(), hook)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings/webhooks/%d", owner, repoName, hook.ID), http.StatusFound)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.webhookStore.Delete(r.Context(), hookID)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings/webhooks", owner, repoName), http.StatusFound)
}

func (h *WebhookHandler) TestDeliver(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "admin"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	hookID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	hook, err := h.webhookStore.GetByID(r.Context(), hookID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "test",
		"repository": map[string]interface{}{
			"name":      repo.Name,
			"full_name": owner + "/" + repoName,
		},
		"sender": map[string]interface{}{
			"username": user.Username,
		},
	})

	delivery := &models.WebhookDelivery{
		WebhookID: hook.ID,
		Event:     "ping",
		Payload:   payload,
	}

	// Do inline delivery for test
	h.dispatcher.Dispatch(r.Context(), repo.ID, "ping", map[string]interface{}{
		"action": "test",
		"hook_id": hook.ID,
	})

	_ = delivery

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings/webhooks/%d", owner, repoName, hook.ID), http.StatusFound)
}
