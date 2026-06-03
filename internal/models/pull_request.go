package models

import "time"

type PullRequest struct {
	ID          int64      `json:"id"`
	RepoID      int64      `json:"repo_id"`
	Number      int        `json:"number"`
	AuthorID    int64      `json:"author_id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	HeadBranch  string     `json:"head_branch"`
	BaseBranch  string     `json:"base_branch"`
	MergeCommit string     `json:"merge_commit"`
	MergedBy    *int64     `json:"merged_by"`
	MergedAt    *time.Time `json:"merged_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Author    *User       `json:"author,omitempty"`
	Labels    []*Label    `json:"labels,omitempty"`
	Assignees []*User     `json:"assignees,omitempty"`
	Reviews   []*PRReview `json:"reviews,omitempty"`
}

type PRComment struct {
	ID        int64     `json:"id"`
	PRID      int64     `json:"pr_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	Path      *string   `json:"path"`
	Line      *int      `json:"line"`
	Side      *string   `json:"side"`
	CommitSHA *string   `json:"commit_sha"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Author *User `json:"author,omitempty"`
}

type PRReview struct {
	ID        int64     `json:"id"`
	PRID      int64     `json:"pr_id"`
	AuthorID  int64     `json:"author_id"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`

	Author *User `json:"author,omitempty"`
}

type PRFilter struct {
	State      string
	AuthorID   *int64
	AssigneeID *int64
	LabelIDs   []int64
	Query      string
	Page       int
	Limit      int
}
