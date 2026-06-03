package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
	"github.com/go-chi/chi/v5"
)

type IssueHandler struct {
	issueStore *store.IssueStore
	repoStore  *store.RepoStore
	userStore  *store.UserStore
	renderer   Renderer
}

func NewIssueHandler(is *store.IssueStore, rs *store.RepoStore, us *store.UserStore, r Renderer) *IssueHandler {
	return &IssueHandler{issueStore: is, repoStore: rs, userStore: us, renderer: r}
}

func (h *IssueHandler) getRepo(r *http.Request) (*models.Repository, error) {
	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	return h.repoStore.GetByOwnerAndName(r.Context(), owner, repoName)
}

func (h *IssueHandler) List(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	filter := models.IssueFilter{
		State: state,
		Page:  1,
		Limit: 30,
	}
	if p := r.URL.Query().Get("page"); p != "" {
		filter.Page, _ = strconv.Atoi(p)
	}
	if q := r.URL.Query().Get("q"); q != "" {
		filter.Query = q
	}
	if lid := r.URL.Query().Get("label"); lid != "" {
		id, _ := strconv.ParseInt(lid, 10, 64)
		filter.LabelIDs = []int64{id}
	}
	if mid := r.URL.Query().Get("milestone"); mid != "" {
		id, _ := strconv.ParseInt(mid, 10, 64)
		filter.MilestoneID = &id
	}

	issues, total, _ := h.issueStore.List(r.Context(), repo.ID, filter)
	labels, _ := h.issueStore.ListLabels(r.Context(), repo.ID)
	milestones, _ := h.issueStore.ListMilestones(r.Context(), repo.ID)

	h.renderer.Render(w, "issue_list", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Issues":     issues,
		"Total":      total,
		"State":      state,
		"Filter":     filter,
		"Labels":     labels,
		"Milestones": milestones,
		"Page":       filter.Page,
		"HasNext":    filter.Page*filter.Limit < total,
	})
}

func (h *IssueHandler) View(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	comments, _ := h.issueStore.ListComments(r.Context(), issue.ID)
	labels, _ := h.issueStore.ListLabels(r.Context(), repo.ID)
	milestones, _ := h.issueStore.ListMilestones(r.Context(), repo.ID)

	h.renderer.Render(w, "issue_view", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Issue":      issue,
		"Comments":   comments,
		"Labels":     labels,
		"Milestones": milestones,
	})
}

func (h *IssueHandler) NewPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	labels, _ := h.issueStore.ListLabels(r.Context(), repo.ID)
	milestones, _ := h.issueStore.ListMilestones(r.Context(), repo.ID)

	h.renderer.Render(w, "issue_new", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Labels":     labels,
		"Milestones": milestones,
	})
}

func (h *IssueHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")

	if title == "" {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/new", repo.Owner.Username, repo.Name), http.StatusSeeOther)
		return
	}

	num, _ := h.issueStore.NextNumber(r.Context(), repo.ID)
	issue := &models.Issue{
		RepoID:   repo.ID,
		Number:   num,
		AuthorID: user.ID,
		Title:    title,
		Body:     body,
	}

	if mid := r.FormValue("milestone_id"); mid != "" {
		id, _ := strconv.ParseInt(mid, 10, 64)
		issue.MilestoneID = &id
	}

	if err := h.issueStore.Create(r.Context(), issue); err != nil {
		http.Error(w, "Failed to create issue", http.StatusInternalServerError)
		return
	}

	if labelIDs := r.Form["label_ids"]; len(labelIDs) > 0 {
		for _, lid := range labelIDs {
			id, _ := strconv.ParseInt(lid, 10, 64)
			h.issueStore.AddLabel(r.Context(), issue.ID, id)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) EditPage(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	labels, _ := h.issueStore.ListLabels(r.Context(), repo.ID)
	milestones, _ := h.issueStore.ListMilestones(r.Context(), repo.ID)

	h.renderer.Render(w, "issue_edit", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Issue":      issue,
		"Labels":     labels,
		"Milestones": milestones,
	})
}

func (h *IssueHandler) Update(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	issue.Title = strings.TrimSpace(r.FormValue("title"))
	issue.Body = r.FormValue("body")

	if mid := r.FormValue("milestone_id"); mid != "" && mid != "0" {
		id, _ := strconv.ParseInt(mid, 10, 64)
		issue.MilestoneID = &id
	} else {
		issue.MilestoneID = nil
	}

	h.issueStore.Update(r.Context(), issue)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) Close(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	issue.IsClosed = true
	now := time.Now()
	issue.ClosedAt = &now
	h.issueStore.Update(r.Context(), issue)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	issue.IsClosed = false
	issue.ClosedAt = nil
	h.issueStore.Update(r.Context(), issue)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
		return
	}

	comment := &models.IssueComment{
		IssueID:  issue.ID,
		AuthorID: user.ID,
		Body:     body,
	}
	h.issueStore.AddComment(r.Context(), comment)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d#comment-%d", repo.Owner.Username, repo.Name, issue.Number, comment.ID), http.StatusSeeOther)
}

func (h *IssueHandler) EditComment(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentID"), 10, 64)

	comment, err := h.issueStore.GetComment(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if comment.AuthorID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	comment.Body = strings.TrimSpace(r.FormValue("body"))
	h.issueStore.UpdateComment(r.Context(), comment)

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "number")
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%s#comment-%d", owner, repoName, num, comment.ID), http.StatusSeeOther)
}

