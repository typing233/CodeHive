package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/codehive/codehive/internal/models"
)

type IssueStore struct {
	db *sql.DB
}

func NewIssueStore(db *sql.DB) *IssueStore {
	return &IssueStore{db: db}
}

func (s *IssueStore) NextNumber(ctx context.Context, repoID int64) (int, error) {
	var num int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE repo_id = $1`, repoID,
	).Scan(&num)
	return num, err
}

func (s *IssueStore) Create(ctx context.Context, issue *models.Issue) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO issues (repo_id, number, author_id, title, body, milestone_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		issue.RepoID, issue.Number, issue.AuthorID, issue.Title, issue.Body, issue.MilestoneID,
	).Scan(&issue.ID, &issue.CreatedAt, &issue.UpdatedAt)
}

func (s *IssueStore) GetByNumber(ctx context.Context, repoID int64, number int) (*models.Issue, error) {
	issue := &models.Issue{Author: &models.User{}}
	var milestoneID sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT i.id, i.repo_id, i.number, i.author_id, i.title, i.body, i.is_closed,
		        i.milestone_id, i.created_at, i.updated_at, i.closed_at,
		        u.id, u.username, u.full_name
		 FROM issues i JOIN users u ON i.author_id = u.id
		 WHERE i.repo_id = $1 AND i.number = $2`, repoID, number,
	).Scan(&issue.ID, &issue.RepoID, &issue.Number, &issue.AuthorID, &issue.Title, &issue.Body,
		&issue.IsClosed, &milestoneID, &issue.CreatedAt, &issue.UpdatedAt, &issue.ClosedAt,
		&issue.Author.ID, &issue.Author.Username, &issue.Author.FullName)
	if err != nil {
		return nil, err
	}
	if milestoneID.Valid {
		mid := milestoneID.Int64
		issue.MilestoneID = &mid
		issue.Milestone, _ = s.GetMilestone(ctx, mid)
	}

	issue.Labels, _ = s.getIssueLabels(ctx, issue.ID)
	issue.Assignees, _ = s.getIssueAssignees(ctx, issue.ID)
	issue.Reactions, _ = s.getReactions(ctx, &issue.ID, nil)

	return issue, nil
}

func (s *IssueStore) List(ctx context.Context, repoID int64, filter models.IssueFilter) ([]*models.Issue, int, error) {
	where := []string{"i.repo_id = $1"}
	args := []interface{}{repoID}
	argN := 2

	if filter.State == "open" {
		where = append(where, "i.is_closed = FALSE")
	} else if filter.State == "closed" {
		where = append(where, "i.is_closed = TRUE")
	}

	if filter.MilestoneID != nil {
		where = append(where, fmt.Sprintf("i.milestone_id = $%d", argN))
		args = append(args, *filter.MilestoneID)
		argN++
	}

	if filter.AuthorID != nil {
		where = append(where, fmt.Sprintf("i.author_id = $%d", argN))
		args = append(args, *filter.AuthorID)
		argN++
	}

	if filter.AssigneeID != nil {
		where = append(where, fmt.Sprintf("EXISTS(SELECT 1 FROM issue_assignees WHERE issue_id=i.id AND user_id=$%d)", argN))
		args = append(args, *filter.AssigneeID)
		argN++
	}

	if len(filter.LabelIDs) > 0 {
		for _, lid := range filter.LabelIDs {
			where = append(where, fmt.Sprintf("EXISTS(SELECT 1 FROM issue_labels WHERE issue_id=i.id AND label_id=$%d)", argN))
			args = append(args, lid)
			argN++
		}
	}

	if filter.Query != "" {
		where = append(where, fmt.Sprintf("(i.title ILIKE $%d OR i.body ILIKE $%d)", argN, argN))
		args = append(args, "%"+filter.Query+"%")
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM issues i WHERE %s", whereClause)
	s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)

	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.Limit

	query := fmt.Sprintf(
		`SELECT i.id, i.repo_id, i.number, i.author_id, i.title, i.is_closed,
		        i.created_at, i.updated_at, u.username, u.full_name
		 FROM issues i JOIN users u ON i.author_id = u.id
		 WHERE %s ORDER BY i.created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1,
	)
	args = append(args, filter.Limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var issues []*models.Issue
	for rows.Next() {
		i := &models.Issue{Author: &models.User{}}
		if err := rows.Scan(&i.ID, &i.RepoID, &i.Number, &i.AuthorID, &i.Title, &i.IsClosed,
			&i.CreatedAt, &i.UpdatedAt, &i.Author.Username, &i.Author.FullName); err != nil {
			return nil, 0, err
		}
		i.Labels, _ = s.getIssueLabels(ctx, i.ID)
		issues = append(issues, i)
	}
	return issues, total, rows.Err()
}

func (s *IssueStore) Update(ctx context.Context, issue *models.Issue) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE issues SET title=$1, body=$2, is_closed=$3, milestone_id=$4, closed_at=$5, updated_at=NOW()
		 WHERE id=$6`,
		issue.Title, issue.Body, issue.IsClosed, issue.MilestoneID, issue.ClosedAt, issue.ID,
	)
	return err
}

