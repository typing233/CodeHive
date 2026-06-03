package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
)

var repoNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,100}$`)

type RepoHandler struct {
	repoStore *store.RepoStore
	userStore *store.UserStore
	gitSvc    *gitbackend.Service
	cfg       *config.Config
	renderer  Renderer
}

func NewRepoHandler(rs *store.RepoStore, us *store.UserStore, gs *gitbackend.Service, cfg *config.Config, r Renderer) *RepoHandler {
	return &RepoHandler{repoStore: rs, userStore: us, gitSvc: gs, cfg: cfg, renderer: r}
}

func (h *RepoHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repos, _ := h.repoStore.ListByUser(r.Context(), user.ID)

	h.renderer.Render(w, "dashboard", map[string]interface{}{
		"User":  user,
		"Repos": repos,
	})
}

func (h *RepoHandler) Explore(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repos, _ := h.repoStore.ListPublic(r.Context(), 1, 50)

	h.renderer.Render(w, "explore", map[string]interface{}{
		"User":  user,
		"Repos": repos,
	})
}

func (h *RepoHandler) NewRepoPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	h.renderer.Render(w, "repo_new", map[string]interface{}{
		"User":  user,
		"Error": r.URL.Query().Get("error"),
	})
}

func (h *RepoHandler) CreateRepo(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	name := strings.TrimSpace(r.FormValue("name"))
	desc := strings.TrimSpace(r.FormValue("description"))
	visibility := r.FormValue("visibility")
	defaultBranch := r.FormValue("default_branch")

	if !repoNameRegex.MatchString(name) {
		http.Redirect(w, r, "/new?error=Invalid+repository+name", http.StatusSeeOther)
		return
	}

	if defaultBranch == "" {
		defaultBranch = "main"
	}

	diskPath := fmt.Sprintf("%s/%s.git", user.Username, name)

	repo := &models.Repository{
		OwnerID:       user.ID,
		Name:          name,
		Description:   desc,
		IsPrivate:     visibility == "private",
		DefaultBranch: defaultBranch,
		DiskPath:      diskPath,
	}

	if err := h.gitSvc.InitBare(diskPath, defaultBranch); err != nil {
		http.Redirect(w, r, "/new?error=Failed+to+create+repository", http.StatusSeeOther)
		return
	}

	if err := h.repoStore.Create(r.Context(), repo); err != nil {
		h.gitSvc.Delete(diskPath)
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Redirect(w, r, "/new?error=Repository+already+exists", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/new?error=Failed+to+create+repository", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s", user.Username, name), http.StatusSeeOther)
}

func (h *RepoHandler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.OwnerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	collabs, _ := h.repoStore.ListCollaborators(r.Context(), repo.ID)

	h.renderer.Render(w, "repo_settings", map[string]interface{}{
		"User":          user,
		"Repo":          repo,
		"Owner":         repo.Owner,
		"Collaborators": collabs,
		"Error":         r.URL.Query().Get("error"),
		"Success":       r.URL.Query().Get("success"),
	})
}

func (h *RepoHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if repo.OwnerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	action := r.FormValue("action")
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner, repoName)

	switch action {
	case "rename":
		newName := strings.TrimSpace(r.FormValue("new_name"))
		if !repoNameRegex.MatchString(newName) {
			http.Redirect(w, r, settingsURL+"?error=Invalid+name", http.StatusSeeOther)
			return
		}
		oldDiskPath := repo.DiskPath
		newDiskPath := fmt.Sprintf("%s/%s.git", user.Username, newName)
		if err := h.gitSvc.Rename(oldDiskPath, newDiskPath); err != nil {
			http.Redirect(w, r, settingsURL+"?error=Rename+failed", http.StatusSeeOther)
			return
		}
		repo.Name = newName
		repo.DiskPath = newDiskPath
		h.repoStore.Update(r.Context(), repo)
		h.repoStore.UpdateDiskPath(r.Context(), repo.ID, newDiskPath)
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings?success=Repository+renamed", owner, newName), http.StatusSeeOther)

	case "visibility":
		repo.IsPrivate = r.FormValue("visibility") == "private"
		h.repoStore.Update(r.Context(), repo)
		http.Redirect(w, r, settingsURL+"?success=Visibility+updated", http.StatusSeeOther)

	case "default_branch":
		repo.DefaultBranch = r.FormValue("default_branch")
		h.repoStore.Update(r.Context(), repo)
		http.Redirect(w, r, settingsURL+"?success=Default+branch+updated", http.StatusSeeOther)

	case "add_collaborator":
		username := strings.TrimSpace(r.FormValue("collaborator"))
		role := r.FormValue("role")
		if role == "" {
			role = "write"
		}
		collabUser, err := h.userStore.GetByUsername(r.Context(), username)
		if err != nil {
			http.Redirect(w, r, settingsURL+"?error=User+not+found", http.StatusSeeOther)
			return
		}
		h.repoStore.AddCollaborator(r.Context(), repo.ID, collabUser.ID, role)
		http.Redirect(w, r, settingsURL+"?success=Collaborator+added", http.StatusSeeOther)

	case "remove_collaborator":
		var collabID int64
		fmt.Sscanf(r.FormValue("user_id"), "%d", &collabID)
		h.repoStore.RemoveCollaborator(r.Context(), repo.ID, collabID)
		http.Redirect(w, r, settingsURL+"?success=Collaborator+removed", http.StatusSeeOther)

	default:
		http.Redirect(w, r, settingsURL, http.StatusSeeOther)
	}
}

func (h *RepoHandler) DeleteRepo(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if repo.OwnerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	confirm := r.FormValue("confirm")
	if confirm != repo.Name {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/settings?error=Please+type+the+repository+name+to+confirm", owner, repoName), http.StatusSeeOther)
		return
	}

	h.gitSvc.Delete(repo.DiskPath)
	h.repoStore.Delete(r.Context(), repo.ID)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
