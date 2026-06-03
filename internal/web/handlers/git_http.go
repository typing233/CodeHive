package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/codehive/codehive/internal/config"
	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type GitHTTPHandler struct {
	repoStore  *store.RepoStore
	userStore  *store.UserStore
	tokenStore *store.TokenStore
	gitSvc     *gitbackend.Service
	cfg        *config.Config
}

func NewGitHTTPHandler(rs *store.RepoStore, us *store.UserStore, ts *store.TokenStore, gs *gitbackend.Service, cfg *config.Config) *GitHTTPHandler {
	return &GitHTTPHandler{repoStore: rs, userStore: us, tokenStore: ts, gitSvc: gs, cfg: cfg}
}

func (h *GitHTTPHandler) authenticateGitUser(r *http.Request) (int64, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return 0, false
	}

	tokenHash := store.HashToken(password)
	token, err := h.tokenStore.GetByHash(r.Context(), tokenHash)
	if err == nil {
		return token.UserID, true
	}

	user, err := h.userStore.GetByUsername(r.Context(), username)
	if err != nil {
		return 0, false
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return 0, false
	}
	return user.ID, true
}

func (h *GitHTTPHandler) InfoRefs(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	service := r.URL.Query().Get("service")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	userID, authenticated := h.authenticateGitUser(r)

	if repo.IsPrivate || service == "git-receive-pack" {
		if !authenticated {
			w.Header().Set("WWW-Authenticate", `Basic realm="CodeHive"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		minRole := "read"
		if service == "git-receive-pack" {
			minRole = "write"
		}
		hasAccess, _ := h.repoStore.HasAccess(r.Context(), repo.ID, userID, minRole)
		if !hasAccess {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	absPath := h.gitSvc.AbsPath(repo.DiskPath)
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	pktLine := fmt.Sprintf("# service=%s\n", service)
	pktHeader := fmt.Sprintf("%04x%s0000", len(pktLine)+4, pktLine)
	w.Write([]byte(pktHeader))

	gitCmd := strings.TrimPrefix(service, "git-")
	cmd := exec.CommandContext(r.Context(), "git", gitCmd, "--stateless-rpc", "--advertise-refs", absPath)
	cmd.Stdout = w
	cmd.Stderr = io.Discard
	cmd.Run()
}

func (h *GitHTTPHandler) UploadPack(w http.ResponseWriter, r *http.Request) {
	h.serveGitPack(w, r, "upload-pack", "read")
}

func (h *GitHTTPHandler) ReceivePack(w http.ResponseWriter, r *http.Request) {
	h.serveGitPack(w, r, "receive-pack", "write")
}

func (h *GitHTTPHandler) serveGitPack(w http.ResponseWriter, r *http.Request, gitCmd, minRole string) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	needsAuth := repo.IsPrivate || minRole == "write"
	if needsAuth {
		userID, authenticated := h.authenticateGitUser(r)
		if !authenticated {
			w.Header().Set("WWW-Authenticate", `Basic realm="CodeHive"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		hasAccess, _ := h.repoStore.HasAccess(r.Context(), repo.ID, userID, minRole)
		if !hasAccess {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	absPath := h.gitSvc.AbsPath(repo.DiskPath)
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", gitCmd))
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.CommandContext(r.Context(), "git", gitCmd, "--stateless-rpc", absPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w
	cmd.Stderr = io.Discard
	cmd.Run()
}