func (s *IssueStore) AddLabel(ctx context.Context, issueID, labelID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO issue_labels (issue_id, label_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		issueID, labelID,
	)
	return err
}

func (s *IssueStore) RemoveLabel(ctx context.Context, issueID, labelID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM issue_labels WHERE issue_id=$1 AND label_id=$2`, issueID, labelID,
	)
	return err
}

func (s *IssueStore) AddAssignee(ctx context.Context, issueID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO issue_assignees (issue_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		issueID, userID,
	)
	return err
}

func (s *IssueStore) RemoveAssignee(ctx context.Context, issueID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM issue_assignees WHERE issue_id=$1 AND user_id=$2`, issueID, userID,
	)
	return err
}

func (s *IssueStore) AddComment(ctx context.Context, comment *models.IssueComment) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO issue_comments (issue_id, author_id, body)
		 VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`,
		comment.IssueID, comment.AuthorID, comment.Body,
	).Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
}

func (s *IssueStore) UpdateComment(ctx context.Context, comment *models.IssueComment) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE issue_comments SET body=$1, updated_at=NOW() WHERE id=$2`,
		comment.Body, comment.ID,
	)
	return err
}

func (s *IssueStore) DeleteComment(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM issue_comments WHERE id=$1`, id)
	return err
}

func (s *IssueStore) ListComments(ctx context.Context, issueID int64) ([]*models.IssueComment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.issue_id, c.author_id, c.body, c.created_at, c.updated_at,
		        u.id, u.username, u.full_name
		 FROM issue_comments c JOIN users u ON c.author_id = u.id
		 WHERE c.issue_id = $1 ORDER BY c.created_at ASC`, issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*models.IssueComment
	for rows.Next() {
		c := &models.IssueComment{Author: &models.User{}}
		if err := rows.Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
			&c.Author.ID, &c.Author.Username, &c.Author.FullName); err != nil {
			return nil, err
		}
		c.Reactions, _ = s.getReactions(ctx, nil, &c.ID)
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (s *IssueStore) GetComment(ctx context.Context, id int64) (*models.IssueComment, error) {
	c := &models.IssueComment{Author: &models.User{}}
	err := s.db.QueryRowContext(ctx,
		`SELECT c.id, c.issue_id, c.author_id, c.body, c.created_at, c.updated_at,
		        u.id, u.username, u.full_name
		 FROM issue_comments c JOIN users u ON c.author_id = u.id
		 WHERE c.id = $1`, id,
	).Scan(&c.ID, &c.IssueID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt,
		&c.Author.ID, &c.Author.Username, &c.Author.FullName)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *IssueStore) AddReaction(ctx context.Context, reaction *models.Reaction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO reactions (user_id, emoji, issue_id, comment_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		reaction.UserID, reaction.Emoji, reaction.IssueID, reaction.CommentID,
	)
	return err
}

func (s *IssueStore) RemoveReaction(ctx context.Context, userID int64, emoji string, issueID, commentID *int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM reactions WHERE user_id=$1 AND emoji=$2
		 AND COALESCE(issue_id, 0)=COALESCE($3, 0)
		 AND COALESCE(comment_id, 0)=COALESCE($4, 0)`,
		userID, emoji, issueID, commentID,
	)
	return err
}

func (s *IssueStore) getIssueLabels(ctx context.Context, issueID int64) ([]*models.Label, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l.id, l.repo_id, l.name, l.color, l.description
		 FROM labels l JOIN issue_labels il ON l.id = il.label_id
		 WHERE il.issue_id = $1`, issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []*models.Label
	for rows.Next() {
		l := &models.Label{}
		if err := rows.Scan(&l.ID, &l.RepoID, &l.Name, &l.Color, &l.Description); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, nil
}

func (s *IssueStore) getIssueAssignees(ctx context.Context, issueID int64) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.full_name
		 FROM users u JOIN issue_assignees ia ON u.id = ia.user_id
		 WHERE ia.issue_id = $1`, issueID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.FullName); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *IssueStore) getReactions(ctx context.Context, issueID, commentID *int64) ([]*models.ReactionGroup, error) {
	var rows *sql.Rows
	var err error
	if issueID != nil {
		rows, err = s.db.QueryContext(ctx,
			`SELECT emoji, COUNT(*) as cnt FROM reactions WHERE issue_id=$1 GROUP BY emoji ORDER BY MIN(created_at)`,
			*issueID,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT emoji, COUNT(*) as cnt FROM reactions WHERE comment_id=$1 GROUP BY emoji ORDER BY MIN(created_at)`,
			*commentID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*models.ReactionGroup
	for rows.Next() {
		g := &models.ReactionGroup{}
		if err := rows.Scan(&g.Emoji, &g.Count); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// Label CRUD
func (s *IssueStore) CreateLabel(ctx context.Context, label *models.Label) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO labels (repo_id, name, color, description) VALUES ($1, $2, $3, $4) RETURNING id`,
		label.RepoID, label.Name, label.Color, label.Description,
	).Scan(&label.ID)
}

func (s *IssueStore) UpdateLabel(ctx context.Context, label *models.Label) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE labels SET name=$1, color=$2, description=$3 WHERE id=$4`,
		label.Name, label.Color, label.Description, label.ID,
	)
	return err
}

func (s *IssueStore) DeleteLabel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM labels WHERE id=$1`, id)
	return err
}

func (s *IssueStore) ListLabels(ctx context.Context, repoID int64) ([]*models.Label, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, repo_id, name, color, description FROM labels WHERE repo_id=$1 ORDER BY name`, repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []*models.Label
	for rows.Next() {
		l := &models.Label{}
		if err := rows.Scan(&l.ID, &l.RepoID, &l.Name, &l.Color, &l.Description); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, nil
}

func (s *IssueStore) GetLabel(ctx context.Context, id int64) (*models.Label, error) {
	l := &models.Label{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, name, color, description FROM labels WHERE id=$1`, id,
	).Scan(&l.ID, &l.RepoID, &l.Name, &l.Color, &l.Description)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Milestone CRUD
func (s *IssueStore) CreateMilestone(ctx context.Context, m *models.Milestone) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO milestones (repo_id, title, description, due_date) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		m.RepoID, m.Title, m.Description, m.DueDate,
	).Scan(&m.ID, &m.CreatedAt)
}

func (s *IssueStore) UpdateMilestone(ctx context.Context, m *models.Milestone) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE milestones SET title=$1, description=$2, due_date=$3, is_closed=$4, updated_at=NOW() WHERE id=$5`,
		m.Title, m.Description, m.DueDate, m.IsClosed, m.ID,
	)
	return err
}

func (s *IssueStore) DeleteMilestone(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM milestones WHERE id=$1`, id)
	return err
}

func (s *IssueStore) ListMilestones(ctx context.Context, repoID int64) ([]*models.Milestone, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.repo_id, m.title, m.description, m.due_date, m.is_closed, m.created_at, m.updated_at,
		        (SELECT COUNT(*) FROM issues WHERE milestone_id=m.id AND is_closed=FALSE) as open_count,
		        (SELECT COUNT(*) FROM issues WHERE milestone_id=m.id AND is_closed=TRUE) as closed_count
		 FROM milestones m WHERE m.repo_id=$1 ORDER BY m.created_at DESC`, repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var milestones []*models.Milestone
	for rows.Next() {
		m := &models.Milestone{}
		if err := rows.Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.DueDate, &m.IsClosed,
			&m.CreatedAt, &m.UpdatedAt, &m.OpenCount, &m.ClosedCount); err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, nil
}

func (s *IssueStore) GetMilestone(ctx context.Context, id int64) (*models.Milestone, error) {
	m := &models.Milestone{}
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.repo_id, m.title, m.description, m.due_date, m.is_closed, m.created_at, m.updated_at,
		        (SELECT COUNT(*) FROM issues WHERE milestone_id=m.id AND is_closed=FALSE),
		        (SELECT COUNT(*) FROM issues WHERE milestone_id=m.id AND is_closed=TRUE)
		 FROM milestones m WHERE m.id=$1`, id,
	).Scan(&m.ID, &m.RepoID, &m.Title, &m.Description, &m.DueDate, &m.IsClosed,
		&m.CreatedAt, &m.UpdatedAt, &m.OpenCount, &m.ClosedCount)
	if err != nil {
		return nil, err
	}
	return m, nil
}
