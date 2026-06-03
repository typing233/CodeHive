package handlers

import (
	"net/http"
	"strconv"

	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/middleware"
	"github.com/go-chi/chi/v5"
)

type AuditHandler struct {
	auditStore *store.AuditStore
	renderer   Renderer
}

func NewAuditHandler(as *store.AuditStore, rend Renderer) *AuditHandler {
	return &AuditHandler{auditStore: as, renderer: rend}
}

func (h *AuditHandler) SiteAudit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	filter := h.parseFilter(r)
	entries, total, _ := h.auditStore.List(r.Context(), filter)

	h.renderer.Render(w, "audit_log", map[string]interface{}{
		"Entries":    entries,
		"Total":     total,
		"Filter":    filter,
		"User":      user,
		"Scope":     "site",
		"TotalPages": (total + filter.Limit - 1) / filter.Limit,
	})
}

func (h *AuditHandler) OrgAudit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	orgName := chi.URLParam(r, "org")

	filter := h.parseFilter(r)
	filter.TargetType = "organization"

	entries, total, _ := h.auditStore.List(r.Context(), filter)

	h.renderer.Render(w, "audit_log", map[string]interface{}{
		"Entries":    entries,
		"Total":     total,
		"Filter":    filter,
		"User":      user,
		"Scope":     "org",
		"OrgName":   orgName,
		"TotalPages": (total + filter.Limit - 1) / filter.Limit,
	})
}

func (h *AuditHandler) RepoAudit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	filter := h.parseFilter(r)
	filter.TargetType = "repository"

	entries, total, _ := h.auditStore.List(r.Context(), filter)

	h.renderer.Render(w, "audit_log", map[string]interface{}{
		"Entries":    entries,
		"Total":     total,
		"Filter":    filter,
		"User":      user,
		"Scope":     "repo",
		"Owner":     owner,
		"RepoName":  repoName,
		"TotalPages": (total + filter.Limit - 1) / filter.Limit,
	})
}

func (h *AuditHandler) parseFilter(r *http.Request) models.AuditFilter {
	filter := models.AuditFilter{
		Page:  1,
		Limit: 50,
	}
	if p := r.URL.Query().Get("page"); p != "" {
		filter.Page, _ = strconv.Atoi(p)
	}
	if a := r.URL.Query().Get("action"); a != "" {
		filter.Action = a
	}
	if t := r.URL.Query().Get("target_type"); t != "" {
		filter.TargetType = t
	}
	return filter
}
