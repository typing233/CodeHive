package models

import "time"

type Label struct {
	ID          int64  `json:"id"`
	RepoID      int64  `json:"repo_id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type Milestone struct {
	ID          int64      `json:"id"`
	RepoID      int64      `json:"repo_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date"`
	IsClosed    bool       `json:"is_closed"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	OpenCount   int `json:"open_count"`
	ClosedCount int `json:"closed_count"`
}

type Issue struct {
	ID          int64      `json:"id"`
	RepoID      int64      `json:"repo_id"`
	Number      int        `json:"number"`
	AuthorID    int64      `json:"author_id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	IsClosed    bool       `json:"is_closed"`
	MilestoneID *int64     `json:"milestone_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`

	Author    *User      `json:"author,omitempty"`
	Labels    []*Label   `json:"labels,omitempty"`
	Assignees []*User    `json:"assignees,omitempty"`
	Milestone *Milestone `json:"milestone,omitempty"`
	Reactions []*ReactionGroup `json:"reactions,omitempty"`
}

type IssueComment struct {
	ID        int64     `json:"id"`
	IssueID   int64     `json:"issue_id"`
	AuthorID  int64     `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Author    *User           `json:"author,omitempty"`
	Reactions []*ReactionGroup `json:"reactions,omitempty"`
}

type Reaction struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Emoji     string    `json:"emoji"`
	IssueID   *int64    `json:"issue_id"`
	CommentID *int64    `json:"comment_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ReactionGroup struct {
	Emoji string  `json:"emoji"`
	Count int     `json:"count"`
	Users []*User `json:"users,omitempty"`
}

type IssueFilter struct {
	State       string
	LabelIDs    []int64
	MilestoneID *int64
	AssigneeID  *int64
	AuthorID    *int64
	Query       string
	Page        int
	Limit       int
}
