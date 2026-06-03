package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/codehive/codehive/internal/gitbackend"
	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/codehive/codehive/internal/web/middleware"
	"github.com/go-chi/chi/v5"
)

type PRHandler struct {
	prStore    *store.PRStore
	repoStore  *store.RepoStore
	issueStore *store.IssueStore
	userStore  *store.UserStore
	auditStore *store.AuditStore
	gitSvc     *gitbackend.Service
	renderer   Renderer
}

func NewPRHandler(ps *store.PRStore, rs *store.RepoStore, is *store.IssueStore, us *store.UserStore, as *store.AuditStore, gs *gitbackend.Service, rend Renderer) *PRHandler {
	return &PRHandler{
		prStore:    ps,
		repoStore:  rs,
		issueStore: is,
		userStore:  us,
		auditStore: as,
		gitSvc:     gs,
		renderer:   rend,
	}
}

func (h *PRHandler) List(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if repo.IsPrivate {
		if user == nil {
			http.NotFound(w, r)
			return
		}
		if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "read"); !has {
			http.NotFound(w, r)
			return
		}
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	filter := models.PRFilter{State: state, Page: 1, Limit: 20}
	if p := r.URL.Query().Get("page"); p != "" {
		filter.Page, _ = strconv.Atoi(p)
	}
	if q := r.URL.Query().Get("q"); q != "" {
		filter.Query = q
	}

	prs, total, _ := h.prStore.List(r.Context(), repo.ID, filter)

	h.renderer.Render(w, "pr_list", map[string]interface{}{
		"Repo":       repo,
		"PRs":        prs,
		"Total":      total,
		"Filter":     filter,
		"User":       user,
		"State":      state,
		"Owner":      owner,
		"RepoName":   repoName,
		"TotalPages": (total + filter.Limit - 1) / filter.Limit,
	})
}

func (h *PRHandler) NewPage(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	branches, _ := h.gitSvc.ListBranches(repo.DiskPath)

	user := middleware.UserFromContext(r.Context())
	h.renderer.Render(w, "pr_new", map[string]interface{}{
		"Repo":     repo,
		"Branches": branches,
		"User":     user,
		"Owner":    owner,
		"RepoName": repoName,
	})
}

func (h *PRHandler) Create(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "read"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")
	headBranch := r.FormValue("head_branch")
	baseBranch := r.FormValue("base_branch")

	if title == "" || headBranch == "" || baseBranch == "" {
		http.Error(w, "Title, head branch, and base branch are required", http.StatusBadRequest)
		return
	}

	if !h.gitSvc.BranchExists(repo.DiskPath, headBranch) {
		http.Error(w, "Head branch does not exist", http.StatusBadRequest)
		return
	}
	if !h.gitSvc.BranchExists(repo.DiskPath, baseBranch) {
		http.Error(w, "Base branch does not exist", http.StatusBadRequest)
		return
	}

	number, err := h.prStore.NextNumber(r.Context(), repo.ID)
	if err != nil {
		http.Error(w, "Failed to allocate PR number", http.StatusInternalServerError)
		return
	}

	pr := &models.PullRequest{
		RepoID:     repo.ID,
		Number:     number,
		AuthorID:   user.ID,
		Title:      title,
		Body:       body,
		State:      "open",
		HeadBranch: headBranch,
		BaseBranch: baseBranch,
	}

	if err := h.prStore.Create(r.Context(), pr); err != nil {
		http.Error(w, "Failed to create pull request", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number), http.StatusFound)
}

func (h *PRHandler) View(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if repo.IsPrivate {
		if user == nil {
			http.NotFound(w, r)
			return
		}
		if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "read"); !has {
			http.NotFound(w, r)
			return
		}
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	comments, _ := h.prStore.ListComments(r.Context(), pr.ID)
	canMerge := false
	if pr.State == "open" {
		canMerge, _ = h.gitSvc.CanMerge(repo.DiskPath, pr.BaseBranch, pr.HeadBranch)
	}

	var canWrite bool
	if user != nil {
		canWrite, _ = h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "write")
	}

	h.renderer.Render(w, "pr_view", map[string]interface{}{
		"Repo":     repo,
		"PR":       pr,
		"Comments": comments,
		"CanMerge": canMerge,
		"CanWrite": canWrite,
		"User":     user,
		"Owner":    owner,
		"RepoName": repoName,
	})
}

