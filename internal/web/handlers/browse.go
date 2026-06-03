package handlers

import (
	"bytes"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/yuin/goldmark"
)

type BrowseHandler struct {
	repoStore *store.RepoStore
	gitSvc    *gitbackend.Service
	renderer  Renderer
}

func NewBrowseHandler(rs *store.RepoStore, gs *gitbackend.Service, r Renderer) *BrowseHandler {
	return &BrowseHandler{repoStore: rs, gitSvc: gs, renderer: r}
}

func (h *BrowseHandler) RepoRoot(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	if h.gitSvc.IsEmpty(repo.DiskPath) {
		h.renderer.Render(w, "repo_empty", map[string]interface{}{
			"User":  user,
			"Repo":  repo,
			"Owner": repo.Owner,
		})
		return
	}

	ref := h.gitSvc.DefaultRef(repo.DiskPath)
	h.renderTree(w, r, repo, user, ref, "")
}

func (h *BrowseHandler) Tree(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	path := chi.URLParam(r, "*")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	h.renderTree(w, r, repo, user, ref, path)
}

func (h *BrowseHandler) renderTree(w http.ResponseWriter, r *http.Request, repo *models.Repository, user *models.User, ref, path string) {
	entries, err := h.gitSvc.GetTree(repo.DiskPath, ref, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	branches, _ := h.gitSvc.ListBranches(repo.DiskPath)
	tags, _ := h.gitSvc.ListTags(repo.DiskPath)

	var readmeHTML template.HTML
	if path == "" {
		readme, _ := h.gitSvc.GetReadme(repo.DiskPath, ref)
		if readme != "" {
			var buf bytes.Buffer
			goldmark.Convert([]byte(readme), &buf)
			readmeHTML = template.HTML(buf.String())
		}
	}

	breadcrumb := buildBreadcrumb(path)

	h.renderer.Render(w, "repo_tree", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Ref":        ref,
		"Path":       path,
		"Entries":    entries,
		"Branches":   branches,
		"Tags":       tags,
		"ReadmeHTML": readmeHTML,
		"Breadcrumb": breadcrumb,
	})
}

func (h *BrowseHandler) Blob(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	path := chi.URLParam(r, "*")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	content, err := h.gitSvc.GetBlob(repo.DiskPath, ref, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	branches, _ := h.gitSvc.ListBranches(repo.DiskPath)
	tags, _ := h.gitSvc.ListTags(repo.DiskPath)

	isBinary := isBinaryContent(content)
	var lines []string
	if !isBinary {
		lines = strings.Split(string(content), "\n")
	}

	h.renderer.Render(w, "repo_blob", map[string]interface{}{
		"User":     user,
		"Repo":     repo,
		"Owner":    repo.Owner,
		"Ref":      ref,
		"Path":     path,
		"Content":  string(content),
		"Lines":    lines,
		"IsBinary": isBinary,
		"Branches": branches,
		"Tags":     tags,
		"FileName": fileName(path),
	})
}

func (h *BrowseHandler) Raw(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	path := chi.URLParam(r, "*")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	content, err := h.gitSvc.GetBlob(repo.DiskPath, ref, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(content)
}

func (h *BrowseHandler) Commits(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	ref := chi.URLParam(r, "ref")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ = strconv.Atoi(p)
		if page < 1 {
			page = 1
		}
	}

	commits, _ := h.gitSvc.ListCommits(repo.DiskPath, ref, page, 30)
	branches, _ := h.gitSvc.ListBranches(repo.DiskPath)

	h.renderer.Render(w, "repo_commits", map[string]interface{}{
		"User":     user,
		"Repo":     repo,
		"Owner":    repo.Owner,
		"Ref":      ref,
		"Commits":  commits,
		"Branches": branches,
		"Page":     page,
		"HasNext":  len(commits) == 30,
	})
}

func (h *BrowseHandler) Commit(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	sha := chi.URLParam(r, "sha")
	user := CurrentUser(r)

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if repo.IsPrivate && (user == nil || !h.hasRepoAccess(r, repo, user)) {
		http.NotFound(w, r)
		return
	}

	commit, err := h.gitSvc.GetCommit(repo.DiskPath, sha)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	diff, _ := h.gitSvc.GetDiff(repo.DiskPath, sha)

	h.renderer.Render(w, "repo_commit", map[string]interface{}{
		"User":   user,
		"Repo":   repo,
		"Owner":  repo.Owner,
		"Commit": commit,
		"Diff":   diff,
	})
}

func (h *BrowseHandler) hasRepoAccess(r *http.Request, repo *models.Repository, user *models.User) bool {
	if user == nil {
		return !repo.IsPrivate
	}
	has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "read")
	return has
}

type BreadcrumbItem struct {
	Name string
	Path string
}

func buildBreadcrumb(path string) []BreadcrumbItem {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	var items []BreadcrumbItem
	for i, part := range parts {
		items = append(items, BreadcrumbItem{
			Name: part,
			Path: strings.Join(parts[:i+1], "/"),
		})
	}
	return items
}

func fileName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func isBinaryContent(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
