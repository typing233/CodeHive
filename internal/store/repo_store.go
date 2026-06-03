package store

import (
	"context"
	"database/sql"

	"github.com/codehive/codehive/internal/models"
)

type RepoStore struct {
	db *sql.DB
}

func NewRepoStore(db *sql.DB) *RepoStore {
	return &RepoStore{db: db}
}

func (s *RepoStore) Create(ctx context.Context, repo *models.Repository) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO repositories (owner_id, name, description, is_private, default_branch, disk_path)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		repo.OwnerID, repo.Name, repo.Description, repo.IsPrivate, repo.DefaultBranch, repo.DiskPath,
	).Scan(&repo.ID, &repo.CreatedAt, &repo.UpdatedAt)
}

func (s *RepoStore) GetByID(ctx context.Context, id int64) (*models.Repository, error) {
	r := &models.Repository{}
	err := s.db.QueryRowContext(ctx,
		`SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.disk_path, r.size_bytes, r.created_at, r.updated_at,
		        u.id, u.username, u.full_name
		 FROM repositories r JOIN users u ON r.owner_id = u.id
		 WHERE r.id = $1`, id,
	).Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &r.IsPrivate, &r.DefaultBranch,
		&r.DiskPath, &r.SizeBytes, &r.CreatedAt, &r.UpdatedAt,
		&r.Owner.ID, &r.Owner.Username, &r.Owner.FullName)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *RepoStore) GetByOwnerAndName(ctx context.Context, owner, name string) (*models.Repository, error) {
	r := &models.Repository{Owner: &models.User{}}
	err := s.db.QueryRowContext(ctx,
		`SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.disk_path, r.size_bytes, r.created_at, r.updated_at,
		        u.id, u.username, u.full_name
		 FROM repositories r JOIN users u ON r.owner_id = u.id
		 WHERE u.username = $1 AND r.name = $2`, owner, name,
	).Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &r.IsPrivate, &r.DefaultBranch,
		&r.DiskPath, &r.SizeBytes, &r.CreatedAt, &r.UpdatedAt,
		&r.Owner.ID, &r.Owner.Username, &r.Owner.FullName)
	if err == nil {
		return r, nil
	}

	// Fallback: check if owner is an organization name
	r = &models.Repository{Owner: &models.User{}}
	err = s.db.QueryRowContext(ctx,
		`SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.disk_path, r.size_bytes, r.created_at, r.updated_at,
		        u.id, u.username, u.full_name
		 FROM repositories r
		 JOIN organizations o ON r.org_id = o.id
		 JOIN users u ON r.owner_id = u.id
		 WHERE o.name = $1 AND r.name = $2`, owner, name,
	).Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &r.IsPrivate, &r.DefaultBranch,
		&r.DiskPath, &r.SizeBytes, &r.CreatedAt, &r.UpdatedAt,
		&r.Owner.ID, &r.Owner.Username, &r.Owner.FullName)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *RepoStore) ListByUser(ctx context.Context, userID int64) ([]*models.Repository, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.size_bytes, r.created_at, r.updated_at, u.username
		 FROM repositories r JOIN users u ON r.owner_id = u.id
		 WHERE r.owner_id = $1
		 UNION
		 SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.size_bytes, r.created_at, r.updated_at, u.username
		 FROM repositories r JOIN users u ON r.owner_id = u.id
		 JOIN repo_collaborators c ON c.repo_id = r.id
		 WHERE c.user_id = $1
		 ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*models.Repository
	for rows.Next() {
		r := &models.Repository{Owner: &models.User{}}
		if err := rows.Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &r.IsPrivate,
			&r.DefaultBranch, &r.SizeBytes, &r.CreatedAt, &r.UpdatedAt, &r.Owner.Username); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *RepoStore) ListPublic(ctx context.Context, page, limit int) ([]*models.Repository, error) {
	offset := (page - 1) * limit
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.owner_id, r.name, r.description, r.is_private, r.default_branch,
		        r.size_bytes, r.created_at, r.updated_at, u.username
		 FROM repositories r JOIN users u ON r.owner_id = u.id
		 WHERE r.is_private = FALSE
		 ORDER BY r.updated_at DESC LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []*models.Repository
	for rows.Next() {
		r := &models.Repository{Owner: &models.User{}}
		if err := rows.Scan(&r.ID, &r.OwnerID, &r.Name, &r.Description, &r.IsPrivate,
			&r.DefaultBranch, &r.SizeBytes, &r.CreatedAt, &r.UpdatedAt, &r.Owner.Username); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (s *RepoStore) Update(ctx context.Context, repo *models.Repository) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE repositories SET name=$1, description=$2, is_private=$3, default_branch=$4, updated_at=NOW()
		 WHERE id=$5`,
		repo.Name, repo.Description, repo.IsPrivate, repo.DefaultBranch, repo.ID,
	)
	return err
}

func (s *RepoStore) UpdateDiskPath(ctx context.Context, id int64, diskPath string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE repositories SET disk_path=$1, updated_at=NOW() WHERE id=$2`, diskPath, id,
	)
	return err
}