func (h *PRHandler) Diff(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if repo.IsPrivate && user == nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	diffs, _ := h.gitSvc.GetBranchDiff(repo.DiskPath, pr.BaseBranch, pr.HeadBranch)
	added, deleted, filesChanged, _ := h.gitSvc.GetDiffStats(repo.DiskPath, pr.BaseBranch, pr.HeadBranch)

	// Get inline comments grouped by file
	comments, _ := h.prStore.ListComments(r.Context(), pr.ID)
	inlineComments := make(map[string][]*models.PRComment)
	for _, c := range comments {
		if c.Path != nil {
			inlineComments[*c.Path] = append(inlineComments[*c.Path], c)
		}
	}

	h.renderer.Render(w, "pr_diff", map[string]interface{}{
		"Repo":           repo,
		"PR":             pr,
		"Diffs":          diffs,
		"Added":          added,
		"Deleted":        deleted,
		"FilesChanged":   filesChanged,
		"InlineComments": inlineComments,
		"User":           user,
		"Owner":          owner,
		"RepoName":       repoName,
	})
}

func (h *PRHandler) Commits(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	commits, _ := h.gitSvc.ListCommitsBetween(repo.DiskPath, pr.BaseBranch, pr.HeadBranch)
	user := middleware.UserFromContext(r.Context())

	h.renderer.Render(w, "pr_commits", map[string]interface{}{
		"Repo":     repo,
		"PR":       pr,
		"Commits":  commits,
		"User":     user,
		"Owner":    owner,
		"RepoName": repoName,
	})
}

func (h *PRHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number), http.StatusFound)
		return
	}

	comment := &models.PRComment{
		PRID:     pr.ID,
		AuthorID: user.ID,
		Body:     body,
	}

	// Inline comment fields
	if path := r.FormValue("path"); path != "" {
		comment.Path = &path
		if lineStr := r.FormValue("line"); lineStr != "" {
			if line, err := strconv.Atoi(lineStr); err == nil {
				comment.Line = &line
			}
		}
		if side := r.FormValue("side"); side != "" {
			comment.Side = &side
		}
	}

	h.prStore.AddComment(r.Context(), comment)

	redirect := fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number)
	if comment.Path != nil {
		redirect = fmt.Sprintf("/%s/%s/pulls/%d/diff", owner, repoName, pr.Number)
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (h *PRHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	state := r.FormValue("state")
	body := r.FormValue("body")

	validStates := map[string]bool{"approved": true, "changes_requested": true, "commented": true}
	if !validStates[state] {
		http.Error(w, "Invalid review state", http.StatusBadRequest)
		return
	}

	review := &models.PRReview{
		PRID:     pr.ID,
		AuthorID: user.ID,
		State:    state,
		Body:     body,
	}
	h.prStore.AddReview(r.Context(), review)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number), http.StatusFound)
}

var closeIssuePattern = regexp.MustCompile(`(?i)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)`)

