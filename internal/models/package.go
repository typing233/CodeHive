package models

import (
	"encoding/json"
	"time"
)

type Package struct {
	ID          int64     `json:"id"`
	RepoID      *int64    `json:"repo_id"`
	OwnerID     int64     `json:"owner_id"`
	OrgID       *int64    `json:"org_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	IsPrivate   bool      `json:"is_private"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Owner    *User             `json:"owner,omitempty"`
	Versions []*PackageVersion `json:"versions,omitempty"`
}

type PackageVersion struct {
	ID        int64           `json:"id"`
	PackageID int64           `json:"package_id"`
	Version   string          `json:"version"`
	Metadata  json.RawMessage `json:"metadata"`
	SizeBytes int64           `json:"size_bytes"`
	DiskPath  string          `json:"-"`
	CreatedAt time.Time       `json:"created_at"`
}

type PackageBlob struct {
	ID        int64     `json:"id"`
	Digest    string    `json:"digest"`
	SizeBytes int64     `json:"size_bytes"`
	DiskPath  string    `json:"-"`
	RefCount  int       `json:"ref_count"`
	CreatedAt time.Time `json:"created_at"`
}

type PackageCacheEntry struct {
	ID           int64     `json:"id"`
	RegistryURL  string    `json:"registry_url"`
	PackageName  string    `json:"package_name"`
	Version      string    `json:"version"`
	DiskPath     string    `json:"-"`
	SizeBytes    int64     `json:"size_bytes"`
	LastChecked  time.Time `json:"last_checked"`
	ExpiresAt    time.Time `json:"expires_at"`
}
