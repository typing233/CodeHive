CREATE TABLE repo_counters (
    repo_id     BIGINT PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
    next_number INTEGER NOT NULL DEFAULT 1
);

-- Backfill counters for existing repos based on max issue number
INSERT INTO repo_counters (repo_id, next_number)
SELECT r.id, COALESCE(MAX(i.number), 0) + 1
FROM repositories r LEFT JOIN issues i ON r.id = i.repo_id
GROUP BY r.id;

CREATE TABLE pull_requests (
    id           BIGSERIAL PRIMARY KEY,
    repo_id      BIGINT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number       INTEGER NOT NULL,
    author_id    BIGINT NOT NULL REFERENCES users(id),
    title        VARCHAR(500) NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    state        VARCHAR(20) NOT NULL DEFAULT 'open',
    head_branch  VARCHAR(255) NOT NULL,
    base_branch  VARCHAR(255) NOT NULL,
    merge_commit VARCHAR(40),
    merged_by    BIGINT REFERENCES users(id),
    merged_at    TIMESTAMPTZ,
    closed_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(repo_id, number)
);
CREATE INDEX idx_pr_repo_state ON pull_requests(repo_id, state);
CREATE INDEX idx_pr_author ON pull_requests(author_id);

CREATE TABLE pr_comments (
    id         BIGSERIAL PRIMARY KEY,
    pr_id      BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id  BIGINT NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    path       VARCHAR(500),
    line       INTEGER,
    side       VARCHAR(5),
    commit_sha VARCHAR(40),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pr_comments_pr ON pr_comments(pr_id);

CREATE TABLE pr_reviews (
    id         BIGSERIAL PRIMARY KEY,
    pr_id      BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    author_id  BIGINT NOT NULL REFERENCES users(id),
    state      VARCHAR(30) NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pr_reviews_pr ON pr_reviews(pr_id);

CREATE TABLE pr_labels (
    pr_id    BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY(pr_id, label_id)
);

CREATE TABLE pr_assignees (
    pr_id   BIGINT NOT NULL REFERENCES pull_requests(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY(pr_id, user_id)
);
