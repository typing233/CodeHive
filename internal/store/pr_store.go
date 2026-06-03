package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/codehive/codehive/internal/models"
)

type PRStore struct {
	db *sql.DB
}

func NewPRStore(db *sql.DB) *PRStore {
	return &PRStore{db: db}
}

func (s *PRStore) NextNumber(ctx context.Context, repoID int64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var num int
	err = tx.QueryRowContext(ctx,
		`SELECT next_number FROM repo_counters WHERE repo_id=$1 FOR UPDATE`, repoID,
	).Scan(&num)
	if err == sql.ErrNoRows {
		// Initialize counter
		var maxIssue int
		tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(number), 0) FROM issues WHERE repo_id=$1`, repoID,
		).Scan(&maxIssue)
		num = maxIssue + 1
		_, err = tx.ExecContext(ctx,
			`INSERT INTO repo_counters (repo_id, next_number) VALUES ($1, $2)`, repoID, num+1,
		)
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE repo_counters SET next_number = next_number + 1 WHERE repo_id=$1`, repoID,
		)
		if err != nil {
			return 0, err
		}
	}

	return num, tx.Commit()
}

func (s *PRStore) Create(ctx context.Context, pr *models.PullRequest) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO pull_requests (repo_id, number, author_id, title, body, state, head_branch, base_branch)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		pr.RepoID, pr.Number, pr.AuthorID, pr.Title, pr.Body, pr.State, pr.HeadBranch, pr.BaseBranch,
	).Scan(&pr.ID, &pr.CreatedAt, &pr.UpdatedAt)
}

func (s *PRStore) GetByNumber(ctx context.Context, repoID int64, number int) (*models.PullRequest, error) {
	pr := &models.PullRequest{Author: &models.User{}}
	var mergedBy sql.NullInt64
	var mergeCommit sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT p.id, p.repo_id, p.number, p.author_id, p.title, p.body, p.state,
		        p.head_branch, p.base_branch, p.merge_commit, p.merged_by,
		        p.merged_at, p.closed_at, p.created_at, p.updated_at,
		        u.id, u.username, u.full_name
		 FROM pull_requests p JOIN users u ON p.author_id = u.id
		 WHERE p.repo_id = $1 AND p.number = $2`, repoID, number,
	).Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.AuthorID, &pr.Title, &pr.Body, &pr.State,
		&pr.HeadBranch, &pr.BaseBranch, &mergeCommit, &mergedBy,
		&pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
		&pr.Author.ID, &pr.Author.Username, &pr.Author.FullName)
	if err != nil {
		return nil, err
	}
	if mergeCommit.Valid {
		pr.MergeCommit = mergeCommit.String
	}
	if mergedBy.Valid {
		pr.MergedBy = &mergedBy.Int64
	}

	pr.Labels, _ = s.getLabels(ctx, pr.ID)
	pr.Assignees, _ = s.getAssignees(ctx, pr.ID)
	pr.Reviews, _ = s.ListReviews(ctx, pr.ID)
	return pr, nil
}

func (s *PRStore) List(ctx context.Context, repoID int64, filter models.PRFilter) ([]*models.PullRequest, int, error) {
	where := []string{"p.repo_id = $1"}
	args := []interface{}{repoID}
	argN := 2

	if filter.State != "" && filter.State != "all" {
		where = append(where, fmt.Sprintf("p.state = $%d", argN))
		args = append(args, filter.State)
		argN++
	}
	if filter.AuthorID != nil {
		where = append(where, fmt.Sprintf("p.author_id = $%d", argN))
		args = append(args, *filter.AuthorID)
		argN++
	}
	if filter.Query != "" {
		where = append(where, fmt.Sprintf("(p.title ILIKE $%d OR p.body ILIKE $%d)", argN, argN))
		args = append(args, "%"+filter.Query+"%")
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM pull_requests p WHERE %s", whereClause), args...,
	).Scan(&total)

	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.Limit

	query := fmt.Sprintf(
		`SELECT p.id, p.repo_id, p.number, p.author_id, p.title, p.state,
		        p.head_branch, p.base_branch, p.created_at, p.updated_at,
		        u.username, u.full_name
		 FROM pull_requests p JOIN users u ON p.author_id = u.id
		 WHERE %s ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1,
	)
	args = append(args, filter.Limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var prs []*models.PullRequest
	for rows.Next() {
		p := &models.PullRequest{Author: &models.User{}}
		if err := rows.Scan(&p.ID, &p.RepoID, &p.Number, &p.AuthorID, &p.Title, &p.State,
			&p.HeadBranch, &p.BaseBranch, &p.CreatedAt, &p.UpdatedAt,
			&p.Author.Username, &p.Author.FullName); err != nil {
			return nil, 0, err
		}
		p.Labels, _ = s.getLabels(ctx, p.ID)
		prs = append(prs, p)
	}
	return prs, total, rows.Err()
}