func (h *PRHandler) Merge(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if has, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "write"); !has {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))

	prLookup, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Use transaction with row lock for concurrent merge safety
	tx, err := h.prStore.DB().BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	pr, err := h.prStore.LockForMerge(r.Context(), tx, prLookup.ID)
	if err != nil {
		http.Error(w, "PR not found", http.StatusNotFound)
		return
	}

	if pr.State != "open" {
		http.Error(w, "PR is not open", http.StatusConflict)
		return
	}

	method := r.FormValue("method")
	if method == "" {
		method = "merge"
	}

	message := fmt.Sprintf("Merge pull request #%d from %s into %s", pr.Number, pr.HeadBranch, pr.BaseBranch)
	if method == "squash" {
		message = fmt.Sprintf("Squash merge pull request #%d from %s", pr.Number, pr.HeadBranch)
	}

	commitSHA, err := h.gitSvc.MergeBranches(repo.DiskPath, pr.BaseBranch, pr.HeadBranch, method, message, user.FullName, user.Email)
	if err != nil {
		http.Error(w, "Merge failed: "+err.Error(), http.StatusConflict)
		return
	}

	// Update PR state
	now := "NOW()"
	_, err = tx.ExecContext(r.Context(),
		`UPDATE pull_requests SET state='merged', merge_commit=$1, merged_by=$2, merged_at=`+now+`, updated_at=`+now+` WHERE id=$3`,
		commitSHA, user.ID, pr.ID,
	)
	if err != nil {
		http.Error(w, "Failed to update PR", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit", http.StatusInternalServerError)
		return
	}

	// Auto-close linked issues
	fullPR, _ := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if fullPR != nil {
		h.autoCloseIssues(r, repo, fullPR)
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number), http.StatusFound)
}

func (h *PRHandler) autoCloseIssues(r *http.Request, repo *models.Repository, pr *models.PullRequest) {
	matches := closeIssuePattern.FindAllStringSubmatch(pr.Body, -1)

	// Also scan commit messages
	commits, _ := h.gitSvc.ListCommitsBetween(repo.DiskPath, pr.BaseBranch, pr.HeadBranch)
	for _, c := range commits {
		matches = append(matches, closeIssuePattern.FindAllStringSubmatch(c.Message, -1)...)
	}

	closed := make(map[int]bool)
	for _, m := range matches {
		if len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			if num > 0 && !closed[num] {
				closed[num] = true
				issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
				if err == nil && !issue.IsClosed {
					issue.IsClosed = true
					now := issue.UpdatedAt
					issue.ClosedAt = &now
					h.issueStore.Update(r.Context(), issue)
				}
			}
		}
	}
}

func (h *PRHandler) Close(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	canWrite, _ := h.repoStore.HasAccess(r.Context(), repo.ID, user.ID, "write")
	if !canWrite && pr.AuthorID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.prStore.Close(r.Context(), pr.ID)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number), http.StatusFound)
}

func (h *PRHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if pr.State != "closed" {
		http.Error(w, "Can only reopen closed PRs", http.StatusBadRequest)
		return
	}

	h.prStore.Reopen(r.Context(), pr.ID)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number), http.StatusFound)
}

func (h *PRHandler) UpdateLabels(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	labelIDs := r.Form["label_ids"]

	// Remove all current labels and re-add selected
	for _, l := range pr.Labels {
		h.prStore.RemoveLabel(r.Context(), pr.ID, l.ID)
	}
	for _, idStr := range labelIDs {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			h.prStore.AddLabel(r.Context(), pr.ID, id)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number), http.StatusFound)
}

func (h *PRHandler) UpdateAssignees(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	repo, err := h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	number, _ := strconv.Atoi(chi.URLParam(r, "number"))
	pr, err := h.prStore.GetByNumber(r.Context(), repo.ID, number)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	userIDs := r.Form["user_ids"]

	for _, a := range pr.Assignees {
		h.prStore.RemoveAssignee(r.Context(), pr.ID, a.ID)
	}
	for _, idStr := range userIDs {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			h.prStore.AddAssignee(r.Context(), pr.ID, id)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, number), http.StatusFound)
}

func (h *PRHandler) EditComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentID"), 10, 64)
	comment, err := h.prStore.GetComment(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if comment.AuthorID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body != "" {
		comment.Body = body
		h.prStore.UpdateComment(r.Context(), comment)
	}

	pr, _ := h.prStore.GetByID(r.Context(), comment.PRID)
	if pr != nil {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number), http.StatusFound)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (h *PRHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")

	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentID"), 10, 64)
	comment, err := h.prStore.GetComment(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	user := middleware.UserFromContext(r.Context())
	if comment.AuthorID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.prStore.DeleteComment(r.Context(), commentID)

	pr, _ := h.prStore.GetByID(r.Context(), comment.PRID)
	if pr != nil {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Number), http.StatusFound)
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
