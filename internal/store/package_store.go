package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/codehive/codehive/internal/models"
)

type PackageStore struct {
	db *sql.DB
}

func NewPackageStore(db *sql.DB) *PackageStore {
	return &PackageStore{db: db}
}

func (s *PackageStore) DB() *sql.DB {
	return s.db
}

func (s *PackageStore) Create(ctx context.Context, pkg *models.Package) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO packages (repo_id, owner_id, org_id, name, type, description, is_private)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`,
		pkg.RepoID, pkg.OwnerID, pkg.OrgID, pkg.Name, pkg.Type, pkg.Description, pkg.IsPrivate,
	).Scan(&pkg.ID, &pkg.CreatedAt, &pkg.UpdatedAt)
}

func (s *PackageStore) GetByID(ctx context.Context, id int64) (*models.Package, error) {
	pkg := &models.Package{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
		 FROM packages WHERE id = $1`, id,
	).Scan(&pkg.ID, &pkg.RepoID, &pkg.OwnerID, &pkg.OrgID, &pkg.Name, &pkg.Type,
		&pkg.Description, &pkg.IsPrivate, &pkg.CreatedAt, &pkg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (s *PackageStore) GetByOwnerAndName(ctx context.Context, ownerID int64, pkgType, name string) (*models.Package, error) {
	pkg := &models.Package{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
		 FROM packages WHERE owner_id = $1 AND type = $2 AND name = $3 AND org_id IS NULL`, ownerID, pkgType, name,
	).Scan(&pkg.ID, &pkg.RepoID, &pkg.OwnerID, &pkg.OrgID, &pkg.Name, &pkg.Type,
		&pkg.Description, &pkg.IsPrivate, &pkg.CreatedAt, &pkg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (s *PackageStore) GetByOrgAndName(ctx context.Context, orgID int64, pkgType, name string) (*models.Package, error) {
	pkg := &models.Package{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
		 FROM packages WHERE org_id = $1 AND type = $2 AND name = $3`, orgID, pkgType, name,
	).Scan(&pkg.ID, &pkg.RepoID, &pkg.OwnerID, &pkg.OrgID, &pkg.Name, &pkg.Type,
		&pkg.Description, &pkg.IsPrivate, &pkg.CreatedAt, &pkg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (s *PackageStore) ListByOwner(ctx context.Context, ownerID int64, pkgType string, page, limit int) ([]*models.Package, int, error) {
	if limit == 0 {
		limit = 20
	}
	if page == 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var total int
	if pkgType != "" {
		s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM packages WHERE owner_id=$1 AND type=$2 AND org_id IS NULL`, ownerID, pkgType).Scan(&total)
	} else {
		s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM packages WHERE owner_id=$1 AND org_id IS NULL`, ownerID).Scan(&total)
	}

	var rows *sql.Rows
	var err error
	if pkgType != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
			 FROM packages WHERE owner_id=$1 AND type=$2 AND org_id IS NULL
			 ORDER BY updated_at DESC LIMIT $3 OFFSET $4`, ownerID, pkgType, limit, offset)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
			 FROM packages WHERE owner_id=$1 AND org_id IS NULL
			 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, ownerID, limit, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var packages []*models.Package
	for rows.Next() {
		p := &models.Package{}
		if err := rows.Scan(&p.ID, &p.RepoID, &p.OwnerID, &p.OrgID, &p.Name, &p.Type,
			&p.Description, &p.IsPrivate, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, err
		}
		packages = append(packages, p)
	}
	return packages, total, rows.Err()
}

func (s *PackageStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM packages WHERE id=$1`, id)
	return err
}

func (s *PackageStore) CreateVersion(ctx context.Context, v *models.PackageVersion) error {
	meta, _ := json.Marshal(v.Metadata)
	if v.Metadata == nil {
		meta = []byte("{}")
	}
	return s.db.QueryRowContext(ctx,
		`INSERT INTO package_versions (package_id, version, metadata, size_bytes, disk_path)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		v.PackageID, v.Version, meta, v.SizeBytes, v.DiskPath,
	).Scan(&v.ID, &v.CreatedAt)
}

func (s *PackageStore) GetVersion(ctx context.Context, packageID int64, version string) (*models.PackageVersion, error) {
	v := &models.PackageVersion{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, package_id, version, metadata, size_bytes, disk_path, created_at
		 FROM package_versions WHERE package_id=$1 AND version=$2`, packageID, version,
	).Scan(&v.ID, &v.PackageID, &v.Version, &v.Metadata, &v.SizeBytes, &v.DiskPath, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *PackageStore) ListVersions(ctx context.Context, packageID int64) ([]*models.PackageVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, package_id, version, metadata, size_bytes, disk_path, created_at
		 FROM package_versions WHERE package_id=$1 ORDER BY created_at DESC`, packageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*models.PackageVersion
	for rows.Next() {
		v := &models.PackageVersion{}
		if err := rows.Scan(&v.ID, &v.PackageID, &v.Version, &v.Metadata, &v.SizeBytes, &v.DiskPath, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (s *PackageStore) DeleteVersion(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM package_versions WHERE id=$1`, id)
	return err
}

func (s *PackageStore) GetOrCreateBlob(ctx context.Context, digest string, sizeBytes int64, diskPath string) (*models.PackageBlob, error) {
	blob := &models.PackageBlob{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO package_blobs (digest, size_bytes, disk_path)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (digest) DO UPDATE SET ref_count = package_blobs.ref_count + 1
		 RETURNING id, digest, size_bytes, disk_path, ref_count, created_at`,
		digest, sizeBytes, diskPath,
	).Scan(&blob.ID, &blob.Digest, &blob.SizeBytes, &blob.DiskPath, &blob.RefCount, &blob.CreatedAt)
	if err != nil {
		return nil, err
	}
	return blob, nil
}

func (s *PackageStore) GetBlob(ctx context.Context, digest string) (*models.PackageBlob, error) {
	blob := &models.PackageBlob{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, digest, size_bytes, disk_path, ref_count, created_at
		 FROM package_blobs WHERE digest=$1`, digest,
	).Scan(&blob.ID, &blob.Digest, &blob.SizeBytes, &blob.DiskPath, &blob.RefCount, &blob.CreatedAt)
	if err != nil {
		return nil, err
	}
	return blob, nil
}

func (s *PackageStore) DecrementBlobRef(ctx context.Context, digest string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE package_blobs SET ref_count = ref_count - 1 WHERE digest=$1`, digest,
	)
	return err
}

func (s *PackageStore) FindPublicByName(ctx context.Context, pkgType, name string) (*models.Package, error) {
	pkg := &models.Package{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, repo_id, owner_id, org_id, name, type, description, is_private, created_at, updated_at
		 FROM packages WHERE type=$1 AND name=$2 AND is_private=FALSE LIMIT 1`, pkgType, name,
	).Scan(&pkg.ID, &pkg.RepoID, &pkg.OwnerID, &pkg.OrgID, &pkg.Name, &pkg.Type,
		&pkg.Description, &pkg.IsPrivate, &pkg.CreatedAt, &pkg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}