func (s *PRStore) Update(ctx context.Context, pr *models.PullRequest) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pull_requests SET title=$1, body=$2, state=$3, updated_at=NOW() WHERE id=$4`,
		pr.Title, pr.Body, pr.State, pr.ID,
	)
	return err
}

func (s *PRStore) SetMerged(ctx context.Context, prID int64, mergedBy int64, mergeCommit string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE pull_requests SET state='merged', merge_commit=$1, merged_by=$2, merged_at=$3, updated_at=$3
		 WHERE id=$4`,
		mergeCommit, mergedBy, now, prID,
	)
	return err
}

func (s *PRStore) Close(ctx context.Context, prID int64) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE pull_requests SET state='closed', closed_at=$1, updated_at=$1 WHERE id=$2`,
		now, prID,
	)
	return err
}

func (s *PRStore) Reopen(ctx context.Context, prID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pull_requests SET state='open', closed_at=NULL, updated_at=NOW() WHERE id=$1`, prID,
	)
	return err
}

func (s *PRStore) AddComment(ctx context.Context, c *models.PRComment) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO pr_comments (pr_id, author_id, body, path, line, side, commit_sha)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`,
		c.PRID, c.AuthorID, c.Body, c.Path, c.Line, c.Side, c.CommitSHA,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (s *PRStore) ListComments(ctx context.Context, prID int64) ([]*models.PRComment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.pr_id, c.author_id, c.body, c.path, c.line, c.side, c.commit_sha,
		        c.created_at, c.updated_at, u.id, u.username, u.full_name
		 FROM pr_comments c JOIN users u ON c.author_id = u.id
		 WHERE c.pr_id = $1 ORDER BY c.created_at ASC`, prID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*models.PRComment
	for rows.Next() {
		c := &models.PRComment{Author: &models.User{}}
		if err := rows.Scan(&c.ID, &c.PRID, &c.AuthorID, &c.Body, &c.Path, &c.Line, &c.Side,
			&c.CommitSHA, &c.CreatedAt, &c.UpdatedAt,
			&c.Author.ID, &c.Author.Username, &c.Author.FullName); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (s *PRStore) GetComment(ctx context.Context, id int64) (*models.PRComment, error) {
	c := &models.PRComment{Author: &models.User{}}
	err := s.db.QueryRowContext(ctx,
		`SELECT c.id, c.pr_id, c.author_id, c.body, c.path, c.line, c.side, c.commit_sha,
		        c.created_at, c.updated_at, u.id, u.username, u.full_name
		 FROM pr_comments c JOIN users u ON c.author_id = u.id
		 WHERE c.id = $1`, id,
	).Scan(&c.ID, &c.PRID, &c.AuthorID, &c.Body, &c.Path, &c.Line, &c.Side,
		&c.CommitSHA, &c.CreatedAt, &c.UpdatedAt,
		&c.Author.ID, &c.Author.Username, &c.Author.FullName)
	return c, err
}

func (s *PRStore) UpdateComment(ctx context.Context, c *models.PRComment) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pr_comments SET body=$1, updated_at=NOW() WHERE id=$2`, c.Body, c.ID,
	)
	return err
}

func (s *PRStore) DeleteComment(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pr_comments WHERE id=$1`, id)
	return err
}

