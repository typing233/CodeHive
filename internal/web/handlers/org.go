package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/middleware"
	"github.com/go-chi/chi/v5"
)

type OrgHandler struct {
	orgStore   *store.OrgStore
	repoStore  *store.RepoStore
	userStore  *store.UserStore
	tokenStore *store.TokenStore
	auditStore *store.AuditStore
	cfg        *config.Config
	renderer   Renderer
}

func NewOrgHandler(os *store.OrgStore, rs *store.RepoStore, us *store.UserStore, ts *store.TokenStore, as *store.AuditStore, cfg *config.Config, rend Renderer) *OrgHandler {
	return &OrgHandler{
		orgStore:   os,
		repoStore:  rs,
		userStore:  us,
		tokenStore: ts,
		auditStore: as,
		cfg:        cfg,
		renderer:   rend,
	}
}

func (h *OrgHandler) NewPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	h.renderer.Render(w, "org_new", map[string]interface{}{"User": user})
}

func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))

	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	org := &models.Organization{
		Name:        name,
		DisplayName: displayName,
		Description: r.FormValue("description"),
		IsPublic:    r.FormValue("is_public") == "on",
	}

	if err := h.orgStore.Create(r.Context(), org); err != nil {
		http.Error(w, "Failed to create organization", http.StatusInternalServerError)
		return
	}

	// Add creator as owner
	h.orgStore.AddMember(r.Context(), org.ID, user.ID, "owner")

	h.auditStore.Log(r.Context(), &user.ID, "org.create", "organization", &org.ID,
		map[string]interface{}{"name": org.Name}, r.RemoteAddr)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s", org.Name), http.StatusFound)
}

func (h *OrgHandler) Profile(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	members, _ := h.orgStore.ListMembers(r.Context(), org.ID)

	h.renderer.Render(w, "org_profile", map[string]interface{}{
		"Org":     org,
		"Members": members,
		"User":    user,
	})
}

func (h *OrgHandler) Members(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	members, _ := h.orgStore.ListMembers(r.Context(), org.ID)
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)

	h.renderer.Render(w, "org_members", map[string]interface{}{
		"Org":      org,
		"Members":  members,
		"User":     user,
		"UserRole": role,
	})
}

func (h *OrgHandler) Teams(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	teams, _ := h.orgStore.ListTeams(r.Context(), org.ID)

	h.renderer.Render(w, "org_teams", map[string]interface{}{
		"Org":   org,
		"Teams": teams,
		"User":  user,
	})
}

func (h *OrgHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	memberRole := r.FormValue("role")
	if memberRole == "" {
		memberRole = "member"
	}

	target, err := h.userStore.GetByUsername(r.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusBadRequest)
		return
	}

	h.orgStore.AddMember(r.Context(), org.ID, target.ID, memberRole)

	h.auditStore.Log(r.Context(), &user.ID, "org.member.add", "organization", &org.ID,
		map[string]interface{}{"username": username, "role": memberRole}, r.RemoteAddr)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/members", orgName), http.StatusFound)
}

func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	targetID, _ := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	h.orgStore.RemoveMember(r.Context(), org.ID, targetID)

	h.auditStore.Log(r.Context(), &user.ID, "org.member.remove", "organization", &org.ID,
		map[string]interface{}{"target_user_id": targetID}, r.RemoteAddr)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/members", orgName), http.StatusFound)
}

func (h *OrgHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	team := &models.Team{
		OrgID:       org.ID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: r.FormValue("description"),
		Permission:  r.FormValue("permission"),
	}
	if team.Permission == "" {
		team.Permission = "read"
	}

	h.orgStore.CreateTeam(r.Context(), team)
	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/teams", orgName), http.StatusFound)
}