func (s *RepoStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM repositories WHERE id=$1`, id)
	return err
}

func (s *RepoStore) AddCollaborator(ctx context.Context, repoID, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO repo_collaborators (repo_id, user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (repo_id, user_id) DO UPDATE SET role=$3`,
		repoID, userID, role,
	)
	return err
}

func (s *RepoStore) RemoveCollaborator(ctx context.Context, repoID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM repo_collaborators WHERE repo_id=$1 AND user_id=$2`, repoID, userID,
	)
	return err
}

func (s *RepoStore) ListCollaborators(ctx context.Context, repoID int64) ([]*models.RepoCollaborator, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.repo_id, c.user_id, c.role, c.created_at, u.username, u.full_name
		 FROM repo_collaborators c JOIN users u ON c.user_id = u.id
		 WHERE c.repo_id = $1 ORDER BY u.username`, repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collabs []*models.RepoCollaborator
	for rows.Next() {
		c := &models.RepoCollaborator{User: &models.User{}}
		if err := rows.Scan(&c.ID, &c.RepoID, &c.UserID, &c.Role, &c.CreatedAt,
			&c.User.Username, &c.User.FullName); err != nil {
			return nil, err
		}
		collabs = append(collabs, c)
	}
	return collabs, rows.Err()
}

func (s *RepoStore) HasAccess(ctx context.Context, repoID, userID int64, minRole string) (bool, error) {
	var ownerID int64
	var isPrivate bool
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_id, is_private FROM repositories WHERE id=$1`, repoID,
	).Scan(&ownerID, &isPrivate)
	if err != nil {
		return false, err
	}

	if ownerID == userID {
		return true, nil
	}

	if !isPrivate && minRole == "read" {
		return true, nil
	}

	// Check direct collaborator role
	var role string
	err = s.db.QueryRowContext(ctx,
		`SELECT role FROM repo_collaborators WHERE repo_id=$1 AND user_id=$2`, repoID, userID,
	).Scan(&role)
	if err == nil && roleAtLeast(role, minRole) {
		return true, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	// Check team-based access (org repos)
	var teamPerm string
	err = s.db.QueryRowContext(ctx,
		`SELECT tr.permission FROM team_repos tr
		 JOIN team_members tm ON tr.team_id = tm.team_id
		 WHERE tm.user_id = $1 AND tr.repo_id = $2
		 ORDER BY CASE tr.permission WHEN 'admin' THEN 3 WHEN 'write' THEN 2 ELSE 1 END DESC
		 LIMIT 1`, userID, repoID,
	).Scan(&teamPerm)
	if err == nil && roleAtLeast(teamPerm, minRole) {
		return true, nil
	}

	// Check if user is org owner (org owners have admin on all org repos)
	var orgID sql.NullInt64
	s.db.QueryRowContext(ctx,
		`SELECT org_id FROM repositories WHERE id=$1`, repoID,
	).Scan(&orgID)
	if orgID.Valid {
		var orgRole string
		err = s.db.QueryRowContext(ctx,
			`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID.Int64, userID,
		).Scan(&orgRole)
		if err == nil && orgRole == "owner" {
			return true, nil
		}
	}

	return false, nil
}

func roleAtLeast(role, minRole string) bool {
	levels := map[string]int{"read": 1, "write": 2, "admin": 3}
	return levels[role] >= levels[minRole]
}
