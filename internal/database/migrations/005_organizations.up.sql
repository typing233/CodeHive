CREATE TABLE organizations (
    id           BIGSERIAL PRIMARY KEY,
    name         VARCHAR(40) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    is_public    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE org_members (
    id         BIGSERIAL PRIMARY KEY,
    org_id     BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(20) NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, user_id)
);

CREATE TABLE teams (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    permission  VARCHAR(20) NOT NULL DEFAULT 'read',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, name)
);

CREATE TABLE team_members (
    team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY(team_id, user_id)
);

CREATE TABLE team_repos (
    team_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    repo_id    BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    permission VARCHAR(20) NOT NULL DEFAULT 'read',
    PRIMARY KEY(team_id, repo_id)
);

ALTER TABLE repositories ADD COLUMN org_id BIGINT REFERENCES organizations(id) ON DELETE CASCADE;
CREATE INDEX idx_repositories_org ON repositories(org_id);