func (h *OrgHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	team, err := h.orgStore.GetTeam(r.Context(), teamID)
	if err != nil || team.OrgID != org.ID {
		http.NotFound(w, r)
		return
	}

	team.Name = strings.TrimSpace(r.FormValue("name"))
	team.Description = r.FormValue("description")
	team.Permission = r.FormValue("permission")
	h.orgStore.UpdateTeam(r.Context(), team)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/teams", orgName), http.StatusFound)
}

func (h *OrgHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	h.orgStore.DeleteTeam(r.Context(), teamID)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/teams", orgName), http.StatusFound)
}

func (h *OrgHandler) UpdateTeamMembers(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	username := strings.TrimSpace(r.FormValue("username"))
	action := r.FormValue("action")

	target, err := h.userStore.GetByUsername(r.Context(), username)
	if err != nil {
		http.Error(w, "User not found", http.StatusBadRequest)
		return
	}

	if action == "remove" {
		h.orgStore.RemoveTeamMember(r.Context(), teamID, target.ID)
	} else {
		h.orgStore.AddTeamMember(r.Context(), teamID, target.ID)
	}

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/teams", orgName), http.StatusFound)
}

func (h *OrgHandler) UpdateTeamRepos(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	repoID, _ := strconv.ParseInt(r.FormValue("repo_id"), 10, 64)
	permission := r.FormValue("permission")
	action := r.FormValue("action")

	if action == "remove" {
		h.orgStore.RemoveTeamRepo(r.Context(), teamID, repoID)
	} else {
		if permission == "" {
			permission = "read"
		}
		h.orgStore.AddTeamRepo(r.Context(), teamID, repoID, permission)
	}

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/teams", orgName), http.StatusFound)
}

func (h *OrgHandler) Settings(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.renderer.Render(w, "org_settings", map[string]interface{}{
		"Org":  org,
		"User": user,
	})
}

func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	org.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	org.Description = r.FormValue("description")
	org.IsPublic = r.FormValue("is_public") == "on"
	h.orgStore.Update(r.Context(), org)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/settings", orgName), http.StatusFound)
}

func (h *OrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.auditStore.Log(r.Context(), &user.ID, "org.delete", "organization", &org.ID,
		map[string]interface{}{"name": org.Name}, r.RemoteAddr)

	h.orgStore.Delete(r.Context(), org.ID)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *OrgHandler) TokensPage(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	tokens, _ := h.tokenStore.ListByOrg(r.Context(), org.ID)

	h.renderer.Render(w, "org_tokens", map[string]interface{}{
		"Org":    org,
		"Tokens": tokens,
		"User":   user,
	})
}

func (h *OrgHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	raw, hash, err := store.GenerateToken()
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	token := &models.AccessToken{
		UserID:    user.ID,
		Name:      strings.TrimSpace(r.FormValue("name")),
		TokenHash: hash,
		Scopes:    r.FormValue("scopes"),
	}

	if err := h.tokenStore.Create(r.Context(), token); err != nil {
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	h.auditStore.Log(r.Context(), &user.ID, "token.create", "organization", &org.ID,
		map[string]interface{}{"token_name": token.Name}, r.RemoteAddr)

	tokens, _ := h.tokenStore.ListByOrg(r.Context(), org.ID)
	h.renderer.Render(w, "org_tokens", map[string]interface{}{
		"Org":      org,
		"Tokens":   tokens,
		"User":     user,
		"NewToken": raw,
	})
}

func (h *OrgHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	orgName := chi.URLParam(r, "org")
	org, err := h.orgStore.GetByName(r.Context(), orgName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	role, _ := h.orgStore.GetMemberRole(r.Context(), org.ID, user.ID)
	if role != "owner" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	tokenID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.tokenStore.DeleteOrgToken(r.Context(), tokenID, org.ID)

	h.auditStore.Log(r.Context(), &user.ID, "token.delete", "organization", &org.ID,
		map[string]interface{}{"token_id": tokenID}, r.RemoteAddr)

	http.Redirect(w, r, fmt.Sprintf("/orgs/%s/settings/tokens", orgName), http.StatusFound)
}
