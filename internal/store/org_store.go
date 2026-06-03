package store

import (
	"context"
	"database/sql"

	"github.com/codehive/codehive/internal/models"
)

type OrgStore struct {
	db *sql.DB
}

func NewOrgStore(db *sql.DB) *OrgStore {
	return &OrgStore{db: db}
}

func (s *OrgStore) Create(ctx context.Context, org *models.Organization) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO organizations (name, display_name, description, is_public)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at`,
		org.Name, org.DisplayName, org.Description, org.IsPublic,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
}

func (s *OrgStore) GetByID(ctx context.Context, id int64) (*models.Organization, error) {
	o := &models.Organization{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, description, is_public, created_at, updated_at
		 FROM organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.IsPublic, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *OrgStore) GetByName(ctx context.Context, name string) (*models.Organization, error) {
	o := &models.Organization{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, description, is_public, created_at, updated_at
		 FROM organizations WHERE name = $1`, name,
	).Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.IsPublic, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *OrgStore) ListByUser(ctx context.Context, userID int64) ([]*models.Organization, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.name, o.display_name, o.description, o.is_public, o.created_at, o.updated_at
		 FROM organizations o JOIN org_members m ON o.id = m.org_id
		 WHERE m.user_id = $1 ORDER BY o.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*models.Organization
	for rows.Next() {
		o := &models.Organization{}
		if err := rows.Scan(&o.ID, &o.Name, &o.DisplayName, &o.Description, &o.IsPublic, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (s *OrgStore) Update(ctx context.Context, org *models.Organization) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE organizations SET display_name=$1, description=$2, is_public=$3, updated_at=NOW()
		 WHERE id=$4`,
		org.DisplayName, org.Description, org.IsPublic, org.ID,
	)
	return err
}

func (s *OrgStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM organizations WHERE id=$1`, id)
	return err
}

func (s *OrgStore) AddMember(ctx context.Context, orgID, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role=$3`,
		orgID, userID, role,
	)
	return err
}

func (s *OrgStore) RemoveMember(ctx context.Context, orgID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID,
	)
	return err
}

func (s *OrgStore) ListMembers(ctx context.Context, orgID int64) ([]*models.OrgMember, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.org_id, m.user_id, m.role, m.created_at, u.username, u.full_name
		 FROM org_members m JOIN users u ON m.user_id = u.id
		 WHERE m.org_id = $1 ORDER BY u.username`, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*models.OrgMember
	for rows.Next() {
		m := &models.OrgMember{User: &models.User{}}
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.CreatedAt,
			&m.User.Username, &m.User.FullName); err != nil {
			return nil, err
		}
		m.User.ID = m.UserID
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *OrgStore) GetMemberRole(ctx context.Context, orgID, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id=$1 AND user_id=$2`, orgID, userID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

func (s *OrgStore) CreateTeam(ctx context.Context, team *models.Team) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO teams (org_id, name, description, permission)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		team.OrgID, team.Name, team.Description, team.Permission,
	).Scan(&team.ID, &team.CreatedAt)
}

func (s *OrgStore) UpdateTeam(ctx context.Context, team *models.Team) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE teams SET name=$1, description=$2, permission=$3 WHERE id=$4`,
		team.Name, team.Description, team.Permission, team.ID,
	)
	return err
}

func (s *OrgStore) DeleteTeam(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM teams WHERE id=$1`, id)
	return err
}

func (s *OrgStore) GetTeam(ctx context.Context, id int64) (*models.Team, error) {
	t := &models.Team{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, description, permission, created_at FROM teams WHERE id=$1`, id,
	).Scan(&t.ID, &t.OrgID, &t.Name, &t.Description, &t.Permission, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Members, _ = s.ListTeamMembers(ctx, t.ID)
	return t, nil
}

func (s *OrgStore) ListTeams(ctx context.Context, orgID int64) ([]*models.Team, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, description, permission, created_at
		 FROM teams WHERE org_id=$1 ORDER BY name`, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*models.Team
	for rows.Next() {
		t := &models.Team{}
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.Description, &t.Permission, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

func (s *OrgStore) AddTeamMember(ctx context.Context, teamID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		teamID, userID,
	)
	return err
}

func (s *OrgStore) RemoveTeamMember(ctx context.Context, teamID, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID,
	)
	return err
}

func (s *OrgStore) ListTeamMembers(ctx context.Context, teamID int64) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.full_name
		 FROM users u JOIN team_members tm ON u.id = tm.user_id
		 WHERE tm.team_id = $1 ORDER BY u.username`, teamID,
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
	return users, rows.Err()
}

func (s *OrgStore) AddTeamRepo(ctx context.Context, teamID, repoID int64, permission string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO team_repos (team_id, repo_id, permission) VALUES ($1, $2, $3)
		 ON CONFLICT (team_id, repo_id) DO UPDATE SET permission=$3`,
		teamID, repoID, permission,
	)
	return err
}

func (s *OrgStore) RemoveTeamRepo(ctx context.Context, teamID, repoID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM team_repos WHERE team_id=$1 AND repo_id=$2`, teamID, repoID,
	)
	return err
}

func (s *OrgStore) GetEffectiveRepoPermission(ctx context.Context, userID, repoID int64) (string, error) {
	var permission string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(
			(SELECT tr.permission FROM team_repos tr
			 JOIN team_members tm ON tr.team_id = tm.team_id
			 WHERE tm.user_id = $1 AND tr.repo_id = $2
			 ORDER BY CASE tr.permission WHEN 'admin' THEN 3 WHEN 'write' THEN 2 ELSE 1 END DESC
			 LIMIT 1),
		'')`, userID, repoID,
	).Scan(&permission)
	if err != nil {
		return "", err
	}
	return permission, nil
}
