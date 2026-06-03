CREATE TABLE repositories (
    id             BIGSERIAL PRIMARY KEY,
    owner_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           VARCHAR(100) NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    is_private     BOOLEAN NOT NULL DEFAULT FALSE,
    default_branch VARCHAR(100) NOT NULL DEFAULT 'main',
    disk_path      VARCHAR(500) NOT NULL UNIQUE,
    size_bytes     BIGINT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(owner_id, name)
);
CREATE INDEX idx_repositories_owner ON repositories(owner_id);

CREATE TABLE repo_collaborators (
    id         BIGSERIAL PRIMARY KEY,
    repo_id    BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL DEFAULT 'write',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(repo_id, user_id)
);
