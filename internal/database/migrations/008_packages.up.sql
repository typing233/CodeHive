CREATE TABLE packages (
    id          BIGSERIAL PRIMARY KEY,
    repo_id     BIGINT REFERENCES repositories(id) ON DELETE SET NULL,
    owner_id    BIGINT NOT NULL REFERENCES users(id),
    org_id      BIGINT REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(20) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_private  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_packages_owner ON packages(owner_id);
CREATE INDEX idx_packages_org ON packages(org_id);
CREATE UNIQUE INDEX idx_packages_owner_type_name ON packages(owner_id, type, name) WHERE org_id IS NULL;
CREATE UNIQUE INDEX idx_packages_org_type_name ON packages(org_id, type, name) WHERE org_id IS NOT NULL;

CREATE TABLE package_versions (
    id          BIGSERIAL PRIMARY KEY,
    package_id  BIGINT NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    version     VARCHAR(255) NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}',
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    disk_path   VARCHAR(500) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(package_id, version)
);

CREATE TABLE package_blobs (
    id         BIGSERIAL PRIMARY KEY,
    digest     VARCHAR(100) NOT NULL UNIQUE,
    size_bytes BIGINT NOT NULL,
    disk_path  VARCHAR(500) NOT NULL,
    ref_count  INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE package_cache_entries (
    id            BIGSERIAL PRIMARY KEY,
    registry_url  VARCHAR(500) NOT NULL,
    package_name  VARCHAR(255) NOT NULL,
    version       VARCHAR(255) NOT NULL,
    disk_path     VARCHAR(500) NOT NULL,
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    last_checked  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(registry_url, package_name, version)
);