func (s *PRStore) AddReview(ctx context.Context, r *models.PRReview) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO pr_reviews (pr_id, author_id, state, body)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		r.PRID, r.AuthorID, r.State, r.Body,
	).Scan(&r.ID, &r.CreatedAt)
}

func (s *PRStore) ListReviews(ctx context.Context, prID int64) ([]*models.PRReview, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.pr_id, r.author_id, r.state, r.body, r.created_at,
		        u.id, u.username, u.full_name
		 FROM pr_reviews r JOIN users u ON r.author_id = u.id
		 WHERE r.pr_id = $1 ORDER BY r.created_at ASC`, prID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*models.PRReview
	for rows.Next() {
		r := &models.PRReview{Author: &models.User{}}
		if err := rows.Scan(&r.ID, &r.PRID, &r.AuthorID, &r.State, &r.Body, &r.CreatedAt,
			&r.Author.ID, &r.Author.Username, &r.Author.FullName); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

func (s *PRStore) AddLabel(ctx context.Context, prID, labelID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pr_labels (pr_id, label_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		prID, labelID,
	)
	return err
}

func (s *PRStore) RemoveLabel(ctx context.Context, prID, labelID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM pr_labels WHERE pr_id=$1 AND label_id=$2`, prID, labelID,
	)
	return err
}

func (s *PRStore) AddAssignee(ctx context.Context, prID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pr_assignees (pr_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		prID, userID,
	)
	return err
}

func (s *PRStore) RemoveAssignee(ctx context.Context, prID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM pr_assignees WHERE pr_id=$1 AND user_id=$2`, prID, userID,
	)
	return err
}

func (s *PRStore) getLabels(ctx context.Context, prID int64) ([]*models.Label, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT l.id, l.repo_id, l.name, l.color, l.description
		 FROM labels l JOIN pr_labels pl ON l.id = pl.label_id
		 WHERE pl.pr_id = $1`, prID,
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

func (s *PRStore) getAssignees(ctx context.Context, prID int64) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.full_name
		 FROM users u JOIN pr_assignees pa ON u.id = pa.user_id
		 WHERE pa.pr_id = $1`, prID,
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

func (s *PRStore) GetByID(ctx context.Context, id int64) (*models.PullRequest, error) {
	pr := &models.PullRequest{Author: &models.User{}}
	var mergedBy sql.NullInt64
	var mergeCommit sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT p.id, p.repo_id, p.number, p.author_id, p.title, p.body, p.state,
		        p.head_branch, p.base_branch, p.merge_commit, p.merged_by,
		        p.merged_at, p.closed_at, p.created_at, p.updated_at,
		        u.id, u.username, u.full_name
		 FROM pull_requests p JOIN users u ON p.author_id = u.id
		 WHERE p.id = $1`, id,
	).Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.AuthorID, &pr.Title, &pr.Body, &pr.State,
		&pr.HeadBranch, &pr.BaseBranch, &mergeCommit, &mergedBy,
		&pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
		&pr.Author.ID, &pr.Author.Username, &pr.Author.FullName)
	if err != nil {
		return nil, err
	}
	if mergeCommit.Valid {
		pr.MergeCommit = mergeCommit.String
	}
	if mergedBy.Valid {
		pr.MergedBy = &mergedBy.Int64
	}
	return pr, nil
}

func (s *PRStore) LockForMerge(ctx context.Context, tx *sql.Tx, prID int64) (*models.PullRequest, error) {
	pr := &models.PullRequest{}
	err := tx.QueryRowContext(ctx,
		`SELECT id, repo_id, number, state, head_branch, base_branch FROM pull_requests WHERE id=$1 FOR UPDATE`, prID,
	).Scan(&pr.ID, &pr.RepoID, &pr.Number, &pr.State, &pr.HeadBranch, &pr.BaseBranch)
	return pr, err
}

func (s *PRStore) DB() *sql.DB {
	return s.db
}