func (h *IssueHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentID"), 10, 64)

	comment, err := h.issueStore.GetComment(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if comment.AuthorID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	h.issueStore.DeleteComment(r.Context(), commentID)

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "number")
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%s", owner, repoName, num), http.StatusSeeOther)
}

func (h *IssueHandler) UpdateLabels(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	for _, l := range issue.Labels {
		h.issueStore.RemoveLabel(r.Context(), issue.ID, l.ID)
	}

	if labelIDs := r.Form["label_ids"]; len(labelIDs) > 0 {
		for _, lid := range labelIDs {
			id, _ := strconv.ParseInt(lid, 10, 64)
			h.issueStore.AddLabel(r.Context(), issue.ID, id)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) UpdateAssignees(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	for _, a := range issue.Assignees {
		h.issueStore.RemoveAssignee(r.Context(), issue.ID, a.ID)
	}

	if userIDs := r.Form["assignee_ids"]; len(userIDs) > 0 {
		for _, uid := range userIDs {
			id, _ := strconv.ParseInt(uid, 10, 64)
			h.issueStore.AddAssignee(r.Context(), issue.ID, id)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	num, _ := strconv.Atoi(chi.URLParam(r, "number"))
	issue, err := h.issueStore.GetByNumber(r.Context(), repo.ID, num)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	emoji := r.FormValue("emoji")
	if emoji == "" {
		http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
		return
	}

	reaction := &models.Reaction{
		UserID:  user.ID,
		Emoji:   emoji,
		IssueID: &issue.ID,
	}
	h.issueStore.AddReaction(r.Context(), reaction)

	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%d", repo.Owner.Username, repo.Name, issue.Number), http.StatusSeeOther)
}

func (h *IssueHandler) AddCommentReaction(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentID"), 10, 64)
	emoji := r.FormValue("emoji")

	reaction := &models.Reaction{
		UserID:    user.ID,
		Emoji:     emoji,
		CommentID: &commentID,
	}
	h.issueStore.AddReaction(r.Context(), reaction)

	owner := chi.URLParam(r, "owner")
	repoName := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "number")
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/issues/%s#comment-%d", owner, repoName, num, commentID), http.StatusSeeOther)
}

// Label management
func (h *IssueHandler) Labels(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	labels, _ := h.issueStore.ListLabels(r.Context(), repo.ID)

	h.renderer.Render(w, "labels", map[string]interface{}{
		"User":   user,
		"Repo":   repo,
		"Owner":  repo.Owner,
		"Labels": labels,
	})
}

func (h *IssueHandler) CreateLabel(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	label := &models.Label{
		RepoID:      repo.ID,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Color:       r.FormValue("color"),
		Description: strings.TrimSpace(r.FormValue("description")),
	}

	if label.Color == "" {
		label.Color = "#cccccc"
	}

	h.issueStore.CreateLabel(r.Context(), label)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/labels", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	label, err := h.issueStore.GetLabel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	label.Name = strings.TrimSpace(r.FormValue("name"))
	label.Color = r.FormValue("color")
	label.Description = strings.TrimSpace(r.FormValue("description"))

	h.issueStore.UpdateLabel(r.Context(), label)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/labels", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.issueStore.DeleteLabel(r.Context(), id)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/labels", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

// Milestone management
func (h *IssueHandler) Milestones(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	milestones, _ := h.issueStore.ListMilestones(r.Context(), repo.ID)

	h.renderer.Render(w, "milestones", map[string]interface{}{
		"User":       user,
		"Repo":       repo,
		"Owner":      repo.Owner,
		"Milestones": milestones,
	})
}

func (h *IssueHandler) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	m := &models.Milestone{
		RepoID:      repo.ID,
		Title:       strings.TrimSpace(r.FormValue("title")),
		Description: r.FormValue("description"),
	}

	if due := r.FormValue("due_date"); due != "" {
		t, err := time.Parse("2006-01-02", due)
		if err == nil {
			m.DueDate = &t
		}
	}

	h.issueStore.CreateMilestone(r.Context(), m)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/milestones", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.issueStore.GetMilestone(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	m.Title = strings.TrimSpace(r.FormValue("title"))
	m.Description = r.FormValue("description")
	if due := r.FormValue("due_date"); due != "" {
		t, err := time.Parse("2006-01-02", due)
		if err == nil {
			m.DueDate = &t
		}
	} else {
		m.DueDate = nil
	}

	h.issueStore.UpdateMilestone(r.Context(), m)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/milestones", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) CloseMilestone(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.issueStore.GetMilestone(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	m.IsClosed = true
	h.issueStore.UpdateMilestone(r.Context(), m)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/milestones", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) ReopenMilestone(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.issueStore.GetMilestone(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	m.IsClosed = false
	h.issueStore.UpdateMilestone(r.Context(), m)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/milestones", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}

func (h *IssueHandler) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	repo, err := h.getRepo(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.issueStore.DeleteMilestone(r.Context(), id)
	http.Redirect(w, r, fmt.Sprintf("/%s/%s/milestones", repo.Owner.Username, repo.Name), http.StatusSeeOther)
}
