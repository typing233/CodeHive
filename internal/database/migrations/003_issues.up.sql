CREATE TABLE labels (
    id          BIGSERIAL PRIMARY KEY,
    repo_id     BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    name        VARCHAR(50) NOT NULL,
    color       VARCHAR(7) NOT NULL DEFAULT '#cccccc',
    description TEXT NOT NULL DEFAULT '',
    UNIQUE(repo_id, name)
);

CREATE TABLE milestones (
    id          BIGSERIAL PRIMARY KEY,
    repo_id     BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    title       VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    due_date    TIMESTAMPTZ,
    is_closed   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE issues (
    id           BIGSERIAL PRIMARY KEY,
    repo_id      BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number       INTEGER NOT NULL,
    author_id    BIGINT NOT NULL REFERENCES users(id),
    title        VARCHAR(500) NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    is_closed    BOOLEAN NOT NULL DEFAULT FALSE,
    milestone_id BIGINT REFERENCES milestones(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at    TIMESTAMPTZ,
    UNIQUE(repo_id, number)
);
CREATE INDEX idx_issues_repo ON issues(repo_id);
CREATE INDEX idx_issues_author ON issues(author_id);
CREATE INDEX idx_issues_milestone ON issues(milestone_id);

CREATE TABLE issue_labels (
    issue_id BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, label_id)
);

CREATE TABLE issue_assignees (
    issue_id BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, user_id)
);

CREATE TABLE issue_comments (
    id         BIGSERIAL PRIMARY KEY,
    issue_id   BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_id  BIGINT NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_issue_comments_issue ON issue_comments(issue_id);

CREATE TABLE reactions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      VARCHAR(20) NOT NULL,
    issue_id   BIGINT REFERENCES issues(id) ON DELETE CASCADE,
    comment_id BIGINT REFERENCES issue_comments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT reaction_target CHECK (
        (issue_id IS NOT NULL AND comment_id IS NULL) OR
        (issue_id IS NULL AND comment_id IS NOT NULL)
    )
);
CREATE UNIQUE INDEX idx_reactions_unique ON reactions(user_id, emoji, COALESCE(issue_id, 0), COALESCE(comment_id, 0));
